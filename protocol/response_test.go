package protocol

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// FailingWriter is a test helper that simulates write failures.
type FailingWriter struct {
	failAfter int // Number of bytes after which to fail.
	written   int // Number of bytes written so far.
}

// Write implements the `io.Writer` interface and fails after writing `failAfter` bytes.
func (fw *FailingWriter) Write(p []byte) (int, error) {
	if fw.written >= fw.failAfter {
		return 0, io.ErrShortWrite
	}
	n := len(p)
	if fw.written+n > fw.failAfter {
		n = fw.failAfter - fw.written
	}
	fw.written += n
	if n < len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

// CustomErrorReader simulates specific read error conditions.
type CustomErrorReader struct {
	data        []byte // Data to read from.
	pos         int    // Current read position.
	errorAt     int    // Position to trigger the error.
	customError error  // Error to return when triggered.
	returnBytes int    // Number of bytes to return after `errorAt` and before returning the error.
}

// Read implements the `io.Reader` interface and triggers a custom error at a specific position.
func (r *CustomErrorReader) Read(p []byte) (int, error) {
	if r.pos >= r.errorAt {
		if r.returnBytes > 0 && r.pos < r.errorAt+r.returnBytes {
			n := min(r.returnBytes, len(p), len(r.data)-r.pos)
			copy(p, r.data[r.pos:r.pos+n])
			r.pos += n
			return n, nil
		}
		return 0, r.customError
	}
	remaining := r.errorAt - r.pos
	n := min(len(p), remaining, len(r.data)-r.pos)
	copy(p, r.data[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

// TestWriteResponseNilWriter tests `WriteResponse` to ensure that
// it expectedly handles a nil writer.
func TestWriteResponseNilWriter(t *testing.T) {
	err := WriteResponse(nil, ResponseStatusSuccess, "test message")

	if err == nil {
		t.Error("expected error for the nil writer")
	}
	if !strings.Contains(err.Error(), "writer is nil") {
		t.Fatalf("expected 'writer is nil' error, got: %v", err)
	}
}

// TestWriteResponseInvalidStatus tests `WriteResponse` to ensure that
// it expectedly handles invalid status values.
func TestWriteResponseInvalidStatus(t *testing.T) {
	var buf bytes.Buffer
	invalidStatus := uint8(255)

	err := WriteResponse(&buf, invalidStatus, "test message")

	if err == nil {
		t.Error("expected error for the invalid status")
	}
	if !strings.Contains(err.Error(), "invalid response status") {
		t.Fatalf("expected 'invalid response status' error, got: %v", err)
	}
}

// TestWriteResponseMessageTooLong tests `WriteResponse` to ensure that
// it expectedly handles messages exceeding the maximum length.
func TestWriteResponseMessageTooLong(t *testing.T) {
	var buf bytes.Buffer
	excessiveMessage := strings.Repeat("a", MaxResponseMessageLength+1)

	err := WriteResponse(&buf, ResponseStatusSuccess, excessiveMessage)

	if err == nil {
		t.Error("expected error for message beyond maximum length")
	}
	if !strings.Contains(err.Error(), "invalid message length") {
		t.Fatalf("expected 'invalid message length' error, got: %v", err)
	}
}

// TestReadResponseNilReader tests `ReadResponse` to ensure that
// it expectedly handles a nil reader.
func TestReadResponseNilReader(t *testing.T) {
	_, _, err := ReadResponse(nil)

	if err == nil {
		t.Error("expected error for the nil reader")
	}
	if !strings.Contains(err.Error(), "reader is nil") {
		t.Fatalf("expected 'reader is nil' error, got: %v", err)
	}
}

// TestReadResponseEOFOnStatus tests `ReadResponse` to ensure that
// it expectedly handles EOF while reading status.
func TestReadResponseEOFOnStatus(t *testing.T) {
	buf := bytes.NewBuffer([]byte{})

	_, _, err := ReadResponse(buf)

	if err == nil {
		t.Error("expected error for EOF on status read")
	}
	if !strings.Contains(err.Error(), "unexpected end of stream while reading the response status") {
		t.Fatalf("expected 'unexpected end of stream' error, got: %v", err)
	}
}

// TestReadResponseInvalidStatus tests `ReadResponse` to ensure that
// it expectedly handles invalid status values.
func TestReadResponseInvalidStatus(t *testing.T) {
	buf := bytes.NewBuffer([]byte{255})

	_, _, err := ReadResponse(buf)

	if err == nil {
		t.Error("expected error for the invalid status")
	}
	if !strings.Contains(err.Error(), "invalid response status") {
		t.Fatalf("expected 'invalid response status' error, got: %v", err)
	}
}

// TestReadResponseEOFOnMessageLength tests `ReadResponse` to ensure that
// it expectedly handles EOF while reading the message length.
func TestReadResponseEOFOnMessageLength(t *testing.T) {
	// Intentionally provide only the status byte.
	buf := bytes.NewBuffer([]byte{ResponseStatusSuccess})

	_, _, err := ReadResponse(buf)

	if err == nil {
		t.Error("expected error for EOF on the message length read")
	}
	if !strings.Contains(err.Error(), "unexpected end of stream while reading the message length") {
		t.Fatalf("expected 'unexpected end of stream' error, got: %v", err)
	}
}

// TestReadResponseMessageLengthExceedsMax tests `ReadResponse` to ensure that
// it expectedly handles message lengths exceeding the maximum allowed.
func TestReadResponseMessageLengthExceedsMax(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte(ResponseStatusSuccess)
	// Intentionally write a message length exceeding the maximum.
	exceedingLength := uint32(MaxResponseMessageLength + 1)
	// `0x12345678` in big-endian (`[0x12, 0x34, 0x56, 0x78]`).
	buf.WriteByte(byte(exceedingLength >> 24)) // Write the highest byte (`0x12`).
	buf.WriteByte(byte(exceedingLength >> 16)) // Write the next byte (`0x34`).
	buf.WriteByte(byte(exceedingLength >> 8))  // Write the next byte (`0x56`).
	buf.WriteByte(byte(exceedingLength))       // Write the lowest byte (`0x78`).

	_, _, err := ReadResponse(&buf)

	if err == nil {
		t.Error("expected error for message length exceeding the maximum")
	}
	if !strings.Contains(err.Error(), "invalid message length") {
		t.Fatalf("expected 'invalid message length' error, got: %v", err)
	}
}

// TestReadResponseEOFOnMessage tests `ReadResponse` to ensure that
// it expectedly handles EOF while reading the message.
func TestReadResponseEOFOnMessage(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte(ResponseStatusSuccess)
	buf.WriteByte(0)
	buf.WriteByte(0)
	buf.WriteByte(0)
	// Specify a message length of 10 bytes.
	buf.WriteByte(10)
	// Intentionally write fewer than 10 bytes of message data (actually 4 bytes).
	buf.WriteString("test")

	_, _, err := ReadResponse(&buf)

	if err == nil {
		t.Error("expected error for the incomplete message")
	}
	if !strings.Contains(err.Error(), "unexpected end of stream while reading the message") {
		t.Fatalf("expected 'unexpected end of stream' error, got: %v", err)
	}
}

// TestReadWriteResponseRoundTripSuccessWithMessage tests a round-trip write and read
// of a success response with a non-empty message.
func TestReadWriteResponseRoundTripSuccessWithMessage(t *testing.T) {
	var buf bytes.Buffer
	message := "operation completed successfully"

	err := WriteResponse(&buf, ResponseStatusSuccess, message)
	if err != nil {
		t.Fatalf("failed to write the response: %v", err)
	}

	status, gotMessage, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("failed to read the response: %v", err)
	}

	if status != ResponseStatusSuccess {
		t.Fatalf("expected status %d, got %d", ResponseStatusSuccess, status)
	}
	if gotMessage != message {
		t.Fatalf("expected message %q, got %q", message, gotMessage)
	}
}

// TestReadWriteResponseRoundTripErrorWithMessage tests a round-trip write and read
// of an error response with a non-empty message.
func TestReadWriteResponseRoundTripErrorWithMessage(t *testing.T) {
	var buf bytes.Buffer
	message := "operation failed: invalid input"

	err := WriteResponse(&buf, ResponseStatusError, message)
	if err != nil {
		t.Fatalf("failed to write the response: %v", err)
	}

	status, gotMessage, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("failed to read the response: %v", err)
	}

	if status != ResponseStatusError {
		t.Fatalf("expected status %d, got %d", ResponseStatusError, status)
	}
	if gotMessage != message {
		t.Fatalf("expected message %q, got %q", message, gotMessage)
	}
}

// TestReadWriteResponseRoundTripEmptyMessage tests a round-trip write and read
// of a response with an empty message.
func TestReadWriteResponseRoundTripEmptyMessage(t *testing.T) {
	var buf bytes.Buffer
	message := ""

	err := WriteResponse(&buf, ResponseStatusSuccess, message)
	if err != nil {
		t.Fatalf("failed to write the response: %v", err)
	}

	status, gotMessage, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("failed to read the response: %v", err)
	}

	if status != ResponseStatusSuccess {
		t.Fatalf("expected status %d, got %d", ResponseStatusSuccess, status)
	}
	if gotMessage != message {
		t.Fatalf("expected message %q, got %q", message, gotMessage)
	}
}

// TestReadWriteResponseRoundTripLongMessage tests a round-trip write and read
// of a response with a long message.
func TestReadWriteResponseRoundTripLongMessage(t *testing.T) {
	var buf bytes.Buffer
	message := strings.Repeat("a", 1000)

	err := WriteResponse(&buf, ResponseStatusError, message)
	if err != nil {
		t.Fatalf("failed to write the response: %v", err)
	}

	status, gotMessage, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("failed to read the response: %v", err)
	}

	if status != ResponseStatusError {
		t.Fatalf("expected status %d, got %d", ResponseStatusError, status)
	}
	if gotMessage != message {
		t.Fatalf("expected message %q, got %q", message, gotMessage)
	}
}

