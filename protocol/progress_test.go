package protocol

import (
	"io"
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

// TestNewProgressTracker tests the `Update` method of the `ProgressTracker` struct to ensure that
// it expectedly initializes with given total bytes and description.
func TestNewProgressTracker(t *testing.T) {
	pt := NewProgressTracker(0, "")
	if pt.totalBytes != 0 {
		t.Errorf("Expected totalBytes to be 0, got %d", pt.totalBytes)
	}
	if pt.description != "" {
		t.Errorf("Expected description to be empty, got %s", pt.description)
	}
}

// TestProgressTrackerUpdate tests the `Update` method of the `ProgressTracker` struct to ensure that
// it expectedly updates the bytes transferred.
func TestProgressTrackerUpdate(t *testing.T) {
	pt := NewProgressTracker(1000, "Test Transfer")
	pt.Update(500)
	if pt.bytesTransferred != 500 {
		t.Errorf("Expected bytesTransferred to be 500, got %d", pt.bytesTransferred)
	}
}

// TestProgressTrackerUpdateMultiple tests the `Update` method with multiple calls to ensure that
// it expectedly accumulates the bytes transferred.
func TestProgressTrackerUpdateMultiple(t *testing.T) {
	pt := NewProgressTracker(1000, "Test Transfer")
	pt.Update(100)
	pt.Update(300)
	pt.Update(800)
	if pt.bytesTransferred != 800 {
		t.Errorf("Expected bytesTransferred to be 800, got %d", pt.bytesTransferred)
	}
}

// TestProgressTrackerComplete tests the `Complete` method to ensure that
// it expectedly sets `bytesTransferred` to the total bytes when marked complete.
func TestProgressTrackerComplete(t *testing.T) {
	pt := NewProgressTracker(1000, "Test Transfer")
	pt.Update(500)
	// Intentionally complete the transfer even though not all bytes transferred.
	pt.Complete()
	if pt.bytesTransferred != 1000 {
		t.Errorf("Expected bytesTransferred to be 1000 after Complete(), got %d", pt.bytesTransferred)
	}
}

// TestCreateProgressBar tests the `createProgressBar` method to ensure that
// it expectedly generates the correct progress bar representation.
func TestCreateProgressBar(t *testing.T) {
	tests := []struct {
		name       string
		percentage float64
		expected   string
	}{
		{"0% progress", 0, "[------------------------------]"},
		{"25% progress", 25, "[=======-----------------------]"},
		{"50% progress", 50, "[===============---------------]"},
		{"75% progress", 75, "[======================--------]"},
		{"100% progress", 100, "[==============================]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := NewProgressTracker(1000, "Test")
			got := pt.createProgressBar(tt.percentage)
			if got != tt.expected {
				t.Errorf("createProgressBar(%.1f) = %q; expected %q", tt.percentage, got, tt.expected)
			}
		})
	}
}

// TestCreateProgressBarEdgeCases tests the `createProgressBar` method with edge-case percentages.
func TestCreateProgressBarEdgeCases(t *testing.T) {
	pt := NewProgressTracker(1000, "Test")

	// Test a very low percentage just above 0%.
	bar := pt.createProgressBar(0.1)
	if bar != "[------------------------------]" {
		t.Errorf("Expected an empty bar for 0.1%%, got %q", bar)
	}

	// Test a very high percentage just below 100%.
	bar = pt.createProgressBar(99.9)
	if bar != "[=============================-]" {
		t.Errorf("Expected a nearly full bar for 99.9%%, got %q", bar)
	}
}

// TestNewProgressReader tests the `NewProgressReader` constructor to ensure that
// it expectedly initializes the reader and the progress tracker.
func TestNewProgressReader(t *testing.T) {
	reader := strings.NewReader("test data")
	pr := NewProgressReader(reader, 100, "Download")

	if pr.reader == nil {
		t.Errorf("Expected the reader to be initialized")
	}
	if pr.tracker == nil {
		t.Errorf("Expected the tracker to be initialized")
	}
	if pr.tracker.totalBytes != 100 {
		t.Errorf("Expected totalBytes to be 100, got %d", pr.tracker.totalBytes)
	}
	if pr.tracker.description != "Download" {
		t.Errorf("Expected description to be 'Download', got %s", pr.tracker.description)
	}
}

