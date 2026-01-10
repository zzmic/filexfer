package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"filexfer/protocol"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

// TestSanitizePathEmptyPath tests the `sanitizePath` function to ensure that
// it expectedly handles an empty user path.
func TestSanitizePathEmptyPath(t *testing.T) {
	base := t.TempDir()
	userPath := ""

	_, err := sanitizePath(base, userPath)
	if err == nil {
		t.Fatalf("expected error for empty user path")
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
// it expectedly handles nested file and directory paths.
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
// it expectedly handles paths with dot segments.
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
// it expectedly rejects absolute paths.
func TestSanitizePathAbsolutePath(t *testing.T) {
	base := t.TempDir()
	userPath := string(filepath.Separator) + "etc" + string(filepath.Separator) + "passwd"

	_, err := sanitizePath(base, userPath)
	if err == nil {
		t.Fatal("expected error for absolute path")
	}

}

// TestSanitizePathUnixStylePathTraversal tests the `sanitizePath` function to ensure that
// it expectedly rejects unix-style path traversal attempts.
func TestSanitizePathUnixStylePathTraversal(t *testing.T) {
	base := t.TempDir()
	userPath := "dir/../secret.txt"

	_, err := sanitizePath(base, userPath)
	if err == nil {
		t.Fatal("expected error for unix style path traversal")
	}
}

// TestSanitizePathBackslashPathTraversal tests the `sanitizePath` function to ensure that
// it expectedly rejects backslash path traversal attempts.
func TestSanitizePathBackslashPathTraversal(t *testing.T) {
	base := t.TempDir()
	userPath := "dir\\..\\secret.txt"

	_, err := sanitizePath(base, userPath)
	if err == nil {
		t.Fatal("expected error for backslash path traversal")
	}
}

// TestSanitizePathMixedPathTraversal tests the `sanitizePath` function to ensure that
// it expectedly rejects mixed path traversal attempts.
func TestSanitizePathMixedPathTraversal(t *testing.T) {
	base := t.TempDir()
	userPath := "dir/..\\secret.txt"

	_, err := sanitizePath(base, userPath)
	if err == nil {
		t.Fatal("expected error for mixed path traversal")
	}
}

// TestValidateHeaderNilHeader tests the `validateHeader` function to ensure that
// it expectedly handles a nil header.
func TestValidateHeaderNilHeader(t *testing.T) {
	err := validateHeader(nil, "127.0.0.1:12345")
	if err == nil {
		t.Fatal("expected error for the nil header")
	}
}

// TestValidateHeaderFileSizeExceeded tests the `validateHeader` function to ensure that
// it expectedly handles a header with file size exceeding the maximum allowed.
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
		t.Fatal("expected error for the exceeded file size")
	}
}

// TestValidateHeaderEmptyFileName tests the `validateHeader` function to ensure that
// it expectedly handles a header with an empty file name.
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
		t.Fatal("expected error for the empty file name")
	}
}

// TestValidateHeaderSanitizeFailure tests the `validateHeader` function to ensure that
// it expectedly handles a header with a file name that fails sanitization.
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
		t.Fatal("expected error for failing to sanitize the file name")
	}
}

// TestValidateHeaderDirectorySizeValidation tests the `validateHeader` function to ensure that
// it expectedly handles a directory header with size exceeding the maximum allowed.
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
		t.Fatal("expected error for directory size exceeded")
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
		t.Fatalf("unexpected error for valid directory size: %v", err)
	}
}

// TestValidateHeaderDirectorySizeExceededOnTransfer tests the `validateHeader` function to ensure that
// it expectedly rejects a directory transfer if the cumulative size would exceed the limit.
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
		t.Fatal("expected error for the exceeded directory size on transfer")
	}
	if !strings.Contains(err.Error(), "would exceed the maximum allowed size") {
		t.Fatalf("expected 'would exceed' error, got: %v", err)
	}
}

// TestValidateHeaderDirectorySizeAcceptedOnTransfer tests the `validateHeader` function to ensure that
// it expectedly accepts a directory transfer if the cumulative size is within the limit.
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
// it expectedly validates a correct file header.
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
		t.Fatalf("unexpected error for valid header: %v", err)
	}
}

// TestGetDirectoryStatsNonEmpty tests the `getDirectoryStats` function to ensure that
// it expectedly calculates the number of clients and total directory size.
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
// it expectedly handles an empty `directorySizes` map.
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
// it expectedly logs an error when writing the response fails.
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
// it expectedly logs an error when writing the response fails.
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
// it expectedly handles a non-existent file path.
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
// it expectedly handles an existing file path with the overwrite strategy.
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
// it expectedly handles an existing file path with the rename strategy.
func TestResolveFilePathSkip(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "existing.txt")

	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := resolveFilePath(filePath, StrategySkip)
	if err == nil {
		t.Fatal("expected error for the skip strategy on an existing file")
	}
}

// TestResolveFilePathUnknownStrategy tests the `resolveFilePath` function to ensure that
// it expectedly handles an unknown strategy.
func TestResolveFilePathUnknownStrategy(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "existing.txt")

	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := resolveFilePath(filePath, "invalid-strategy")
	if err == nil {
		t.Fatal("expected error for an unknown strategy")
	}
	if !strings.Contains(err.Error(), "unknown file conflict-resolution strategy") {
		t.Fatalf("expected 'unknown file conflict-resolution strategy' error, got: %v", err)
	}
}

