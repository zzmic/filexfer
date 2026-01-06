package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"testing"
)

// newValidHeader creates a valid `Header` for testing.
func newValidHeader() *Header {
	return &Header{
		MessageType:   MessageTypeTransfer,                      // Valid message type.
		FileSize:      1234,                                     // Example file size.
		FileName:      "example.txt",                            // Example file name.
		Checksum:      bytes.Repeat([]byte{0xAA}, ChecksumSize), // Valid checksum.
		TransferType:  TransferTypeFile,                         // Valid transfer type.
		DirectoryPath: "",                                       // Empty string for file transfer.
	}
}

// u64Bytes converts a `uint64` to a byte slice in big-endian order.
func u64Bytes(v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return b[:]
}

// u32Bytes converts a `uint32` to a byte slice in big-endian order.
func u32Bytes(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return b[:]
}

// failingWriter is an `io.Writer` that fails after a specified number of writes.
type failingWriter struct {
	failOn int
	calls  int
}

// Write implements the `io.Writer` interface and fails on the specified call.
func (w *failingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failOn {
		return 0, fmt.Errorf("forced write error")
	}
	return len(p), nil
}

// readStep represents a single read operation in a `scriptedReader`.
type readStep struct {
	data []byte
	err  error
}

// scriptedReader is an `io.Reader` that follows a predefined sequence of read steps.
type scriptedReader struct {
	steps []readStep
	idx   int
}

// Read implements the `io.Reader` interface and follows the scripted read steps.
func (r *scriptedReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.steps) {
		return 0, io.EOF
	}
	step := r.steps[r.idx]
	r.idx++
	if len(step.data) > 0 {
		n := copy(p, step.data)
		return n, step.err
	}
	return 0, step.err
}

// TestValidateHeaderSuccess tests the `validateHeader` function to ensure that
// it expectedly validates valid headers.
func TestValidateHeaderSuccess(t *testing.T) {
	// Validate a standard valid header.
	if err := validateHeader(newValidHeader()); err != nil {
		t.Fatalf("expected valid transfer header, got error: %v", err)
	}

	// Validate a valid validation header (with empty filename).
	validationHeader := newValidHeader()
	validationHeader.MessageType = MessageTypeValidate
	validationHeader.FileName = ""
	if err := validateHeader(validationHeader); err != nil {
		t.Fatalf("expected valid validation header, got error: %v", err)
	}
}

