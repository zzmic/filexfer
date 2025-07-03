package protocol

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Struct to track the progress of file transfers.
type ProgressTracker struct {
	totalBytes        int64
	bytesTransferred  int64
	startTime         time.Time
	lastUpdate        time.Time
	barUpdateInterval time.Duration
	description       string
}

// Function to create a new progress tracker.
func NewProgressTracker(totalBytes int64, description string) *ProgressTracker {
	return &ProgressTracker{
		totalBytes:        totalBytes,
		bytesTransferred:  0,
		startTime:         time.Now(),
		lastUpdate:        time.Now(),
		barUpdateInterval: 250 * time.Millisecond,
		description:       description,
	}
}

// Function to update the progress and display it if enough time has passed.
func (pt *ProgressTracker) Update(bytesTransferred int64) {
	pt.bytesTransferred = bytesTransferred

	now := time.Now()
	if now.Sub(pt.lastUpdate) >= pt.barUpdateInterval {
		pt.displayProgress()
		pt.lastUpdate = now
	}
}

// Function to display the final progress and transfer statistics.
func (pt *ProgressTracker) Complete() {
	pt.bytesTransferred = pt.totalBytes
	pt.displayProgress()

	duration := time.Since(pt.startTime)
	rate := float64(pt.totalBytes) / duration.Seconds() / 1024 / 1024 // MB/s.

	fmt.Printf("\n%s completed! %d bytes in %v (%.2f MB/s)\n",
		pt.description, pt.totalBytes, duration, rate)
}

// Function to display the current progress with a progress bar.
func (pt *ProgressTracker) displayProgress() {
	if pt.totalBytes == 0 {
		return
	}

	percentage := float64(pt.bytesTransferred) / float64(pt.totalBytes) * 100
	progressBar := pt.createProgressBar(percentage)
	duration := time.Since(pt.startTime)
	rate := float64(pt.bytesTransferred) / duration.Seconds() / 1024 / 1024 // MB/s.
	fmt.Printf("\r%s %s %.1f%% (%d/%d bytes, %.2f MB/s)",
		pt.description, progressBar, percentage, pt.bytesTransferred, pt.totalBytes, rate)
}

// Function to create a visual progress bar.
func (pt *ProgressTracker) createProgressBar(percentage float64) string {
	const barWidth = 30
	filled := int(percentage / 100 * barWidth)

	bar := strings.Repeat("=", filled)
	bar += strings.Repeat("-", barWidth-filled)

	return "[" + bar + "]"
}

// Struct to wrap an io.Reader and track progress.
type ProgressReader struct {
	reader  io.Reader
	tracker *ProgressTracker
}

// Function to create a new progress reader.
func NewProgressReader(reader io.Reader, totalBytes int64, description string) *ProgressReader {
	return &ProgressReader{
		reader:  reader,
		tracker: NewProgressTracker(totalBytes, description),
	}
}

// Function to read from the reader and update the progress.
func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	if n > 0 {
		pr.tracker.Update(pr.tracker.bytesTransferred + int64(n))
	}
	return n, err
}

// Function to mark the transfer as complete.
func (pr *ProgressReader) Complete() {
	pr.tracker.Complete()
}

// Struct to wrap an io.Writer and track progress.
type ProgressWriter struct {
	writer  io.Writer
	tracker *ProgressTracker
}

// Function to create a new progress writer.
func NewProgressWriter(writer io.Writer, totalBytes int64, description string) *ProgressWriter {
	return &ProgressWriter{
		writer:  writer,
		tracker: NewProgressTracker(totalBytes, description),
	}
}

// Function to write to the writer and update the progress.
func (pw *ProgressWriter) Write(p []byte) (n int, err error) {
	n, err = pw.writer.Write(p)
	if n > 0 {
		pw.tracker.Update(pw.tracker.bytesTransferred + int64(n))
	}
	return n, err
}

// Function to mark the transfer as complete.
func (pw *ProgressWriter) Complete() {
	pw.tracker.Complete()
}
