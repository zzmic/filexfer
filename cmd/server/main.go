package main

import (
	"filexfer/protocol"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
)

func handleConnection(conn net.Conn, waitGroup *sync.WaitGroup) {
	// Defer the done ("Done decrements the [WaitGroup] counter by one") of the wait group and
	// the close of the connection ("Close closes the connection.
	// Any blocked Read or Write operations will be unblocked and return errors.").
	defer waitGroup.Done()
	defer conn.Close()

	fmt.Printf("Connection established from %s\n", conn.RemoteAddr().String())

	// Read the file transfer header.
	header, err := protocol.ReadHeader(conn)
	if err != nil {
		log.Printf("Failed to read the file transfer header: %v", err)
		return
	}

	fmt.Printf("Receiving file: %s (size: %d bytes)\n", header.Filename, header.FileSize)

	// Create the directory to save the received file (if it doesn't exist).
	// `0755`: "OwnerCanDoAllExecuteGroupOtherCanReadExecute" (https://pkg.go.dev/gitlab.com/evatix-go/core/filemode).
	receivedDir := "test"
	if err := os.MkdirAll(receivedDir, 0755); err != nil {
		log.Printf("Failed to create the directory to save the received file: %v", err)
		return
	}

	// Create the output file by first joining the received directory and the filename.
	receivedFileName := "received_" + header.Filename
	outputPath := filepath.Join(receivedDir, receivedFileName)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Printf("Failed to create the output file: %v", err)
		return
	}
	// Ensure the output file is closed after the main function exits.
	defer outputFile.Close()

	// Read and write the file content.
	bytesWritten, err := io.CopyN(outputFile, conn, int64(header.FileSize))
	if err != nil {
		log.Printf("Failed to receive the file content: %v", err)
		return
	}

	fmt.Printf("File received successfully! %d bytes written to %s\n", bytesWritten, outputPath)
	fmt.Println("Connection closed.")
}

func main() {
	// Establish a listener on port 8080 and listen for incoming connections.
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to start listening for incoming connections: %v", err)
	}

	// Ensure the listener is closed after the main function exits.
	defer listener.Close()

	fmt.Println("Server is listening on port 8080...")

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
		<-receiveSigChannel
		fmt.Println("\nShutdown signal received. Closing listener...")
		// Close the listener (stop accepting new connections).
		listener.Close()
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
				fmt.Println("Stopped accepting new connections.")
				// Wait for all connections to finish.
				waitGroup.Wait()
				fmt.Println("All active connections finished. Server exiting.")
				return
			default:
				log.Printf("Failed to accept the client's connection: %v", err)
				continue
			}
		}
		// Increment the `sync.WaitGroup` counter by 1 to indicate that a new client connection (handled in a new goroutine) has started,
		// so the server will wait for this connection to finish before shutting down.
		waitGroup.Add(1)
		go handleConnection(conn, &waitGroup) // Handle the connection.
	}
}
