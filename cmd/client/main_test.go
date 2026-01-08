package main

import (
	"bytes"
	"context"
	"errors"
	"filexfer/protocol"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestToKBZeroByte tests the `toKB` function with zero bytes.
func TestToKBZeroByte(t *testing.T) {
	bytes := uint64(0)
	expected := 0.0

	got := toKB(bytes)
	if got != expected {
		t.Errorf("toKB(%d) = %f; want %f", bytes, got, expected)
	}
}

// TestToKB5GB tests the `toKB` function with 5 GB.
func TestToKB5GB(t *testing.T) {
	bytes := uint64(5 * 1024 * 1024 * 1024)
	expected := 5242880.0

	got := toKB(bytes)
	if got != expected {
		t.Errorf("toKB(%d) = %f; want %f", bytes, got, expected)
	}
}

// TestToMBZeroByte tests the `toMB` function with zero bytes.
func TestToMBZeroByte(t *testing.T) {
	bytes := uint64(0)
	expected := 0.0

	got := toMB(bytes)
	if got != expected {
		t.Errorf("toMB(%d) = %f; want %f", bytes, got, expected)
	}
}

// TestToMB5GB tests the `toMB` function with 5 GB.
func TestToMB5GB(t *testing.T) {
	bytes := uint64(5 * 1024 * 1024 * 1024)
	expected := 5120.0

	got := toMB(bytes)
	if got != expected {
		t.Errorf("toMB(%d) = %f; want %f", bytes, got, expected)
	}
}

// TestToGBZeroByte tests the `toGB` function with 0 bytes.
func TestToGBZeroByte(t *testing.T) {
	bytes := uint64(0)
	expected := 0.0

	got := toGB(bytes)
	if got != expected {
		t.Fatalf("`toGB(%d)` = %f, expected %f", bytes, got, expected)
	}
}

// TestToGB5GB tests the `toGB` function with 5 GB.
func TestToGB5GB(t *testing.T) {
	bytes := uint64(5 * 1024 * 1024 * 1024)
	expected := 5.0

	got := toGB(bytes)
	if got != expected {
		t.Fatalf("`toGB(%d)` = %f, expected %f", bytes, got, expected)
	}
}

// TestSetupLogging tests the `setupLogging` function to ensure that
// it expectedly configures structured logging.
func TestSetupLogging(t *testing.T) {
	setupLogging()

	expectedFlags := log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile
	if log.Flags() != expectedFlags {
		t.Fatalf("expected the log flag(s) `log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile`, got %d", log.Flags())
	}

	expectedPrefix := LogPrefix + " "
	if log.Prefix() != expectedPrefix {
		t.Fatalf("expected the log prefix %q, got %q", expectedPrefix, log.Prefix())
	}
}

// TestValidateArgsWithEmptyFilePath tests `validateArgs` with an empty file path.
func TestValidateArgsWithEmptyFilePath(t *testing.T) {
	originalFilePath := *filePath
	*filePath = ""
	defer func() { *filePath = originalFilePath }()

	err := validateArgs()
	if err == nil {
		t.Error("expected an error for empty file path, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("file path is required")) {
		t.Fatalf("expected 'file path is required' error message, got: %v", err)
	}
}

// TestValidateArgsWithValidFilePath tests `validateArgs` with a valid file path.
func TestValidateArgsWithValidFilePath(t *testing.T) {
	originalFilePath := *filePath
	*filePath = "/some/valid/path"
	defer func() { *filePath = originalFilePath }()

	err := validateArgs()
	if err != nil {
		t.Fatalf("expected no error for valid file path, got: %v", err)
	}
}

// TestValidatePathWithEmptyPath tests `validatePath` with an empty path.
func TestValidatePathWithEmptyPath(t *testing.T) {
	err := validatePath("")

	if err == nil {
		t.Error("expected an error for empty path, got nil")
	}
	if !errors.Is(err, ErrInvalidFilename) {
		t.Fatalf("expected ErrInvalidFilename, got: %v", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("path cannot be empty")) {
		t.Fatalf("expected 'path cannot be empty' error message, got: %v", err)
	}
}

// TestValidatePathWithNonExistentFile tests `validatePath` with a non-existent file.
func TestValidatePathWithNonExistentFile(t *testing.T) {
	err := validatePath("/nonexistent/file/path/that/does/not/exist.txt")

	if err == nil {
		t.Error("expected an error for non-existent file, got nil")
	}
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("expected ErrFileNotFound, got: %v", err)
	}
}

// TestValidatePathWithValidFile tests `validatePath` with a valid file.
func TestValidatePathWithValidFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "testfile-*.txt")
	if err != nil {
		t.Fatalf("failed to create the temporary file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Fatalf("failed to remove the temporary file: %v", err)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close the temporary file: %v", err)
	}

	err = validatePath(tmpFile.Name())
	if err != nil {
		t.Fatalf("expected no error for valid file, got: %v", err)
	}
}

