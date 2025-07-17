package protocol

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// A ProgressTracker tracks the progress of file transfers.
type ProgressTracker struct {
	totalBytes        int64
	bytesTransferred  int64
	startTime         time.Time
	lastUpdate        time.Time
	barUpdateInterval time.Duration
	description       string
}

// A ProgressReader wraps an io.Reader and tracks progress.
type ProgressReader struct {
	reader  io.Reader
	tracker *ProgressTracker
}

// A ProgressWriter wraps an io.Writer and tracks progress.
type ProgressWriter struct {
	writer  io.Writer
	tracker *ProgressTracker
}

// NewProgressTracker creates a new progress tracker.
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

// Update updates the progress and displays it if enough time has passed.
func (pt *ProgressTracker) Update(bytesTransferred int64) {
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
		rate = float64(pt.totalBytes) / duration.Seconds() / 1024 / 1024 // MB/s.
	} else {
		rate = 0
	}

	if pt.totalBytes < 1024 {
		fmt.Printf("\n%s completed! %d bytes in %v\n",
			pt.description, pt.totalBytes, duration)
	} else if pt.totalBytes < 1024*1024 {
		fmt.Printf("\n%s completed! %.1f KB in %v (%.2f MB/s)\n",
			pt.description, float64(pt.totalBytes)/1024, duration, rate)
	} else {
		fmt.Printf("\n%s completed! %.1f MB in %v (%.2f MB/s)\n",
			pt.description, float64(pt.totalBytes)/1024/1024, duration, rate)
	}
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
		rate = float64(pt.bytesTransferred) / duration.Seconds() / 1024 / 1024 // MB/s.
	} else {
		rate = 0
	}

	var sizeDisplay string
	if pt.totalBytes < 1024 {
		sizeDisplay = fmt.Sprintf("%d/%d bytes", pt.bytesTransferred, pt.totalBytes)
	} else if pt.totalBytes < 1024*1024 {
		sizeDisplay = fmt.Sprintf("%.1f/%.1f KB",
			float64(pt.bytesTransferred)/1024, float64(pt.totalBytes)/1024)
	} else {
		sizeDisplay = fmt.Sprintf("%.1f/%.1f MB",
			float64(pt.bytesTransferred)/1024/1024, float64(pt.totalBytes)/1024/1024)
	}

	fmt.Printf("\r%s %s %.1f%% (%s, %.2f MB/s)",
		pt.description, progressBar, percentage, sizeDisplay, rate)
}

// createProgressBar creates a visual progress bar.
func (pt *ProgressTracker) createProgressBar(percentage float64) string {
	const barWidth = 30
	filled := int(percentage / 100 * barWidth)

	bar := strings.Repeat("=", filled)
	bar += strings.Repeat("-", barWidth-filled)

	return "[" + bar + "]"
}

// NewProgressReader creates a new progress reader.
func NewProgressReader(reader io.Reader, totalBytes int64, description string) *ProgressReader {
	return &ProgressReader{
		reader:  reader,
		tracker: NewProgressTracker(totalBytes, description),
	}
}

// Read reads from the reader and updates the progress.
func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	if n > 0 {
		pr.tracker.Update(pr.tracker.bytesTransferred + int64(n))
	}
	return n, err
}

// Complete (for ProgressReader) marks the transfer as complete.
func (pr *ProgressReader) Complete() {
	pr.tracker.Complete()
}

// NewProgressWriter creates a new progress writer.
func NewProgressWriter(writer io.Writer, totalBytes int64, description string) *ProgressWriter {
	return &ProgressWriter{
		writer:  writer,
		tracker: NewProgressTracker(totalBytes, description),
	}
}

// Write writes to the writer and updates the progress.
func (pw *ProgressWriter) Write(p []byte) (n int, err error) {
	n, err = pw.writer.Write(p)
	if n > 0 {
		pw.tracker.Update(pw.tracker.bytesTransferred + int64(n))
	}
	return n, err
}

// Complete (for ProgressWriter) marks the transfer as complete.
func (pw *ProgressWriter) Complete() {
	pw.tracker.Complete()
}
