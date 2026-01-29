package metricslog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	logDirName          = "logs"
	fileDateLayout      = "2006-01-02"
	writerFlushInterval = 1 * time.Second
	writerQueueSize     = 1024
)

var defaultWriter = newJSONLWriter(writerQueueSize, writerFlushInterval)

// Enqueue attempts to enqueue a log line for writing.
// It never blocks the caller; when the internal queue is full, the line is dropped.
func Enqueue(line MetricsLogLine) {
	defaultWriter.Enqueue(line)
}

type jsonlWriter struct {
	queue chan MetricsLogLine

	flushEvery time.Duration

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	doneCh    chan struct{}
}

func newJSONLWriter(queueSize int, flushEvery time.Duration) *jsonlWriter {
	if queueSize <= 0 {
		queueSize = writerQueueSize
	}
	if flushEvery <= 0 {
		flushEvery = writerFlushInterval
	}
	w := &jsonlWriter{
		queue:      make(chan MetricsLogLine, queueSize),
		flushEvery: flushEvery,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
	// Ensure the background worker is running.
	w.Start()
	return w
}

func (w *jsonlWriter) Start() {
	if w == nil {
		return
	}
	w.startOnce.Do(func() {
		go w.run()
	})
}

func (w *jsonlWriter) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stopCh)
		<-w.doneCh
	})
}

func (w *jsonlWriter) Enqueue(line MetricsLogLine) {
	if w == nil {
		return
	}
	w.Start()
	select {
	case w.queue <- line:
	default:
		// Drop on queue full to ensure this stays off the main request path.
	}
}

func (w *jsonlWriter) run() {
	defer close(w.doneCh)

	ticker := time.NewTicker(w.flushEvery)
	defer ticker.Stop()

	var (
		currentDay string
		file       *os.File
		bufw       *bufio.Writer
		enc        *json.Encoder
	)

	closeFile := func() {
		if bufw != nil {
			_ = bufw.Flush() // best-effort; failures are ignored
		}
		if file != nil {
			_ = file.Close()
		}
		file = nil
		bufw = nil
		enc = nil
	}

	openForNow := func(now time.Time) {
		day := now.Format(fileDateLayout)
		if day == currentDay && enc != nil {
			return
		}

		closeFile()

		// Fixed location and filename as per plan.
		_ = os.MkdirAll(logDirName, 0o755)
		path := filepath.Join(logDirName, "metrics-"+day+".jsonl")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			currentDay = day
			return
		}
		file = f
		bufw = bufio.NewWriterSize(file, 64*1024)
		enc = json.NewEncoder(bufw)
		enc.SetEscapeHTML(false)
		currentDay = day
	}

	drain := func(line MetricsLogLine) {
		if line.Timestamp.IsZero() {
			line.Timestamp = time.Now().UTC()
		}
		now := line.Timestamp
		openForNow(now)
		if enc == nil {
			return
		}
		// Encode writes a trailing newline, making it a natural JSON Lines writer.
		if err := enc.Encode(line); err != nil {
			// Best-effort: treat as a dropped line and continue.
			return
		}
	}

	for {
		select {
		case <-w.stopCh:
			// Drain queued items quickly on shutdown.
			for {
				select {
				case line := <-w.queue:
					drain(line)
				default:
					closeFile()
					return
				}
			}
		case line := <-w.queue:
			drain(line)
		case <-ticker.C:
			if bufw != nil {
				_ = bufw.Flush()
			}
			// Rotate to a new day even if idle.
			openForNow(time.Now().UTC())
		}
	}
}
