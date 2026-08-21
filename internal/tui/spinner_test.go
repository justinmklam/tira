package tui

import (
	"errors"
	"testing"
)

// TestRunWithSpinner_NonInteractive verifies that RunWithSpinner falls back
// to calling fn synchronously (no bubbletea spinner UI) when stdin/stderr
// aren't a TTY — this is always true in `go test`, so this test doubles as a
// regression guard for the "error opening TTY" failures agents hit when
// piping/redirecting tira commands in non-interactive shells.
func TestRunWithSpinner_NonInteractive(t *testing.T) {
	if isInteractive() {
		t.Skip("test process has a real TTY on stdin/stderr; fallback path not exercised")
	}

	got, err := RunWithSpinner("doing work…", func() (string, error) {
		return "result", nil
	})
	if err != nil {
		t.Fatalf("RunWithSpinner returned unexpected error: %v", err)
	}
	if got != "result" {
		t.Errorf("RunWithSpinner() = %q, want %q", got, "result")
	}
}

func TestRunWithSpinner_NonInteractive_PropagatesError(t *testing.T) {
	if isInteractive() {
		t.Skip("test process has a real TTY on stdin/stderr; fallback path not exercised")
	}

	wantErr := errors.New("boom")
	_, err := RunWithSpinner("doing work…", func() (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("RunWithSpinner() error = %v, want %v", err, wantErr)
	}
}
