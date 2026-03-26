package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func restoreDefaultTheme(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if err := SetTheme("default"); err != nil {
			t.Fatalf("restoring default theme: %v", err)
		}
	})
}

func TestSetTheme_Default(t *testing.T) {
	restoreDefaultTheme(t)

	if err := SetTheme("default"); err != nil {
		t.Fatalf("SetTheme(\"default\"): %v", err)
	}

	if ColorError != lipgloss.Color("9") {
		t.Errorf("ColorError = %q, want %q", ColorError, "9")
	}
	if ColorAccent != lipgloss.Color("12") {
		t.Errorf("ColorAccent = %q, want %q", ColorAccent, "12")
	}
	if ColorSurface != lipgloss.Color("237") {
		t.Errorf("ColorSurface = %q, want %q", ColorSurface, "237")
	}
}

func TestSetTheme_Catppuccin(t *testing.T) {
	restoreDefaultTheme(t)

	if err := SetTheme("catppuccin"); err != nil {
		t.Fatalf("SetTheme(\"catppuccin\"): %v", err)
	}

	if ColorError != lipgloss.Color("#f38ba8") {
		t.Errorf("ColorError = %q, want %q", ColorError, "#f38ba8")
	}
	if ColorAccent != lipgloss.Color("#cba6f7") {
		t.Errorf("ColorAccent = %q, want %q", ColorAccent, "#cba6f7")
	}
	if ColorSpinner != lipgloss.Color("#cba6f7") {
		t.Errorf("ColorSpinner = %q, want %q", ColorSpinner, "#cba6f7")
	}
}

func TestSetTheme_Unknown(t *testing.T) {
	if err := SetTheme("nonexistent"); err == nil {
		t.Fatal("SetTheme(\"nonexistent\") should return error")
	}
}

func TestSetTheme_RebuildStyles(t *testing.T) {
	restoreDefaultTheme(t)

	if err := SetTheme("catppuccin"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}

	// MutedStyle should use the catppuccin Muted color.
	got := MutedStyle.GetForeground()
	want := lipgloss.Color("#6c7086")
	if got != want {
		t.Errorf("MutedStyle foreground = %v, want %v", got, want)
	}
}

func TestSetTheme_EpicPalette(t *testing.T) {
	restoreDefaultTheme(t)

	if err := SetTheme("catppuccin"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}

	color := EpicColor("PROJ-1")
	if color == "" {
		t.Error("EpicColor returned empty after theme switch")
	}

	// Verify the color is from the catppuccin palette.
	cp := themes["catppuccin"]
	found := false
	for _, c := range cp.EpicPalette {
		if color == c {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("EpicColor(%q) = %q, not in catppuccin palette", "PROJ-1", color)
	}
}

func TestThemeNames(t *testing.T) {
	names := ThemeNames()
	if len(names) < 2 {
		t.Fatalf("expected at least 2 themes, got %d", len(names))
	}
	// Should be sorted.
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("ThemeNames not sorted: %v", names)
			break
		}
	}
}
