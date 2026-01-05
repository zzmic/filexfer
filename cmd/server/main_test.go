package main

import (
	"bytes"
	"context"
	"filexfer/protocol"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSetupLogging tests the `setupLogging` function to ensure that
// it configures structured logging.
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

// TestToGB tests the `toGB` function to ensure that
// it handles bytes to gigabytes conversion.
func TestToGB(t *testing.T) {
	tests := []struct {
		name     string
		bytes    uint64
		expected float64
	}{
		{
			name:     "zero bytes",
			bytes:    0,
			expected: 0.0,
		},
		{
			name:     "1 GB",
			bytes:    1024 * 1024 * 1024,
			expected: 1.0,
		},
		{
			name:     "5 GB",
			bytes:    5 * 1024 * 1024 * 1024,
			expected: 5.0,
		},
		{
			name:     "1 MB",
			bytes:    1024 * 1024,
			expected: 0.0009765625,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toGB(tt.bytes)
			const epsilon = 1e-7
			if math.Abs(got-tt.expected) > epsilon {
				t.Fatalf("`toGB(...)` = %f, expected %f", got, tt.expected)
			}
		})
	}
}

// TestSanitizePathEmptyPath tests the `sanitizePath` function to ensure that
// it handles an empty user path.
func TestSanitizePathEmptyPath(t *testing.T) {
	base := t.TempDir()
	userPath := ""

	_, err := sanitizePath(base, userPath)
	if err == nil {
		t.Fatalf("expected an error for empty user path")
	}
}

