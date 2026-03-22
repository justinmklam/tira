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
	once = sync.Once{}
}

// inTempDir changes the working directory to a temp dir for the duration of the test,
// then restores it. debug.Init() creates debug.log in the current directory.
func inTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

// TestWritesBeforeCloseAppearInFile is the core contract: content written
// before Close must be present in debug.log after Close. This is the invariant
// violated by the original bug, where defer Close() inside PersistentPreRunE
// closed the file before RunE (and its Logf calls) executed.
func TestWritesBeforeCloseAppearInFile(t *testing.T) {
	dir := inTempDir(t)
	defer resetState(t)

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	Logf("sentinel message")

	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "debug.log"))
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
	dir := inTempDir(t)
	defer resetState(t)

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Simulates writes that happen in RunE after PersistentPreRunE's defer fires.
	Logf("this must not appear")

	content, _ := os.ReadFile(filepath.Join(dir, "debug.log"))
	if strings.Contains(string(content), "this must not appear") {
		t.Error("found content written after Close — Close() was called too early")
	}
}

// TestIsEnabledAfterInit verifies IsEnabled reflects initialisation state.
func TestIsEnabledAfterInit(t *testing.T) {
	inTempDir(t)
	defer resetState(t)

	if IsEnabled() {
		t.Error("IsEnabled() = true before Init()")
	}
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !IsEnabled() {
		t.Error("IsEnabled() = false after Init()")
	}
}
