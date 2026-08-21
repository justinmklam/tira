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

func TestSprintColor_DeterministicAndIndexed(t *testing.T) {
	if got := fmt.Sprint(SprintColor(0)); got != fmt.Sprint(SprintColor(0)) {
		t.Fatalf("SprintColor is not deterministic: %q", got)
	}
	if fmt.Sprint(SprintColor(0)) == fmt.Sprint(SprintColor(1)) {
		t.Fatal("adjacent sprint indexes should use distinct palette colors")
	}
	if fmt.Sprint(SprintColor(-1)) != fmt.Sprint(ColorMuted) {
		t.Fatalf("negative sprint index should use muted color, got %v", SprintColor(-1))
	}
}