// TestSanitizePathBasicPath tests the `sanitizePath` function to ensure that
// it handles basic (non-nested) file and directory paths.
func TestSanitizePathBasicPath(t *testing.T) {
	base := t.TempDir()
	userPath := "file.txt"
	expectedPath := filepath.Join(base, "file.txt")

	got, err := sanitizePath(base, userPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expectedPath {
		t.Fatalf("expected %q, got %q", expectedPath, got)
	}

	userPath = "dir"
	expectedPath = filepath.Join(base, "dir")

	got, err = sanitizePath(base, userPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expectedPath {
		t.Fatalf("expected %q, got %q", expectedPath, got)
	}
}

// TestSanitizePathNestedPath tests the `sanitizePath` function to ensure that
// it handles nested file and directory paths.
func TestSanitizePathNestedPath(t *testing.T) {
	base := t.TempDir()
	userPath := "dir/subdir/file.txt"
	expectedPath := filepath.Join(base, "dir", "subdir", "file.txt")

	got, err := sanitizePath(base, userPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expectedPath {
		t.Fatalf("expected %q, got %q", expectedPath, got)
	}
}

// TestSanitizePathDotSegments tests the `sanitizePath` function to ensure that
// it handles paths with dot segments.
func TestSanitizePathDotSegments(t *testing.T) {
	base := t.TempDir()
	userPath := "dir/./subdir/./file.txt"
	expectedPath := filepath.Join(base, "dir", "subdir", "file.txt")

	got, err := sanitizePath(base, userPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expectedPath {
		t.Fatalf("expected %q, got %q", expectedPath, got)
	}
}

// TestSanitizePathAbsolutePath tests the `sanitizePath` function to ensure that
// it rejects absolute paths.
func TestSanitizePathAbsolutePath(t *testing.T) {
	base := t.TempDir()
	userPath := string(filepath.Separator) + "etc" + string(filepath.Separator) + "passwd"

	_, err := sanitizePath(base, userPath)
	if err == nil {
		t.Fatal("expected an error for absolute path")
	}

}

// TestSanitizePathUnixStylePathTraversal tests the `sanitizePath` function to ensure that
// it rejects unix-style path traversal attempts.
func TestSanitizePathUnixStylePathTraversal(t *testing.T) {
	base := t.TempDir()
	userPath := "dir/../secret.txt"

	_, err := sanitizePath(base, userPath)
	if err == nil {
		t.Fatal("expected an error for unix style path traversal")
	}
}

// TestSanitizePathBackslashPathTraversal tests the `sanitizePath` function to ensure that
// it rejects backslash path traversal attempts.
func TestSanitizePathBackslashPathTraversal(t *testing.T) {
	base := t.TempDir()
	userPath := "dir\\..\\secret.txt"

	_, err := sanitizePath(base, userPath)
	if err == nil {
		t.Fatal("expected an error for backslash path traversal")
	}
}

// TestSanitizePathMixedPathTraversal tests the `sanitizePath` function to ensure that
// it rejects mixed path traversal attempts.
func TestSanitizePathMixedPathTraversal(t *testing.T) {
	base := t.TempDir()
	userPath := "dir/..\\secret.txt"

	_, err := sanitizePath(base, userPath)
	if err == nil {
		t.Fatal("expected an error for mixed path traversal")
	}
}

// TestValidateHeaderNilHeader tests the `validateHeader` function to ensure that
// it handles a nil header.
func TestValidateHeaderNilHeader(t *testing.T) {
	err := validateHeader(nil, "127.0.0.1:12345")
	if err == nil {
		t.Fatal("expected an error for the nil header")
	}
}

// TestValidateHeaderFileSizeExceeded tests the `validateHeader` function to ensure that
// it handles a header with file size exceeding the maximum allowed.
func TestValidateHeaderFileSizeExceeded(t *testing.T) {
	header := &protocol.Header{
		TransferType: protocol.TransferTypeFile,
		MessageType:  protocol.MessageTypeTransfer,
		FileSize:     uint64(MaxFileSize) + 1,
		FileName:     "large.txt",
		Checksum:     make([]byte, 32),
	}

	err := validateHeader(header, "127.0.0.1:12345")
	if err == nil {
		t.Fatal("expected an error for the exceeded file size")
	}
}

// TestValidateHeaderEmptyFileName tests the `validateHeader` function to ensure that
// it handles a header with an empty file name.
func TestValidateHeaderEmptyFileName(t *testing.T) {
	header := &protocol.Header{
		TransferType: protocol.TransferTypeFile,
		MessageType:  protocol.MessageTypeTransfer,
		FileSize:     1024,
		FileName:     "",
		Checksum:     make([]byte, 32),
	}

	err := validateHeader(header, "127.0.0.1:12345")
	if err == nil {
		t.Fatal("expected an error for the empty file name")
	}
}

// TestValidateHeaderSanitizeFailure tests the `validateHeader` function to ensure that
// it handles a header with a file name that fails sanitization.
func TestValidateHeaderSanitizeFailure(t *testing.T) {
	header := &protocol.Header{
		TransferType: protocol.TransferTypeFile,
		MessageType:  protocol.MessageTypeTransfer,
		FileSize:     1024,
		FileName:     "../secret.txt",
		Checksum:     make([]byte, 32),
	}

	err := validateHeader(header, "127.0.0.1:12345")
	if err == nil {
		t.Fatal("expected an error for failing to sanitize the file name")
	}
}

// TestValidateHeaderDirectorySizeValidation tests the `validateHeader` function to ensure that
// it handles a directory header with size exceeding the maximum allowed.
func TestValidateHeaderDirectorySizeValidation(t *testing.T) {
	oldMaxDirSize := *maxDirectorySize
	defer func() {
		*maxDirectorySize = oldMaxDirSize
	}()
	*maxDirectorySize = 100 * 1024 * 1024

	header := &protocol.Header{
		TransferType: protocol.TransferTypeDirectory,
		MessageType:  protocol.MessageTypeValidate,
		FileSize:     200 * 1024 * 1024,
		FileName:     "",
		Checksum:     make([]byte, 32),
	}

	err := validateHeader(header, "127.0.0.1:12345")
	if err == nil {
		t.Fatal("expected an error for directory size exceeded")
	}

	header = &protocol.Header{
		TransferType: protocol.TransferTypeDirectory,
		MessageType:  protocol.MessageTypeValidate,
		FileSize:     50 * 1024 * 1024,
		FileName:     "",
		Checksum:     make([]byte, 32),
	}

	err = validateHeader(header, "127.0.0.1:12345")
	if err != nil {
		t.Fatalf("unexpected an error for valid directory size: %v", err)
	}
}

// TestValidateHeaderDirectorySizeExceededOnTransfer tests the `validateHeader` function to ensure that
// it rejects a directory transfer if the cumulative size would exceed the limit.
func TestValidateHeaderDirectorySizeExceededOnTransfer(t *testing.T) {
	oldMaxDirSize := *maxDirectorySize
	defer func() {
		*maxDirectorySize = oldMaxDirSize
	}()
	*maxDirectorySize = 1000

	clientAddr := "127.0.0.1:12345"
	dirSizeMutex.Lock()
	directorySizes = make(map[string]uint64)
	directorySizes[clientAddr] = 600
	dirSizeMutex.Unlock()
	defer func() {
		dirSizeMutex.Lock()
		delete(directorySizes, clientAddr)
		dirSizeMutex.Unlock()
	}()

	header := &protocol.Header{
		TransferType: protocol.TransferTypeDirectory,
		MessageType:  protocol.MessageTypeTransfer,
		FileSize:     500, // 600 + 500 = 1100, which exceeds 1000.
		FileName:     "file.txt",
		Checksum:     make([]byte, 32),
	}

	err := validateHeader(header, clientAddr)
	if err == nil {
		t.Fatal("expected an error for the exceeded directory size on transfer")
	}
	if !strings.Contains(err.Error(), "would exceed the maximum allowed size") {
		t.Fatalf("expected 'would exceed' error, got: %v", err)
	}
}

// TestValidateHeaderDirectorySizeAcceptedOnTransfer tests the `validateHeader` function to ensure that
// it accepts a directory transfer if the cumulative size is within the limit.
func TestValidateHeaderDirectorySizeAcceptedOnTransfer(t *testing.T) {
	oldMaxDirSize := *maxDirectorySize
	defer func() {
		*maxDirectorySize = oldMaxDirSize
	}()
	*maxDirectorySize = 1000

	clientAddr := "127.0.0.1:12345"
	dirSizeMutex.Lock()
	directorySizes = make(map[string]uint64)
	directorySizes[clientAddr] = 600
	dirSizeMutex.Unlock()
	defer func() {
		dirSizeMutex.Lock()
		delete(directorySizes, clientAddr)
		dirSizeMutex.Unlock()
	}()

	header := &protocol.Header{
		TransferType: protocol.TransferTypeDirectory,
		MessageType:  protocol.MessageTypeTransfer,
		FileSize:     300, // 600 + 300 = 900, which is within 1000.
		FileName:     "file.txt",
		Checksum:     make([]byte, 32),
	}

	err := validateHeader(header, clientAddr)
	if err != nil {
		t.Fatalf("unexpected error for a valid directory size on transfer: %v", err)
	}
}

// TestValidateHeaderValidFile tests the `validateHeader` function to ensure that
// it validates a correct file header.
func TestValidateHeaderValidFile(t *testing.T) {
	base := t.TempDir()
	oldDestDir := *destDir
	defer func() {
		*destDir = oldDestDir
	}()
	*destDir = base

	header := &protocol.Header{
		TransferType: protocol.TransferTypeFile,
		MessageType:  protocol.MessageTypeTransfer,
		FileSize:     1024,
		FileName:     "test.txt",
		Checksum:     make([]byte, 32),
	}

	err := validateHeader(header, "127.0.0.1:12345")
	if err != nil {
		t.Fatalf("unexpected an error for valid header: %v", err)
	}
}

// TestGetDirectoryStatsNonEmpty tests the `getDirectoryStats` function to ensure that
// it calculates the number of clients and total directory size.
func TestGetDirectoryStatsNonEmpty(t *testing.T) {
	dirSizeMutex.Lock()
	directorySizes = make(map[string]uint64)
	directorySizes["client1"] = 100
	directorySizes["client2"] = 200
	directorySizes["client3"] = 300
	dirSizeMutex.Unlock()

	numClients, totalSize := getDirectoryStats()

	if numClients != 3 {
		t.Fatalf("expected 3 clients, got %d", numClients)
	}
	if totalSize != 600 {
		t.Fatalf("expected total size 600, got %d", totalSize)
	}
}

// TestGetDirectoryStatsEmpty tests the `getDirectoryStats` function to ensure that
// it handles an empty `directorySizes` map.
func TestGetDirectoryStatsEmpty(t *testing.T) {
	dirSizeMutex.Lock()
	directorySizes = make(map[string]uint64)
	dirSizeMutex.Unlock()

	numClients, totalSize := getDirectoryStats()

	if numClients != 0 {
		t.Fatalf("expected 0 clients in total, got %d", numClients)
	}
	if totalSize != 0 {
		t.Fatalf("expected the total size 0, got %d", totalSize)
	}
}

// TestSendErrorResponseWriteFailure tests the `sendErrorResponse` function to ensure that
// it logs an error when writing the response fails.
func TestSendErrorResponseWriteFailure(t *testing.T) {
	conn1, conn2 := net.Pipe()

	// Intentionally close the connections to cause a write failure.
	if err := conn1.Close(); err != nil {
		t.Fatalf("failed to close conn1: %v", err)
	}
	if err := conn2.Close(); err != nil {
		t.Fatalf("failed to close conn2: %v", err)
	}

	var logBuf bytes.Buffer
	oldOutput := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()

	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldOutput)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}()

	sendErrorResponse(conn1, "test error")

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "Failed to send an error response to the client") {
		t.Fatalf("expected the log to contain 'Failed to send an error response to the client', got: %q", logOutput)
	}
}

// TestSendSuccessResponseWriteFailure tests the `sendSuccessResponse` function to ensure that
// it logs an error when writing the response fails.
func TestSendSuccessResponseWriteFailure(t *testing.T) {
	conn1, conn2 := net.Pipe()

	// Intentionally close the connections to cause a write failure.
	if err := conn1.Close(); err != nil {
		t.Fatalf("failed to close `conn1`: %v", err)
	}
	if err := conn2.Close(); err != nil {
		t.Fatalf("failed to close `conn2`: %v", err)
	}

	var logBuf bytes.Buffer
	oldOutput := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()

	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldOutput)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}()

	sendSuccessResponse(conn1, "test success")

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "Failed to send a success response to the client") {
		t.Fatalf("expected the log to contain 'Failed to send a success response to the client', got: %q", logOutput)
	}
}

