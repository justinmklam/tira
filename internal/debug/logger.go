package debug

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var (
	logger  *log.Logger
	file    *os.File
	logPath string
	once    sync.Once
	mu      sync.Mutex
)

// DefaultLogPath returns the default debug log location:
// $XDG_STATE_HOME/tira/debug.log, falling back to ~/.local/state/tira/debug.log
// when $XDG_STATE_HOME is unset. Prior to this, debug.log was written to the
// current working directory, which cluttered whatever directory the user
// happened to be in.
func DefaultLogPath() string {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			// Last-resort fallback: current directory (previous behavior).
			return "debug.log"
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, "tira", "debug.log")
}

// Init initializes the debug logger, writing to path (creating its parent
// directory if needed). If path is empty, DefaultLogPath() is used.
// If the file already exists, it will be overwritten.
// This function is safe to call multiple times, but only the first call will have effect.
func Init(path string) error {
	var err error
	once.Do(func() {
		if path == "" {
			path = DefaultLogPath()
		}

		if dir := filepath.Dir(path); dir != "." && dir != "" {
			if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
				err = fmt.Errorf("creating log directory %q: %w", dir, mkErr)
				return
			}
		}

		file, err = os.Create(path)
		if err != nil {
			err = fmt.Errorf("creating %s: %w", path, err)
			return
		}
		logPath = path
		logger = log.New(file, "", log.LstdFlags|log.Lmicroseconds)
	})
	return err
}

// LogPath returns the path the debug logger is writing to. Empty until Init
// has been called successfully.
func LogPath() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
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
