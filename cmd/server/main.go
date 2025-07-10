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
	ErrInvalidFileSize = errors.New("invalid file size")
	ErrEmptyFilename   = errors.New("empty file name")
	ErrFileTooLarge    = errors.New("file size exceeds maximum allowed size")
)

// Constants to represent the file conflict handling strategies.
const (
	StrategyOverwrite = "overwrite" // Overwrite the existing file.
	StrategyRename    = "rename"    // Rename the file to avoid conflicts.
	StrategySkip      = "skip"      // Skip the file if it already exists.
)

// Constants to constrain the maximum file size, log prefix, and timeouts.
const (
	MaxFileSize     = 2 * 1024 * 1024 * 1024 // 2GB limit.
	LogPrefix       = "[SERVER]"             // Log prefix.
	ReadTimeout     = 30 * time.Second       // Read timeout.
	WriteTimeout    = 30 * time.Second       // Write timeout.
	ShutdownTimeout = 30 * time.Second       // Shutdown timeout.
)

// Command-line flags.
var (
	listenPort   = flag.String("port", "8080", "Listening port")
	destDir      = flag.String("dir", "test", "Destination directory for received files")
	fileStrategy = flag.String("strategy", "rename", "File conflict strategy: overwrite, rename, or skip")
)

// Function to configure structured logging with timestamps and custom prefix.
func setupLogging() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.SetPrefix(LogPrefix + " ")
}

// Function to perform comprehensive validation of the file transfer header.
func validateHeader(header *protocol.Header) error {
	if header == nil {
		return fmt.Errorf("header is nil")
	}

	if header.FileName == "" {
		return fmt.Errorf("%w: file name cannot be empty", ErrEmptyFilename)
	}

	// Check the file size based on the transfer type.
	maxSize := MaxFileSize
	if header.TransferType == protocol.TransferTypeDirectory {
		maxSize = protocol.MaxDirectorySize
	}

	if header.FileSize > uint64(maxSize) {
		return fmt.Errorf("%w: file size %d exceeds maximum allowed size %d",
			ErrFileTooLarge, header.FileSize, maxSize)
	}

	// Validate the file name to prevent directory traversal (for security).
	// Allow relative paths but prevent absolute paths and parent directory traversal.
	if filepath.IsAbs(header.FileName) {
		return fmt.Errorf("invalid file name: absolute paths not allowed: %s", header.FileName)
	}
	if strings.Contains(header.FileName, "..") {
		return fmt.Errorf("invalid file name: parent directory traversal not allowed: %s", header.FileName)
	}

	return nil
}

// Function to send an error message to the client.
func sendErrorResponse(conn net.Conn, message string) {
	errorMsg := fmt.Sprintf("ERROR: %s\n", message)
	if _, err := conn.Write([]byte(errorMsg)); err != nil {
		log.Printf("Failed to send error response to client: %v", err)
	}
}

// Function to handle file conflicts by applying the specified strategy.
func handleFileConflict(originalPath, fileName string, strategy string) (string, error) {
	// Check if the file exists.
	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		// If the file doesn't exist, return the original path.
		return originalPath, nil
	}

	switch strategy {
	case StrategyOverwrite:
		// Remove the existing file and return the original path.
		if err := os.Remove(originalPath); err != nil {
			return "", fmt.Errorf("failed to remove existing file: %v", err)
		}
		log.Printf("Overwriting existing file: %s", originalPath)
		return originalPath, nil

	case StrategyRename:
		// Generate a new file name with a suffix.
		return generateUniqueFilename(originalPath, fileName), nil

	case StrategySkip:
		// Return an error to indicate that the file should be skipped.
		return "", fmt.Errorf("file already exists and skip strategy is enabled: %s", originalPath)

	default:
		return "", fmt.Errorf("unknown file conflict strategy: %s", strategy)
	}
}

// Function to generate a unique file name by adding a numeric suffix.
func generateUniqueFilename(originalPath, fileName string) string {
	dir := filepath.Dir(originalPath)
	ext := filepath.Ext(fileName)
	baseName := strings.TrimSuffix(fileName, ext)

	counter := 1
	for {
		newFileName := fmt.Sprintf("%s_%d%s", baseName, counter, ext)
		newPath := filepath.Join(dir, newFileName)

		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			// If the file doesn't exist, return the new path.
			log.Printf("Renaming file to avoid conflict: %s -> %s", fileName, newFileName)
			return newPath
		}
		// Increment the counter and try again if the file already exists.
		counter++
	}
}

