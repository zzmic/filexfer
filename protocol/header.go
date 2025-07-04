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
	HeaderSize   = 8 + 256 // 264 bytes for the header (8 bytes for file size + 256 bytes for file name).
	FilenameSize = 256     // 256 bytes for the file name.
)

// Custom error types for protocol errors.
var (
	ErrInvalidHeaderSize = errors.New("invalid header size")
	ErrInvalidFileSize   = errors.New("invalid file size in header")
	ErrInvalidFilename   = errors.New("invalid filename in header")
	ErrHeaderTooLarge    = errors.New("header size exceeds the maximum allowed size")
)

// Struct to represent the file transfer header.
type Header struct {
	FileSize uint64
	Filename string
}

// Function to validate the header data.
func validateHeader(header *Header) error {
	if header == nil {
		return fmt.Errorf("header is nil")
	}

	if header.Filename == "" {
		return fmt.Errorf("%w: filename cannot be empty", ErrInvalidFilename)
	}

	if len(header.Filename) > FilenameSize {
		return fmt.Errorf("%w: filename length %d exceeds the maximum %d",
			ErrInvalidFilename, len(header.Filename), FilenameSize)
	}

	if strings.ContainsRune(header.Filename, 0) {
		return fmt.Errorf("%w: filename contains null bytes", ErrInvalidFilename)
	}

	if header.FileSize == 0 {
		return fmt.Errorf("%w: file size cannot be zero", ErrInvalidFileSize)
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
	filenameBytes := make([]byte, FilenameSize)
	copy(filenameBytes, []byte(header.Filename))

	// Write the file name to the writer.
	n, err = w.Write(filenameBytes)
	if err != nil {
		return fmt.Errorf("failed to write filename: %w", err)
	}
	if n != FilenameSize {
		return fmt.Errorf("incomplete write of filename: wrote %d bytes, expected %d", n, FilenameSize)
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
	filenameBytes := headerBytes[8:]

	// Find the first null byte and trim from there.
	filename := ""
	for i, b := range filenameBytes {
		if b == 0 {
			filename = string(filenameBytes[:i])
			break
		}
	}

	// If no null byte found, use the entire file name field.
	if filename == "" {
		filename = string(filenameBytes)
	}

	// Create and validate the header.
	header := &Header{
		FileSize: fileSize,
		Filename: filename,
	}
	if err := validateHeader(header); err != nil {
		return nil, fmt.Errorf("invalid header read from stream: %w", err)
	}

	return header, nil
}
