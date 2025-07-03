package main

import (
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
	"sync"
	"syscall"
	"time"
)

// Custom error types for better error handling.
var (
	ErrInvalidFileSize = errors.New("invalid file size")
	ErrEmptyFilename   = errors.New("empty filename")
	ErrFileTooLarge    = errors.New("file size exceeds maximum allowed size")
)

// Constants to constrain the maximum file size and log prefix.
const (
	MaxFileSize = 100 * 1024 * 1024 // 100MB limit.
	LogPrefix   = "[SERVER]"        // Log prefix.
)

// Command-line flags.
var (
	listenPort = flag.String("port", "8080", "Listening port")
	destDir    = flag.String("dir", "test", "Destination directory for received files")
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

	if header.Filename == "" {
		return fmt.Errorf("%w: filename cannot be empty", ErrEmptyFilename)
	}

	if header.FileSize == 0 {
		return fmt.Errorf("%w: file size cannot be zero", ErrInvalidFileSize)
	}

	if header.FileSize > MaxFileSize {
		return fmt.Errorf("%w: file size %d exceeds maximum allowed size %d",
			ErrFileTooLarge, header.FileSize, MaxFileSize)
	}

	// Validate the file name to prevent directory traversal (for security).
	if filepath.Base(header.Filename) != header.Filename {
		return fmt.Errorf("invalid filename: contains path separators: %s", header.Filename)
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

// Function to handle a client connection.
func handleConnection(conn net.Conn, waitGroup *sync.WaitGroup) {
	// Get the start time and the client address of the connection.
	startTime := time.Now()
	clientAddr := conn.RemoteAddr().String()

	// Defer the done ("Done decrements the [WaitGroup] counter by one") of the wait group and
	// the close of the connection ("Close closes the connection.
	// Any blocked Read or Write operations will be unblocked and return errors.").
	defer func() {
		waitGroup.Done()
		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection to %s: %v", clientAddr, err)
		}
		log.Printf("Connection to %s closed (duration: %v)", clientAddr, time.Since(startTime))
	}()

	log.Printf("New connection established from %s", clientAddr)

	// Set connection deadline to prevent hanging connections.
	if err := conn.SetDeadline(time.Now().Add(5 * time.Minute)); err != nil {
		log.Printf("Failed to set connection deadline for %s: %v", clientAddr, err)
		sendErrorResponse(conn, "Internal server error")
		return
	}

	// Read the file transfer header.
	header, err := protocol.ReadHeader(conn)
	if err != nil {
		log.Printf("Failed to read file transfer header from %s: %v", clientAddr, err)
		if errors.Is(err, io.EOF) {
			log.Printf("Client %s disconnected before sending header", clientAddr)
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			log.Printf("Client %s sent incomplete header", clientAddr)
		}
		// Fallback to a generic message.
		sendErrorResponse(conn, "Failed to read file header")
		return
	}

	// Validate the header.
	if err := validateHeader(header); err != nil {
		log.Printf("Invalid header from %s: %v", clientAddr, err)
		sendErrorResponse(conn, fmt.Sprintf("Invalid file header: %v", err))
		return
	}

	// Log transfer start.
	log.Printf("Starting file transfer from %s: %s (size: %d bytes)", clientAddr, header.Filename, header.FileSize)

	// Create the directory to save the received file (if it doesn't exist).
	// `0755`: "OwnerCanDoAllExecuteGroupOtherCanReadExecute" (https://pkg.go.dev/gitlab.com/evatix-go/core/filemode).
	if err := os.MkdirAll(*destDir, 0755); err != nil {
		log.Printf("Failed to create directory %s for client %s: %v", *destDir, clientAddr, err)
		sendErrorResponse(conn, "Failed to create output directory")
		return
	}

	// Create the output file by first joining the received directory and the filename.
	receivedFileName := "received_" + header.Filename
	outputPath := filepath.Join(*destDir, receivedFileName)

	// Note: Uncomment this if we want to prevent overwriting existing files.
	// Check if the file already exists.
	/*
		if _, err := os.Stat(outputPath); err == nil {
			log.Printf("File already exists: %s", outputPath)
			sendErrorResponse(conn, "File already exists")
			return
		}
	*/

	// Create the output file.
	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Printf("Failed to create output file %s for client %s: %v", outputPath, clientAddr, err)
		sendErrorResponse(conn, "Failed to create output file")
		return
	}

	// Close the output file when the surrounding function exits.
	defer func() {
		if err := outputFile.Close(); err != nil {
			log.Printf("Error closing output file %s: %v", outputPath, err)
		}
	}()

	// Read and write the file content with progress tracking.
	// Create a progress writer to track download progress.
	progressWriter := protocol.NewProgressWriter(outputFile, int64(header.FileSize), fmt.Sprintf("Receiving %s", header.Filename))

	bytesWritten, err := io.CopyN(progressWriter, conn, int64(header.FileSize))
	if err != nil {
		log.Printf("Failed to receive file content from %s: %v", clientAddr, err)
		if errors.Is(err, io.EOF) {
			log.Printf("Client %s disconnected during file transfer", clientAddr)
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			log.Printf("Client %s sent incomplete file data", clientAddr)
		}
		// Fallback to a generic message.
		sendErrorResponse(conn, "Failed to receive file content")

		// Clean up the incomplete file.
		if err := os.Remove(outputPath); err != nil {
			log.Printf("Failed to remove partial file %s: %v", outputPath, err)
		}
		return
	}

	// Mark transfer as complete and log the final statistics.
	progressWriter.Complete()

	// Verify if the bytes written are equal to the file size.
	if bytesWritten != int64(header.FileSize) {
		log.Printf("File size mismatch for client %s: expected %d, received %d",
			clientAddr, header.FileSize, bytesWritten)
		sendErrorResponse(conn, "File size mismatch")
		// Clean up the incomplete file.
		if err := os.Remove(outputPath); err != nil {
			log.Printf("Failed to remove incomplete file %s: %v", outputPath, err)
		}
		return
	}

	// Send success response to the client.
	successMsg := fmt.Sprintf("SUCCESS: File received successfully! %d bytes written to %s\n",
		bytesWritten, outputPath)
	if _, err := conn.Write([]byte(successMsg)); err != nil {
		log.Printf("Failed to send success response to client %s: %v", clientAddr, err)
	}

	transferDuration := time.Since(startTime)
	transferRate := float64(bytesWritten) / transferDuration.Seconds() / 1024 / 1024 // MB/s.
	log.Printf("File transfer completed from %s: %d bytes written to %s (duration: %v, rate: %.2f MB/s)",
		clientAddr, bytesWritten, outputPath, transferDuration, transferRate)
}