// Struct to wrap a net.Conn to support context cancellation and coordination of the transfer with shutdown.
type contextReader struct {
	ctx  context.Context
	conn net.Conn
}

// Function to handle a client connection with context support for graceful shutdown.
func handleConnection(ctx context.Context, conn net.Conn, wg *sync.WaitGroup) {
	// Get the start time and the client address of the connection.
	startTime := time.Now()
	clientAddr := conn.RemoteAddr().String()

	// Defer the done ("Done decrements the [WaitGroup] counter by one") of the wait group and
	// the close of the connection ("Close closes the connection.
	// Any blocked Read or Write operations will be unblocked and return errors.").
	defer func() {
		wg.Done()
		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection to %s: %v", clientAddr, err)
		}
		log.Printf("Connection to %s closed (duration: %v)", clientAddr, time.Since(startTime))
	}()

	log.Printf("New connection established from %s", clientAddr)

	// Set connection timeouts to prevent hanging connections.
	if err := conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		log.Printf("Failed to set read deadline: %v", err)
		sendErrorResponse(conn, "Internal server error")
		return
	}
	if err := conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		log.Printf("Failed to set write deadline: %v", err)
		sendErrorResponse(conn, "Internal server error")
		return
	}

	// Read the file transfer header.
	header, err := protocol.ReadHeader(conn)
	if err != nil {
		log.Printf("Failed to read file transfer header from %s: %v", clientAddr, err)

		// Create an error message with the error details.
		errorMsg := "Failed to read file transfer header: " + err.Error()

		// Send the error response to the client.
		// Only send error response if connection is still valid.
		if !errors.Is(err, io.EOF) {
			sendErrorResponse(conn, errorMsg)
		}
		return
	}

	// Validate the header.
	if err := validateHeader(header); err != nil {
		log.Printf("Invalid header from %s: %v", clientAddr, err)
		sendErrorResponse(conn, fmt.Sprintf("Invalid file header: %v", err))
		return
	}

	// Log transfer reception start.
	transferType := "file"
	if header.TransferType == protocol.TransferTypeDirectory {
		transferType = "directory"
	}
	log.Printf("Receiving %s from %s: %s (size: %d bytes)", transferType, clientAddr, header.FileName, header.FileSize)

	// Create the directory to save the received file (if it doesn't exist).
	// `0755`: "OwnerCanDoAllExecuteGroupOtherCanReadExecute" (https://pkg.go.dev/gitlab.com/evatix-go/core/filemode).
	if err := os.MkdirAll(*destDir, 0755); err != nil {
		log.Printf("Failed to create directory %s for client %s: %v", *destDir, clientAddr, err)
		sendErrorResponse(conn, "Failed to create output directory")
		return
	}

	// Handle the file path:
	var outputPath string
	var receivedFileName string

	// If the file name contains directory separators (either `/` or `\`), it is a relative path,
	// so preserve the directory structure.
	if strings.Contains(header.FileName, "/") || strings.Contains(header.FileName, string(filepath.Separator)) {
		outputPath = filepath.Join(*destDir, header.FileName)
		receivedFileName = header.FileName
		// Create the directory structure if it doesn't exist.
		outputDir := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Printf("Failed to create directory structure %s for client %s: %v", outputDir, clientAddr, err)
			sendErrorResponse(conn, "Failed to create directory structure")
			return
		}
	} else {
		// Since it is a simple file name, use the original name.
		receivedFileName = header.FileName
		outputPath = filepath.Join(*destDir, receivedFileName)
	}

	// Handle file conflicts using the specified strategy.
	finalPath, err := handleFileConflict(outputPath, receivedFileName, *fileStrategy)
	if err != nil {
		if strings.Contains(err.Error(), "skip strategy is enabled") {
			log.Printf("Skipping file from %s: %v", clientAddr, err)
			sendErrorResponse(conn, "File already exists and skip strategy is enabled")
		} else {
			log.Printf("Failed to handle file conflict for %s: %v", clientAddr, err)
			sendErrorResponse(conn, fmt.Sprintf("Failed to handle file conflict: %v", err))
		}
		return
	}

	// Create the output file.
	outputFile, err := os.Create(finalPath)
	if err != nil {
		log.Printf("Failed to create output file %s for client %s: %v", finalPath, clientAddr, err)
		sendErrorResponse(conn, "Failed to create output file")
		return
	}

	// Close the output file when the surrounding function exits.
	defer func() {
		if err := outputFile.Close(); err != nil {
			log.Printf("Error closing output file %s: %v", finalPath, err)
		}
	}()

	// Read the file content into a buffer for verification.
	log.Printf("Receiving file content from %s...", clientAddr)

	// Create a buffer to hold the entire file content for verification.
	fileBuffer := make([]byte, header.FileSize)

	// Create a context-aware reader that can be interrupted during shutdown.
	ctxReader := &contextReader{
		ctx:  ctx,
		conn: conn,
	}

	// Read the entire file content into the buffer.
	_, err = io.ReadFull(ctxReader, fileBuffer)
	if err != nil {
		log.Printf("Failed to receive file content from %s: %v", clientAddr, err)
		if errors.Is(err, io.EOF) {
			log.Printf("Client %s disconnected during file transfer", clientAddr)
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			log.Printf("Client %s sent incomplete file data", clientAddr)
		}
		if ctx.Err() != nil {
			log.Printf("Transfer interrupted due to server shutdown: %v", ctx.Err())
		}
		// Fallback to a generic message.
		sendErrorResponse(conn, "Failed to receive file content")
		return
	}

	// Verify the data checksum before writing to disk.
	log.Printf("Verifying received data integrity...")
	if err := protocol.VerifyDataChecksum(fileBuffer, header.Checksum); err != nil {
		log.Printf("Data checksum verification failed for client %s: %v", clientAddr, err)
		sendErrorResponse(conn, "Data integrity check failed")
		return
	}
	log.Printf("Data checksum verification successful")

	// Handle single file transfer (directories are now handled as individual files).
	if err := handleFileTransfer(ctx, conn, clientAddr, header, fileBuffer, finalPath); err != nil {
		log.Printf("Failed to handle file transfer from %s: %v", clientAddr, err)
		sendErrorResponse(conn, fmt.Sprintf("Failed to handle file transfer: %v", err))
		return
	}

	// Send the success response to the client.
	successMsg := "SUCCESS: Transfer received successfully!\n"
	if _, err := conn.Write([]byte(successMsg)); err != nil {
		log.Printf("Failed to send success response to client %s: %v", clientAddr, err)
	}

	transferDuration := time.Since(startTime)
	log.Printf("Transfer completed from %s (duration: %v)", clientAddr, transferDuration)
}