// TestResolveFilePathNonExistent tests the `resolveFilePath` function to ensure that
// it handles a non-existent file path.
func TestResolveFilePathNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "newfile.txt")

	got, err := resolveFilePath(filePath, StrategyOverwrite)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filePath {
		t.Fatalf("expected %q, got %q", filePath, got)
	}
}

// TestResolveFilePathOverwrite tests the `resolveFilePath` function to ensure that
// it handles an existing file path with the overwrite strategy.
func TestResolveFilePathOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "existing.txt")

	if err := os.WriteFile(filePath, []byte("old content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	got, err := resolveFilePath(filePath, StrategyOverwrite)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filePath {
		t.Fatalf("expected %q, got %q", filePath, got)
	}

	if _, err := os.Stat(filePath); err == nil {
		t.Fatal("file should have been removed")
	}
}

// TestResolveFilePathRename tests the `resolveFilePath` function to ensure that
// it handles an existing file path with the rename strategy.
func TestResolveFilePathSkip(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "existing.txt")

	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := resolveFilePath(filePath, StrategySkip)
	if err == nil {
		t.Fatal("expected an error for the skip strategy on an existing file")
	}
}

// TestResolveFilePathUnknownStrategy tests the `resolveFilePath` function to ensure that
// it handles an unknown strategy.
func TestResolveFilePathUnknownStrategy(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "existing.txt")

	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := resolveFilePath(filePath, "invalid-strategy")
	if err == nil {
		t.Fatal("expected an error for an unknown strategy")
	}
	if !strings.Contains(err.Error(), "unknown file conflict-resolution strategy") {
		t.Fatalf("expected 'unknown file conflict-resolution strategy' error, got: %v", err)
	}
}