// TestGenerateUniqueFile tests the `generateUniqueFile` function to ensure that
// it expectedly generates a unique file name when a conflict exists.
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
// it expectedly generates a unique file name when the original file already exists.
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
// it expectedly generates a unique file name when the original file already exists.
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
// it expectedly reads data from the connection.
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

	testData := []byte("test data")
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

// TestReadContextDeadlineExceeded tests the `Read` method of the `contextReader` struct to ensure that
// it expectedly respects context deadlines.
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

// TestReadSetReadDeadlineFailure tests the `Read` method of the `contextReader` struct to ensure that
// it expectedly handles `SetReadDeadline` failures.
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
		t.Fatal("expected error when SetReadDeadline fails on closed connection")
	}
}

// generateTestCert generates a self-signed TLS certificate for testing.
func generateTestCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	tmpDir := t.TempDir()
	certFile = filepath.Join(tmpDir, "test.crt")
	keyFile = filepath.Join(tmpDir, "test.key")

	// Generates a 2048-bit RSA key.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate the private key: %v", err)
	}

	// Create a certificate template.
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
			CommonName:   "localhost",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:    []string{"localhost"},
	}

	// Create a self-signed certificate valid for 365 days.
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create the certificate: %v", err)
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("failed to create the certificate file: %v", err)
	}
	defer func() {
		if err := certOut.Close(); err != nil {
			t.Fatalf("failed to close the certificate file: %v", err)
		}
	}()

	// Encode and write the certificate to a PEM file.
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatalf("failed to write the certificate: %v", err)
	}

	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatalf("failed to create the key file: %v", err)
	}
	defer func() {
		if err := keyOut.Close(); err != nil {
			t.Fatalf("failed to close the key file: %v", err)
		}
	}()

	// Encode and write the private key to a PEM file.
	keyDER := x509.MarshalPKCS1PrivateKey(key)
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatalf("failed to write the key: %v", err)
	}

	return certFile, keyFile
}

// TestLoadTLSConfigNoCertificates tests that `loadTLSConfig` returns nil when no certificates are provided.
func TestLoadTLSConfigNoCertificates(t *testing.T) {
	oldCertFile := *tlsCertFile
	oldKeyFile := *tlsKeyFile
	defer func() {
		*tlsCertFile = oldCertFile
		*tlsKeyFile = oldKeyFile
	}()

	*tlsCertFile = ""
	*tlsKeyFile = ""

	config, err := loadTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config != nil {
		t.Fatal("expected nil config when no certificates are provided")
	}
}

// TestLoadTLSConfigWithValidCertificates tests that `loadTLSConfig` expectedly loads valid certificates.
func TestLoadTLSConfigWithValidCertificates(t *testing.T) {
	oldCertFile := *tlsCertFile
	oldKeyFile := *tlsKeyFile
	defer func() {
		*tlsCertFile = oldCertFile
		*tlsKeyFile = oldKeyFile
	}()

	certFile, keyFile := generateTestCert(t)
	*tlsCertFile = certFile
	*tlsKeyFile = keyFile

	config, err := loadTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config == nil {
		t.Fatal("expected non-nil config when certificates are provided")
	}
	if len(config.Certificates) != 1 {
		t.Fatalf("expected 1 certificate, got %d", len(config.Certificates))
	}
	if config.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum version, got %x", config.MinVersion)
	}
}

// TestLoadTLSConfigWithInvalidCertFile tests that `loadTLSConfig` returns an error for invalid certificate file paths.
func TestLoadTLSConfigWithInvalidCertFile(t *testing.T) {
	oldCertFile := *tlsCertFile
	oldKeyFile := *tlsKeyFile
	defer func() {
		*tlsCertFile = oldCertFile
		*tlsKeyFile = oldKeyFile
	}()

	*tlsCertFile = "/nonexistent/cert.crt"
	*tlsKeyFile = "/nonexistent/key.key"

	config, err := loadTLSConfig()
	if err == nil {
		t.Fatal("expected error for the invalid certificate file")
	}
	if config != nil {
		t.Fatal("expected nil config on error")
	}
	if !strings.Contains(err.Error(), "failed to load the TLS certificate") {
		t.Fatalf("expected 'failed to load the TLS certificate' in error, got: %v", err)
	}
}

// TestLoadTLSConfigWithOnlyCertFile tests that `loadTLSConfig` returns nil when only a cert file is provided.
func TestLoadTLSConfigWithOnlyCertFile(t *testing.T) {
	oldCertFile := *tlsCertFile
	oldKeyFile := *tlsKeyFile
	defer func() {
		*tlsCertFile = oldCertFile
		*tlsKeyFile = oldKeyFile
	}()

	certFile, _ := generateTestCert(t)
	*tlsCertFile = certFile
	*tlsKeyFile = ""

	config, err := loadTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config != nil {
		t.Fatal("expected nil config when only the cert file is provided")
	}
}

// TestLoadTLSConfigWithOnlyKeyFile tests that `loadTLSConfig` returns nil when only a key file is provided.
func TestLoadTLSConfigWithOnlyKeyFile(t *testing.T) {
	oldCertFile := *tlsCertFile
	oldKeyFile := *tlsKeyFile
	defer func() {
		*tlsCertFile = oldCertFile
		*tlsKeyFile = oldKeyFile
	}()

	_, keyFile := generateTestCert(t)
	*tlsCertFile = ""
	*tlsKeyFile = keyFile

	config, err := loadTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config != nil {
		t.Fatal("expected nil config when only the key file is provided")
	}
}
