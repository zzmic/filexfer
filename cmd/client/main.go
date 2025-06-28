package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
)

func main() {
	// Define the server's address with the target server's IP address (or hostname) and port number.
	// localhost is the IP address of the machine running the server.
	// 8080 is the port number the server is listening on.
	serverAddress := "localhost:8080"
	fmt.Printf("Connecting to %s...\n", serverAddress)

	// Establish a TCP connection to the server using the server's address.
	// `conn` is a network connection object that represents the connection to the server.
	conn, err := net.Dial("tcp", serverAddress)
	if err != nil {
		log.Fatalf("Connection failed: %v", err)
	}

	// Ensure the connection is closed when the surrounding function (the main function in this case) exits.
	defer conn.Close()

	fmt.Println("Connected successfully!")

	// Read a message from the user's terminal input.
	fmt.Print("Enter a message to send: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	message := scanner.Text() + "\n"

	// Send the user's message to the server.
	// Convert the string message to a byte slice and send it to the server.
	_, err = conn.Write([]byte(message))
	if err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}

	fmt.Printf("Sent message: %s", message)
	fmt.Println("Client shutting down.")
}
