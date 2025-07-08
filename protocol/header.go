package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Constants to represent the header size and the maximum file name size.
const (
	HeaderSize   = 8 + 256 + 32 + 1 + 256 // 329 bytes for the header (8 bytes for file size + 256 bytes for file name + 32 bytes for SHA256 checksum + 1 byte for transfer type + 256 bytes for directory path).
	FileNameSize = 256                    // 256 bytes for the file name.
	ChecksumSize = 32                     // 32 bytes for SHA256 checksum (hexadecimal representation in string).
	DirPathSize  = 256                    // 256 bytes for directory path.
)

// Constants to represent the transfer types.
const (
	TransferTypeFile      = 0 // Transfer type for single file.
	TransferTypeDirectory = 1 // Transfer type for directory.
)

// Custom error types for protocol errors.
var (
	ErrInvalidHeaderSize    = errors.New("invalid header size in header")
	ErrInvalidFileSize      = errors.New("invalid file size in header")
	ErrInvalidFileName      = errors.New("invalid filename in header")
	ErrHeaderTooLarge       = errors.New("header size exceeds maximum allowed size in header")
	ErrInvalidChecksum      = errors.New("invalid checksum in header")
	ErrChecksumMismatch     = errors.New("checksum mismatch in header")
	ErrInvalidDirectoryPath = errors.New("invalid directory path in header")
	ErrInvalidTransferType  = errors.New("invalid transfer type in header")
)

// Struct to represent the file transfer header.
type Header struct {
	FileSize      uint64 // Size of the file or directory in bytes.
	FileName      string // Name of the file or directory.
	Checksum      []byte // SHA256 checksum of the file or directory.
	TransferType  uint8  // Transfer type (0 for single file, 1 for directory).
	DirectoryPath string // Path of the directory (only used for directory transfers).
}

// Function to validate the header data.
func validateHeader(header *Header) error {
	if header == nil {
		return fmt.Errorf("header is nil")
	}

	if header.FileName == "" {
		return fmt.Errorf("%w: filename cannot be empty", ErrInvalidFileName)
	}

	if len(header.FileName) > FileNameSize {
		return fmt.Errorf("%w: filename length %d exceeds maximum %d",
			ErrInvalidFileName, len(header.FileName), FileNameSize)
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

	if header.TransferType == TransferTypeDirectory && len(header.DirectoryPath) > DirPathSize {
		return fmt.Errorf("%w: directory path length %d exceeds maximum %d",
			ErrInvalidDirectoryPath, len(header.DirectoryPath), DirPathSize)
	}

	return nil
}

// Function to write the header to the given writer.
func WriteHeader(w io.Writer, header *Header) error {
	if w == nil {
		return fmt.Errorf("writer is nil")
	}

	if err := validateHeader(header); err != nil {
		return fmt.Errorf("invalid header for writing: %w", err)
	}

	// Write the file size as 8 bytes in the big-endian format.
	sizeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(sizeBytes, header.FileSize)

	// Write the file size to the writer.
	n, err := w.Write(sizeBytes)
	if err != nil {
		return fmt.Errorf("failed to write file size: %w", err)
	}
	if n != 8 {
		return fmt.Errorf("incomplete write of file size: wrote %d bytes, expected 8", n)
	}

	// Write the file name as fixed-size bytes (pad with zeros if shorter than the maximum file name size).
	fileNameBytes := make([]byte, FileNameSize)
	copy(fileNameBytes, []byte(header.FileName))

	// Write the file name to the writer.
	n, err = w.Write(fileNameBytes)
	if err != nil {
		return fmt.Errorf("failed to write filename: %w", err)
	}
	if n != FileNameSize {
		return fmt.Errorf("incomplete write of filename: wrote %d bytes, expected %d", n, FileNameSize)
	}

	// Write the checksum as fixed-size bytes.
	checksumBytes := make([]byte, ChecksumSize)
	copy(checksumBytes, header.Checksum)

	// Write the checksum to the writer.
	n, err = w.Write(checksumBytes)
	if err != nil {
		return fmt.Errorf("failed to write checksum: %w", err)
	}
	if n != ChecksumSize {
		return fmt.Errorf("incomplete write of checksum: wrote %d bytes, expected %d", n, ChecksumSize)
	}

	// Write the transfer type as a single byte.
	transferTypeBytes := []byte{header.TransferType}
	n, err = w.Write(transferTypeBytes)
	if err != nil {
		return fmt.Errorf("failed to write transfer type: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("incomplete write of transfer type: wrote %d bytes, expected 1", n)
	}

	// Write the directory path as fixed-size bytes (pad with zeros if shorter than the maximum directory path size).
	dirPathBytes := make([]byte, DirPathSize)
	copy(dirPathBytes, []byte(header.DirectoryPath))

	// Write the directory path to the writer.
	n, err = w.Write(dirPathBytes)
	if err != nil {
		return fmt.Errorf("failed to write directory path: %w", err)
	}
	if n != DirPathSize {
		return fmt.Errorf("incomplete write of directory path: wrote %d bytes, expected %d", n, DirPathSize)
	}

	return nil
}

// Function to read the header from the given reader.
func ReadHeader(r io.Reader) (*Header, error) {
	if r == nil {
		return nil, fmt.Errorf("reader is nil")
	}

	// Read the entire header into a buffer (in bytes).
	headerBytes := make([]byte, HeaderSize)
	// Read the header from the reader.
	n, err := io.ReadFull(r, headerBytes)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("unexpected end of stream while reading header: %w", err)
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("incomplete header read: got %d bytes, expected %d: %w",
				n, HeaderSize, err)
		}
		return nil, fmt.Errorf("failed to read header: %w", err)
	}
	if n != HeaderSize {
		return nil, fmt.Errorf("%w: read %d bytes, expected %d", ErrInvalidHeaderSize, n, HeaderSize)
	}

	// Extract the file size (first 8 bytes in the big-endian format).
	fileSize := binary.BigEndian.Uint64(headerBytes[:8])

	// Extract the file name (next 256 bytes, trim null bytes).
	// The file name is stored in the next 256 bytes of the header.
	fileNameBytes := headerBytes[8 : 8+FileNameSize]

	// Find the first null byte and trim from there.
	fileName := ""
	for i, b := range fileNameBytes {
		if b == 0 {
			fileName = string(fileNameBytes[:i])
			break
		}
	}

	// If no null byte found, use the entire file name field.
	if fileName == "" {
		fileName = string(fileNameBytes)
	}

	// Extract the checksum (next 32 bytes, trim null bytes).
	checksumBytes := headerBytes[8+FileNameSize : 8+FileNameSize+ChecksumSize]

	// Extract the transfer type (next 1 byte).
	transferType := headerBytes[8+FileNameSize+ChecksumSize]

	// Extract the directory path (next 256 bytes, trim null bytes).
	directoryPathBytes := headerBytes[8+FileNameSize+ChecksumSize+1 : 8+FileNameSize+ChecksumSize+1+DirPathSize]

	// Find the first null byte and trim from there.
	dirPath := ""
	for i, b := range directoryPathBytes {
		if b == 0 {
			dirPath = string(directoryPathBytes[:i])
			break
		}
	}

	// If no null byte found, use the entire directory path field.
	if dirPath == "" {
		dirPath = string(directoryPathBytes)
	}

	// Create and validate the header.
	header := &Header{
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
