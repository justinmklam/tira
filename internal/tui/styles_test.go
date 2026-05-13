package tui

import (
	"fmt"
	"testing"
)

func TestIssueTypeColor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Bug", fmt.Sprint(ColorError)},
		{"bug", fmt.Sprint(ColorError)},
		{"Story", fmt.Sprint(ColorSuccess)},
		{"Task", fmt.Sprint(ColorAccent)},
		{"Epic", fmt.Sprint(ColorSpecial)},
		{"Sub-task", fmt.Sprint(ColorWarning)},
		{"subtask", fmt.Sprint(ColorWarning)},
		{"Unknown", fmt.Sprint(ColorMuted)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := fmt.Sprint(IssueTypeColor(tt.input))
			if got != tt.want {
				t.Errorf("IssueTypeColor(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEpicColor_Empty(t *testing.T) {
	got := EpicColor("")
	if got != nil {
		t.Errorf("EpicColor(\"\") = %v, want nil", got)
	}
}

func TestEpicColor_Deterministic(t *testing.T) {
	c1 := EpicColor("PROJ-100")
	c2 := EpicColor("PROJ-100")
	if fmt.Sprint(c1) != fmt.Sprint(c2) {
		t.Errorf("EpicColor not deterministic: %v != %v", c1, c2)
	}
}

func TestEpicColor_DifferentKeys(t *testing.T) {
	// Different keys should produce valid colors (not nil).
	keys := []string{"PROJ-1", "PROJ-2", "PROJ-3", "OTHER-99"}
	for _, key := range keys {
		got := EpicColor(key)
		if got == nil {
			t.Errorf("EpicColor(%q) returned nil", key)
		}
	}
}
