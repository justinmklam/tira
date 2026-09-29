package tui

import (
	"fmt"
	"image/color"
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

func TestStatusColor(t *testing.T) {
	tests := []struct {
		input string
		want  color.Color
	}{
		{"To Do", ColorStatusTodo},
		{"In Progress", ColorStatusInProgress},
		{"Done", ColorStatusDone},
		{"Blocked", ColorStatusBlocked},
		{"In Review", ColorStatusInProgress},
		{"Not Done", ColorStatusDone},
		{"Won't Do", ColorStatusDone},
		{"Underway", ColorMuted},
		{"", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := StatusColor(tt.input)
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("StatusColor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStatusColorDistinctAndGlyphs(t *testing.T) {
	seen := map[string]bool{}
	glyphs := map[string]bool{}
	for _, name := range []string{"To Do", "In Progress", "Done"} {
		c := StatusColor(name)
		if c == nil {
			t.Fatalf("StatusColor(%q) is nil", name)
		}
		key := fmt.Sprint(c)
		if seen[key] {
			t.Errorf("StatusColor(%q) shares a colour with an earlier status", name)
		}
		seen[key] = true
		g := StatusGlyph(name)
		if glyphs[g] {
			t.Errorf("StatusGlyph(%q) = %q duplicates an earlier status glyph", name, g)
		}
		glyphs[g] = true
	}
}

func TestPriorityColor(t *testing.T) {
	tests := []struct {
		input string
		want  color.Color
	}{
		{"Urgent", ColorPriorityUrgent},
		{"High", ColorPriorityHigh},
		{"Medium", ColorPriorityMedium},
		{"Low", ColorPriorityLow},
		{"Highest", ColorPriorityUrgent},
		{"", nil},
		{"Whatever", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := PriorityColor(tt.input)
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("PriorityColor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestPersonColorDeterministic(t *testing.T) {
	if PersonColor("") != nil {
		t.Error("PersonColor(\"\") should be nil")
	}
	first := PersonColor("Ada Lovelace")
	second := PersonColor("Ada Lovelace")
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Error("PersonColor is not deterministic")
	}
	if PersonColor("Ada Lovelace") == nil {
		t.Error("PersonColor should not be nil for a non-empty name")
	}
}

func TestInitials(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Ada Lovelace", "AL"},
		{"Grace", "G"},
		{"Ada B. Lovelace", "AL"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		if got := Initials(tt.input); got != tt.want {
			t.Errorf("Initials(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGlyphsAreOneCell(t *testing.T) {
	var glyphs []string
	for _, name := range []string{"To Do", "In Progress", "Done", "Blocked", "Underway"} {
		glyphs = append(glyphs, StatusGlyph(name))
	}
	for _, name := range []string{"Urgent", "High", "Medium", "Low", "Whatever"} {
		glyphs = append(glyphs, PriorityGlyph(name))
	}
	for _, name := range []string{"Bug", "Story", "Task", "Epic", "Sub-task", "Other"} {
		glyphs = append(glyphs, TypeGlyph(name))
	}
	for _, g := range glyphs {
		if g == "" {
			continue
		}
		if got := DisplayWidth(g); got != 1 {
			t.Errorf("glyph %q has display width %d, want 1", g, got)
		}
	}
	if got := TypeGlyph("something-unknown"); got == "" {
		t.Error("TypeGlyph must never return the empty string")
	}
	if got := StatusGlyph(""); got != "" {
		t.Errorf("StatusGlyph(\"\") = %q, want \"\"", got)
	}
	if got := PriorityGlyph(""); got != "" {
		t.Errorf("PriorityGlyph(\"\") = %q, want \"\"", got)
	}
}
