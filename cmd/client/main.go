package main

import (
	"bufio"
	"errors"
	"filexfer/protocol"
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

const (
	MaxFileSize = 100 * 1024 * 1024 // 100MB limit.
	LogPrefix   = "[CLIENT]"        // Log prefix.
	ServerAddr  = "localhost:8080"  // Server address.
)

// Function to configure structured logging with timestamps and custom prefix.
func setupLogging() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.SetPrefix(LogPrefix + " ")
}

// Function to perform comprehensive validation of the file to be sent.
func validateFile(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("%w: file path cannot be empty", ErrInvalidFilename)
	}

	// Check if the file exists.
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrFileNotFound, filePath)
		}
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	// Check if the file is a regular file.
	if fileInfo.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", filePath)
	}

	// Check if the file size is zero.
	if fileInfo.Size() == 0 {
		return fmt.Errorf("%w: %s", ErrFileEmpty, filePath)
	}

	// Check if the file size exceeds the maximum allowed size.
	if fileInfo.Size() > MaxFileSize {
		return fmt.Errorf("%w: file size %d exceeds maximum allowed size %d",
			ErrFileTooLarge, fileInfo.Size(), MaxFileSize)
	}

	// Validate the file name to prevent directory traversal (for security).
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
		// Check if the error is an EOF error.
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("server closed connection unexpectedly")
		}
		// Check if the error is a general read error.
		return fmt.Errorf("failed to read server response: %w", err)
	}

	// Convert the response to a string.
	responseStr := string(response[:n])

	// Check if the response is an error.
	if strings.HasPrefix(responseStr, "ERROR:") {
		return fmt.Errorf("server error: %s", strings.TrimSpace(strings.TrimPrefix(responseStr, "ERROR:")))
	}

	// Check if the response is a success.
	if strings.HasPrefix(responseStr, "SUCCESS:") {
		log.Printf("Server response: %s", strings.TrimSpace(responseStr))
		return nil
	}

	log.Printf("Server response: %s", strings.TrimSpace(responseStr))
	return nil
}

func main() {
	// Setup structured logging.
	setupLogging()

	log.Printf("Starting the file transfer client...")

	// Get the file path from the user's terminal input.
	fmt.Print("Enter the path to the file to send: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	filePath := strings.TrimSpace(scanner.Text())

	// Validate the file before attempting to connect.
	if err := validateFile(filePath); err != nil {
		log.Fatalf("File validation failed: %v", err)
	}

	log.Printf("Connecting to the server at %s...", ServerAddr)

	// Establish a TCP connection to the server using the server's address.
	// `conn`: a network connection object that represents the connection to the server.
	// `net.DialTimeout`: a function to connect to the address on the named network.
	// `ServerAddr`: the address of the server to connect to.
	// `30*time.Second`: the timeout for the connection.
	conn, err := net.DialTimeout("tcp", ServerAddr, 30*time.Second)
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

	log.Printf("Connected successfully to the server at %s", ServerAddr)

	// Open the file to send.
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open file %s: %v", filePath, err)
	}

	// Close the file when the surrounding function exits.
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing file %s: %v", filePath, err)
		}
	}()

	// Get the file information.
	fileInfo, err := file.Stat()
	if err != nil {
		log.Fatalf("Failed to get file information for %s: %v", filePath, err)
	}

	// Create the header.
	header := &protocol.Header{
		FileSize: uint64(fileInfo.Size()),
		Filename: filepath.Base(filePath),
	}

	log.Printf("Sending file: %s (size: %d bytes)", header.Filename, header.FileSize)

	// Set connection deadline for the entire transfer.
	if err := conn.SetDeadline(time.Now().Add(10 * time.Minute)); err != nil {
		log.Fatalf("Failed to set connection deadline: %v", err)
	}

	// Send the header first.
	if err := protocol.WriteHeader(conn, header); err != nil {
		log.Fatalf("Failed to send file transfer header: %v", err)
	}

	// Send the file content with progress tracking.
	startTime := time.Now()
	bytesWritten, err := io.Copy(conn, file)
	if err != nil {
		log.Fatalf("Failed to send file content: %v", err)
	}

	// Verify if the bytes written are equal to the file size.
	if bytesWritten != int64(header.FileSize) {
		log.Fatalf("File transfer incomplete: expected %d bytes, sent %d bytes",
			header.FileSize, bytesWritten)
	}

	// Read the server response.
	if err := readServerResponse(conn); err != nil {
		log.Fatalf("Failed to read server response: %v", err)
	}

	// Calculate the transfer duration and rate.
	transferDuration := time.Since(startTime)
	transferRate := float64(bytesWritten) / transferDuration.Seconds() / 1024 / 1024 // MB/s.

	log.Printf("File sent successfully! %d bytes sent in %v (%.2f MB/s)",
		bytesWritten, transferDuration, transferRate)
	log.Printf("Client shutting down.")
}