// TestWriteResponseStatusWriteFailure tests `WriteResponse` to ensure that
// it expectedly handles failures when writing the status byte.
func TestWriteResponseStatusWriteFailure(t *testing.T) {
	// Fail immediately on the first byte (status byte).
	failingWriter := &FailingWriter{failAfter: 0}

	err := WriteResponse(failingWriter, ResponseStatusSuccess, "test")

	if err == nil {
		t.Error("expected error for the status write failure")
	}
	if !strings.Contains(err.Error(), "failed to write") {
		t.Fatalf("expected error about write failure, got: %v", err)
	}
}

// TestWriteResponseMessageLengthWriteFailure tests `WriteResponse` to ensure that
// it expectedly handles failures when writing the message length.
func TestWriteResponseMessageLengthWriteFailure(t *testing.T) {
	// Fail after 1 byte (status byte written, but `binary.Write` fails on the message length).
	failingWriter := &FailingWriter{failAfter: 1}

	err := WriteResponse(failingWriter, ResponseStatusSuccess, "test")

	if err == nil {
		t.Error("expected error for the message length write failure")
	}
	if !strings.Contains(err.Error(), "failed to write") {
		t.Fatalf("expected error about write failure, got: %v", err)
	}
}

// TestWriteResponseMessageBytesWriteFailure tests `WriteResponse` to ensure that
// it expectedly handles failures when writing the message bytes.
func TestWriteResponseMessageBytesWriteFailure(t *testing.T) {
	// Fail after 5 bytes (1-byte status + 4-bytes message length = 5, but the message bytes write fails).
	failingWriter := &FailingWriter{failAfter: 5}

	err := WriteResponse(failingWriter, ResponseStatusSuccess, "test message")

	if err == nil {
		t.Error("expected error for the message bytes write failure")
	}
	if !strings.Contains(err.Error(), "failed to write") {
		t.Fatalf("expected error about write failure, got: %v", err)
	}
}

