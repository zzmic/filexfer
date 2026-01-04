package main

import (
	"bytes"
	"context"
	"crypto/sha256"
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

// Errors for representing specific validation failures.
var (
	ErrInvalidFileSize   = errors.New("invalid file size")
	ErrEmptyFilename     = errors.New("empty file name")
	ErrFileTooLarge      = errors.New("file size exceeds maximum allowed size")
	ErrDirectoryTooLarge = errors.New("directory transfer size exceeds maximum allowed size")
)

// Constants for file conflict-resolution strategies.
const (
	StrategyOverwrite = "overwrite" // Overwrite the existing file.
	StrategyRename    = "rename"    // Rename the file to avoid conflicts.
	StrategySkip      = "skip"      // Skip the file if it already exists.
)

// Constants for server configuration.
const (
	MaxFileSize      = 5 * 1024 * 1024 * 1024  // 5GB limit.
	MaxDirectorySize = 50 * 1024 * 1024 * 1024 // 50GB limit for directory transfers.
	LogPrefix        = "[SERVER]"              // Log prefix.
	ReadTimeout      = 30 * time.Second        // Read timeout.
	WriteTimeout     = 30 * time.Second        // Write timeout.
	ShutdownTimeout  = 30 * time.Second        // Shutdown timeout.
)

// Command-line flags for server configuration.
var (
	listenPort       = flag.String("port", "8080", "Listening port")
	destDir          = flag.String("dir", "test", "Destination directory for received files")
	fileStrategy     = flag.String("strategy", "rename", "File conflict-resolution strategy: overwrite, rename, or skip")
	maxDirectorySize = flag.Uint64("max-dir-size", MaxDirectorySize, "Maximum directory transfer size in bytes")
)

// Global variables for tracking directory sizes per client.
var (
	directorySizes = make(map[string]uint64) // `clientAddr` -> total directory size.
	dirSizeMutex   sync.RWMutex              // Mutex for synchronizing access to `directorySizes` map.
)

// contextReader supports reading from a connection with context cancellation support.
type contextReader struct {
	ctx  context.Context
	conn net.Conn
}

// setupLogging configures structured logging with timestamps and custom prefix.
func setupLogging() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.SetPrefix(LogPrefix + " ")
}

// toGB converts bytes to gigabytes.
func toGB(bytes uint64) float64 {
	return float64(bytes) / 1024 / 1024 / 1024
}

// sanitizePath performs deep sanitization of file paths to prevent path traversal attacks.
// It normalizes the path using `filepath.Clean` and verifies the result is a sub-path of the base directory.
func sanitizePath(baseDir, userPath string) (string, error) {
	if userPath == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	if filepath.IsAbs(userPath) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", userPath)
	}

	if strings.Contains(userPath, "..") {
		return "", fmt.Errorf("parent directory traversal is not allowed: %s", userPath)
	}

	baseDir = filepath.Clean(baseDir)

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %v", err)
	}

	fullPath := filepath.Clean(filepath.Join(baseDir, userPath))

	absFull, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve full path: %v", err)
	}

	relPath, err := filepath.Rel(absBase, absFull)
	if err != nil {
		return "", fmt.Errorf("failed to compute relative path: %v", err)
	}

	if strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
		return "", fmt.Errorf("path traversal attempt detected: %s escapes the base directory", userPath)
	}

	if !strings.HasPrefix(absFull+string(filepath.Separator), absBase+string(filepath.Separator)) && absFull != absBase {
		return "", fmt.Errorf("path is outside the base directory: %s", userPath)
	}

	return fullPath, nil
}

