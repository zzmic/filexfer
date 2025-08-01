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

// Custom error types for better error handling.
var (
	ErrFileNotFound     = errors.New("file not found")
	ErrFileTooLarge     = errors.New("file size exceeds maximum allowed size")
	ErrInvalidFilename  = errors.New("invalid filename")
	ErrConnectionFailed = errors.New("connection failed")
)

// Constants to constrain the maximum file size, log prefix, and timeouts.
const (
	MaxFileSize       = 5 * 1024 * 1024 * 1024 // 5GB limit.
	LogPrefix         = "[CLIENT]"             // Log prefix.
	ConnectionTimeout = 30 * time.Second       // Connection timeout.
	ReadTimeout       = 30 * time.Second       // Read timeout.
	WriteTimeout      = 30 * time.Second       // Write timeout.
	ShutdownTimeout   = 30 * time.Second       // Shutdown timeout.
)

// Command-line flags for the client.
var (
	serverAddr = flag.String("server", "localhost:8080", "Server address (IP:Port)")
	filePath   = flag.String("file", "", "File or directory to be transferred (required)")
)

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

// validatePath performs comprehensive validation of the file or directory to be sent.
func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("%w: path cannot be empty", ErrInvalidFilename)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrFileNotFound, path)
		}
		return fmt.Errorf("failed to get path information for %s: %w", path, err)
	}

	// For files, perform additional validation.
	if !fileInfo.IsDir() {
		if fileInfo.Size() > MaxFileSize {
			return fmt.Errorf("%w: file size %d exceeds maximum allowed size %d",
				ErrFileTooLarge, fileInfo.Size(), MaxFileSize)
		}

		filename := filepath.Base(path)
		if filepath.Base(filename) != filename {
			return fmt.Errorf("invalid filename: contains path separators: %s", filename)
		}
	}

	return nil
}

// readServerResponse reads and processes the server's response.
func readServerResponse(conn net.Conn) error {
	// Set a short timeout for reading the response.
	if err := conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		return fmt.Errorf("failed to set a read deadline: %w", err)
	}

	// Read the response from the server.
	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("server closed connection unexpectedly")
		}
		return fmt.Errorf("failed to read server response: %w", err)
	}

	// Convert the response to a string.
	responseStr := string(response[:n])
	if _, after, found := strings.Cut(responseStr, "ERROR:"); found {
		return fmt.Errorf("server error: %s", strings.TrimSpace(after))
	}
	if strings.HasPrefix(responseStr, "SUCCESS:") {
		log.Printf("Server response: %s", strings.TrimSpace(responseStr))
		return nil
	}

	// Fallback to a generic message.
	log.Printf("Server response: %s", strings.TrimSpace(responseStr))
	return nil
}

// A contextWriter wraps a net.Conn to support context cancellation and coordination of the transfer with shutdown.
type contextWriter struct {
	ctx  context.Context
	conn net.Conn
}

// Write (for contextWriter) writes to the connection with context support.
func (cw *contextWriter) Write(p []byte) (n int, err error) {
	// Check if context is cancelled before writing.
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

	return cw.conn.Write(p)
}

