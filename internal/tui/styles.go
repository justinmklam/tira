package tui

import (
	"image/color"
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
)

// Shared terminal color constants used across all TUI views.
var (
	ColorSpinner color.Color = lipgloss.Color("12")

	ColorError            color.Color = lipgloss.Color("9")
	ColorSuccess          color.Color = lipgloss.Color("10")
	ColorWarning          color.Color = lipgloss.Color("11")
	ColorAccent           color.Color = lipgloss.Color("12")
	ColorSpecial          color.Color = lipgloss.Color("13")
	ColorCaution          color.Color = lipgloss.Color("208")
	ColorHighlight        color.Color = lipgloss.Color("15")
	ColorForeground       color.Color = lipgloss.Color("252")
	ColorForegroundBright color.Color = lipgloss.Color("255")
	ColorMuted            color.Color = lipgloss.Color("248")
	ColorSubtle           color.Color = lipgloss.Color("242")
	ColorSurface          color.Color = lipgloss.Color("237")

	ColorOnChrome color.Color = lipgloss.Color("235")

	ColorStatusTodo       color.Color = lipgloss.Color("245")
	ColorStatusInProgress color.Color = lipgloss.Color("214")
	ColorStatusDone       color.Color = lipgloss.Color("114")
	ColorStatusBlocked    color.Color = lipgloss.Color("203")

	ColorPriorityUrgent color.Color = lipgloss.Color("203")
	ColorPriorityHigh   color.Color = lipgloss.Color("208")
	ColorPriorityMedium color.Color = lipgloss.Color("214")
	ColorPriorityLow    color.Color = lipgloss.Color("75")
)

// Reusable styles shared across TUI views.
var (
	MutedStyle    = lipgloss.NewStyle().Foreground(ColorMuted)
	BoldAccent    = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	SurfaceBg     = lipgloss.NewStyle().Background(ColorSurface)
	OnChromeStyle = lipgloss.NewStyle().Foreground(ColorOnChrome)
)

// personPalette is the colour palette used by PersonColor. It is overwritten by
// SetTheme. It deliberately overlaps EpicPalette's values: the two index
// palettes of different lengths, so association is decorrelated by construction.
var personPalette = []color.Color{lipgloss.Color("39"), lipgloss.Color("141"), lipgloss.Color("43"), lipgloss.Color("203"), lipgloss.Color("45"), lipgloss.Color("220"), lipgloss.Color("214"), lipgloss.Color("208")}

// IssueTypeColor returns the terminal color for a given issue type.
func IssueTypeColor(issueType string) color.Color {
	switch strings.ToLower(issueType) {
	case "bug":
		return ColorError
	case "story":
		return ColorSuccess
	case "task":
		return ColorAccent
	case "epic":
		return ColorSpecial
	case "sub-task", "subtask":
		return ColorWarning
	default:
		return ColorMuted
	}
}

// statusKeywords maps a status class to the whole words that select it. Keeping
// them in a var means a misclassified custom status is fixed by adding a word,
// not by editing control flow.
var statusKeywords = map[string][]string{
	"blocked":    {"block", "blocked", "impeded"},
	"cancelled":  {"cancel", "cancelled", "wont", "duplicate", "rejected"},
	"inprogress": {"progress", "doing", "review", "testing", "qa", "dev", "active"},
	"todo":       {"todo", "open", "backlog", "new", "selected", "ready", "planned"},
	"done":       {"done", "closed", "complete", "completed", "resolved", "shipped", "merged"},
}

// priorityKeywords maps a priority class to the whole words that select it.
var priorityKeywords = map[string][]string{
	"urgent": {"highest", "urgent", "blocker", "critical", "p1"},
	"high":   {"high", "major", "p2"},
	"medium": {"medium", "normal", "p3", "default"},
	"low":    {"low", "lowest", "minor", "trivial", "p4", "p5"},
}

// normalizeWords lower-cases s, drops apostrophes so a contraction such as
// "Won't" matches the "wont" keyword, and splits on any non-alphanumeric rune.
// Matching is on whole words: substring matching would classify "Not Done" as
// todo via the "t do" fragment.
func normalizeWords(s string) []string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, "\u2019", "")
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// statusClass classifies a status name. Precedence is blocked, cancelled (done),
// in progress, to do, done, then unclassified.
func statusClass(name string) string {
	words := normalizeWords(name)
	has := func(key string) bool {
		for _, w := range words {
			for _, kw := range statusKeywords[key] {
				if w == kw {
					return true
				}
			}
		}
		return false
	}
	hasWord := func(target string) bool {
		for _, w := range words {
			if w == target {
				return true
			}
		}
		return false
	}

	switch {
	case has("blocked"):
		return "blocked"
	case has("cancelled"):
		return "done"
	case has("inprogress"):
		return "inprogress"
	case has("todo") || (hasWord("to") && hasWord("do")):
		return "todo"
	case has("done"):
		return "done"
	default:
		return "other"
	}
}

// StatusColor returns the semantic colour for a status name. It is nil only for
// an empty name; an unclassified status falls back to the muted colour, which is
// uninformative rather than misleading.
func StatusColor(name string) color.Color {
	if name == "" {
		return nil
	}
	switch statusClass(name) {
	case "blocked":
		return ColorStatusBlocked
	case "inprogress":
		return ColorStatusInProgress
	case "done":
		return ColorStatusDone
	case "todo":
		return ColorStatusTodo
	default:
		return ColorMuted
	}
}

