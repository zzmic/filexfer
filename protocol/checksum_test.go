package protocol

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// FailingReader is a test helper that simulates read failures.
type FailingReader struct {
	failAfter int // Number of bytes after which to fail.
	read      int // Number of bytes read so far.
}

// FailingRead implements the `io.Reader` interface and fails after reading `failAfter` bytes.
func (fr *FailingReader) Read(p []byte) (int, error) {
	if fr.read >= fr.failAfter {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, []byte("test data")[:fr.failAfter-fr.read])
	fr.read += n
	return n, nil
}

// TestCalculateFileChecksumNilFile tests `CalculateFileChecksum` to ensure that
// it expectedly handles a nil file reader.
func TestCalculateFileChecksumNilFile(t *testing.T) {
	got, err := CalculateFileChecksum(nil)

	if err == nil {
		t.Errorf("expected error for the nil file reader, got nil")
	}
	if got != nil {
		t.Errorf("expected nil checksum for the nil file reader, got %x", got)
	}
}

// TestCalculateFileChecksumSuccess tests `CalculateFileChecksum` to ensure that
// it expectedly calculates the checksum of file data.
func TestCalculateFileChecksumSuccess(t *testing.T) {
	testData := []byte("test data")

	fileReader := bytes.NewReader(testData)
	got, err := CalculateFileChecksum(fileReader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil checksum")
	}
	if len(got) != 32 {
		t.Fatalf("expected checksum length 32, got %d", len(got))
	}

	fileReader2 := bytes.NewReader(testData)
	got2, err := CalculateFileChecksum(fileReader2)
	if err != nil {
		t.Fatalf("unexpected error on the second calculation: %v", err)
	}
	if !bytes.Equal(got, got2) {
		t.Fatalf("checksums differ for the same data: %x v.s. %x", got, got2)
	}
}

// TestCalculateFileChecksumReadError tests `CalculateFileChecksum` to ensure that
// it expectedly handles read errors during checksum calculation.
func TestCalculateFileChecksumReadError(t *testing.T) {
	failingReader := &FailingReader{failAfter: 5}

	got, err := CalculateFileChecksum(failingReader)

	if err == nil {
		t.Errorf("expected error for the failing reader, got nil")
	}
	if got != nil {
		t.Errorf("expected nil checksum on an error, got %x", got)
	}
	if !strings.Contains(err.Error(), "failed to read file for checksum calculation") {
		t.Fatalf("expected 'failed to read file' error message, got: %v", err)
	}
}

// TestCalculateDataChecksumEmpty tests `CalculateDataChecksum` to ensure that
// it expectedly handles empty data.
func TestCalculateDataChecksumEmpty(t *testing.T) {
	got := CalculateDataChecksum([]byte{})

	if got == nil {
		t.Fatal("expected a non-nil checksum for empty data")
	}
	if len(got) != 32 {
		t.Fatalf("expected checksum length 32, got %d", len(got))
	}
}

// TestCalculateDataChecksumSuccess tests `CalculateDataChecksum` to ensure that
// it expectedly calculates checksums for data.
func TestCalculateDataChecksumSuccess(t *testing.T) {
	testData := []byte("test data")

	got := CalculateDataChecksum(testData)
	if got == nil {
		t.Fatal("expected a non-nil checksum")
	}
	if len(got) != 32 {
		t.Fatalf("expected checksum length 32, got %d", len(got))
	}

	got2 := CalculateDataChecksum(testData)
	if !bytes.Equal(got, got2) {
		t.Fatalf("checksums differ for same data: %x vs %x", got, got2)
	}

	got3 := CalculateDataChecksum([]byte("different data"))
	if bytes.Equal(got, got3) {
		t.Fatal("different data should produce different checksums")
	}
}

// TestCompareChecksums tests the `compareChecksums` function.
func TestCompareChecksums(t *testing.T) {
	a := []byte{1, 2, 3, 4, 5}
	b := []byte{1, 2, 3, 4, 5}
	c := []byte{1, 2, 3, 4, 6}
	d := []byte{1, 2, 3, 4}

	if compareChecksums(a, d) {
		t.Error("expected checksums a and d to be different due to length")
	}
	if !compareChecksums(a, b) {
		t.Error("expected checksums a and b to be equal")
	}
	if compareChecksums(a, c) {
		t.Error("expected checksums a and c to be different")
	}
}

// TestVerifyDataChecksumNilExpected tests `VerifyDataChecksum` to ensure that
// it expectedly handles a nil expected checksum.
func TestVerifyDataChecksumNilExpected(t *testing.T) {
	err := VerifyDataChecksum([]byte("test data"), nil)

	if err == nil {
		t.Error("expected error for a nil expected checksum")
	}
	if !strings.Contains(err.Error(), "expected checksum is nil") {
		t.Fatalf("expected 'expected checksum is nil' error, got: %v", err)
	}
}

// TestVerifyDataChecksumMatching tests `VerifyDataChecksum` to ensure that
// it expectedly succeeds when checksums match.
func TestVerifyDataChecksumMatching(t *testing.T) {
	testData := []byte("test data")
	expectedChecksum := CalculateDataChecksum(testData)

	err := VerifyDataChecksum(testData, expectedChecksum)

	if err != nil {
		t.Fatalf("unexpected error for matching checksums: %v", err)
	}
}

// TestVerifyDataChecksumMismatch tests `VerifyDataChecksum` to ensure that
// it expectedly fails when checksums don't match.
func TestVerifyDataChecksumMismatch(t *testing.T) {
	testData := []byte("test data")
	wrongChecksum := CalculateDataChecksum([]byte("different data"))

	err := VerifyDataChecksum(testData, wrongChecksum)

	if err == nil {
		t.Error("expected error for mismatched checksums")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected 'checksum mismatch' error, got: %v", err)
	}
}
