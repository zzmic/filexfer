package main

import (
	"bufio"
	"filexfer/protocol"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
)

func main() {
	// Define the server's address with the target server's IP address (or hostname) and port number.
	// localhost is the IP address of the machine running the server.
	// 8080 is the port number the server is listening on.
	serverAddress := "localhost:8080"
	fmt.Printf("Connecting to the server at %s...\n", serverAddress)

	// Establish a TCP connection to the server using the server's address.
	// `conn` is a network connection object that represents the connection to the server.
	conn, err := net.Dial("tcp", serverAddress)
	if err != nil {
		log.Fatalf("Failed to establish a TCP connection to the server: %v", err)
	}

	// Ensure the connection is closed when the surrounding function (the main function in this case) exits.
	defer conn.Close()

	fmt.Println("Connected successfully to the server!")

	// Get the file path from the user's terminal input.
	fmt.Print("Enter the path to the file to send: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	filePath := scanner.Text()

	// Open the file to send.
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open the file to send: %v", err)
	}
	defer file.Close()

	// Get the file information.
	fileInfo, err := file.Stat()
	if err != nil {
		log.Fatalf("Failed to get the file information: %v", err)
	}

	// Create the header.
	header := &protocol.Header{
		FileSize: uint64(fileInfo.Size()),
		Filename: filepath.Base(filePath),
	}

	fmt.Printf("Sending file: %s (size: %d bytes)\n", header.Filename, header.FileSize)

	// Send the header first.
	if err := protocol.WriteHeader(conn, header); err != nil {
		log.Fatalf("Failed to send the file transfer header: %v", err)
	}

	// Send the file content.
	bytesWritten, err := io.Copy(conn, file)
	if err != nil {
		log.Fatalf("Failed to send the file content: %v", err)
	}

	fmt.Printf("File sent successfully! %d bytes sent\n", bytesWritten)
	fmt.Println("Client shutting down.")
}
