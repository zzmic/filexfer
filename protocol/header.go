package protocol

import (
	"encoding/binary"
	"io"
)

const (
	// `HeaderSize` represents the total size of the header in bytes.
	// `8` bytes for file size + `256` bytes for filename
	HeaderSize = 8 + 256
	// `FilenameSize` represents the maximum size of the `filename` in bytes.
	FilenameSize = 256
)

// `Header` represents the file transfer header.
type Header struct {
	FileSize uint64
	Filename string
}

// Function to write the header to the given writer.
func WriteHeader(w io.Writer, header *Header) error {
	// Write the file size as 8 bytes in big-endian format.
	sizeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(sizeBytes, header.FileSize)
	if _, err := w.Write(sizeBytes); err != nil {
		return err
	}

	// Write the filename as fixed-size bytes (pad with zeros if shorter).
	filenameBytes := make([]byte, FilenameSize)
	copy(filenameBytes, []byte(header.Filename))
	_, err := w.Write(filenameBytes)
	return err
}

// Function to read the header from the given reader.
func ReadHeader(r io.Reader) (*Header, error) {
	// Read the entire header into a buffer.
	headerBytes := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, headerBytes); err != nil {
		return nil, err
	}

	// Extract the file size (first 8 bytes).
	fileSize := binary.BigEndian.Uint64(headerBytes[:8])

	// Extract the filename (next 256 bytes, trim null bytes).
	filename := string(headerBytes[8:])
	// Find the first null byte and trim from there.
	for i, b := range filename {
		if b == 0 {
			filename = filename[:i]
			break
		}
	}

	return &Header{
		FileSize: fileSize,
		Filename: filename,
	}, nil
}
