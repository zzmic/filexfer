package main

import (
	"context"
	"errors"
	"filexfer/protocol"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Errors for representing various failure scenarios.
var (
	ErrFileNotFound     = errors.New("file not found")
	ErrFileTooLarge     = errors.New("file size exceeds the maximum allowed size")
	ErrInvalidFilename  = errors.New("invalid filename")
	ErrConnectionFailed = errors.New("connection failed")
)

// MaxFileSize is the maximum allowed file size for transfers (5GB).
// It's defined as a variable to allow modification during testing, although it should remain constant in practice.
var MaxFileSize int64 = 5 * 1024 * 1024 * 1024

// Other constants for client configuration.
const (
	LogPrefix         = "[CLIENT]"       // Log prefix for client logs.
	ConnectionTimeout = 30 * time.Second // Connection timeout duration.
	ReadTimeout       = 30 * time.Second // Read timeout duration.
	WriteTimeout      = 30 * time.Second // Write timeout duration.
	ShutdownTimeout   = 30 * time.Second // Shutdown timeout duration.
)

// Command-line flags for the client.
var (
	serverAddr = flag.String("server", "localhost:8080", "Server address (IP:Port)")
	filePath   = flag.String("file", "", "File or directory to be transferred (required)")
)

// toKB converts bytes to kilobytes.
func toKB(bytes uint64) float64 {
	return float64(bytes) / 1024
}

// toMB converts bytes to megabytes.
func toMB(bytes uint64) float64 {
	return float64(bytes) / 1024 / 1024
}

// toGB converts bytes to gigabytes.
func toGB(bytes uint64) float64 {
	return float64(bytes) / 1024 / 1024 / 1024
}

// setupLogging configures structured logging with timestamps and custom prefix.
func setupLogging() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.SetPrefix(LogPrefix + " ")
}

// validateArgs validates command-line arguments.
func validateArgs() error {
	if *filePath == "" {
		return fmt.Errorf("file path is required: use -file flag to specify the source file")
	}

	return nil
}

// validatePath performs validation on the provided file or directory path before a transfer.
func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("%w: path cannot be empty", ErrInvalidFilename)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrFileNotFound, path)
		}
		return fmt.Errorf("failed to get the path information for %s: %w", path, err)
	}

	// If it's a file, check if its size exceeds the maximum allowed size.
	if !fileInfo.IsDir() {
		if fileInfo.Size() > MaxFileSize {
			return fmt.Errorf("%w: file size %d exceeds the maximum allowed size %d",
				ErrFileTooLarge, fileInfo.Size(), MaxFileSize)
		}
	}

	return nil
}

// readServerResponse reads and processes the server's response after a file transfer.
func readServerResponse(conn net.Conn) error {
	if err := conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		return fmt.Errorf("failed to set a read deadline: %w", err)
	}

	status, message, err := protocol.ReadResponse(conn)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("server closed connection unexpectedly")
		}
		return fmt.Errorf("failed to read the server response: %w", err)
	}

	if status == protocol.ResponseStatusError {
		return fmt.Errorf("server error: %s", message)
	}

	if message != "" {
		log.Printf("Server response: %s", message)
	}
	return nil
}

// contextWriter is a writer that supports context cancellation and coordination of the transfer with shutdown.
type contextWriter struct {
	ctx  context.Context
	conn net.Conn
}

// Write implements the `io.Writer` interface with context awareness.
func (cw *contextWriter) Write(p []byte) (n int, err error) {
	// Check if the context is done before proceeding with the write.
	select {
	case <-cw.ctx.Done():
		return 0, cw.ctx.Err()
	default:
		// Do nothing.
	}

	// Set a write deadline for this write operation.
	if err := cw.conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		return 0, err
	}

	// Perform the actual write to the connection.
	return cw.conn.Write(p)
}

