package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
)

func main() {
	// Establish a listener on port 8080 and listen for incoming connections.
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to start listening: %v", err)
	}

	// Ensure the listener is closed after the main function exits.
	defer listener.Close()

	fmt.Println("Server is listening on port 8080...")

	// Accept a client connection.
	conn, err := listener.Accept()
	if err != nil {
		log.Fatalf("Failed to accept connection: %v", err)
	}

	// Print the client's remote (network) address.
	fmt.Printf("Connection established from %s\n", conn.RemoteAddr().String())

	// Read and print the received message from the client.
	// Create a new buffered reader that reads from the connection.
	// The message is read until a newline character is encountered.
	reader := bufio.NewReader(conn)
	message, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("Failed to read message: %v", err)
	} else {
		fmt.Printf("Received message: %s", message)
	}

	// Close the connection.
	conn.Close()
	fmt.Println("Connection closed. Server shutting down.")
}