// TestProgressReaderRead tests the `Read` method of `ProgressReader` to ensure that
// it expectedly reads from the underlying reader and updates progress.
func TestProgressReaderRead(t *testing.T) {
	reader := strings.NewReader("hello world")
	pr := NewProgressReader(reader, 11, "Download")

	buf := make([]byte, 5)
	n, err := pr.Read(buf)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("Expected 5 bytes read, got %d", n)
	}
	if pr.tracker.bytesTransferred != 5 {
		t.Errorf("Expected bytesTransferred to be 5, got %d", pr.tracker.bytesTransferred)
	}
	if string(buf) != "hello" {
		t.Errorf("Expected to read 'hello', got %q", string(buf))
	}
}

// TestProgressReaderReadMultiple tests the `Read` method with multiple reads to ensure that
// it expectedly accumulates the bytes transferred.
func TestProgressReaderReadMultiple(t *testing.T) {
	reader := strings.NewReader("hello world test")
	pr := NewProgressReader(reader, 16, "Download")

	buf := make([]byte, 5)
	n1, _ := pr.Read(buf)
	n2, _ := pr.Read(buf)
	n3, _ := pr.Read(buf)

	totalRead := n1 + n2 + n3
	if pr.tracker.bytesTransferred != uint64(totalRead) {
		t.Errorf("Expected bytesTransferred to be %d, got %d", totalRead, pr.tracker.bytesTransferred)
	}
}

// TestProgressReaderReadEOF tests that the `Read` method of `ProgressReader` to ensure that
// it expectedly returns `io.EOF` when the underlying reader is saturated.
func TestProgressReaderReadEOF(t *testing.T) {
	reader := strings.NewReader("test")
	pr := NewProgressReader(reader, 4, "Download")

	buf := make([]byte, 10)
	n, err := pr.Read(buf)

	// The first read should succeed with 4 bytes and yield no error.
	if err != nil {
		t.Errorf("Expected nil error on first read, got %v", err)
	}
	if n != 4 {
		t.Errorf("Expected 4 bytes read, got %d", n)
	}
	if pr.tracker.bytesTransferred != 4 {
		t.Errorf("Expected bytesTransferred to be 4, got %d", pr.tracker.bytesTransferred)
	}

	// The second read should return `io.EOF` since there's no more data to read.
	n, err = pr.Read(buf)
	if err != io.EOF {
		t.Errorf("Expected io.EOF on second read, got %v", err)
	}
	if n != 0 {
		t.Errorf("Expected 0 bytes read on EOF, got %d", n)
	}
}

// TestNewProgressWriter tests the `NewProgressWriter` constructor to ensure that
// it expectedly initializes the writer and the progress tracker.
func TestNewProgressWriter(t *testing.T) {
	writer := &strings.Builder{}
	pw := NewProgressWriter(writer, 100, "Upload")

	if pw.writer == nil {
		t.Errorf("Expected writer to be initialized")
	}
	if pw.tracker == nil {
		t.Errorf("Expected tracker to be initialized")
	}
	if pw.tracker.totalBytes != 100 {
		t.Errorf("Expected totalBytes to be 100, got %d", pw.tracker.totalBytes)
	}
	if pw.tracker.description != "Upload" {
		t.Errorf("Expected description to be 'Upload', got %s", pw.tracker.description)
	}
}

// TestProgressWriterWrite tests the `Write` method of `ProgressWriter` to ensure that
// it expectedly writes to the underlying writer and updates the progress.
func TestProgressWriterWrite(t *testing.T) {
	writer := &strings.Builder{}
	pw := NewProgressWriter(writer, 11, "Upload")

	n, err := pw.Write([]byte("hello world"))

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if n != 11 {
		t.Errorf("Expected 11 bytes written, got %d", n)
	}
	if pw.tracker.bytesTransferred != 11 {
		t.Errorf("Expected bytesTransferred to be 11, got %d", pw.tracker.bytesTransferred)
	}
	if writer.String() != "hello world" {
		t.Errorf("Expected to write 'hello world', got %q", writer.String())
	}
}

// TestProgressWriterWriteMultiple tests the `Write` method of `ProgressWriter` with multiple writes to ensure that
// it expectedly accumulates the bytes transferred.
func TestProgressWriterWriteMultiple(t *testing.T) {
	writer := &strings.Builder{}
	pw := NewProgressWriter(writer, 15, "Upload")

	if _, err := pw.Write([]byte("hello")); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if _, err := pw.Write([]byte(" ")); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if _, err := pw.Write([]byte("world")); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if pw.tracker.bytesTransferred != 11 {
		t.Errorf("Expected bytesTransferred to be 11, got %d", pw.tracker.bytesTransferred)
	}
	if writer.String() != "hello world" {
		t.Errorf("Expected to write 'hello world', got %q", writer.String())
	}
}