// validateHeader performs a series of checks on the file transfer header to ensure it meets security and protocol requirements.
func validateHeader(header *protocol.Header, clientAddr string) error {
	if header == nil {
		return fmt.Errorf("header is nil")
	}

	if header.TransferType == protocol.TransferTypeDirectory {
		if header.MessageType == protocol.MessageTypeValidate {
			if header.FileSize > *maxDirectorySize {
				return fmt.Errorf("%w: directory size %d bytes exceeds maximum allowed size %d bytes",
					ErrDirectoryTooLarge, header.FileSize, *maxDirectorySize)
			}
			return nil
		}

		dirSizeMutex.RLock()
		currentDirSize := directorySizes[clientAddr]
		newTotalSize := currentDirSize + header.FileSize
		dirSizeMutex.RUnlock()

		if newTotalSize > *maxDirectorySize {
			return fmt.Errorf("%w: directory transfer size %d bytes would exceed maximum allowed size %d bytes (current: %d bytes, adding: %d bytes, expected total: %d bytes, exceeds by: %d bytes)",
				ErrDirectoryTooLarge, newTotalSize, *maxDirectorySize, currentDirSize, header.FileSize, newTotalSize, newTotalSize-*maxDirectorySize)
		}
	} else {
		maxSize := uint64(MaxFileSize)
		if header.FileSize > maxSize {
			return fmt.Errorf("%w: file size %d bytes exceeds maximum allowed size %d bytes",
				ErrFileTooLarge, header.FileSize, maxSize)
		}
	}

	if header.MessageType == protocol.MessageTypeTransfer && header.FileName == "" {
		return fmt.Errorf("%w: file name cannot be empty", ErrEmptyFilename)
	}

	if header.MessageType == protocol.MessageTypeTransfer {
		if _, err := sanitizePath(*destDir, header.FileName); err != nil {
			return fmt.Errorf("invalid file name: %v", err)
		}
	}

	return nil
}

// sendErrorResponse sends a structured error response to the client.
func sendErrorResponse(conn net.Conn, message string) {
	if err := protocol.WriteResponse(conn, protocol.ResponseStatusError, message); err != nil {
		log.Printf("Failed to send an error response to client: %v", err)
	}
}

// sendSuccessResponse sends a structured success response to the client.
func sendSuccessResponse(conn net.Conn, message string) {
	if err := protocol.WriteResponse(conn, protocol.ResponseStatusSuccess, message); err != nil {
		log.Printf("Failed to send a success response to client: %v", err)
	}
}

// getDirectoryStats gets the stats of active directory transfers.
func getDirectoryStats() (int, uint64) {
	dirSizeMutex.RLock()
	defer dirSizeMutex.RUnlock()

	numClient := len(directorySizes)
	var totalSize uint64
	for _, size := range directorySizes {
		totalSize += size
	}

	return numClient, totalSize
}

// resolveFilePath resolves the file path for the "overwrite" and "skip" conflict-resolution strategies.
func resolveFilePath(originalPath string, strategy string) (string, error) {
	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		return originalPath, nil
	}

	switch strategy {
	case StrategyOverwrite:
		if err := os.Remove(originalPath); err != nil {
			return "", fmt.Errorf("failed to remove existing file: %v", err)
		}
		log.Printf("Overwriting existing file: %s", originalPath)
		return originalPath, nil

	case StrategySkip:
		return "", fmt.Errorf("file already exists and skip conflict-resolution strategy is enabled: %s", originalPath)

	default:
		return "", fmt.Errorf("unknown file conflict-resolution strategy: %s", strategy)
	}
}

// generateUniqueFile atomically creates a unique file by adding a numeric suffix for the "rename" strategy.
func generateUniqueFile(originalPath, fileName string) (*os.File, string, error) {
	dir := filepath.Dir(originalPath)
	ext := filepath.Ext(fileName)
	baseName := strings.TrimSuffix(fileName, ext)

	counter := 1
	for {
		newFileName := fmt.Sprintf("%s_%d%s", baseName, counter, ext)
		newPath := filepath.Join(dir, newFileName)

		// Use `os.OpenFile` with `os.O_RDWR|os.O_CREATE|os.O_EXCL` to create the file atomically,
		// thereby preventing race conditions when multiple clients upload files with the same name concurrently.
		f, err := os.OpenFile(newPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			log.Printf("Renaming file to avoid conflict: %s -> %s", fileName, newFileName)
			return f, newPath, nil
		}

		// If the error is not "file exists", return the error; otherwise, try the next suffix.
		if !os.IsExist(err) {
			return nil, "", fmt.Errorf("failed to create a unique file: %v", err)
		}
		counter++
	}
}

