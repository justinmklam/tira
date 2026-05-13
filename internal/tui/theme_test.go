package tui

import (
	"fmt"
	"testing"
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

	if fmt.Sprint(ColorError) != fmt.Sprint(ColorError) {
		t.Error("ColorError should be set")
	}
	if fmt.Sprint(ColorAccent) != fmt.Sprint(ColorAccent) {
		t.Error("ColorAccent should be set")
	}
}

func TestSetTheme_Catppuccin(t *testing.T) {
	restoreDefaultTheme(t)

	if err := SetTheme("catppuccin"); err != nil {
		t.Fatalf("SetTheme(\"catppuccin\"): %v", err)
	}

	if ColorAccent == nil {
		t.Error("ColorAccent should not be nil")
	}
	if ColorSpinner == nil {
		t.Error("ColorSpinner should not be nil")
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

	// MutedStyle should have a foreground color set.
	got := MutedStyle.GetForeground()
	if got == nil {
		t.Error("MutedStyle foreground should not be nil")
	}
}

func TestSetTheme_EpicPalette(t *testing.T) {
	restoreDefaultTheme(t)

	if err := SetTheme("catppuccin"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}

	color := EpicColor("PROJ-1")
	if color == nil {
		t.Error("EpicColor returned nil after theme switch")
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
