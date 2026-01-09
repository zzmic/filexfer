package protocol

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// A ProgressTracker tracks the progress of file transfers.
type ProgressTracker struct {
	totalBytes        uint64        // Total number of bytes to transfer.
	bytesTransferred  uint64        // Bytes transferred so far.
	startTime         time.Time     // Time when the transfer started.
	lastUpdate        time.Time     // Time of the last progress update.
	barUpdateInterval time.Duration // Interval between progress bar updates.
	description       string        // Description of the transfer.
	writer            io.Writer     // Writer for progress output (defaults to os.Stderr).
}

// A ProgressReader tracks the progress of reading from an `io.Reader`.
type ProgressReader struct {
	reader  io.Reader        // Underlying reader.
	tracker *ProgressTracker // Encapsulated progress tracker.
}

// A ProgressWriter wraps an `io.Writer` and tracks the progress of writing.
type ProgressWriter struct {
	writer  io.Writer        // Underlying writer.
	tracker *ProgressTracker // Encapsulated progress tracker.
}

// toKB converts bytes to kilobytes.
func toKB(bytes uint64) float64 {
	return float64(bytes) / 1024
}

// toMB converts bytes to megabytes.
func toMB(bytes uint64) float64 {
	return float64(bytes) / 1024 / 1024
}

// NewProgressTracker instantiates a new progress tracker.
// If writer is nil, it defaults to os.Stderr to keep os.Stdout clean for piping.
func NewProgressTracker(totalBytes uint64, description string, writer io.Writer) *ProgressTracker {
	if writer == nil {
		writer = os.Stderr
	}
	return &ProgressTracker{
		totalBytes:        totalBytes,
		bytesTransferred:  0,
		startTime:         time.Now(),
		lastUpdate:        time.Now(),
		barUpdateInterval: 250 * time.Millisecond, // Update every 250ms.
		description:       description,
		writer:            writer,
	}
}

// Update updates the progress and displays it if `barUpdateInterval` has passed.
func (pt *ProgressTracker) Update(bytesTransferred uint64) {
	pt.bytesTransferred = bytesTransferred

	now := time.Now()
	if now.Sub(pt.lastUpdate) >= pt.barUpdateInterval {
		pt.displayProgress()
		pt.lastUpdate = now
	}
}

// Complete displays the final progress and transfer statistics.
func (pt *ProgressTracker) Complete() {
	pt.bytesTransferred = pt.totalBytes
	pt.displayProgress()

	duration := time.Since(pt.startTime)
	rate := pt.calculateRate()

	if pt.totalBytes < 1024 {
		if _, err := fmt.Fprintf(pt.writer, "\n%s completed! %d bytes in %v\n",
			pt.description, pt.totalBytes, duration); err != nil {
			log.Printf("Failed to write the transfer completion message: %v", err)
		}
	} else if pt.totalBytes < 1024*1024 {
		if _, err := fmt.Fprintf(pt.writer, "\n%s completed! %.1f KB in %v (%.2f MB/s)\n",
			pt.description, toKB(pt.totalBytes), duration, rate); err != nil {
			log.Printf("Failed to write the transfer completion message: %v", err)
		}

	} else {
		if _, err := fmt.Fprintf(pt.writer, "\n%s completed! %.1f MB in %v (%.2f MB/s)\n",
			pt.description, toMB(pt.totalBytes), duration, rate); err != nil {
			log.Printf("Failed to write the transfer completion message: %v", err)
		}
	}
}

// createProgressBar creates a visual progress bar.
func (pt *ProgressTracker) createProgressBar(percentage float64) string {
	const barWidth = 30
	filled := int(percentage / 100 * barWidth)

	bar := strings.Repeat("=", filled)
	bar += strings.Repeat("-", barWidth-filled)

	return "[" + bar + "]"
}

// calculateRate calculates the transfer rate in MB/s.
func (pt *ProgressTracker) calculateRate() float64 {
	duration := time.Since(pt.startTime)
	if duration.Seconds() > 0 {
		return toMB(pt.bytesTransferred) / duration.Seconds()
	}
	return 0
}

// displayProgress displays the current progress with a progress bar.
func (pt *ProgressTracker) displayProgress() {
	if pt.totalBytes == 0 {
		return
	}

	percentage := float64(pt.bytesTransferred) / float64(pt.totalBytes) * 100
	progressBar := pt.createProgressBar(percentage)
	rate := pt.calculateRate()

	var sizeDisplay string
	if pt.totalBytes < 1024 {
		sizeDisplay = fmt.Sprintf("%d/%d bytes", pt.bytesTransferred, pt.totalBytes)
	} else if pt.totalBytes < 1024*1024 {
		sizeDisplay = fmt.Sprintf("%.1f/%.1f KB",
			toKB(pt.bytesTransferred), toKB(pt.totalBytes))
	} else {
		sizeDisplay = fmt.Sprintf("%.1f/%.1f MB",
			toMB(pt.bytesTransferred), toMB(pt.totalBytes))
	}

	_, _ = fmt.Fprintf(pt.writer, "\r%s %s %.1f%% (%s, %.2f MB/s)",
		pt.description, progressBar, percentage, sizeDisplay, rate)
}

// NewProgressReader creates a new progress reader.
// If writer is nil, progress output defaults to os.Stderr to keep os.Stdout clean for piping.
func NewProgressReader(reader io.Reader, totalBytes uint64, description string, writer io.Writer) *ProgressReader {
	return &ProgressReader{
		reader:  reader,
		tracker: NewProgressTracker(totalBytes, description, writer),
	}
}

// Read implements the `io.Reader` interface and updates progress.
func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	if n > 0 {
		pr.tracker.Update(pr.tracker.bytesTransferred + uint64(n))
	}
	return n, err
}

// Complete marks the transfer as complete.
func (pr *ProgressReader) Complete() {
	pr.tracker.Complete()
}

// NewProgressWriter creates a new progress writer.
// If progressWriter is nil, progress output defaults to os.Stderr to keep os.Stdout clean for piping.
func NewProgressWriter(writer io.Writer, totalBytes uint64, description string, progressWriter io.Writer) *ProgressWriter {
	return &ProgressWriter{
		writer:  writer,
		tracker: NewProgressTracker(totalBytes, description, progressWriter),
	}
}

// Write implements the `io.Writer` interface and updates progress.
func (pw *ProgressWriter) Write(p []byte) (n int, err error) {
	n, err = pw.writer.Write(p)
	if n > 0 {
		pw.tracker.Update(pw.tracker.bytesTransferred + uint64(n))
	}
	return n, err
}

// Complete marks the transfer as complete.
func (pw *ProgressWriter) Complete() {
	pw.tracker.Complete()
}
