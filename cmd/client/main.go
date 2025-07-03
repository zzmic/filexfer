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
	"path/filepath"
	"strings"
	"time"
)

// Custom error types for better error handling.
var (
	ErrFileNotFound     = errors.New("file not found")
	ErrFileEmpty        = errors.New("file is empty")
	ErrFileTooLarge     = errors.New("file size exceeds maximum allowed size")
	ErrInvalidFilename  = errors.New("invalid filename")
	ErrConnectionFailed = errors.New("connection failed")
)

// Constants to constrain the maximum file size and log prefix.
const (
	MaxFileSize = 100 * 1024 * 1024 // 100MB limit.
	LogPrefix   = "[CLIENT]"        // Log prefix.
)

// Command-line flags for the client.
var (
	serverAddr = flag.String("server", "localhost:8080", "Server address (IP:Port)")
	filePath   = flag.String("file", "", "File to be transferred (required)")
)

// Function to configure structured logging with timestamps and custom prefix.
func setupLogging() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.SetPrefix(LogPrefix + " ")
}

// Function to validate command-line arguments
func validateArgs() error {
	if *filePath == "" {
		return fmt.Errorf("file path is required. Use -file flag to specify the source file")
	}

	return nil
}

// Function to perform comprehensive validation of the file to be sent.
func validateFile(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("%w: file path cannot be empty", ErrInvalidFilename)
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrFileNotFound, filePath)
		}
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	if fileInfo.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", filePath)
	}

	if fileInfo.Size() == 0 {
		return fmt.Errorf("%w: %s", ErrFileEmpty, filePath)
	}

	if fileInfo.Size() > MaxFileSize {
		return fmt.Errorf("%w: file size %d exceeds maximum allowed size %d",
			ErrFileTooLarge, fileInfo.Size(), MaxFileSize)
	}

	filename := filepath.Base(filePath)
	if filepath.Base(filename) != filename {
		return fmt.Errorf("invalid filename: contains path separators: %s", filename)
	}

	return nil
}

// Function to read and process the server's response.
func readServerResponse(conn net.Conn) error {
	// Set a short timeout for reading the response.
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
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

	// Validate the file before attempting to connect.
	if err := validateFile(*filePath); err != nil {
		log.Fatalf("File validation failed: %v", err)
	}

	log.Printf("Connecting to the server at %s...", *serverAddr)

	// Establish a TCP connection to the server using the server's address.
	// `conn`: a network connection object that represents the connection to the server.
	// `net.DialTimeout`: a function to connect to the address on the named network.
	// `ServerAddr`: the address of the server to connect to.
	// `30*time.Second`: the timeout for the connection.
	conn, err := net.DialTimeout("tcp", *serverAddr, 30*time.Second)
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

	// Open the file to send.
	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("Failed to open file %s: %v", *filePath, err)
	}

	// Close the file when the surrounding function exits.
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing file %s: %v", *filePath, err)
		}
	}()

	// Get the file information.
	fileInfo, err := file.Stat()
	if err != nil {
		log.Fatalf("Failed to get file information for %s: %v", *filePath, err)
	}

	// Create the header.
	header := &protocol.Header{
		FileSize: uint64(fileInfo.Size()),
		Filename: filepath.Base(*filePath),
	}

	// Log transfer start.
	fmt.Printf("Starting file transfer: %s (%d bytes)\n", header.Filename, header.FileSize)

	// Set connection deadline for the entire transfer.
	if err := conn.SetDeadline(time.Now().Add(10 * time.Minute)); err != nil {
		log.Fatalf("Failed to set connection deadline: %v", err)
	}

	// Send the header first.
	fmt.Printf("Sending file header...\n")
	if err := protocol.WriteHeader(conn, header); err != nil {
		log.Fatalf("Failed to send file transfer header: %v", err)
	}
	fmt.Printf("Header sent successfully. Starting file transfer...\n")

	// Send the file content with progress tracking.
	startTime := time.Now()

	// Create a progress writer to track download progress.
	progressReader := protocol.NewProgressReader(file, int64(header.FileSize), "Uploading")

	// Send the file content with progress tracking.
	bytesWritten, err := io.Copy(conn, progressReader)
	if err != nil {
		log.Fatalf("Failed to send file content: %v", err)
	}

	// Mark transfer as complete and show final statistics.
	progressReader.Complete()

	// Verify if the bytes written are equal to the file size.
	if bytesWritten != int64(header.FileSize) {
		log.Fatalf("File transfer incomplete: expected %d bytes, sent %d bytes",
			header.FileSize, bytesWritten)
	}

	// Read the server response.
	if err := readServerResponse(conn); err != nil {
		log.Fatalf("Failed to read server response: %v", err)
	}

	// Calculate and log the transfer duration and rate.
	transferDuration := time.Since(startTime)
	transferRate := float64(bytesWritten) / transferDuration.Seconds() / 1024 / 1024 // MB/s.
	log.Printf("File sent successfully! %d bytes sent in %v (%.2f MB/s)",
		bytesWritten, transferDuration, transferRate)

	log.Printf("Client shutting down.")
}