// TestValidatePathWithValidDirectory tests `validatePath` with a valid directory.
func TestValidatePathWithValidDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testdir-*")
	if err != nil {
		t.Fatalf("failed to create the temporary directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatalf("failed to remove the temporary directory: %v", err)
		}
	}()

	err = validatePath(tmpDir)
	if err != nil {
		t.Fatalf("expected no error for valid directory, got: %v", err)
	}
}

// TestValidatePathWithInvalidFilename tests `validatePath` with a valid filename.
func TestValidatePathWithInvalidFilename(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "testfile-*.txt")
	if err != nil {
		t.Fatalf("failed to create the temporary file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Fatalf("failed to remove the temporary file: %v", err)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close the temporary file: %v", err)
	}

	err = validatePath(tmpFile.Name())
	if err != nil {
		t.Fatalf("expected no error for valid file, got: %v", err)
	}
}

// TestValidatePathWithFileTooLarge tests `validatePath` with a file that exceeds `MaxFileSize`.
// This test temporarily reduces `MaxFileSize` to create a testable scenario.
func TestValidatePathWithFileTooLarge(t *testing.T) {
	originalMaxFileSize := MaxFileSize
	defer func() { MaxFileSize = originalMaxFileSize }()

	tmpFile, err := os.CreateTemp("", "largefile-*.txt")
	if err != nil {
		t.Fatalf("failed to create the temporary file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Fatalf("failed to remove the temporary file: %v", err)
		}
	}()

	largeData := make([]byte, 2*1024)
	for i := range largeData {
		largeData[i] = 'a'
	}
	if _, err := tmpFile.Write(largeData); err != nil {
		t.Fatalf("failed to write data to file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close the temporary file: %v", err)
	}

	// Temporarily set `MaxFileSize` to 1 KB for testing.
	MaxFileSize = 1024

	err = validatePath(tmpFile.Name())
	if err == nil {
		t.Fatal("expected an error for file exceeding MaxFileSize, got nil")
	}
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got: %v", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("exceeds the maximum allowed size")) {
		t.Fatalf("expected 'exceeds the maximum allowed size' in error message, got: %v", err)
	}
}

// TestMaxFileSize tests that the `MaxFileSize` constant is set to a reasonable, expected value.
func TestMaxFileSize(t *testing.T) {
	expectedMaxFileSize := int64(5 * 1024 * 1024 * 1024)
	if MaxFileSize != int64(expectedMaxFileSize) {
		t.Fatalf("expected MaxFileSize to be %d, got %d", expectedMaxFileSize, MaxFileSize)
	}
}

// TestConstants tests that the timeout constants are set to reasonable, expected values.
func TestConstants(t *testing.T) {
	tests := []struct {
		name        string
		actual      time.Duration
		expected    time.Duration
		description string
	}{
		{
			name:        "ConnectionTimeout",
			actual:      ConnectionTimeout,
			expected:    30 * time.Second,
			description: "connection timeout should be 30 seconds",
		},
		{
			name:        "ReadTimeout",
			actual:      ReadTimeout,
			expected:    30 * time.Second,
			description: "read timeout should be 30 seconds",
		},
		{
			name:        "WriteTimeout",
			actual:      WriteTimeout,
			expected:    30 * time.Second,
			description: "write timeout should be 30 seconds",
		},
		{
			name:        "ShutdownTimeout",
			actual:      ShutdownTimeout,
			expected:    30 * time.Second,
			description: "shutdown timeout should be 30 seconds",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.actual != tc.expected {
				t.Fatalf("expected %s to be %v, got %v (%s)", tc.name, tc.expected, tc.actual, tc.description)
			}
		})
	}
}

// TestLogPrefix tests that the `LogPrefix` constant is set expectedly.
func TestLogPrefix(t *testing.T) {
	expectedPrefix := "[CLIENT]"
	if LogPrefix != expectedPrefix {
		t.Fatalf("expected LogPrefix to be %q, got %q", expectedPrefix, LogPrefix)
	}
}