func main() {
	// Parse command-line flags.
	flag.Parse()

	// Setup structured logging.
	setupLogging()

	log.Printf("Starting the file transfer client...")

	// Validate command-line arguments.
	if err := validateArgs(); err != nil {
		log.Fatalf("Invalid command-line arguments: %v", err)
	}

	// Validate the path before attempting to connect.
	if err := validatePath(*filePath); err != nil {
		log.Fatalf("Path validation failed: %v", err)
	}

	// Check if the path is a directory or file.
	fileInfo, err := os.Stat(*filePath)
	if err != nil {
		log.Fatalf("Failed to get path information: %v", err)
	}

	isDirectory := fileInfo.IsDir()

	if isDirectory {
		log.Printf("Preparing directory transfer: %s", *filePath)
	} else {
		log.Printf("Preparing file transfer: %s", *filePath)
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

	// Handle a directory transfer.
	if isDirectory {
		if err := transferDirectory(ctx, *filePath); err != nil {
			log.Fatalf("Directory transfer failed: %v", err)
		}
		return
	}

	// Handle a single file transfer.
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

	if isDirectory {
		// Handle directory transfer.
		if err := transferDirectory(ctx, *filePath); err != nil {
			log.Fatalf("Directory transfer failed: %v", err)
		}
	} else {
		// Handle single file transfer.
		if err := transferFile(ctx, conn, *filePath); err != nil {
			log.Fatalf("File transfer failed: %v", err)
		}
	}

	log.Printf("Client shutting down.")
}

// transferFile transfers a single file.
func transferFile(ctx context.Context, conn net.Conn, filePath string, relPath ...string) error {
	// Open the file to send.
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filePath, err)
	}

	// Close the file when the surrounding function exits.
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing file %s: %v", filePath, err)
		}
	}()

	// Get the file information.
	statInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file information for %s: %v", filePath, err)
	}

	// Calculate the file checksum.
	fmt.Printf("Calculating file checksum...\n")
	checksum, err := protocol.CalculateFileChecksum(file)
	if err != nil {
		return fmt.Errorf("failed to calculate file checksum: %v", err)
	}
	fmt.Printf("File checksum: %x\n", checksum)

	// Reset the file position to the beginning for the transfer.
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to reset file position: %v", err)
	}

	// Determine the file name to use in the header.
	fileName := filepath.Base(filePath)
	// If there exists at least one relative path, meaning that the file is a subfile of a directory,
	// use the relative path instead of the file name.
	if len(relPath) > 0 {
		fileName = relPath[0]
	}

	// Create the header.
	// Determine the transfer type: if this is part of a directory transfer (`relPath` provided), use `TransferTypeDirectory`.
	transferType := uint8(protocol.TransferTypeFile)
	if len(relPath) > 0 {
		transferType = uint8(protocol.TransferTypeDirectory)
	}
	header := &protocol.Header{
		FileSize:      uint64(statInfo.Size()),
		FileName:      fileName,
		Checksum:      checksum,
		TransferType:  transferType,
		DirectoryPath: "",
	}

	fmt.Printf("Starting file transfer: %s (%d bytes)\n", header.FileName, header.FileSize)

	// Send the header first.
	fmt.Printf("Sending file header...\n")
	if err := protocol.WriteHeader(conn, header); err != nil {
		return fmt.Errorf("failed to send file transfer header: %v", err)
	}
	fmt.Printf("Header sent successfully. Starting file transfer...\n")

	// Send the file content with progress tracking.
	startTime := time.Now()

	// Create a progress reader to track upload progress.
	progressReader := protocol.NewProgressReader(file, int64(header.FileSize), "Uploading")

	// Create a context-aware writer that can be interrupted during shutdown.
	ctxWriter := &contextWriter{
		ctx:  ctx,
		conn: conn,
	}

	// Use a WaitGroup to coordinate the transfer with shutdown.
	var transferWg sync.WaitGroup
	transferWg.Add(1)

	var bytesWritten int64
	var transferErr error

	go func() {
		defer transferWg.Done()
		bytesWritten, transferErr = io.Copy(ctxWriter, progressReader)
	}()

	// Wait for the transfer to complete or the context to be cancelled.
	transferDoneChan := make(chan struct{})
	go func() {
		transferWg.Wait()
		close(transferDoneChan)
	}()

	select {
	case <-transferDoneChan:
		// Transfer completed.
	case <-ctx.Done():
		log.Printf("Transfer interrupted due to shutdown signal")
		// Wait for a while for the transfer to finish gracefully.
		select {
		case <-transferDoneChan:
			log.Printf("Transfer completed after shutdown signal")
		case <-time.After(ShutdownTimeout):
			log.Printf("Transfer did not complete within shutdown timeout")
		}
	}

	// Mark the transfer as complete.
	progressReader.Complete()

	if transferErr != nil {
		return fmt.Errorf("failed to send file content: %v", transferErr)
	}

	// Verify if the bytes written are equal to the file size.
	if bytesWritten != int64(header.FileSize) {
		return fmt.Errorf("file transfer incomplete: expected %d bytes, sent %d bytes",
			header.FileSize, bytesWritten)
	}

	// Read the server response.
	if err := readServerResponse(conn); err != nil {
		return fmt.Errorf("failed to read server response: %v", err)
	}

	transferDuration := time.Since(startTime)

	// Calculate the transfer rate.
	var transferRate float64
	if transferDuration.Seconds() > 0 {
		transferRate = float64(bytesWritten) / transferDuration.Seconds() / 1024 / 1024 // MB/s.
	} else {
		transferRate = 0
	}

	// Format the output based on the t	ransfer size.
	if bytesWritten < 1024 {
		log.Printf("File sent successfully! %d bytes sent in %v",
			bytesWritten, transferDuration)
	} else if bytesWritten < 1024*1024 {
		log.Printf("File sent successfully! %.1f KB sent in %v (%.2f MB/s)",
			float64(bytesWritten)/1024, transferDuration, transferRate)
	} else {
		log.Printf("File sent successfully! %.1f MB sent in %v (%.2f MB/s)",
			float64(bytesWritten)/1024/1024, transferDuration, transferRate)
	}

	return nil
}

