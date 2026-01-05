package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Constants for protocol limits and sizes.
const (
	// ChecksumSize is the fixed size for SHA-256 checksum (32 bytes).
	ChecksumSize = 32
	// MaxFileNameLength is the maximum allowed file name length (64KB).
	MaxFileNameLength = 64 * 1024
	// MaxDirPathLength is the maximum allowed directory path length (64KB).
	MaxDirPathLength = 64 * 1024
)

// Constants to represent the transfer types.
const (
	TransferTypeFile      = 0 // Transfer type for single file.
	TransferTypeDirectory = 1 // Transfer type for directory.
)

// Constants to represent message types.
const (
	MessageTypeValidate = 1 // Message type for validation requests.
	MessageTypeTransfer = 2 // Message type for file transfer requests.
)

// Custom error types for protocol errors.
var (
	ErrInvalidFileSize      = errors.New("invalid file size in header")
	ErrInvalidFileName      = errors.New("invalid filename in header")
	ErrFileNameTooLong      = errors.New("filename length exceeds the maximum allowed size")
	ErrInvalidChecksum      = errors.New("invalid checksum in header")
	ErrChecksumMismatch     = errors.New("checksum mismatch in header")
	ErrInvalidDirectoryPath = errors.New("invalid directory path in header")
	ErrDirectoryPathTooLong = errors.New("directory path length exceeds the maximum allowed size")
	ErrInvalidTransferType  = errors.New("invalid transfer type in header")
	ErrInvalidMessageType   = errors.New("invalid message type in header")
)

// A Header represents the file transfer header.
type Header struct {
	MessageType   uint8  // Message type (1 for Validate, 2 for Transfer).
	FileSize      uint64 // Size of the file or directory in bytes.
	FileName      string // Name of the file or directory.
	Checksum      []byte // SHA-256 checksum of the file or directory.
	TransferType  uint8  // Transfer type (0 for single file, 1 for directory).
	DirectoryPath string // Path of the directory (only used for directory transfers).
}

// validateHeader validates the header data.
func validateHeader(header *Header) error {
	if header == nil {
		return fmt.Errorf("header is nil")
	}

	if header.MessageType != MessageTypeValidate && header.MessageType != MessageTypeTransfer {
		return fmt.Errorf("%w: message type %d is invalid, expected %d (Validate) or %d (Transfer)",
			ErrInvalidMessageType, header.MessageType, MessageTypeValidate, MessageTypeTransfer)
	}

	// `FileName` can be empty for validation messages.
	if header.MessageType == MessageTypeTransfer && header.FileName == "" {
		return fmt.Errorf("%w: filename cannot be empty for transfer messages", ErrInvalidFileName)
	}

	if len(header.FileName) > MaxFileNameLength {
		return fmt.Errorf("%w: filename length %d exceeds the maximum %d",
			ErrFileNameTooLong, len(header.FileName), MaxFileNameLength)
	}

	if strings.ContainsRune(header.FileName, 0) {
		return fmt.Errorf("%w: filename contains null bytes", ErrInvalidFileName)
	}

	if header.Checksum == nil {
		return fmt.Errorf("%w: checksum cannot be nil", ErrInvalidChecksum)
	}

	if len(header.Checksum) != ChecksumSize {
		return fmt.Errorf("%w: checksum length %d is invalid, expected %d",
			ErrInvalidChecksum, len(header.Checksum), ChecksumSize)
	}

	if header.TransferType != TransferTypeFile && header.TransferType != TransferTypeDirectory {
		return fmt.Errorf("%w: transfer type %d is invalid, expected %d or %d",
			ErrInvalidTransferType, header.TransferType, TransferTypeFile, TransferTypeDirectory)
	}

	if header.TransferType == TransferTypeDirectory && len(header.DirectoryPath) > MaxDirPathLength {
		return fmt.Errorf("%w: directory path length %d exceeds the maximum %d",
			ErrDirectoryPathTooLong, len(header.DirectoryPath), MaxDirPathLength)
	}

	return nil
}

// WriteHeader writes the header to the given writer using length-prefixed format.
func WriteHeader(w io.Writer, header *Header) error {
	if w == nil {
		return fmt.Errorf("writer is nil")
	}

	if err := validateHeader(header); err != nil {
		return fmt.Errorf("invalid header for writing: %w", err)
	}

	// Write the message type as a single byte.
	if _, err := w.Write([]byte{header.MessageType}); err != nil {
		return fmt.Errorf("failed to write message type: %w", err)
	}

	// Write the file size as 8 bytes in big-endian format.
	if err := binary.Write(w, binary.BigEndian, header.FileSize); err != nil {
		return fmt.Errorf("failed to write file size: %w", err)
	}

	// Write the file name length as 4 bytes in big-endian format, followed by the file name.
	fileNameBytes := []byte(header.FileName)
	fileNameLength := uint32(len(fileNameBytes))
	if err := binary.Write(w, binary.BigEndian, fileNameLength); err != nil {
		return fmt.Errorf("failed to write filename length: %w", err)
	}
	if _, err := w.Write(fileNameBytes); err != nil {
		return fmt.Errorf("failed to write filename: %w", err)
	}

	// Write the checksum as fixed-size bytes (32 bytes for SHA-256).
	if len(header.Checksum) != ChecksumSize {
		return fmt.Errorf("invalid checksum size: expected %d, got %d", ChecksumSize, len(header.Checksum))
	}
	if _, err := w.Write(header.Checksum); err != nil {
		return fmt.Errorf("failed to write checksum: %w", err)
	}

	// Write the transfer type as a single byte.
	if _, err := w.Write([]byte{header.TransferType}); err != nil {
		return fmt.Errorf("failed to write transfer type: %w", err)
	}

	// Write the directory path length as 4 bytes in big-endian format, followed by the directory path.
	dirPathBytes := []byte(header.DirectoryPath)
	dirPathLength := uint32(len(dirPathBytes))
	if err := binary.Write(w, binary.BigEndian, dirPathLength); err != nil {
		return fmt.Errorf("failed to write directory path length: %w", err)
	}
	if _, err := w.Write(dirPathBytes); err != nil {
		return fmt.Errorf("failed to write directory path: %w", err)
	}

	return nil
}