// TestGenerateUniqueFile tests the `generateUniqueFile` function to ensure that
// it generates a unique file name when a conflict exists.
func TestGenerateUniqueFile(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := filepath.Join(tmpDir, "file.txt")

	f, finalPath, err := generateUniqueFile(originalPath, "file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("failed to close file: %v", err)
		}
	}()

	expectedPath := filepath.Join(tmpDir, "file_1.txt")
	if finalPath != expectedPath {
		t.Fatalf("expected %q, got %q", expectedPath, finalPath)
	}

	info, err := os.Stat(finalPath)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if info.IsDir() {
		t.Fatal("expected file, got directory")
	}
}

// TestGenerateUniqueFileWithExisting tests the `generateUniqueFile` function to ensure that
// it generates a unique file name when the original file already exists.
func TestGenerateUniqueFileWithExisting(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := filepath.Join(tmpDir, "file.txt")

	if err := os.WriteFile(originalPath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	f, finalPath, err := generateUniqueFile(originalPath, "file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("failed to close file: %v", err)
		}
	}()

	expectedPath := filepath.Join(tmpDir, "file_1.txt")
	if finalPath != expectedPath {
		t.Fatalf("expected %q, got %q", expectedPath, finalPath)
	}

	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("renamed file not created: %v", err)
	}
}