// TestReadResponseCustomErrorOnStatus tests `ReadResponse` to ensure that
// it expectedly handles non-EOF error paths when reading status.
func TestReadResponseCustomErrorOnStatus(t *testing.T) {
	reader := &CustomErrorReader{
		data:        []byte{},
		errorAt:     0, // Error on the first byte (status byte).
		customError: io.ErrNoProgress,
	}

	_, _, err := ReadResponse(reader)

	if err == nil {
		t.Error("expected error for the custom read error on status")
	}
	if !strings.Contains(err.Error(), "failed to read the response status") {
		t.Fatalf("expected 'failed to read the response status' error, got: %v", err)
	}
}

// TestReadResponseCustomErrorOnMessageLength tests `ReadResponse` to ensure that
// it expectedly handles non-EOF error paths when reading message length.
func TestReadResponseCustomErrorOnMessageLength(t *testing.T) {
	reader := &CustomErrorReader{
		data:        []byte{ResponseStatusSuccess},
		errorAt:     1, // Error after reading the status byte.
		customError: io.ErrNoProgress,
	}

	_, _, err := ReadResponse(reader)

	if err == nil {
		t.Error("expected error for custom read error on message length")
	}
	if !strings.Contains(err.Error(), "failed to read the message length") {
		t.Fatalf("expected 'failed to read the message length' error, got: %v", err)
	}
}

// TestReadResponseCustomErrorOnMessage tests `ReadResponse` to ensure that
// it expectedly handles non-EOF error paths when reading message.
func TestReadResponseCustomErrorOnMessage(t *testing.T) {
	reader := &CustomErrorReader{
		data:        []byte{ResponseStatusSuccess, 0, 0, 0, 10}, // Status + message length of 10.
		errorAt:     5,                                          // Error after reading status and length.
		customError: io.ErrNoProgress,
	}

	_, _, err := ReadResponse(reader)

	if err == nil {
		t.Error("expected error for custom read error on message")
	}
	if !strings.Contains(err.Error(), "failed to read the message") {
		t.Fatalf("expected 'failed to read the message' error, got: %v", err)
	}
}