// TestValidateHeaderErrors tests the `validateHeader` function to ensure that
// it expectedly rejects invalid headers.
func TestValidateHeaderErrors(t *testing.T) {
	tests := []struct {
		name   string
		header *Header
	}{
		{"nil header", nil},
		{"invalid message type", func() *Header { h := newValidHeader(); h.MessageType = 3; return h }()},
		{"empty filename for transfer", func() *Header { h := newValidHeader(); h.FileName = ""; return h }()},
		{"filename too long", func() *Header { h := newValidHeader(); h.FileName = strings.Repeat("a", MaxFileNameLength+1); return h }()},
		{"filename contains null", func() *Header { h := newValidHeader(); h.FileName = "bad\x00name"; return h }()},
		{"nil checksum", func() *Header { h := newValidHeader(); h.Checksum = nil; return h }()},
		{"checksum wrong length", func() *Header {
			h := newValidHeader()
			h.Checksum = bytes.Repeat([]byte{0x01}, ChecksumSize-1)
			return h
		}()},
		{"invalid transfer type", func() *Header { h := newValidHeader(); h.TransferType = 3; return h }()},
		{"directory path too long", func() *Header {
			h := newValidHeader()
			h.TransferType = TransferTypeDirectory
			h.DirectoryPath = strings.Repeat("d", MaxDirPathLength+1)
			return h
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHeader(tt.header)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// TestWriteAndReadHeaderRoundTrip tests a round-trip write and read of a header.
func TestWriteAndReadHeaderRoundTrip(t *testing.T) {
	buf := &bytes.Buffer{}
	header := newValidHeader()

	if err := WriteHeader(buf, header); err != nil {
		t.Fatalf("WriteHeader returned error: %v", err)
	}

	got, err := ReadHeader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ReadHeader returned error: %v", err)
	}

	if got.MessageType != header.MessageType {
		t.Errorf("MessageType mismatch: got %d, want %d", got.MessageType, header.MessageType)
	}
	if got.FileSize != header.FileSize {
		t.Errorf("FileSize mismatch: got %d, want %d", got.FileSize, header.FileSize)
	}
	if got.FileName != header.FileName {
		t.Errorf("FileName mismatch: got %s, want %s", got.FileName, header.FileName)
	}
	if !bytes.Equal(got.Checksum, header.Checksum) {
		t.Errorf("Checksum mismatch: got %v, want %v", got.Checksum, header.Checksum)
	}
	if got.TransferType != header.TransferType {
		t.Errorf("TransferType mismatch: got %d, want %d", got.TransferType, header.TransferType)
	}
	if got.DirectoryPath != header.DirectoryPath {
		t.Errorf("DirectoryPath mismatch: got %s, want %s", got.DirectoryPath, header.DirectoryPath)
	}
}

// TestWriteHeaderErrors tests the `WriteHeader` function to ensure that it
// expectedly returns errors for invalid conditions.
func TestWriteHeaderErrors(t *testing.T) {
	if err := WriteHeader(nil, newValidHeader()); err == nil {
		t.Fatalf("expected error for nil writer, got nil")
	}

	invalid := newValidHeader()
	invalid.MessageType = 9
	if err := WriteHeader(&bytes.Buffer{}, invalid); err == nil {
		t.Fatalf("expected error for invalid header, got nil")
	}

	badChecksum := newValidHeader()
	badChecksum.Checksum = bytes.Repeat([]byte{0xFF}, ChecksumSize-2)
	if err := WriteHeader(&bytes.Buffer{}, badChecksum); err == nil {
		t.Fatalf("expected error for invalid checksum size, got nil")
	}

	tests := []struct {
		name        string
		failOn      int
		expectError string
	}{
		{"message type write error", 1, "failed to write the message type"},
		{"file size write error", 2, "failed to write the file size"},
		{"filename length write error", 3, "failed to write the filename length"},
		{"filename write error", 4, "failed to write the filename"},
		{"checksum write error", 5, "failed to write the checksum"},
		{"transfer type write error", 6, "failed to write the transfer type"},
		{"directory path length write error", 7, "failed to write the directory path length"},
		{"directory path write error", 8, "failed to write the directory path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := &failingWriter{failOn: tt.failOn}
			header := newValidHeader()
			header.DirectoryPath = "dir"
			if err := WriteHeader(fw, header); err == nil || !strings.Contains(err.Error(), tt.expectError) {
				t.Fatalf("expected error containing %q, got %v", tt.expectError, err)
			}
		})
	}
}

// TestReadHeaderErrors tests the `ReadHeader` function to ensure that it
// expectedly returns errors for invalid conditions.
func TestReadHeaderErrors(t *testing.T) {
	// Nil reader.
	if _, err := ReadHeader(nil); err == nil {
		t.Fatalf("expected error for the nil reader, got nil")
	}

	// EOF on the first byte (message type).
	if _, err := ReadHeader(bytes.NewReader([]byte{})); err == nil {
		t.Fatalf("expected error for the empty reader, got nil")
	}

	// Filename too long.
	buf := &bytes.Buffer{}
	buf.WriteByte(MessageTypeTransfer)
	if err := binary.Write(buf, binary.BigEndian, uint64(0)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(MaxFileNameLength+1)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	if _, err := ReadHeader(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatalf("expected error for the filename being too long, got nil")
	}

	// Truncated checksum.
	buf.Reset()
	buf.WriteByte(MessageTypeTransfer)
	if err := binary.Write(buf, binary.BigEndian, uint64(10)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	name := []byte("name")
	if err := binary.Write(buf, binary.BigEndian, uint32(len(name))); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	buf.Write(name)
	buf.Write(bytes.Repeat([]byte{0x01}, 10)) // shorter than ChecksumSize
	if _, err := ReadHeader(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatalf("expected error for the truncated checksum, got nil")
	}

	// Directory path length exceeds the limit.
	buf.Reset()
	buf.WriteByte(MessageTypeTransfer)
	if err := binary.Write(buf, binary.BigEndian, uint64(0)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(1)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	buf.Write([]byte("a"))
	buf.Write(bytes.Repeat([]byte{0x02}, ChecksumSize))
	buf.WriteByte(TransferTypeDirectory)
	if err := binary.Write(buf, binary.BigEndian, uint32(MaxDirPathLength+1)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	if _, err := ReadHeader(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatalf("expected error for directory path too long, got nil")
	}

	customErr := fmt.Errorf("custom read error")
	errTests := []struct {
		name   string
		reader *scriptedReader
		expect string
	}{
		{"message type read error", &scriptedReader{steps: []readStep{{data: nil, err: customErr}}}, "failed to read the message type"},
		{"file size read error", &scriptedReader{steps: []readStep{{data: []byte{MessageTypeTransfer}}, {data: nil, err: customErr}}}, "failed to read the file size"},
		{"filename length read error", &scriptedReader{steps: []readStep{{data: []byte{MessageTypeTransfer}}, {data: u64Bytes(1)}, {data: nil, err: customErr}}}, "failed to read the filename length"},
		{"filename read error", &scriptedReader{steps: []readStep{{data: []byte{MessageTypeTransfer}}, {data: u64Bytes(1)}, {data: u32Bytes(1)}, {data: nil, err: customErr}}}, "failed to read the filename"},
		{"checksum read error", &scriptedReader{steps: []readStep{{data: []byte{MessageTypeTransfer}}, {data: u64Bytes(1)}, {data: u32Bytes(1)}, {data: []byte("f")}, {data: nil, err: customErr}}}, "failed to read the checksum"},
		{"transfer type read error", &scriptedReader{steps: []readStep{{data: []byte{MessageTypeTransfer}}, {data: u64Bytes(1)}, {data: u32Bytes(1)}, {data: []byte("f")}, {data: bytes.Repeat([]byte{0x01}, ChecksumSize)}, {data: nil, err: customErr}}}, "failed to read the transfer type"},
		{"directory path length read error", &scriptedReader{steps: []readStep{{data: []byte{MessageTypeTransfer}}, {data: u64Bytes(1)}, {data: u32Bytes(1)}, {data: []byte("f")}, {data: bytes.Repeat([]byte{0x01}, ChecksumSize)}, {data: []byte{TransferTypeDirectory}}, {data: nil, err: customErr}}}, "failed to read the directory path length"},
		{"directory path read error", &scriptedReader{steps: []readStep{{data: []byte{MessageTypeTransfer}}, {data: u64Bytes(1)}, {data: u32Bytes(1)}, {data: []byte("f")}, {data: bytes.Repeat([]byte{0x01}, ChecksumSize)}, {data: []byte{TransferTypeDirectory}}, {data: u32Bytes(1)}, {data: nil, err: customErr}}}, "failed to read the directory path"},
	}

	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ReadHeader(tt.reader); err == nil || !strings.Contains(err.Error(), tt.expect) {
				t.Fatalf("expected error containing %q, got %v", tt.expect, err)
			}
		})
	}

	// EOF while reading the file size.
	if _, err := ReadHeader(bytes.NewReader([]byte{MessageTypeTransfer})); err == nil {
		t.Fatalf("expected error for EOF while reading the file size, got nil")
	}

	// EOF while reading the filename length.
	buf.Reset()
	buf.WriteByte(MessageTypeTransfer)
	if err := binary.Write(buf, binary.BigEndian, uint64(10)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	if _, err := ReadHeader(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatalf("expected error for EOF while reading the filename length, got nil")
	}

	// EOF while reading the filename.
	buf.Reset()
	buf.WriteByte(MessageTypeTransfer)
	if err := binary.Write(buf, binary.BigEndian, uint64(10)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(4)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	// Intentionally provide fewer bytes than the declared length to trigger an unexpected EOF on the filename.
	buf.Write([]byte("na"))
	if _, err := ReadHeader(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatalf("expected error for EOF while reading the filename, got nil")
	}

	// EOF while reading the transfer type.
	buf.Reset()
	buf.WriteByte(MessageTypeTransfer)
	if err := binary.Write(buf, binary.BigEndian, uint64(1)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	name = []byte("f")
	if err := binary.Write(buf, binary.BigEndian, uint32(len(name))); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	buf.Write(name)
	buf.Write(bytes.Repeat([]byte{0x01}, ChecksumSize))
	if _, err := ReadHeader(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatalf("expected error for EOF while reading the transfer type, got nil")
	}

	// EOF while reading the directory path length.
	buf.Reset()
	buf.WriteByte(MessageTypeTransfer)
	if err := binary.Write(buf, binary.BigEndian, uint64(1)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	name = []byte("f")
	if err := binary.Write(buf, binary.BigEndian, uint32(len(name))); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	buf.Write(name)
	buf.Write(bytes.Repeat([]byte{0x01}, ChecksumSize))
	buf.WriteByte(TransferTypeDirectory)
	if _, err := ReadHeader(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatalf("expected error for EOF while reading the directory path length, got nil")
	}

	// EOF while reading the directory path.
	buf.Reset()
	buf.WriteByte(MessageTypeTransfer)
	if err := binary.Write(buf, binary.BigEndian, uint64(1)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	name = []byte("f")
	if err := binary.Write(buf, binary.BigEndian, uint32(len(name))); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	buf.Write(name)
	buf.Write(bytes.Repeat([]byte{0x01}, ChecksumSize))
	buf.WriteByte(TransferTypeDirectory)
	if err := binary.Write(buf, binary.BigEndian, uint32(2)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	// Intentionally provide fewer bytes than the declared length to trigger an unexpected EOF on the directory path.
	buf.Write([]byte("d"))
	if _, err := ReadHeader(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatalf("expected error for EOF while reading the directory path, got nil")
	}

	// Invalid transfer type even the message is structurally complete (but semantically invalid).
	buf.Reset()
	buf.WriteByte(MessageTypeTransfer)
	if err := binary.Write(buf, binary.BigEndian, uint64(1)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	name = []byte("f")
	if err := binary.Write(buf, binary.BigEndian, uint32(len(name))); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	buf.Write(name)
	buf.Write(bytes.Repeat([]byte{0x01}, ChecksumSize))
	// Intentionally write an invalid transfer type.
	buf.WriteByte(3)
	if err := binary.Write(buf, binary.BigEndian, uint32(0)); err != nil {
		t.Fatalf("failed to write to the buffer: %v", err)
	}
	if _, err := ReadHeader(bytes.NewReader(buf.Bytes())); err == nil || !strings.Contains(err.Error(), "invalid transfer type in the header") {
		t.Fatalf("expected 'invalid transfer type in the header' error, got %v", err)
	}
}