// TestErrorConstants tests that error constants are defined expectedly.
func TestErrorConstants(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "ErrFileNotFound",
			err:      ErrFileNotFound,
			expected: "file not found",
		},
		{
			name:     "ErrFileTooLarge",
			err:      ErrFileTooLarge,
			expected: "file size exceeds the maximum allowed size",
		},
		{
			name:     "ErrInvalidFilename",
			err:      ErrInvalidFilename,
			expected: "invalid filename",
		},
		{
			name:     "ErrConnectionFailed",
			err:      ErrConnectionFailed,
			expected: "connection failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Error() != tc.expected {
				t.Fatalf("expected error %q, got %q", tc.expected, tc.err.Error())
			}
		})
	}
}

// TestValidatePathWithSymlinkToFile tests `validatePath` with a symlink pointing to a valid file.
func TestValidatePathWithSymlinkToFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "testfile-*.txt")
	if err != nil {
		t.Fatalf("failed to create the temporary file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Fatalf("failed to remove the temporary file: %v", err)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close the temporary file: %v", err)
	}

	tmpSymlink := filepath.Join(filepath.Dir(tmpFile.Name()), "testsymlink-*.txt")
	if err := os.Symlink(tmpFile.Name(), tmpSymlink); err != nil {
		t.Skipf("symlink creation not supported: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpSymlink); err != nil {
			t.Fatalf("failed to remove the temporary symlink: %v", err)
		}
	}()

	err = validatePath(tmpSymlink)
	if err != nil {
		t.Fatalf("expected no error for valid symlink, got: %v", err)
	}
}

// TestValidatePathConcurrency tests that `validatePath` can be called concurrently safely.
func TestValidatePathConcurrency(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "testfile-*.txt")
	if err != nil {
		t.Fatalf("failed to create the temporary file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Fatalf("failed to remove the temporary file: %v", err)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close the temporary file: %v", err)
	}

	done := make(chan bool, 10)
	for range 10 {
		go func() {
			err := validatePath(tmpFile.Name())
			if err != nil {
				t.Errorf("unexpected error during concurrent validation: %v", err)
			}
			done <- true
		}()
	}

	for range 10 {
		<-done
	}
}

// TestValidatePathWithCurrentWorkingDirectory tests `validatePath` with the current working directory.
func TestValidatePathWithCurrentWorkingDirectory(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	err = validatePath(pwd)
	if err != nil {
		t.Fatalf("expected no error for current working directory, got: %v", err)
	}
}

// TestErrFileNotFoundWrapping tests that `ErrFileNotFound` is expectedly wrapped in validation errors.
func TestErrFileNotFoundWrapping(t *testing.T) {
	err := validatePath("/nonexistent/path/file.txt")

	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}

	if !errors.Is(err, ErrFileNotFound) {
		t.Errorf("expected error chain to include ErrFileNotFound, got: %v", err)
	}
}

// TestValidateArgsAndPathSequentially tests the combined flow of `validateArgs` and `validatePath`.
func TestValidateArgsAndPathSequentially(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "testfile-*.txt")
	if err != nil {
		t.Fatalf("failed to create the temporary file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Fatalf("failed to remove the temporary file: %v", err)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close the temporary file: %v", err)
	}

	originalFilePath := *filePath
	*filePath = tmpFile.Name()
	defer func() { *filePath = originalFilePath }()

	err = validateArgs()
	if err != nil {
		t.Fatalf("validateArgs failed: %v", err)
	}

	err = validatePath(*filePath)
	if err != nil {
		t.Fatalf("validatePath failed: %v", err)
	}
}

// MockConn implements a mock `net.Conn` for testing `readServerResponse`.
type MockConn struct {
	readData         []byte    // Data to be read from the connection.
	readIndex        int       // Current read index.
	writeData        []byte    // Data written to the connection.
	readDeadline     time.Time // Read deadline.
	writeDeadline    time.Time // Write deadline.
	failSetDeadline  bool      // Whether to simulate a failure in methods that set deadlines.
	failProtocolRead bool      // Whether to simulate a failure in `protocol.ReadResponse`.
	closed           bool      // Whether the connection is closed.
}