// Function to handle the transfer of a single file.
func handleFileTransfer(ctx context.Context, conn net.Conn, clientAddr string, header *protocol.Header, fileBuffer []byte, finalPath string) error {
	// Create the output file.
	outputFile, err := os.Create(finalPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}

	// Close the output file when the surrounding function exits.
	defer func() {
		if err := outputFile.Close(); err != nil {
			log.Printf("Error closing output file %s: %v", finalPath, err)
		}
	}()

	// Write the verified buffer to disk with progress tracking.
	progressWriter := protocol.NewProgressWriter(outputFile, int64(header.FileSize), fmt.Sprintf("Writing %s", header.FileName))
	bytesWritten, err := progressWriter.Write(fileBuffer)
	if err != nil {
		log.Printf("Failed to receive file content from %s: %v", clientAddr, err)
		if errors.Is(err, io.EOF) {
			log.Printf("Client %s disconnected during file transfer", clientAddr)
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			log.Printf("Client %s sent incomplete file data", clientAddr)
		}
		if ctx.Err() != nil {
			log.Printf("Transfer interrupted due to server shutdown: %v", ctx.Err())
		}
		// Fallback to a generic message.
		sendErrorResponse(conn, "Failed to receive file content")

		// Clean up the incomplete file.
		if err := os.Remove(finalPath); err != nil {
			log.Printf("Failed to remove partial file %s: %v", finalPath, err)
		}
		return fmt.Errorf("failed to write file content: %v", err)
	}

	// Mark transfer as complete and log the final statistics.
	progressWriter.Complete()

	// Verify if the bytes written are equal to the file size.
	if bytesWritten != len(fileBuffer) {
		log.Printf("File size mismatch for client %s: expected %d, received %d",
			clientAddr, len(fileBuffer), bytesWritten)
		sendErrorResponse(conn, "File size mismatch")
		// Clean up the incomplete file.
		if err := os.Remove(finalPath); err != nil {
			log.Printf("Failed to remove incomplete file %s: %v", finalPath, err)
		}
		return fmt.Errorf("file size mismatch: expected %d, received %d", len(fileBuffer), bytesWritten)
	}

	// File data integrity verified successfully.
	log.Printf("File integrity verified for %s", header.FileName)
	return nil
}