// TestProgressWriterWriteEmpty tests the `Write` method of `ProgressWriter` when writing an empty byte to ensure that
// it expectedly handles zero-length writes.
func TestProgressWriterWriteEmpty(t *testing.T) {
	writer := &strings.Builder{}
	pw := NewProgressWriter(writer, 10, "Upload")

	n, err := pw.Write([]byte{})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("Expected 0 bytes written, got %d", n)
	}
	if pw.tracker.bytesTransferred != 0 {
		t.Errorf("Expected bytesTransferred to be 0, got %d", pw.tracker.bytesTransferred)
	}
}

// TestProgressReaderComplete tests the `Complete` method of `ProgressReader` to ensure that
// it expectedly sets `bytesTransferred` to the total bytes when marked complete.
func TestProgressReaderComplete(t *testing.T) {
	reader := strings.NewReader("hello")
	pr := NewProgressReader(reader, 5, "Download")

	pr.tracker.Update(3)
	pr.Complete()

	if pr.tracker.bytesTransferred != 5 {
		t.Errorf("Expected bytesTransferred to be 5 after Complete(), got %d", pr.tracker.bytesTransferred)
	}
}

// TestProgressTrackerCompleteVariousSizes tests the `Complete` method of `ProgressTracker` with various total byte sizes to ensure that
// it expectedly handles different file sizes covering all output formatting branches.
func TestProgressTrackerCompleteVariousSizes(t *testing.T) {
	tests := []struct {
		name  string
		bytes uint64
	}{
		{"tiny file", 100},
		{"small file", 512},
		{"exactly 1KB", 1024},
		{"medium file", 512 * 1024},
		{"exactly 1MB", 1024 * 1024},
		{"large file", 100 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := NewProgressTracker(tt.bytes, tt.name)
			pt.Update(tt.bytes)
			pt.Complete()

			if pt.bytesTransferred != tt.bytes {
				t.Errorf("Expected bytesTransferred to be %d, got %d", tt.bytes, pt.bytesTransferred)
			}
		})
	}
}

// TestProgressTrackerUpdateWithTimeDilation tests the `Update` method of `ProgressTracker` to ensure that
// it expectedly calls `displayProgress` when enough time has passed since the last update.
func TestProgressTrackerUpdateWithTimeDilation(t *testing.T) {
	pt := NewProgressTracker(1000, "Delayed Transfer")

	// The first update should not trigger `displayProgress` since less than `barUpdateInterval` has passed.
	pt.Update(100)

	// Intentionally set `lastUpdate` to a time much earlier to simulate passage.
	pt.lastUpdate = time.Now().Add(-500 * time.Millisecond)

	// This update should trigger `displayProgress` due to time elapsed.
	pt.Update(500)

	if pt.bytesTransferred != 500 {
		t.Errorf("Expected bytesTransferred to be 500, got %d", pt.bytesTransferred)
	}
}

// TestProgressTrackerZeroTotalBytes tests the `displayProgress` method of `ProgressTracker` when totalBytes is zero to ensure that
// it expectedly handles the edge case without errors.
func TestProgressTrackerZeroTotalBytes(t *testing.T) {
	pt := NewProgressTracker(0, "Zero Transfer")
	pt.displayProgress()
	// No panic or error should occur.
}

// TestProgressTrackerDisplayProgressVariousSizes tests the `displayProgress` method of `ProgressTracker` with different file sizes to ensure that
// it expectedly handles various progress display scenarios.
func TestProgressTrackerDisplayProgressVariousSizes(t *testing.T) {
	tests := []struct {
		name          string
		totalBytes    uint64
		bytesProgress uint64
	}{
		{"small file", 512, 256},
		{"exactly 1KB", 1024, 512},
		{"medium file", 512 * 1024, 256 * 1024},
		{"exactly 1MB", 1024 * 1024, 512 * 1024},
		{"large file", 100 * 1024 * 1024, 50 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := NewProgressTracker(tt.totalBytes, tt.name)
			pt.Update(tt.bytesProgress)
			pt.lastUpdate = time.Now().Add(-500 * time.Millisecond)
			pt.displayProgress()

			if pt.bytesTransferred != tt.bytesProgress {
				t.Errorf("Expected bytesTransferred to be %d, got %d", tt.bytesProgress, pt.bytesTransferred)
			}
		})
	}
}