// validateDirectorySize validates the directory size with the server before starting transfers.
func validateDirectorySize(totalSize int64) error {
	// Create a connection to validate directory size.
	conn, err := net.DialTimeout("tcp", *serverAddr, ConnectionTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect for directory size validation: %v", err)
	}
	defer conn.Close()

	// Set connection timeouts.
	if err := conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		return fmt.Errorf("failed to set read deadline: %v", err)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		return fmt.Errorf("failed to set write deadline: %v", err)
	}

	// Create a special header for directory size validation.
	header := &protocol.Header{
		FileSize:      uint64(totalSize),              // Total size of the directory.
		FileName:      "DIRECTORY_SIZE_VALIDATION",    // Special filename for directory size validation.
		Checksum:      make([]byte, 32),               // Empty checksum for validation.
		TransferType:  protocol.TransferTypeDirectory, // Transfer type is directory.
		DirectoryPath: "",                             // Empty directory path.
	}

	// Send the validation header.
	if err := protocol.WriteHeader(conn, header); err != nil {
		return fmt.Errorf("failed to send directory size validation header: %v", err)
	}

	// Read the server response.
	if err := readServerResponse(conn); err != nil {
		return fmt.Errorf("directory size validation failed: %v", err)
	}

	log.Printf("Directory size validation successful: %.2f GB", float64(totalSize)/1024/1024/1024)
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
		return fmt.Errorf("failed to walk directory %s: %v", dirPath, err)
	}

	log.Printf("Found %d files to transfer in directory %s (total size: %.2f GB)",
		len(allFiles), dirPath, float64(totalDirectorySize)/1024/1024/1024)

	// Validate the directory size with the server before starting transfers.
	if err := validateDirectorySize(totalDirectorySize); err != nil {
		return fmt.Errorf("directory transfer rejected: %v", err)
	}

	// Track the transfer statistics.
	var successfulTransfers, failedTransfers int
	var totalBytesTransferred int64

	// Transfer each file individually using separate connections.
	for i, filePath := range allFiles {
		// Check if the context is cancelled.
		select {
		case <-ctx.Done():
			log.Printf("Directory transfer interrupted due to shutdown signal")
			return fmt.Errorf("directory transfer interrupted: %v", ctx.Err())
		default:
		}

		// Create a new connection for each file to avoid protocol issues.
		fileConn, err := net.DialTimeout("tcp", *serverAddr, ConnectionTimeout)
		if err != nil {
			log.Printf("Failed to connect for file %s: %v", filePath, err)
			failedTransfers++
			continue
		}

		// Ensure connection is closed in all cases.
		defer func(conn net.Conn, path string) {
			if err := conn.Close(); err != nil {
				log.Printf("Error closing connection for %s: %v", path, err)
			}
		}(fileConn, filePath)

		// Set connection timeouts.
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

		// Calculate the relative path from the root directory.
		relPath, err := filepath.Rel(dirPath, filePath)
		if err != nil {
			log.Printf("Failed to calculate relative path for %s: %v", filePath, err)
			failedTransfers++
			continue
		}
		fmt.Printf("Transferring file %d/%d: %s\n", i+1, len(allFiles), relPath)

		// Transfer the file with the relative path.
		// The `transferFile` function will then handle the file transfer with the relative path instead of the plain file name.
		if err := transferFile(ctx, fileConn, filePath, relPath); err != nil {
			log.Printf("Failed to transfer file %s: %v", filePath, err)
			failedTransfers++
			continue
		}

		// Get file size for statistics.
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