// TestGenerateUniqueFileMultipleConflicts tests the `generateUniqueFile` function to ensure that
// it generates a unique file name when the original file already exists.
func TestGenerateUniqueFileMultipleConflicts(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := filepath.Join(tmpDir, "file.txt")

	if err := os.WriteFile(originalPath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file_1.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	f, finalPath, err := generateUniqueFile(originalPath, "file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("failed to close file: %v", err)
		}
	}()

	expectedPath := filepath.Join(tmpDir, "file_2.txt")
	if finalPath != expectedPath {
		t.Fatalf("expected %q, got %q", expectedPath, finalPath)
	}
}

// TestReadContextCanceled tests the `Read` method of the `contextReader` to ensure that
// it respects context cancellation.
func TestReadContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Intentionally cancel the context immediately.
	cancel()

	conn1, conn2 := net.Pipe()
	defer func() {
		if err := conn1.Close(); err != nil {
			t.Fatalf("failed to close conn1: %v", err)
		}
		if err := conn2.Close(); err != nil {
			t.Fatalf("failed to close conn2: %v", err)
		}
	}()

	cr := &contextReader{
		ctx:  ctx,
		conn: conn1,
	}

	buf := make([]byte, 10)
	n, err := cr.Read(buf)

	if n != 0 {
		t.Fatalf("expected 0 bytes read, got %d", n)
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled error, got %v", err)
	}
}

// TestReadSuccess tests the `Read` method of the `contextReader` to ensure that
// it reads data from the connection.
func TestReadSuccess(t *testing.T) {
	ctx := context.Background()

	conn1, conn2 := net.Pipe()
	defer func() {
		if err := conn1.Close(); err != nil {
			t.Fatalf("failed to close conn1: %v", err)
		}
		if err := conn2.Close(); err != nil {
			t.Fatalf("failed to close conn2: %v", err)
		}
	}()

	cr := &contextReader{
		ctx:  ctx,
		conn: conn1,
	}

	testData := []byte("testdata")
	errChan := make(chan error, 1)
	go func() {
		_, err := conn2.Write(testData)
		errChan <- err
	}()

	buf := make([]byte, 10)
	n, err := cr.Read(buf)

	if writeErr := <-errChan; writeErr != nil {
		t.Fatalf("failed to write to conn2: %v", writeErr)
	}

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("expected %d bytes read, got %d", len(testData), n)
	}
	if !bytes.Equal(buf[:n], testData) {
		t.Fatalf("expected %q, got %q", testData, buf[:n])
	}
}

// TestReadContextDeadlineExceeded tests the `Read` method of the `contextReader` to ensure that
// it respects context deadlines.
func TestReadContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	conn1, conn2 := net.Pipe()
	defer func() {
		if err := conn1.Close(); err != nil {
			t.Fatalf("failed to close conn1: %v", err)
		}
		if err := conn2.Close(); err != nil {
			t.Fatalf("failed to close conn2: %v", err)
		}
	}()

	cr := &contextReader{
		ctx:  ctx,
		conn: conn1,
	}

	// Intentionally do not write any data to `conn2`.
	buf := make([]byte, 10)
	// Wait for the context deadline to exceed.
	time.Sleep(40 * time.Millisecond)

	n, err := cr.Read(buf)

	if n != 0 {
		t.Fatalf("expected 0 bytes read, got %d", n)
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded error, got %v", err)
	}
}

// TestReadSetReadDeadlineFailure tests the `Read` method of the `contextReader` to ensure that
// it handles `SetReadDeadline` failures.
func TestReadSetReadDeadlineFailure(t *testing.T) {
	ctx := context.Background()

	conn1, conn2 := net.Pipe()
	defer func() {
		if err := conn1.Close(); err != nil {
			t.Fatalf("failed to close conn1: %v", err)
		}
		if err := conn2.Close(); err != nil {
			t.Fatalf("failed to close conn2: %v", err)
		}
	}()

	cr := &contextReader{
		ctx:  ctx,
		conn: conn1,
	}

	// Intentionally close the connection to cause `SetReadDeadline` to fail.
	if err := conn1.Close(); err != nil {
		t.Fatalf("failed to close conn1: %v", err)
	}

	buf := make([]byte, 10)
	// Since the connection is closed, `SetReadDeadline` should fail.
	n, err := cr.Read(buf)

	if n != 0 {
		t.Fatalf("expected 0 bytes read, got %d", n)
	}
	if err == nil {
		t.Fatal("expected an error when SetReadDeadline fails on closed connection")
	}
}