// handleConnection handles a client connection with context support for graceful shutdown.
func handleConnection(ctx context.Context, conn net.Conn, wg *sync.WaitGroup) {
	startTime := time.Now()
	clientAddr := conn.RemoteAddr().String()

	// Defer the done ("Done decrements the [WaitGroup] counter by one") of the wait group and
	// the close of the connection ("Close closes the connection. Any blocked Read or Write operations will be unblocked and return errors.").
	defer func() {
		// Decrement the `sync.WaitGroup` counter by 1 to indicate that a client connection has finished.
		wg.Done()

		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection to %s: %v", clientAddr, err)
		}

		dirSizeMutex.Lock()
		// Since the connection is closed, remove the entry from the map (atomically).
		delete(directorySizes, clientAddr)
		dirSizeMutex.Unlock()

		log.Printf("Connection to %s closed (duration: %v)", clientAddr, time.Since(startTime))
	}()

	log.Printf("New connection established from %s", clientAddr)

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

	// Handle multiple file transfers on the same connection to persist the connection
	// until the client closes the connection or an error occurs.
	for {
		// At the beginning of each iteration,
		// refresh connection timeouts for each file transfer to prevent hanging connections.
		if err := conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
			log.Printf("Failed to set read deadline: %v", err)
			return
		}
		if err := conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
			log.Printf("Failed to set write deadline: %v", err)
			return
		}

		header, err := protocol.ReadHeader(conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Printf("Client %s closed connection (end of session)", clientAddr)
				return
			}

			log.Printf("Failed to read file transfer header from %s: %v", clientAddr, err)
			if !errors.Is(err, io.EOF) {
				sendErrorResponse(conn, "Failed to read file transfer header: "+err.Error())
			}
			return
		}

		if err := validateHeader(header, clientAddr); err != nil {
			log.Printf("Header validation failed from %s: %v", clientAddr, err)
			sendErrorResponse(conn, err.Error())
			return
		}

		if header.MessageType == protocol.MessageTypeValidate {
			log.Printf("Directory size validation request from %s: %d bytes (%.2f GB)",
				clientAddr, header.FileSize, toGB(header.FileSize))
			sendSuccessResponse(conn, "Directory size validated!")
			transferDuration := time.Since(startTime)
			log.Printf("Directory size validation completed from %s (duration: %v)", clientAddr, transferDuration)
			return
		}

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

		var outputPath string
		var receivedFileName string

		outputPath, err = sanitizePath(*destDir, header.FileName)
		if err != nil {
			log.Printf("Path sanitization failed for %s: %v", clientAddr, err)
			sendErrorResponse(conn, fmt.Sprintf("Invalid file path: %v", err))
			return
		}
		receivedFileName = header.FileName

		outputDir := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Printf("Failed to create directory structure %s for client %s: %v", outputDir, clientAddr, err)
			sendErrorResponse(conn, "Failed to create directory structure")
			return
		}

		var outputFile *os.File
		var finalPath string

		if *fileStrategy == StrategyRename {
			if _, statErr := os.Stat(outputPath); os.IsNotExist(statErr) {
				outputFile, err = os.Create(outputPath)
				if err != nil {
					log.Printf("Failed to create output file %s for client %s: %v", outputPath, clientAddr, err)
					sendErrorResponse(conn, "Failed to create output file")
					return
				}
				finalPath = outputPath
			} else {
				outputFile, finalPath, err = generateUniqueFile(outputPath, receivedFileName)
				if err != nil {
					log.Printf("Failed to create unique file for %s: %v", clientAddr, err)
					sendErrorResponse(conn, fmt.Sprintf("Failed to create unique file: %v", err))
					return
				}
			}
		} else {
			// For other strategies ("overwrite", "skip"), resolve the file path.
			finalPath, err = resolveFilePath(outputPath, *fileStrategy)
			if err != nil {
				if strings.Contains(err.Error(), "skip strategy is enabled") {
					log.Printf("Skipping file from %s: %v", clientAddr, err)
					sendErrorResponse(conn, "File already exists and skip strategy is enabled")
				} else {
					log.Printf("Failed to handle file conflict for %s: %v", clientAddr, err)
					sendErrorResponse(conn, fmt.Sprintf("Failed to handle file conflict: %v", err))
				}
				// Continue to next file instead of returning, to allow other files in the session to transfer.
				continue
			}

			outputFile, err = os.Create(finalPath)
			if err != nil {
				log.Printf("Failed to create output file %s for client %s: %v", finalPath, clientAddr, err)
				sendErrorResponse(conn, "Failed to create output file")
				return
			}
		}

		log.Printf("Receiving file content from %s...", clientAddr)

		// Instantiate a `contextReader` to read from the connection with context support (for graceful shutdown).
		ctxReader := &contextReader{
			ctx:  ctx,
			conn: conn,
		}

		// Instantiate a `LimitReader` to prevent reading past the specified file size.
		limitReader := io.LimitReader(ctxReader, int64(header.FileSize))

		// Instantiate a `TeeReader` that reads from network and writes to hash while returning data to be copied to file.
		hasher := sha256.New()
		teeReader := io.TeeReader(limitReader, hasher)

		// Instantiate a `ProgressWriter` to track transfer progress.
		progressWriter := protocol.NewProgressWriter(outputFile, int64(header.FileSize), fmt.Sprintf("Receiving %s", header.FileName))

		bytesWritten, err := io.Copy(progressWriter, teeReader)
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
			if err := os.Remove(finalPath); err != nil {
				log.Printf("Failed to remove partial file %s: %v", finalPath, err)
			}
			if err := outputFile.Close(); err != nil {
				log.Printf("Error closing output file %s: %v", finalPath, err)
			}
			sendErrorResponse(conn, "Failed to receive file content")
			return
		}

		if err := outputFile.Close(); err != nil {
			log.Printf("Error closing output file %s: %v", finalPath, err)
		}

		if bytesWritten != int64(header.FileSize) {
			log.Printf("File size mismatch for client %s: expected %d, received %d",
				clientAddr, header.FileSize, bytesWritten)
			if err := os.Remove(finalPath); err != nil {
				log.Printf("Failed to remove incomplete (partial) file %s: %v", finalPath, err)
			}
			sendErrorResponse(conn, "File size mismatch")
			return
		}

		progressWriter.Complete()

		log.Printf("Verifying received data integrity...")
		calculatedChecksum := hasher.Sum(nil)
		if !bytes.Equal(calculatedChecksum, header.Checksum) {
			log.Printf("Data checksum verification failed for client %s: expected %x, got %x",
				clientAddr, header.Checksum, calculatedChecksum)
			if err := os.Remove(finalPath); err != nil {
				log.Printf("Failed to remove corrupted file %s: %v", finalPath, err)
			}
			sendErrorResponse(conn, "Data integrity check failed")
			return
		}
		log.Printf("Data checksum verification passed")

		log.Printf("File integrity verified for %s", header.FileName)

		if header.TransferType == protocol.TransferTypeDirectory {
			dirSizeMutex.Lock()
			directorySizes[clientAddr] += header.FileSize
			currentTotal := directorySizes[clientAddr]
			dirSizeMutex.Unlock()
			log.Printf("Directory transfer progress for %s: %d bytes (%.2f GB)", clientAddr, currentTotal, toGB(currentTotal))
		}

		sendSuccessResponse(conn, "Transfer received!")

		transferDuration := time.Since(startTime)
		log.Printf("Transfer completed from %s (duration: %v)", clientAddr, transferDuration)

		// Continue to the next file transfer on the same connection.
		// The loop will break when the client closes the connection or an error occurs.
	}
}

