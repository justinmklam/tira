package debug

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// resetState clears all package-level state so tests can call Init() independently.
func resetState(t *testing.T) {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
	}
	logger = nil
	logPath = ""
	once = sync.Once{}
}

// tempLogPath returns a debug.log path inside a fresh temp directory, so
// tests never touch the real default log location.
func tempLogPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "debug.log")
}

// TestWritesBeforeCloseAppearInFile is the core contract: content written
// before Close must be present in debug.log after Close. This is the invariant
// violated by the original bug, where defer Close() inside PersistentPreRunE
// closed the file before RunE (and its Logf calls) executed.
func TestWritesBeforeCloseAppearInFile(t *testing.T) {
	path := tempLogPath(t)
	defer resetState(t)

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	Logf("sentinel message")

	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(content), "sentinel message") {
		t.Errorf("debug.log missing expected content\ngot:\n%s", content)
	}
}

// TestWritesAfterCloseDoNotAppearInFile verifies the complementary half:
// once Close is called, subsequent Logf calls must not write to the file.
// This directly models the regression scenario: if Close() is called too
// early (e.g. via defer inside PersistentPreRunE), writes from RunE are lost.
func TestWritesAfterCloseDoNotAppearInFile(t *testing.T) {
	path := tempLogPath(t)
	defer resetState(t)

	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Simulates writes that happen in RunE after PersistentPreRunE's defer fires.
	Logf("this must not appear")

	content, _ := os.ReadFile(path)
	if strings.Contains(string(content), "this must not appear") {
		t.Error("found content written after Close — Close() was called too early")
	}
}

// TestIsEnabledAfterInit verifies IsEnabled reflects initialisation state.
func TestIsEnabledAfterInit(t *testing.T) {
	path := tempLogPath(t)
	defer resetState(t)

	if IsEnabled() {
		t.Error("IsEnabled() = true before Init()")
	}
	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !IsEnabled() {
		t.Error("IsEnabled() = false after Init()")
	}
}

// TestInit_CreatesParentDirectory verifies Init creates the log file's parent
// directory (needed for the default $XDG_STATE_HOME/tira/debug.log path,
// which won't exist on a fresh machine).
func TestInit_CreatesParentDirectory(t *testing.T) {
	nested := filepath.Join(t.TempDir(), "nested", "dir", "debug.log")
	defer resetState(t)

	if err := Init(nested); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("expected %s to exist: %v", nested, err)
	}
}

// TestLogPath verifies LogPath reflects the path passed to Init.
func TestLogPath(t *testing.T) {
	path := tempLogPath(t)
	defer resetState(t)

	if got := LogPath(); got != "" {
		t.Errorf("LogPath() before Init = %q, want empty", got)
	}
	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := LogPath(); got != path {
		t.Errorf("LogPath() = %q, want %q", got, path)
	}
}

// TestDefaultLogPath_UsesXDGStateHome verifies DefaultLogPath honors
// $XDG_STATE_HOME when set.
func TestDefaultLogPath_UsesXDGStateHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	want := filepath.Join(dir, "tira", "debug.log")
	if got := DefaultLogPath(); got != want {
		t.Errorf("DefaultLogPath() = %q, want %q", got, want)
	}
}

// TestDefaultLogPath_FallsBackToHomeDir verifies DefaultLogPath falls back to
// ~/.local/state/tira/debug.log when $XDG_STATE_HOME is unset.
func TestDefaultLogPath_FallsBackToHomeDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory available in this environment")
	}

	want := filepath.Join(home, ".local", "state", "tira", "debug.log")
	if got := DefaultLogPath(); got != want {
		t.Errorf("DefaultLogPath() = %q, want %q", got, want)
	}
}

// TestInit_EmptyPathUsesDefault verifies Init("") resolves to DefaultLogPath().
func TestInit_EmptyPathUsesDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	defer resetState(t)

	if err := Init(""); err != nil {
		t.Fatalf("Init: %v", err)
	}

	want := filepath.Join(dir, "tira", "debug.log")
	if got := LogPath(); got != want {
		t.Errorf("LogPath() = %q, want %q", got, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected %s to exist: %v", want, err)
	}
}