// transferFile transfers a single file.
func transferFile(ctx context.Context, conn net.Conn, filePath string, relPath ...string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filePath, err)
	}

	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing file %s: %v", filePath, err)
		}
	}()

	statInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file information for %s: %v", filePath, err)
	}

	fmt.Printf("Calculating the file checksum...\n")
	checksum, err := protocol.CalculateFileChecksum(file)
	if err != nil {
		return fmt.Errorf("failed to calculate the file checksum: %v", err)
	}
	fmt.Printf("File checksum: %x\n", checksum)

	// Reset the file position to the beginning for the transfer.
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to reset file position: %v", err)
	}

	fileName := filepath.Base(filePath)
	// If there exists at least one relative path, meaning that the file is a subfile of a directory,
	// use the relative path instead of the file name.
	if len(relPath) > 0 {
		fileName = relPath[0]
	}

	// Determine the transfer type: if this is part of a directory transfer (`relPath` provided), use `TransferTypeDirectory`.
	transferType := uint8(protocol.TransferTypeFile)
	if len(relPath) > 0 {
		transferType = uint8(protocol.TransferTypeDirectory)
	}
	header := &protocol.Header{
		MessageType:   protocol.MessageTypeTransfer, // Message type for file transfer.
		FileSize:      uint64(statInfo.Size()),      // File size in bytes.
		FileName:      fileName,                     // Use relative path if provided.
		Checksum:      checksum,                     // File checksum.
		TransferType:  transferType,                 // Transfer type.
		DirectoryPath: "",                           // Not used for single file transfer.
	}

	fmt.Printf("Starting file transfer: %s (%d bytes)\n", header.FileName, header.FileSize)

	fmt.Printf("Sending file header...\n")
	if err := protocol.WriteHeader(conn, header); err != nil {
		return fmt.Errorf("failed to send file transfer header: %v", err)
	}
	fmt.Printf("Header sent successfully. Starting file transfer...\n")

	startTime := time.Now()

	// Create a progress reader to track the transfer progress.
	progressReader := protocol.NewProgressReader(file, header.FileSize, "Uploading", os.Stderr)

	// Create a context-aware writer that can be interrupted during shutdown.
	ctxWriter := &contextWriter{
		ctx:  ctx,
		conn: conn,
	}

	// Use a `WaitGroup` to coordinate the transfer with shutdown.
	var transferWg sync.WaitGroup
	transferWg.Add(1)

	var bytesWritten int64
	var transferErr error

	// Start the file transfer in a separate goroutine.
	go func() {
		defer transferWg.Done()
		bytesWritten, transferErr = io.Copy(ctxWriter, progressReader)
	}()

	// Wait for the transfer to complete or for a shutdown signal.
	transferDoneChan := make(chan struct{})
	go func() {
		transferWg.Wait()
		close(transferDoneChan)
	}()

	select {
	case <-transferDoneChan:
		// Transfer completed: do nothing.
	case <-ctx.Done():
		log.Printf("Transfer interrupted due to a shutdown signal")
		// Wait for a while for the transfer to finish gracefully.
		select {
		case <-transferDoneChan:
			log.Printf("Transfer completed after a shutdown signal")
		case <-time.After(ShutdownTimeout):
			log.Printf("Transfer did not complete within the shutdown timeout")
		}
	}

	progressReader.Complete()

	if transferErr != nil {
		return fmt.Errorf("failed to send file content: %v", transferErr)
	}

	if bytesWritten != int64(header.FileSize) {
		return fmt.Errorf("file transfer incomplete: expected %d bytes, sent %d bytes",
			header.FileSize, bytesWritten)
	}

	if err := readServerResponse(conn); err != nil {
		return fmt.Errorf("failed to read server response: %v", err)
	}

	transferDuration := time.Since(startTime)

	var transferRate float64
	if transferDuration.Seconds() > 0 {
		transferRate = float64(bytesWritten) / transferDuration.Seconds() / 1024 / 1024 // MB/s.
	} else {
		transferRate = 0
	}

	if bytesWritten < 1024 {
		log.Printf("File sent successfully! %d bytes sent in %v",
			bytesWritten, transferDuration)
	} else if bytesWritten < 1024*1024 {
		log.Printf("File sent successfully! %.1f KB sent in %v (%.2f MB/s)",
			toKB(uint64(bytesWritten)), transferDuration, transferRate)
	} else {
		log.Printf("File sent successfully! %.1f MB sent in %v (%.2f MB/s)",
			toMB(uint64(bytesWritten)), transferDuration, transferRate)
	}

	return nil
}

// validateDirectorySize validates the total size of the directory with the server before starting the transfer.
func validateDirectorySize(totalSize int64) error {
	// Create a connection to validate directory size.
	conn, err := net.DialTimeout("tcp", *serverAddr, ConnectionTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect for directory size validation: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing validation connection: %v", err)
		}
	}()

	if err := conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		return fmt.Errorf("failed to set read deadline: %v", err)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		return fmt.Errorf("failed to set write deadline: %v", err)
	}

	// Create a special header for directory size validation.
	header := &protocol.Header{
		MessageType:   protocol.MessageTypeValidate,   // Message type for validation.
		FileSize:      uint64(totalSize),              // Total size of the directory.
		FileName:      "",                             // Empty filename for validation messages.
		Checksum:      make([]byte, 32),               // Empty checksum for validation.
		TransferType:  protocol.TransferTypeDirectory, // Transfer type is directory.
		DirectoryPath: "",                             // Empty directory path.
	}

	if err := protocol.WriteHeader(conn, header); err != nil {
		return fmt.Errorf("failed to send the directory size validation header: %v", err)
	}

	if err := readServerResponse(conn); err != nil {
		return fmt.Errorf("directory size validation failed: %v", err)
	}

	log.Printf("Directory size validation successful: %.2f GB", toGB(uint64(totalSize)))
	return nil
}