// Read reads data from the connection with context cancellation support.
func (cr *contextReader) Read(p []byte) (n int, err error) {
	select {
	// Return if the context is done (canceled or timed out).
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
		// Do nothing.
	}

	if err := cr.conn.SetReadDeadline(time.Now().Add(ReadTimeout)); err != nil {
		return 0, err
	}

	return cr.conn.Read(p)
}

func main() {
	flag.Parse()

	switch *fileStrategy {
	case StrategyOverwrite, StrategyRename, StrategySkip:
		// Do nothing.
	default:
		log.Fatalf("Invalid file strategy: %s. Must be one of: %s, %s, %s",
			*fileStrategy, StrategyOverwrite, StrategyRename, StrategySkip)
	}

	if *maxDirectorySize == 0 {
		log.Fatalf("Invalid directory size limit: must be greater than 0")
	}

	setupLogging()

	log.Printf("Starting file transfer server...")
	log.Printf("Directory size limit: %d bytes (%.2f GB)", *maxDirectorySize, toGB(*maxDirectorySize))

	// Create a cancellable context for managing graceful shutdown.
	// `ctx` is the context that can be passed to goroutines to listen for cancellation signals.
	// `cancel` is the function that can be called to cancel the context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Establish a listener on the specified port and listen for incoming connections.
	listener, err := net.Listen("tcp", ":"+*listenPort)
	if err != nil {
		log.Fatalf("Failed to start listening for incoming connections: %v", err)
	}

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

	// Launch a goroutine to periodically log directory transfer statistics.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				numClient, totalSize := getDirectoryStats()
				if numClient > 0 {
					log.Printf("Directory transfer stats: %d active clients, %.2f GB total", numClient, toGB(totalSize))
				}
			case <-shutdownChannel:
				return
			}
		}
	}()

	// Launch a goroutine to handle shutdown signals.
	go func() {
		sig := <-receiveSigChannel
		log.Printf("Shutdown signal received: %v. Starting graceful shutdown...", sig)

		// Cancel the context to signal all active transfers to stop.
		cancel()

		if err := listener.Close(); err != nil {
			log.Printf("Error closing listener during shutdown: %v", err)
		}

		close(shutdownChannel)

		log.Printf("Waiting for active transfers to complete (timeout: %v)...", ShutdownTimeout)
		doneChannel := make(chan struct{})
		go func() {
			wg.Wait()
			close(doneChannel)
		}()
		select {
		case <-doneChannel:
			log.Printf("All active transfers completed.")
		case <-time.After(ShutdownTimeout):
			log.Printf("Shutdown timeout reached. Forcing shutdown...")
		}

		numClient, totalSize := getDirectoryStats()
		if numClient > 0 {
			log.Printf("Final directory transfer stats: %d active clients, %.2f GB in total", numClient, toGB(totalSize))
		}
	}()

	// Main loop to accept incoming client connections.
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-shutdownChannel:
				log.Printf("Stopped accepting new connections.")
				wg.Wait()
				log.Printf("All active connections finished. Server exiting.")
				return
			default:
				log.Printf("Failed to accept client connection: %v", err)
				continue
			}
		}
		// Increment the `sync.WaitGroup` counter by `1` to indicate that a new client connection (handled in a new goroutine) has started
		// so that the server will wait for this connection to finish before shutting down.
		wg.Add(1)

		// Launch a new goroutine to handle the client connection so that the server can concurrently handle multiple connections.
		go handleConnection(ctx, conn, &wg)
	}
}