// Function to read from the connection with context support.
func (cr *contextReader) Read(p []byte) (n int, err error) {
	// Check if context is cancelled before reading.
	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
		// Do nothing.
	}

	// Set a read deadline for this read operation.
	if err := cr.conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		return 0, err
	}

	return cr.conn.Read(p)
}

func main() {
	// Parse command-line flags.
	flag.Parse()

	// Validate the file strategy flag.
	switch *fileStrategy {
	case StrategyOverwrite, StrategyRename, StrategySkip:
		// Do nothing.
	default:
		log.Fatalf("Invalid file strategy: %s. Must be one of: %s, %s, %s",
			*fileStrategy, StrategyOverwrite, StrategyRename, StrategySkip)
	}

	// Setup structured logging.
	setupLogging()

	log.Printf("Starting file transfer server...")

	// Create a context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Establish a listener on the specified port and listen for incoming connections.
	listener, err := net.Listen("tcp", ":"+*listenPort)
	if err != nil {
		log.Fatalf("Failed to start listening for incoming connections: %v", err)
	}

	// Close the listener when the surrounding function exits.
	defer func() {
		if err := listener.Close(); err != nil {
			log.Printf("Error closing listener: %v", err)
		}
		log.Printf("Server listener closed")
	}()

	log.Printf("Server is listening on port %s...", *listenPort)

	// Create a wait group to wait for all connections ("a collection of goroutines") to finish.
	var wg sync.WaitGroup

	// Set up signal handling for graceful shutdown.
	// Create a channel to receive signals.
	// The channel is buffered to hold one signal without blocking the sender (the OS signal handler).
	receiveSigChannel := make(chan os.Signal, 1)
	// Set up an OS signal handler to relay signals to the channel.
	signal.Notify(receiveSigChannel, syscall.SIGINT, syscall.SIGTERM)
	// Create a channel that carries an empty struct (since no data is needed to be sent) to signal the main loop to stop accepting new connections.
	// The channel is unbuffered to ensure that the main loop only stops accepting new connections when all active connections have finished.
	shutdownChannel := make(chan struct{})

	// Launch the enclosed function as a goroutine so that it runs concurrently with the main program.
	go func() {
		// Receive a signal from the channel.
		// Block until a signal is received on the channel.
		sig := <-receiveSigChannel
		log.Printf("Shutdown signal received: %v. Starting graceful shutdown...", sig)

		// Cancel the context to signal all active transfers to stop.
		cancel()

		// Close the listener (stop accepting new connections).
		if err := listener.Close(); err != nil {
			log.Printf("Error closing listener during shutdown: %v", err)
		}

		// Close the shutdown channel to signal the main loop to stop accepting new connections.
		close(shutdownChannel)

		// Wait for active transfers with timeout.
		log.Printf("Waiting for active transfers to complete (timeout: %v)...", ShutdownTimeout)
		doneChannel := make(chan struct{})
		go func() {
			wg.Wait()
			close(doneChannel)
		}()
		select {
		case <-doneChannel:
			log.Printf("All active transfers completed successfully.")
		case <-time.After(ShutdownTimeout):
			log.Printf("Shutdown timeout reached. Forcing shutdown...")
		}
	}()

	// Accept incoming connections in an infinite loop.
	for {
		// Accept a client connection.
		conn, err := listener.Accept()
		if err != nil {
			select {
			// If the shutdown channel is closed, (stop accepting new connections and) wait for all connections to finish.
			case <-shutdownChannel:
				log.Printf("Stopped accepting new connections.")
				// Wait for all connections to finish.
				wg.Wait()
				log.Printf("All active connections finished. Server exiting.")
				return
			default:
				log.Printf("Failed to accept client connection: %v", err)
				continue
			}
		}
		// Increment the `sync.WaitGroup` counter by 1 to indicate that a new client connection (handled in a new goroutine) has started
		// so that the server will wait for this connection to finish before shutting down.
		wg.Add(1)

		// Launch a new goroutine to handle the client connection so that the server can concurrently handle multiple connections.
		go handleConnection(ctx, conn, &wg)
	}
}