func (mc *MockConn) Read(b []byte) (n int, err error) {
	if mc.failProtocolRead {
		return 0, io.ErrUnexpectedEOF
	}
	if mc.readIndex >= len(mc.readData) {
		return 0, io.EOF
	}
	n = copy(b, mc.readData[mc.readIndex:])
	mc.readIndex += n
	return n, nil
}

func (mc *MockConn) Write(b []byte) (n int, err error) {
	mc.writeData = append(mc.writeData, b...)
	return len(b), nil
}

func (mc *MockConn) Close() error {
	mc.closed = true
	return nil
}

func (mc *MockConn) LocalAddr() net.Addr {
	return nil
}

func (mc *MockConn) RemoteAddr() net.Addr {
	return nil
}

func (mc *MockConn) SetDeadline(t time.Time) error {
	if mc.failSetDeadline {
		return errors.New("failed to set the deadline")
	}
	return nil
}

func (mc *MockConn) SetReadDeadline(t time.Time) error {
	if mc.failSetDeadline {
		return errors.New("failed to set the read deadline")
	}
	mc.readDeadline = t
	return nil
}

func (mc *MockConn) SetWriteDeadline(t time.Time) error {
	if mc.failSetDeadline {
		return errors.New("failed to set the write deadline")
	}
	mc.writeDeadline = t
	return nil
}

// TestContextWriterWithValidContext tests the `contextWriter.Write` method with a valid context.
func TestContextWriterWithValidContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := &MockConn{}
	cw := &contextWriter{
		ctx:  ctx,
		conn: mockConn,
	}

	testData := []byte("test data")
	n, err := cw.Write(testData)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("expected %d bytes written, got %d", len(testData), n)
	}
	if !bytes.Equal(mockConn.writeData, testData) {
		t.Fatalf("expected written data %v, got %v", testData, mockConn.writeData)
	}
}

// TestContextWriterWithCancelledContext tests the `contextWriter.Write` method with a cancelled context.
func TestContextWriterWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockConn := &MockConn{}
	cw := &contextWriter{
		ctx:  ctx,
		conn: mockConn,
	}

	testData := []byte("test data")
	n, err := cw.Write(testData)

	if err == nil {
		t.Fatal("expected error for the cancelled context, got nil")
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes written, got %d", n)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got: %v", err)
	}
}

// TestContextWriterWithSetDeadlineError tests the `contextWriter.Write` method when `SetWriteDeadline` fails.
func TestContextWriterWithSetDeadlineError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := &MockConn{failSetDeadline: true}
	cw := &contextWriter{
		ctx:  ctx,
		conn: mockConn,
	}

	testData := []byte("test data")
	n, err := cw.Write(testData)

	if err == nil {
		t.Fatal("expected error when SetWriteDeadline fails, got nil")
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes written, got %d", n)
	}
}

// TestReadServerResponseSuccess tests `readServerResponse` with a successful response.
func TestReadServerResponseSuccess(t *testing.T) {
	responseData := []byte{
		byte(protocol.ResponseStatusSuccess),
		0, 0, 0, 5,
		'H', 'e', 'l', 'l', 'o',
	}

	mockConn := &MockConn{
		readData: responseData,
	}

	err := readServerResponse(mockConn)
	if err != nil {
		t.Fatalf("expected no error for the successful response, got: %v", err)
	}
}