// transferDirectory transfers a directory.
func transferDirectory(ctx context.Context, dirPath string) error {
	var allFiles []string
	var totalDirectorySize int64

	// Walk the directory and add all files to the list, calculating the total size.
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			allFiles = append(allFiles, path)
			totalDirectorySize += info.Size()
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk the directory %s: %v", dirPath, err)
	}

	log.Printf("Found %d files to transfer in the directory %s (total size: %.2f GB)",
		len(allFiles), dirPath, toGB(uint64(totalDirectorySize)))

	if err := validateDirectorySize(totalDirectorySize); err != nil {
		return fmt.Errorf("directory transfer rejected: %v", err)
	}

	var successfulTransfers, failedTransfers int
	var totalBytesTransferred int64

	log.Printf("Establishing a persistent connection for the directory transfer...")
	fileConn, err := net.DialTimeout("tcp", *serverAddr, ConnectionTimeout)
	if err != nil {
		return fmt.Errorf("failed to establish the connection for the directory transfer: %v", err)
	}

	defer func() {
		if err := fileConn.Close(); err != nil {
			log.Printf("Error closing the directory transfer connection: %v", err)
		}
		log.Printf("Directory transfer connection closed")
	}()

	if err := fileConn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		return fmt.Errorf("failed to set read deadline: %v", err)
	}
	if err := fileConn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		return fmt.Errorf("failed to set write deadline: %v", err)
	}

	log.Printf("Persistent connection established. Transferring %d files on the same connection...", len(allFiles))

	// Transfer all files in the directory using the persistent connection.
	for i, filePath := range allFiles {
		// Check for a shutdown signal before each file transfer.
		select {
		case <-ctx.Done():
			log.Printf("Directory transfer interrupted due to a shutdown signal")
			return fmt.Errorf("directory transfer interrupted: %v", ctx.Err())
		default:
		}

		// Refresh the connection timeouts for each file transfer.
		if err := fileConn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
			log.Printf("Failed to set read deadline for file %s: %v", filePath, err)
			failedTransfers++
			continue
		}
		if err := fileConn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
			log.Printf("Failed to set write deadline for file %s: %v", filePath, err)
			failedTransfers++
			continue
		}

		relPath, err := filepath.Rel(dirPath, filePath)
		if err != nil {
			log.Printf("Failed to calculate the relative path for %s: %v", filePath, err)
			failedTransfers++
			continue
		}
		fmt.Printf("Transferring file %d/%d: %s\n", i+1, len(allFiles), relPath)

		// The `transferFile` function will then handle the file transfer with the relative path instead of the plain file name.
		if err := transferFile(ctx, fileConn, filePath, relPath); err != nil {
			log.Printf("Failed to transfer file %s: %v", filePath, err)
			failedTransfers++
			// If a connection error is encountered, break the loop, since the connection is likely dead.
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "connection") {
				log.Printf("Connection error detected, aborting remaining transfers")
				break
			}
			continue
		}

		if fileInfo, err := os.Stat(filePath); err == nil {
			totalBytesTransferred += fileInfo.Size()
		}
		successfulTransfers++
	}

	log.Printf("Directory transfer completed: %s", dirPath)
	log.Printf("Transfer summary: %d successful, %d failed, %d total bytes",
		successfulTransfers, failedTransfers, totalBytesTransferred)

	if failedTransfers > 0 {
		return fmt.Errorf("directory transfer completed with %d failed transfers out of %d total files",
			failedTransfers, len(allFiles))
	}

	return nil
}

func main() {
	flag.Parse()

	setupLogging()

	log.Printf("Starting the file transfer client...")

	if err := validateArgs(); err != nil {
		log.Fatalf("Invalid command-line arguments: %v", err)
	}

	if err := validatePath(*filePath); err != nil {
		log.Fatalf("Path validation failed: %v", err)
	}

	fileInfo, err := os.Stat(*filePath)
	if err != nil {
		log.Fatalf("Failed to get the path information: %v", err)
	}

	isDirectory := fileInfo.IsDir()

	if isDirectory {
		log.Printf("Preparing the directory transfer: %s", *filePath)
	} else {
		log.Printf("Preparing the file transfer: %s", *filePath)
	}

	// Create context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Handle shutdown signals.
	go func() {
		sig := <-sigChan
		log.Printf("Shutdown signal received: %v. Starting graceful shutdown...", sig)
		cancel()
	}()

	if isDirectory {
		if err := transferDirectory(ctx, *filePath); err != nil {
			log.Fatalf("Directory transfer failed: %v", err)
		}
		return
	}

	log.Printf("Connecting to the server at %s...", *serverAddr)

	// Establish a TCP connection to the server using the server's address.
	conn, err := net.DialTimeout("tcp", *serverAddr, ConnectionTimeout)
	if err != nil {
		log.Fatalf("Failed to establish TCP connection to the server: %v", err)
	}

	// Close the connection when the surrounding function exits.
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection: %v", err)
		}
		log.Printf("Connection closed")
	}()

	log.Printf("Connected successfully to the server at %s", *serverAddr)

	// Set connection timeouts.
	if err := conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		log.Fatalf("Failed to set read deadline: %v", err)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		log.Fatalf("Failed to set write deadline: %v", err)
	}

	// Handle the single file transfer.
	if err := transferFile(ctx, conn, *filePath); err != nil {
		log.Fatalf("File transfer failed: %v", err)
	}

	log.Printf("Client shutting down.")
}