// TestProgressTrackerUpdateNoDisplayBecauseOfTime tests the `Update` method of `ProgressTracker` to ensure that
// it expectedly does not call `displayProgress` when not enough time has passed since the last update.
func TestProgressTrackerUpdateNoDisplayBecauseOfTime(t *testing.T) {
	pt := NewProgressTracker(1000, "No Display Transfer")
	pt.Update(100)

	// Intentionally update again immediately, which should not trigger `displayProgress` since less than `barUpdateInterval` has passed.
	pt.Update(200)

	if pt.bytesTransferred != 200 {
		t.Errorf("Expected bytesTransferred to be 200, got %d", pt.bytesTransferred)
	}
}

// TestProgressTrackerCompleteZeroRate tests the `Complete` method of `ProgressTracker` when transfer time is very short ($rate \approx 0$) to ensure that
// it expectedly handles the zero-rate scenario.
func TestProgressTrackerCompleteZeroRate(t *testing.T) {
	pt := NewProgressTracker(100, "Instant Transfer")
	// Intentionally complete the transfer immediately.
	pt.Complete()

	if pt.bytesTransferred != 100 {
		t.Errorf("Expected bytesTransferred to be 100, got %d", pt.bytesTransferred)
	}
}

// TestProgressReaderReadWithZeroBytes tests the `Read` method of `ProgressReader` when reading zero bytes to ensure that
// it expectedly handles the zero-byte read scenario.
func TestProgressReaderReadWithZeroBytes(t *testing.T) {
	reader := strings.NewReader("")
	pr := NewProgressReader(reader, 0, "Empty Download")

	buf := make([]byte, 10)
	n, _ := pr.Read(buf)

	if n != 0 {
		t.Errorf("Expected 0 bytes read, got %d", n)
	}
	if pr.tracker.bytesTransferred != 0 {
		t.Errorf("Expected bytesTransferred to be 0, got %d", pr.tracker.bytesTransferred)
	}
}

// TestProgressTrackerCalculateRateVariations tests the `calculateRate` method of `ProgressTracker` with various byte sizes and durations to ensure that
// it expectedly calculates the transfer rate correctly.
func TestProgressTrackerCalculateRateVariations(t *testing.T) {
	tests := []struct {
		name       string
		bytes      uint64
		timePassed time.Duration
	}{
		{"zero duration", 1000, 0},
		{"100ms elapsed with 1MB", 1 * 1024 * 1024, 100 * time.Millisecond},
		{"instant transfer", 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := NewProgressTracker(tt.bytes, tt.name)
			pt.bytesTransferred = tt.bytes
			pt.startTime = time.Now().Add(-tt.timePassed)

			rate := pt.calculateRate()

			if rate < 0 {
				t.Errorf("Expected rate >= 0, got %f", rate)
			}
			if tt.timePassed > 0 && rate <= 0 {
				t.Errorf("Expected rate > 0 with elapsed time %v, got %f", tt.timePassed, rate)
			}
		})
	}
}

// TestProgressTrackerCalculateRateExactlyZero tests the `calculateRate` method of `ProgressTracker` when duration is exactly zero to ensure that
// it expectedly returns a rate of zero.
func TestProgressTrackerCalculateRateExactlyZero(t *testing.T) {
	pt := NewProgressTracker(1000, "Exactly Zero")
	pt.bytesTransferred = 500
	// Set `startTime` to `now + 1` second to simulate zero or negative duration.
	futureTime := time.Now().Add(time.Second)
	pt.startTime = futureTime

	rate := pt.calculateRate()

	// Rate should be zero when duration is less than or equal to zero.
	if rate != 0 {
		t.Errorf("Expected rate == 0 for non-positive duration, got %f", rate)
	}
}

// TestProgressWriterComplete tests the `Complete` method of `ProgressWriter` to ensure that
// it expectedly sets `bytesTransferred` to the total bytes when marked complete.
func TestProgressWriterComplete(t *testing.T) {
	writer := &strings.Builder{}
	pw := NewProgressWriter(writer, 5, "Upload")

	pw.tracker.Update(3)
	pw.Complete()

	if pw.tracker.bytesTransferred != 5 {
		t.Errorf("Expected bytesTransferred to be 5 after Complete(), got %d", pw.tracker.bytesTransferred)
	}
}