// ReadHeader reads the header from the given reader using length-prefixed format.
func ReadHeader(r io.Reader) (*Header, error) {
	if r == nil {
		return nil, fmt.Errorf("reader is nil")
	}

	// Read the message type (1 byte).
	messageTypeBytes := make([]byte, 1)
	n, err := io.ReadFull(r, messageTypeBytes)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading the message type: %w", err)
		}
		return nil, fmt.Errorf("failed to read the message type: %w", err)
	}
	if n != 1 {
		return nil, fmt.Errorf("incomplete message type read: got %d bytes, expected 1", n)
	}
	messageType := messageTypeBytes[0]

	// Read the file size (8 bytes, big-endian).
	var fileSize uint64
	if err := binary.Read(r, binary.BigEndian, &fileSize); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading file size: %w", err)
		}
		return nil, fmt.Errorf("failed to read file size: %w", err)
	}

	// Read the file name length (4 bytes, big-endian).
	var fileNameLength uint32
	if err := binary.Read(r, binary.BigEndian, &fileNameLength); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading filename length: %w", err)
		}
		return nil, fmt.Errorf("failed to read filename length: %w", err)
	}

	// Validate file name length to prevent abuse.
	if fileNameLength > MaxFileNameLength {
		return nil, fmt.Errorf("%w: filename length %d exceeds the maximum %d",
			ErrFileNameTooLong, fileNameLength, MaxFileNameLength)
	}

	// Read the file name (variable length).
	fileNameBytes := make([]byte, fileNameLength)
	if fileNameLength > 0 {
		n, err := io.ReadFull(r, fileNameBytes)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, fmt.Errorf("unexpected end of stream while reading filename: got %d bytes, expected %d: %w",
					n, fileNameLength, err)
			}
			return nil, fmt.Errorf("failed to read filename: %w", err)
		}
		if n != int(fileNameLength) {
			return nil, fmt.Errorf("incomplete filename read: got %d bytes, expected %d", n, fileNameLength)
		}
	}
	fileName := string(fileNameBytes)

	// Read the checksum (32 bytes, fixed size).
	checksumBytes := make([]byte, ChecksumSize)
	n, err = io.ReadFull(r, checksumBytes)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading checksum: got %d bytes, expected %d: %w",
				n, ChecksumSize, err)
		}
		return nil, fmt.Errorf("failed to read checksum: %w", err)
	}
	if n != ChecksumSize {
		return nil, fmt.Errorf("incomplete checksum read: got %d bytes, expected %d", n, ChecksumSize)
	}

	// Read the transfer type (1 byte).
	transferTypeBytes := make([]byte, 1)
	n, err = io.ReadFull(r, transferTypeBytes)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading transfer type: %w", err)
		}
		return nil, fmt.Errorf("failed to read transfer type: %w", err)
	}
	if n != 1 {
		return nil, fmt.Errorf("incomplete transfer type read: got %d bytes, expected 1", n)
	}
	transferType := transferTypeBytes[0]

	// Read the directory path length (4 bytes, big-endian).
	var dirPathLength uint32
	if err := binary.Read(r, binary.BigEndian, &dirPathLength); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading directory path length: %w", err)
		}
		return nil, fmt.Errorf("failed to read directory path length: %w", err)
	}

	// Validate directory path length to prevent abuse.
	if dirPathLength > MaxDirPathLength {
		return nil, fmt.Errorf("%w: directory path length %d exceeds the maximum %d",
			ErrDirectoryPathTooLong, dirPathLength, MaxDirPathLength)
	}

	// Read the directory path (variable length).
	dirPathBytes := make([]byte, dirPathLength)
	if dirPathLength > 0 {
		n, err = io.ReadFull(r, dirPathBytes)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, fmt.Errorf("unexpected end of stream while reading directory path: got %d bytes, expected %d: %w",
					n, dirPathLength, err)
			}
			return nil, fmt.Errorf("failed to read directory path: %w", err)
		}
		if n != int(dirPathLength) {
			return nil, fmt.Errorf("incomplete directory path read: got %d bytes, expected %d", n, dirPathLength)
		}
	}
	dirPath := string(dirPathBytes)

	// Create and validate the header.
	header := &Header{
		MessageType:   messageType,
		FileSize:      fileSize,
		FileName:      fileName,
		Checksum:      checksumBytes,
		TransferType:  transferType,
		DirectoryPath: dirPath,
	}
	if err := validateHeader(header); err != nil {
		return nil, fmt.Errorf("invalid header read from stream: %w", err)
	}

	return header, nil
}
