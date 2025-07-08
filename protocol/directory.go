package protocol

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Custom error types for directory transfer.
var (
	ErrDirectoryNotFound   = errors.New("directory not found")
	ErrDirectoryEmpty      = errors.New("directory is empty")
	ErrDirectoryTooLarge   = errors.New("directory size exceeds maximum allowed size")
	ErrFileOperation       = errors.New("file operation failed")
	ErrChecksumCalculation = errors.New("checksum calculation failed")
)

// Constants for directory transfer.
const (
	MaxDirectorySize = 1 * 1024 * 1024 * 1024 // 1GB limit for directory transfers.
)

// Struct to represent a file within a directory transfer.
type FileInfo struct {
	Path     string      // Path of the file.
	Size     int64       // Size of the file.
	Mode     os.FileMode // Mode of the file.
	IsDir    bool        // Whether the file is a directory.
	Checksum []byte      // SHA256 checksum of the file.
}

// Struct to represent a directory transfer operation.
type DirectoryTransfer struct {
	RootPath  string     // Root path of the directory.
	Files     []FileInfo // Files in the directory.
	TotalSize int64      // Total size of the directory.
}

// Function to create a new directory transfer from the given path.
func NewDirectoryTransfer(rootPath string) (*DirectoryTransfer, error) {
	if rootPath == "" {
		return nil, fmt.Errorf("%w: root path cannot be empty", ErrDirectoryNotFound)
	}

	// Check if the path exists and is a directory.
	fileInfo, err := os.Stat(rootPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: path does not exist: %s", ErrDirectoryNotFound, rootPath)
		}
		return nil, fmt.Errorf("%w: failed to access path %s: %v", ErrDirectoryNotFound, rootPath, err)
	}

	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("%w: path is not a directory: %s", ErrDirectoryNotFound, rootPath)
	}

	dt := &DirectoryTransfer{
		RootPath:  rootPath,     // Root path of the directory.
		Files:     []FileInfo{}, // Files in the directory.
		TotalSize: 0,            // Total size of the directory.
	}

	// Find all files in the directory.
	if err := dt.findAllFilesInDir(); err != nil {
		return nil, fmt.Errorf("%w: failed to scan directory %s: %v", ErrDirectoryEmpty, rootPath, err)
	}

	// Validate the total size of the directory.
	if dt.TotalSize > MaxDirectorySize {
		return nil, fmt.Errorf("%w: directory size %d bytes exceeds maximum allowed size %d bytes",
			ErrDirectoryTooLarge, dt.TotalSize, MaxDirectorySize)
	}

	// Validate the number of files in the directory.
	if len(dt.Files) == 0 {
		return nil, fmt.Errorf("%w: no files found in directory: %s", ErrDirectoryEmpty, rootPath)
	}

	return dt, nil
}

// Function to recursively find all files in the directory.
func (dt *DirectoryTransfer) findAllFilesInDir() error {
	return filepath.Walk(dt.RootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("%w: failed to access file %s: %v", ErrFileOperation, path, err)
		}

		// Skip the root directory itself.
		if path == dt.RootPath {
			return nil
		}

		// Calculate relative path from root.
		relPath, err := filepath.Rel(dt.RootPath, path)
		if err != nil {
			return fmt.Errorf("%w: failed to calculate relative path for %s: %v", ErrFileOperation, path, err)
		}

		// Normalize path separators for cross-platform compatibility.
		relPath = filepath.ToSlash(relPath)

		// Create file info.
		fileInfo := FileInfo{
			Path:  relPath,
			Size:  info.Size(),
			Mode:  info.Mode(),
			IsDir: info.IsDir(),
		}

		// Calculate checksum for regular files.
		if !info.IsDir() && info.Size() > 0 {
			checksum, err := CalculateFileChecksumFromPath(path)
			if err != nil {
				return fmt.Errorf("%w: failed to calculate checksum for file %s: %v", ErrChecksumCalculation, path, err)
			}
			fileInfo.Checksum = checksum
			dt.TotalSize += info.Size()
		}

		// Add file info to the directory transfer.
		dt.Files = append(dt.Files, fileInfo)
		return nil
	})
}

// Function to calculate the SHA256 checksum of a file.
func CalculateFileChecksumFromPath(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open file %s for checksum calculation: %v", ErrChecksumCalculation, filePath, err)
	}
	defer file.Close()

	return CalculateFileChecksum(file)
}

// Function to return statistics regarding the directory transfer.
func (dt *DirectoryTransfer) GetDirectoryStats() (int, int64) {
	fileCount := 0
	totalSize := int64(0)

	for _, file := range dt.Files {
		if !file.IsDir {
			fileCount++
			totalSize += file.Size
		}
	}

	return fileCount, totalSize
}

// Function to return a string representation of the directory transfer.
func (dt *DirectoryTransfer) String() string {
	fileCount, totalSize := dt.GetDirectoryStats()

	return fmt.Sprintf("Directory transfer: %s (%d files, %d bytes total)",
		dt.RootPath, fileCount, totalSize)
}