func main() {
	// Parse command-line flags.
	flag.Parse()

	// Setup structured logging.
	setupLogging()

	log.Printf("Starting file transfer server...")

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
	var waitGroup sync.WaitGroup

	// Set up signal handling for graceful shutdown.
	// Create a channel to receive signals.
	// The channel is buffered to hold one signal without blocking the sender (the OS signal handler).
	receiveSigChannel := make(chan os.Signal, 1)
	// Set up an OS signal handler to notify the channel to receive signals.
	signal.Notify(receiveSigChannel, syscall.SIGINT, syscall.SIGTERM)
	// Create a channel that carries an empty struct (since no data is needed to be sent) to signal the main loop to stop accepting new connections.
	// The channel is unbuffered to ensure that the main loop only stops accepting new connections when all active connections have finished.
	shutdownChannel := make(chan struct{})

	// Launch the enclosed function as a goroutine so that it runs concurrently with the main program.
	go func() {
		// Receive a signal from the channel.
		// Block until a signal is received on the channel.
		sig := <-receiveSigChannel
		log.Printf("Shutdown signal received: %v. Closing listener...", sig)
		// Close the listener (stop accepting new connections).
		if err := listener.Close(); err != nil {
			log.Printf("Error closing listener during shutdown: %v", err)
		}
		// Close the shutdown channel to signal the main loop to stop accepting new connections.
		close(shutdownChannel)
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
				waitGroup.Wait()
				log.Printf("All active connections finished. Server exiting.")
				return
			default:
				log.Printf("Failed to accept client connection: %v", err)
				continue
			}
		}
		// Increment the `sync.WaitGroup` counter by 1 to indicate that a new client connection (handled in a new goroutine) has started,
		// so the server will wait for this connection to finish before shutting down.
		waitGroup.Add(1)
		go handleConnection(conn, &waitGroup) // Handle the connection.
	}
}
