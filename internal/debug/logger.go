package debug

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

var (
	logger *log.Logger
	file   *os.File
	once   sync.Once
	mu     sync.Mutex
)

// Init initializes the debug logger, writing to ./debug.log.
// If the file already exists, it will be overwritten.
// This function is safe to call multiple times, but only the first call will have effect.
func Init() error {
	var err error
	once.Do(func() {
		file, err = os.Create("debug.log")
		if err != nil {
			err = fmt.Errorf("creating debug.log: %w", err)
			return
		}
		logger = log.New(file, "", log.LstdFlags|log.Lmicroseconds)
	})
	return err
}

// Close closes the debug log file. Should be called when the application exits.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		return file.Close()
	}
	return nil
}

// Logf logs a formatted message to the debug log.
func Logf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		logger.Printf(format, args...)
		_ = file.Sync() // Flush to disk immediately
	}
}

// Log logs a message to the debug log.
func Log(args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		logger.Println(args...)
		_ = file.Sync() // Flush to disk immediately
	}
}

// LogError logs an error message to the debug log.
func LogError(prefix string, err error) {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		logger.Printf("ERROR [%s]: %v\n", prefix, err)
		_ = file.Sync() // Flush to disk immediately
	}
}

// LogWarning logs a warning message to the debug log.
func LogWarning(prefix string, msg string) {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		logger.Printf("WARNING [%s]: %s\n", prefix, msg)
		_ = file.Sync() // Flush to disk immediately
	}
}

// IsEnabled returns true if the debug logger has been initialized.
func IsEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return logger != nil
}

// Transport is an http.RoundTripper that logs all requests.
type Transport struct {
	Base http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Log request
	if IsEnabled() {
		logRequest(req)
	} else {
		// Log even if not enabled (shouldn't happen, but useful for debugging)
		mu.Lock()
		if logger != nil {
			logger.Printf("[Transport] RoundTrip called but IsEnabled()=false: %s %s\n", req.Method, req.URL.String())
		}
		mu.Unlock()
	}

	// Perform request
	resp, err := t.Base.RoundTrip(req)

	// Log response
	if IsEnabled() && resp != nil {
		mu.Lock()
		if logger != nil {
			logger.Printf("<-- %s %s (status: %s)\n", req.Method, req.URL.String(), resp.Status)
			_ = file.Sync()
		}
		mu.Unlock()
	}

	return resp, err
}

func logRequest(req *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	var bodyStr string
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err == nil {
			bodyStr = string(body)
			// Restore body for actual request
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
	}

	logger.Printf("--> %s %s\n", req.Method, req.URL.String())
	if bodyStr != "" {
		logger.Printf("    Body: %s\n", bodyStr)
	}
	_ = file.Sync() // Flush to disk immediately
}
