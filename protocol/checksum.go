package protocol

import (
	"crypto/sha256"
	"fmt"
	"io"
)

// Function to calculate SHA256 checksum of a file.
func CalculateFileChecksum(file io.Reader) ([]byte, error) {
	if file == nil {
		return nil, fmt.Errorf("file reader is nil")
	}

	// Create a SHA256 `hash.Hash` object.
	hash := sha256.New()

	// Copy the file content to the `hash.Hash` object.
	_, err := io.Copy(hash, file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file for checksum calculation: %w", err)
	}

	// Get the hash sum as raw bytes.
	checksum := hash.Sum(nil)
	return checksum, nil
}

// Function to calculate SHA256 checksum of data.
func CalculateDataChecksum(data []byte) []byte {
	hash := sha256.New()
	hash.Write(data)
	return hash.Sum(nil)
}

// Function to verify data checksum.
func VerifyDataChecksum(data []byte, expectedChecksum []byte) error {
	if expectedChecksum == nil {
		return fmt.Errorf("expected checksum is nil")
	}

	actualChecksum := CalculateDataChecksum(data)
	if !compareChecksums(actualChecksum, expectedChecksum) {
		return fmt.Errorf("%w: expected %x, got %x", ErrChecksumMismatch, expectedChecksum, actualChecksum)
	}

	return nil
}

// Helper function to compare two checksums.
func compareChecksums(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
