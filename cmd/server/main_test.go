package main

import (
	"filexfer/protocol"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestToGB tests the `toGB` function to ensure it appropriately converts bytes to gigabytes.
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
				t.Fatalf("toGB(...) = %f, expected %f", got, tt.expected)
			}
		})
	}
}

// TestPathSanitization tests the `sanitizePath` function to ensure it appropriately handles various path inputs, including attempts at, perhaps malicious or erroneous, directory traversal.
func TestPathSanitization(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name         string
		userPath     string
		expectedPath string
		errExpected  bool
	}{
		{
			name:         "basic file",
			userPath:     "file.txt",
			expectedPath: filepath.Join(base, "file.txt"),
		},
		{
			name:         "nested path",
			userPath:     "dir/sub/file.txt",
			expectedPath: filepath.Join(base, "dir", "sub", "file.txt"),
		},
		{
			name:         "dot segment",
			userPath:     "dir/./file.txt",
			expectedPath: filepath.Join(base, "dir", "file.txt"),
		},
		{
			name:        "empty path",
			userPath:    "",
			errExpected: true,
		},
		{
			name:        "absolute path",
			userPath:    filepath.Join(string(filepath.Separator), "etc", "passwd"),
			errExpected: true,
		},
		{
			name:        "parent traversal unix style",
			userPath:    "../etc/passwd",
			errExpected: true,
		},
		{
			name:        "parent traversal mixed separators",
			userPath:    "dir/../secret.txt",
			errExpected: true,
		},
		{
			name:        "parent traversal backslash",
			userPath:    "dir\\..\\secret.txt",
			errExpected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizePath(base, tt.userPath)

			if tt.errExpected {
				if err == nil {
					t.Fatalf("sanitizePath(...) error = nil, expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("sanitizePath(...) unexpected error = %v", err)
			}

			if got != tt.expectedPath {
				t.Fatalf("sanitizePath(...) = %q, expected %q", got, tt.expectedPath)
			}
		})
	}
}

// TestValidateHeaderNilHeader tests the `validateHeader` function to ensure it appropriately handles a nil header.
func TestValidateHeaderNilHeader(t *testing.T) {
	err := validateHeader(nil, "127.0.0.1:12345")
	if err == nil {
		t.Fatal("expected error for nil header")
	}
}

// TestValidateHeaderFileSizeExceeded tests the `validateHeader` function to ensure it appropriately handles a header with file size exceeding the maximum allowed.
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
		t.Fatal("expected error for file size exceeded")
	}
}

// TestValidateHeaderEmptyFileName tests the `validateHeader` function to ensure it appropriately handles a header with an empty file name.
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
		t.Fatal("expected error for empty file name")
	}
}

// TestValidateHeaderDirectorySizeValidation tests the `validateHeader` function to ensure it appropriately handles a directory header with size exceeding the maximum allowed.
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
}

// TestValidateHeaderValidFile tests the `validateHeader` function to ensure it appropriately validates a correct file header.
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

// TestGetDirectoryStatsNonEmpty tests the `getDirectoryStats` function to ensure it correctly calculates the number of clients and total directory size.
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

// TestGetDirectoryStatsEmpty tests the `getDirectoryStats` function to ensure it correctly handles an empty `directorySizes` map.
func TestGetDirectoryStatsEmpty(t *testing.T) {
	dirSizeMutex.Lock()
	directorySizes = make(map[string]uint64)
	dirSizeMutex.Unlock()

	numClients, totalSize := getDirectoryStats()

	if numClients != 0 {
		t.Fatalf("expected 0 clients, got %d", numClients)
	}
	if totalSize != 0 {
		t.Fatalf("expected total size 0, got %d", totalSize)
	}
}

// TestResolveFilePathNonExistent tests the `resolveFilePath` function to ensure it correctly handles a non-existent file path.
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

// TestResolveFilePathOverwrite tests the `resolveFilePath` function to ensure it correctly handles an existing file path with the overwrite strategy.
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

// TestResolveFilePathRename tests the `resolveFilePath` function to ensure it correctly handles an existing file path with the rename strategy.
func TestResolveFilePathSkip(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "existing.txt")

	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := resolveFilePath(filePath, StrategySkip)
	if err == nil {
		t.Fatal("expected error for skip strategy on existing file")
	}
}

// TestGenerateUniqueFile tests the `generateUniqueFile` function to ensure it generates a unique file name when a conflict exists.
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

// TestGenerateUniqueFileWithExisting tests the `generateUniqueFile` function to ensure it generates a unique file name when the original file already exists.
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

// TestGenerateUniqueFileMultipleConflicts tests the `generateUniqueFile` function to ensure it generates a unique file name when the original file already exists.
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
