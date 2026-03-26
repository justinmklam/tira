package tui

import (
	"testing"
)

func TestIssueTypeColor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Bug", string(ColorError)},
		{"bug", string(ColorError)},
		{"Story", string(ColorSuccess)},
		{"Task", string(ColorAccent)},
		{"Epic", string(ColorSpecial)},
		{"Sub-task", string(ColorWarning)},
		{"subtask", string(ColorWarning)},
		{"Unknown", string(ColorMuted)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(IssueTypeColor(tt.input))
			if got != tt.want {
				t.Errorf("IssueTypeColor(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEpicColor_Empty(t *testing.T) {
	got := EpicColor("")
	if got != "" {
		t.Errorf("EpicColor(\"\") = %q, want empty", got)
	}
}

func TestEpicColor_Deterministic(t *testing.T) {
	c1 := EpicColor("PROJ-100")
	c2 := EpicColor("PROJ-100")
	if c1 != c2 {
		t.Errorf("EpicColor not deterministic: %q != %q", c1, c2)
	}
}

func TestEpicColor_DifferentKeys(t *testing.T) {
	// Different keys should produce valid colors (not empty).
	keys := []string{"PROJ-1", "PROJ-2", "PROJ-3", "OTHER-99"}
	for _, key := range keys {
		got := EpicColor(key)
		if got == "" {
			t.Errorf("EpicColor(%q) returned empty", key)
		}
	}
}
