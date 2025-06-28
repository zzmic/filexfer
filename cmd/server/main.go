package main

import (
	"filexfer/protocol"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
)

func main() {
	// Establish a listener on port 8080 and listen for incoming connections.
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to start listening for incoming connections: %v", err)
	}

	// Ensure the listener is closed after the main function exits.
	defer listener.Close()

	fmt.Println("Server is listening on port 8080...")

	// Accept a client connection.
	conn, err := listener.Accept()
	if err != nil {
		log.Fatalf("Failed to accept the client's connection: %v", err)
	}

	fmt.Printf("Connection established from %s\n", conn.RemoteAddr().String())

	// Read the file transfer header.
	header, err := protocol.ReadHeader(conn)
	if err != nil {
		log.Fatalf("Failed to read the file transfer header: %v", err)
	}

	fmt.Printf("Receiving file: %s (size: %d bytes)\n", header.Filename, header.FileSize)

	// Create the directory to save the received file (if it doesn't exist).
	// `0755`: "OwnerCanDoAllExecuteGroupOtherCanReadExecute" (https://pkg.go.dev/gitlab.com/evatix-go/core/filemode).
	receivedDir := "test"
	if err := os.MkdirAll(receivedDir, 0755); err != nil {
		log.Fatalf("Failed to create the directory to save the received file: %v", err)
	}

	// Create the output file by first joining the received directory and the filename.
	receivedFileName := "received_" + header.Filename
	outputPath := filepath.Join(receivedDir, receivedFileName)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("Failed to create the output file: %v", err)
	}

	// Ensure the output file is closed after the main function exits.
	defer outputFile.Close()

	// Read and write the file content.
	bytesWritten, err := io.CopyN(outputFile, conn, int64(header.FileSize))
	if err != nil {
		log.Fatalf("Failed to receive the file content: %v", err)
	}

	fmt.Printf("File received successfully! %d bytes written to %s\n", bytesWritten, outputPath)

	// Close the connection.
	conn.Close()
	fmt.Println("Connection closed. Server shutting down.")
}
