package editor

import (
	"os"
	"os/exec"
	"strings"
)

// OpenEditor opens the file at path in the user's preferred editor ($EDITOR →
// $VISUAL → vi) and blocks until it exits.
func OpenEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}
	// Support editors with args, e.g. EDITOR="code --wait".
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