// StatusGlyph returns the one-cell progress glyph for a status name: a "progress
// meter" that is readable without colour. The empty name yields the empty string.
func StatusGlyph(name string) string {
	if name == "" {
		return ""
	}
	switch statusClass(name) {
	case "blocked":
		return "⊘"
	case "inprogress":
		return "◐"
	case "done":
		return "●"
	case "todo":
		return "○"
	default:
		return "·"
	}
}

// priorityClass classifies a priority name by whole word.
func priorityClass(name string) string {
	words := normalizeWords(name)
	order := []string{"urgent", "high", "medium", "low"}
	for _, class := range order {
		for _, w := range words {
			for _, kw := range priorityKeywords[class] {
				if w == kw {
					return class
				}
			}
		}
	}
	return ""
}

// PriorityColor returns the semantic colour for a priority name, or nil when it
// is empty or unclassified.
func PriorityColor(name string) color.Color {
	switch priorityClass(name) {
	case "urgent":
		return ColorPriorityUrgent
	case "high":
		return ColorPriorityHigh
	case "medium":
		return ColorPriorityMedium
	case "low":
		return ColorPriorityLow
	default:
		return nil
	}
}

// PriorityGlyph returns the one-cell glyph for a priority name, or "" when it is
// empty or unclassified. Like StatusGlyph it is meaningful without colour.
func PriorityGlyph(name string) string {
	switch priorityClass(name) {
	case "urgent":
		return "⇈"
	case "high":
		return "↑"
	case "medium":
		return "·"
	case "low":
		return "↓"
	default:
		return ""
	}
}

// TypeGlyph returns the one-cell letter for an issue type. It never returns the
// empty string: an unknown type is the neutral "·".
func TypeGlyph(issueType string) string {
	switch strings.ToLower(issueType) {
	case "bug":
		return "B"
	case "story":
		return "S"
	case "task":
		return "T"
	case "epic":
		return "E"
	default:
		return "·"
	}
}

// PersonColor returns a consistent colour for an assignee name by hashing it
// into personPalette. It returns nil only for an empty name.
func PersonColor(name string) color.Color {
	if name == "" {
		return nil
	}
	if len(personPalette) == 0 {
		return ColorMuted
	}
	var sum int
	for _, r := range name {
		sum += int(r)
	}
	return personPalette[sum%len(personPalette)]
}

// Initials returns up to two upper-case initials for a display name: the first
// letter of the first and last whitespace-separated words.
func Initials(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	first := []rune(fields[0])
	if len(first) == 0 {
		return ""
	}
	if len(fields) == 1 {
		return strings.ToUpper(string(first[0]))
	}
	last := []rune(fields[len(fields)-1])
	if len(last) == 0 {
		return strings.ToUpper(string(first[0]))
	}
	return strings.ToUpper(string(first[0]) + string(last[0]))
}

// epicPalette is the color palette used by EpicColor and SprintColor. It is
// overwritten by SetTheme.
var epicPalette = []color.Color{lipgloss.Color("39"), lipgloss.Color("208"), lipgloss.Color("141"), lipgloss.Color("43"), lipgloss.Color("214"), lipgloss.Color("99"), lipgloss.Color("203"), lipgloss.Color("118"), lipgloss.Color("45"), lipgloss.Color("220")}

// EpicColor returns a consistent terminal color for an epic key by hashing it.
// Returns empty string for empty keys.
func EpicColor(epicKey string) color.Color {
	if epicKey == "" {
		return nil
	}
	var sum int
	for _, r := range epicKey {
		sum += int(r)
	}
	return epicPalette[sum%len(epicPalette)]
}

// SprintColor returns a deterministic palette color for a sprint's board-order
// index. Colors repeat only after the palette is exhausted.
func SprintColor(index int) color.Color {
	if index < 0 || len(epicPalette) == 0 {
		return ColorMuted
	}
	return epicPalette[index%len(epicPalette)]
}

// DaysInColumn calculates the number of days an issue has been in its current status.
// Returns 0 if the date string is empty or invalid.
func DaysInColumn(statusChangedDate string) int {
	if statusChangedDate == "" {
		return 0
	}
	parsed, err := parseDate(statusChangedDate)
	if err != nil {
		return 0
	}
	now := parseDateOrNow("")
	return int(now.Sub(parsed).Hours() / 24)
}

// DaysColor returns a color based on the number of days in column.
// Green: 0-2 days, Yellow: 3-5 days, Orange: 6-9 days, Red: 10+ days
func DaysColor(days int) color.Color {
	switch {
	case days <= 2:
		return ColorSuccess
	case days <= 5:
		return ColorWarning
	case days <= 9:
		return ColorCaution
	default:
		return ColorError
	}
}

// parseDate parses an ISO date string (YYYY-MM-DD) to time.Time in local timezone.
func parseDate(dateStr string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return t, err
	}
	// Convert to local timezone to match time.Now() behavior
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), nil
}

// parseDateOrNow parses an ISO date string or returns time.Now() if empty.
func parseDateOrNow(dateStr string) time.Time {
	if dateStr == "" {
		return time.Now()
	}
	t, err := parseDate(dateStr)
	if err != nil {
		return time.Now()
	}
	return t
}