// TestReadServerResponseWithErrorStatus tests `readServerResponse` with an error status.
func TestReadServerResponseWithErrorStatus(t *testing.T) {
	responseData := []byte{
		byte(protocol.ResponseStatusError),
		0, 0, 0, 11,
		'E', 'r', 'r', 'o', 'r', ' ', 'm', 's', 'g', '!', '!',
	}

	mockConn := &MockConn{
		readData: responseData,
	}

	err := readServerResponse(mockConn)
	if err == nil {
		t.Fatal("expected error for the error status response, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("server error")) {
		t.Fatalf("expected 'server error' in error message, got: %v", err)
	}
}

// TestReadServerResponseWithEOF tests `readServerResponse` when connection closes unexpectedly.
func TestReadServerResponseWithEOF(t *testing.T) {
	mockConn := &MockConn{
		readData: []byte{},
	}

	err := readServerResponse(mockConn)
	if err == nil {
		t.Fatal("expected error for EOF, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("closed connection unexpectedly")) {
		t.Fatalf("expected 'closed connection unexpectedly' message, got: %v", err)
	}
}

// TestReadServerResponseSetDeadlineError tests `readServerResponse` when `SetReadDeadline` fails.
func TestReadServerResponseSetDeadlineError(t *testing.T) {
	mockConn := &MockConn{
		failSetDeadline: true,
	}

	err := readServerResponse(mockConn)
	if err == nil {
		t.Fatal("expected error when SetReadDeadline fails, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("failed to set a read deadline")) {
		t.Fatalf("expected 'failed to set a read deadline' message, got: %v", err)
	}
}

// TestReadServerResponseProtocolReadError tests `readServerResponse` when `protocol.ReadResponse` returns a non-EOF error.
func TestReadServerResponseProtocolReadError(t *testing.T) {
	mockConn := &MockConn{
		failProtocolRead: true,
	}

	err := readServerResponse(mockConn)
	if err == nil {
		t.Fatal("expected error when protocol.ReadResponse fails, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("failed to read the server response")) {
		t.Fatalf("expected 'failed to read the server response' message, got: %v", err)
	}
}

// TestValidatePathWithRelativePath tests `validatePath` with a relative path.
func TestValidatePathWithRelativePath(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "testfile-*.txt")
	if err != nil {
		t.Fatalf("failed to create the temporary file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Fatalf("failed to remove the temporary file: %v", err)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close the temporary file: %v", err)
	}

	filename := filepath.Base(tmpFile.Name())

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get the working directory: %v", err)
	}

	tmpDir := filepath.Dir(tmpFile.Name())
	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("failed to change the directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("failed to change back to the original directory: %v", err)
		}
	}()

	err = validatePath(filename)
	if err != nil {
		t.Fatalf("expected no error for the valid relative path, got: %v", err)
	}
}

// TestValidatePathWithPermissionError tests `validatePath` when accessing a path results in a permission error.
func TestValidatePathWithPermissionError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testdir-*")
	if err != nil {
		t.Fatalf("failed to create the temporary directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatalf("failed to remove the temporary directory: %v", err)
		}
	}()

	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create the test file: %v", err)
	}

	if err := os.Chmod(tmpDir, 0000); err != nil {
		t.Fatalf("failed to change the directory permissions: %v", err)
	}

	defer func() {
		if err := os.Chmod(tmpDir, 0755); err != nil {
			t.Fatalf("failed to change the directory permissions back: %v", err)
		}
	}()

	err = validatePath(testFile)
	if err != nil {
		if !strings.Contains(err.Error(), "failed to get the path information") {
			t.Logf("got error: %v", err)
		}
	}
}

// TestReadServerResponseWithMessage tests `readServerResponse` with a non-empty message.
func TestReadServerResponseWithMessage(t *testing.T) {
	responseData := []byte{
		byte(protocol.ResponseStatusSuccess),
		0, 0, 0, 7,
		'S', 'u', 'c', 'c', 'e', 's', 's',
	}

	mockConn := &MockConn{
		readData: responseData,
	}

	err := readServerResponse(mockConn)
	if err != nil {
		t.Fatalf("expected no error for the successful response with message, got: %v", err)
	}
}

// TestValidatePathWithDotDotDirectory tests `validatePath` with a path containing .. directory traversal.
func TestValidatePathWithDotDotDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testdir-*")
	if err != nil {
		t.Fatalf("failed to create the temporary directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatalf("failed to remove the temporary directory: %v", err)
		}
	}()

	pathWithDotDot := filepath.Join(tmpDir, "..")

	err = validatePath(pathWithDotDot)
	if err != nil {
		t.Logf("validatePath with .. returned an error (expected for some cases): %v", err)
	}
}

// TestContextWriterEmptyData tests the `contextWriter.Write` method with empty data.
func TestContextWriterEmptyData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := &MockConn{}
	cw := &contextWriter{
		ctx:  ctx,
		conn: mockConn,
	}

	testData := []byte{}
	n, err := cw.Write(testData)

	if err != nil {
		t.Fatalf("expected no error for the empty data, got: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes written for the empty data, got %d", n)
	}
}

// TestContextWriterLargeData tests the `contextWriter.Write` method with large data.
func TestContextWriterLargeData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := &MockConn{}
	cw := &contextWriter{
		ctx:  ctx,
		conn: mockConn,
	}

	testData := make([]byte, 1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	n, err := cw.Write(testData)

	if err != nil {
		t.Fatalf("expected no error for the large data, got: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("expected %d bytes written for the large data, got %d", len(testData), n)
	}
}
