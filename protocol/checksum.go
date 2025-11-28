package protocol

import (
	"crypto/sha256"
	"fmt"
	"io"
)

// CalculateFileChecksum calculates the SHA256 checksum of a file and returns it as a byte slice.
func CalculateFileChecksum(file io.Reader) ([]byte, error) {
	if file == nil {
		return nil, fmt.Errorf("file reader is nil")
	}

	hash := sha256.New()

	_, err := io.Copy(hash, file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file for checksum calculation: %w", err)
	}

	checksum := hash.Sum(nil)
	return checksum, nil
}

// CalculateDataChecksum calculates the SHA-256 checksum of data and returns it as a byte slice.
func CalculateDataChecksum(data []byte) []byte {
	hash := sha256.New()
	hash.Write(data)
	return hash.Sum(nil)
}

// VerifyDataChecksum verifies the SHA-256 checksum of data.
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

// compareChecksums compares two checksums.
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
