package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Constants for response status.
const (
	ResponseStatusSuccess = 0
	ResponseStatusError   = 1
)

// Custom error types for response errors.
var (
	ErrInvalidResponseStatus = errors.New("invalid response status")
	ErrInvalidMessageLength  = errors.New("invalid message length in response")
)

// MaxResponseMessageLength is the maximum allowed response message length (64KB).
const MaxResponseMessageLength = 64 * 1024

// WriteResponse writes a structured response to the given writer.
// Format: [1 byte for status] [4 bytes for message length] [variable length for message].
func WriteResponse(w io.Writer, status uint8, message string) error {
	if w == nil {
		return fmt.Errorf("writer is nil")
	}

	if status != ResponseStatusSuccess && status != ResponseStatusError {
		return fmt.Errorf("%w: status %d is invalid, expected %d (Success) or %d (Error)",
			ErrInvalidResponseStatus, status, ResponseStatusSuccess, ResponseStatusError)
	}

	messageBytes := []byte(message)
	messageLength := uint32(len(messageBytes))

	if messageLength > MaxResponseMessageLength {
		return fmt.Errorf("%w: message length %d exceeds maximum %d",
			ErrInvalidMessageLength, messageLength, MaxResponseMessageLength)
	}

	// Write the status byte (1 byte).
	if _, err := w.Write([]byte{status}); err != nil {
		return fmt.Errorf("failed to write response status: %w", err)
	}

	// Write the message length (4 bytes, big-endian).
	if err := binary.Write(w, binary.BigEndian, messageLength); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	// Write the message bytes (variable length).
	if messageLength > 0 {
		if _, err := w.Write(messageBytes); err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}
	}

	return nil
}

// ReadResponse reads a structured response from the given reader.
// Format: [1 byte for status] [4 bytes for message length] [variable length for message].
func ReadResponse(r io.Reader) (status uint8, message string, err error) {
	if r == nil {
		return 0, "", fmt.Errorf("reader is nil")
	}

	// Read the status byte (1 byte).
	statusBytes := make([]byte, 1)
	n, err := io.ReadFull(r, statusBytes)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, "", fmt.Errorf("unexpected end of stream while reading response status: %w", err)
		}
		return 0, "", fmt.Errorf("failed to read response status: %w", err)
	}
	if n != 1 {
		return 0, "", fmt.Errorf("incomplete status read: got %d bytes, expected 1", n)
	}
	status = statusBytes[0]

	if status != ResponseStatusSuccess && status != ResponseStatusError {
		return 0, "", fmt.Errorf("%w: status %d is invalid, expected %d (Success) or %d (Error)",
			ErrInvalidResponseStatus, status, ResponseStatusSuccess, ResponseStatusError)
	}

	// Read the message length (4 bytes, big-endian).
	var messageLength uint32
	if err := binary.Read(r, binary.BigEndian, &messageLength); err != nil {
		if errors.Is(err, io.EOF) {
			return 0, "", fmt.Errorf("unexpected end of stream while reading message length: %w", err)
		}
		return 0, "", fmt.Errorf("failed to read message length: %w", err)
	}

	// Validate message length to prevent abuse.
	if messageLength > MaxResponseMessageLength {
		return 0, "", fmt.Errorf("%w: message length %d exceeds maximum %d",
			ErrInvalidMessageLength, messageLength, MaxResponseMessageLength)
	}

	// Read the message (variable length).
	messageBytes := make([]byte, messageLength)
	if messageLength > 0 {
		n, err = io.ReadFull(r, messageBytes)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return 0, "", fmt.Errorf("unexpected end of stream while reading message: got %d bytes, expected %d: %w",
					n, messageLength, err)
			}
			return 0, "", fmt.Errorf("failed to read message: %w", err)
		}
		if n != int(messageLength) {
			return 0, "", fmt.Errorf("incomplete message read: got %d bytes, expected %d", n, messageLength)
		}
	}
	message = string(messageBytes)

	return status, message, nil
}
