package protocol

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// toMB converts bytes to megabytes.
func toMB(bytes uint64) float64 {
	return float64(bytes) / 1024 / 1024
}

// toKB converts bytes to kilobytes.
func toKB(bytes uint64) float64 {
	return float64(bytes) / 1024
}

// A ProgressTracker tracks the progress of file transfers.
type ProgressTracker struct {
	totalBytes        uint64        // Total number of bytes to transfer.
	bytesTransferred  uint64        // Bytes transferred so far.
	startTime         time.Time     // Time when the transfer started.
	lastUpdate        time.Time     // Time of the last progress update.
	barUpdateInterval time.Duration // Interval between progress bar updates.
	description       string        // Description of the transfer.
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

// NewProgressTracker instantiates a new progress tracker.
func NewProgressTracker(totalBytes uint64, description string) *ProgressTracker {
	return &ProgressTracker{
		totalBytes:        totalBytes,
		bytesTransferred:  0,
		startTime:         time.Now(),
		lastUpdate:        time.Now(),
		barUpdateInterval: 250 * time.Millisecond, // Update every 250ms.
		description:       description,
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

	var rate float64
	if duration.Seconds() > 0 {
		rate = toMB(pt.totalBytes) / duration.Seconds()
	} else {
		rate = 0
	}

	if pt.totalBytes < 1024 {
		fmt.Printf("\n%s completed! %d bytes in %v\n",
			pt.description, pt.totalBytes, duration)
	} else if pt.totalBytes < 1024*1024 {
		fmt.Printf("\n%s completed! %.1f KB in %v (%.2f MB/s)\n",
			pt.description, toKB(pt.totalBytes), duration, rate)

	} else {
		fmt.Printf("\n%s completed! %.1f MB in %v (%.2f MB/s)\n",
			pt.description, toMB(pt.totalBytes), duration, rate)
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

// displayProgress displays the current progress with a progress bar.
func (pt *ProgressTracker) displayProgress() {
	if pt.totalBytes == 0 {
		return
	}

	percentage := float64(pt.bytesTransferred) / float64(pt.totalBytes) * 100
	progressBar := pt.createProgressBar(percentage)
	duration := time.Since(pt.startTime)

	var rate float64
	if duration.Seconds() > 0 {
		rate = toMB(pt.bytesTransferred) / duration.Seconds()
	} else {
		rate = 0
	}

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

	fmt.Printf("\r%s %s %.1f%% (%s, %.2f MB/s)",
		pt.description, progressBar, percentage, sizeDisplay, rate)
}

// NewProgressReader creates a new progress reader.
func NewProgressReader(reader io.Reader, totalBytes uint64, description string) *ProgressReader {
	return &ProgressReader{
		reader:  reader,
		tracker: NewProgressTracker(totalBytes, description),
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
func NewProgressWriter(writer io.Writer, totalBytes uint64, description string) *ProgressWriter {
	return &ProgressWriter{
		writer:  writer,
		tracker: NewProgressTracker(totalBytes, description),
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
