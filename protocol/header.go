package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Constants for header validation.
const (
	ChecksumSize      = 32        // SHA-256 checksum size (32 bytes).
	MaxFileNameLength = 64 * 1024 // Maximum allowed filename length (64KB).
	MaxDirPathLength  = 64 * 1024 // Maximum allowed directory path length (64KB).
)

// Constants for representing transfer types.
const (
	TransferTypeFile      = 0 // Transfer type for single file.
	TransferTypeDirectory = 1 // Transfer type for directory.
)

// Constants for representing message types.
const (
	MessageTypeValidate = 1 // Message type for validation requests.
	MessageTypeTransfer = 2 // Message type for file transfer requests.
)

// Errors for header validation.
var (
	ErrInvalidFileSize      = errors.New("invalid file size in the header")
	ErrInvalidFileName      = errors.New("invalid filename in the header")
	ErrFileNameTooLong      = errors.New("filename length exceeds the maximum allowed size")
	ErrInvalidChecksum      = errors.New("invalid checksum in the header")
	ErrChecksumMismatch     = errors.New("checksum mismatch in the header")
	ErrInvalidDirectoryPath = errors.New("invalid directory path in the header")
	ErrDirectoryPathTooLong = errors.New("directory path length exceeds the maximum allowed size")
	ErrInvalidTransferType  = errors.New("invalid transfer type in the header")
	ErrInvalidMessageType   = errors.New("invalid message type in the header")
)

// Header represents the protocol header for file transfers.
type Header struct {
	MessageType   uint8  // Message type (1 for validation, 2 for transfer).
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

	// `FileName` is permitted to be empty for validation messages.
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
		return fmt.Errorf("failed to write the message type: %w", err)
	}

	// Write the file size as 8 bytes in big-endian format.
	if err := binary.Write(w, binary.BigEndian, header.FileSize); err != nil {
		return fmt.Errorf("failed to write the file size: %w", err)
	}

	// Write the file name length as 4 bytes in big-endian format, followed by the file name.
	fileNameBytes := []byte(header.FileName)
	fileNameLength := uint32(len(fileNameBytes))
	if err := binary.Write(w, binary.BigEndian, fileNameLength); err != nil {
		return fmt.Errorf("failed to write the filename length: %w", err)
	}
	if _, err := w.Write(fileNameBytes); err != nil {
		return fmt.Errorf("failed to write the filename: %w", err)
	}

	// Write the checksum as fixed-size bytes (32 bytes for SHA-256).
	if _, err := w.Write(header.Checksum); err != nil {
		return fmt.Errorf("failed to write the checksum: %w", err)
	}

	// Write the transfer type as a single byte.
	if _, err := w.Write([]byte{header.TransferType}); err != nil {
		return fmt.Errorf("failed to write the transfer type: %w", err)
	}

	// Write the directory path length as 4 bytes in big-endian format, followed by the directory path.
	dirPathBytes := []byte(header.DirectoryPath)
	dirPathLength := uint32(len(dirPathBytes))
	if err := binary.Write(w, binary.BigEndian, dirPathLength); err != nil {
		return fmt.Errorf("failed to write the directory path length: %w", err)
	}
	if _, err := w.Write(dirPathBytes); err != nil {
		return fmt.Errorf("failed to write the directory path: %w", err)
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
	_, err := io.ReadFull(r, messageTypeBytes)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading the message type: %w", err)
		}
		return nil, fmt.Errorf("failed to read the message type: %w", err)
	}
	messageType := messageTypeBytes[0]

	// Read the file size (8 bytes, big-endian).
	var fileSize uint64
	if err := binary.Read(r, binary.BigEndian, &fileSize); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading file size: %w", err)
		}
		return nil, fmt.Errorf("failed to read the file size: %w", err)
	}

	// Read the file name length (4 bytes, big-endian).
	var fileNameLength uint32
	if err := binary.Read(r, binary.BigEndian, &fileNameLength); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading filename length: %w", err)
		}
		return nil, fmt.Errorf("failed to read the filename length: %w", err)
	}

	// Validate filename length to prevent excessive memory allocation.
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
			return nil, fmt.Errorf("failed to read the filename: %w", err)
		}
	}
	fileName := string(fileNameBytes)

	// Local variable for number of bytes read.
	var n int

	// Read the checksum (32 bytes, fixed size).
	checksumBytes := make([]byte, ChecksumSize)
	n, err = io.ReadFull(r, checksumBytes)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading checksum: got %d bytes, expected %d: %w",
				n, ChecksumSize, err)
		}
		return nil, fmt.Errorf("failed to read the checksum: %w", err)
	}

	// Read the transfer type (1 byte).
	transferTypeBytes := make([]byte, 1)
	_, err = io.ReadFull(r, transferTypeBytes)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading transfer type: %w", err)
		}
		return nil, fmt.Errorf("failed to read the transfer type: %w", err)
	}
	transferType := transferTypeBytes[0]

	// Read the directory path length (4 bytes, big-endian).
	var dirPathLength uint32
	if err := binary.Read(r, binary.BigEndian, &dirPathLength); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading directory path length: %w", err)
		}
		return nil, fmt.Errorf("failed to read the directory path length: %w", err)
	}

	// Validate directory path length to prevent excessive memory allocation.
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
			return nil, fmt.Errorf("failed to read the directory path: %w", err)
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
