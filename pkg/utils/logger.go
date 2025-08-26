package utils

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Logger provides different logging levels
type Logger struct {
	debug   bool
	verbose bool
	writer  io.Writer // Where to write logs (os.Stdout by default)

	// optional remote shipping
	// shipper *httpShipper
}

// NewLogger creates a new logger with the specified levels
func NewLogger(debug, verbose bool) *Logger {
	return &Logger{
		debug:   debug,
		verbose: verbose,
		writer:  os.Stdout, // Default to stdout
	}
}

// NewLoggerWithFile creates a new logger that writes to a file
func NewLoggerWithFile(debug, verbose bool, logFilePath string) (*Logger, error) {
	// Ensure directory for the specific log file exists (handles nested paths)
	if err := EnsureDirForFile(logFilePath); err != nil {
		return nil, fmt.Errorf("failed to create log directory for %s: %w", logFilePath, err)
	}

	// Open log file for writing (create or append)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logFilePath, err)
	}

	// Create a multi-writer to write to both stdout and file
	multiWriter := io.MultiWriter(os.Stdout, logFile)

	return &Logger{
		debug:   debug,
		verbose: verbose,
		writer:  multiWriter,
	}, nil
}

// EnableRemoteShipping attaches a non-blocking HTTP shipper. If destination is empty, no-op.
// func (l *Logger) EnableRemoteShipping(destination string, headers map[string]string, provider string) {
// 	if destination == "" {
// 		return
// 	}
// 	l.shipper = newHTTPShipper(destination, headers, provider)
// }

// Info logs informational messages (always shown)
func (l *Logger) Info(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.writer, "[%s] INFO: %s\n", timestamp, msg)
	// if l.shipper != nil {
	// 	l.shipper.enqueue("info", msg)
	// }
}

// Debug logs debug messages (only if debug enabled)
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.debug {
		timestamp := time.Now().Format("15:04:05")
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(l.writer, "[%s] DEBUG: %s\n", timestamp, msg)
		// if l.shipper != nil {
		// 	l.shipper.enqueue("debug", msg)
		// }
	}
}

// Verbose logs verbose messages (only if verbose enabled)
func (l *Logger) Verbose(format string, args ...interface{}) {
	if l.verbose {
		timestamp := time.Now().Format("15:04:05")
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(l.writer, "[%s] VERBOSE: %s\n", timestamp, msg)
		// if l.shipper != nil {
		// 	l.shipper.enqueue("verbose", msg)
		// }
	}
}

// Error logs error messages (always shown)
func (l *Logger) Error(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.writer, "[%s] ERROR: %s\n", timestamp, msg)
	// if l.shipper != nil {
	// 	l.shipper.enqueue("error", msg)
	// }
}

// httpShipper implements a simple non-blocking, batched HTTP log shipper.
// type httpShipper struct {
// 	destURL  string
// 	headers  map[string]string
// 	provider string // "generic" or "datadog"

// 	mu     sync.Mutex
// 	queue  []shippedEvent
// 	cond   *sync.Cond
// 	closed bool
// }

// type shippedEvent struct {
// 	Timestamp time.Time `json:"timestamp"`
// 	Level     string    `json:"level"`
// 	Message   string    `json:"message"`
// }

// func newHTTPShipper(dest string, headers map[string]string, provider string) *httpShipper {
// 	s := &httpShipper{destURL: dest, headers: headers, provider: provider}
// 	s.cond = sync.NewCond(&s.mu)
// 	s.queue = make([]shippedEvent, 0, 256)
// 	go s.run()
// 	return s
// }

// func (s *httpShipper) enqueue(level, msg string) {
// 	s.mu.Lock()
// 	if s.closed {
// 		s.mu.Unlock()
// 		return
// 	}
// 	// bounded queue: drop oldest when > 1000
// 	if len(s.queue) >= 1000 {
// 		s.queue = s.queue[1:]
// 	}
// 	s.queue = append(s.queue, shippedEvent{Timestamp: time.Now(), Level: level, Message: msg})
// 	s.cond.Signal()
// 	s.mu.Unlock()
// }

// func (s *httpShipper) run() {
// 	ticker := time.NewTicker(2 * time.Second)
// 	defer ticker.Stop()
// 	for {
// 		s.mu.Lock()
// 		for len(s.queue) == 0 && !s.closed {
// 			s.cond.Wait()
// 		}
// 		if s.closed && len(s.queue) == 0 {
// 			s.mu.Unlock()
// 			return
// 		}
// 		// drain up to 100 events or whatever is available, or flush on tick
// 		batch := s.drainLocked(100)
// 		s.mu.Unlock()

// 		if len(batch) == 0 {
// 			<-ticker.C
// 			continue
// 		}
// 		_ = s.postBatch(batch) // best-effort
// 	}
// }

// func (s *httpShipper) drainLocked(n int) []shippedEvent {
// 	if len(s.queue) == 0 {
// 		return nil
// 	}
// 	if n > len(s.queue) {
// 		n = len(s.queue)
// 	}
// 	out := make([]shippedEvent, n)
// 	copy(out, s.queue[:n])
// 	s.queue = s.queue[n:]
// 	return out
// }

// func (s *httpShipper) postBatch(batch []shippedEvent) error {
// 	var payload []byte
// 	var err error

// 	switch s.provider {
// 	case "datadog":
// 		// Datadog v2 logs intake accepts an array of events
// 		// Keep it minimal and generic: ddsource/service/status/message
// 		ddEvents := make([]map[string]interface{}, 0, len(batch))
// 		for _, ev := range batch {
// 			ddEvents = append(ddEvents, map[string]interface{}{
// 				"ddsource":  "go-installapplications",
// 				"service":   "go-installapplications",
// 				"status":    ev.Level,
// 				"message":   ev.Message,
// 				"timestamp": ev.Timestamp.UnixMilli(),
// 			})
// 		}
// 		payload, err = json.Marshal(ddEvents)
// 	default:
// 		// generic: simple JSON array
// 		payload, err = json.Marshal(batch)
// 	}
// 	if err != nil {
// 		return err
// 	}

// 	req, err := http.NewRequest("POST", s.destURL, bytes.NewReader(payload))
// 	if err != nil {
// 		return err
// 	}
// 	// headers
// 	for k, v := range s.headers {
// 		// do not log header values anywhere
// 		req.Header.Set(k, v)
// 	}
// 	if req.Header.Get("Content-Type") == "" {
// 		req.Header.Set("Content-Type", "application/json")
// 	}

// 	client := &http.Client{Timeout: 5 * time.Second}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return err
// 	}
// 	io.Copy(io.Discard, resp.Body)
// 	resp.Body.Close()
// 	return nil
// }

// func (s *httpShipper) Close() {
// 	s.mu.Lock()
// 	s.closed = true
// 	s.cond.Broadcast()
// 	s.mu.Unlock()
// }
