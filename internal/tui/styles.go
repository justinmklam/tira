package tui

import (
	"image/color"
	"strings"
	"time"

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
	ColorMuted            color.Color = lipgloss.Color("244")
	ColorSubtle           color.Color = lipgloss.Color("240")
	ColorSurface          color.Color = lipgloss.Color("237")
)

// Reusable styles shared across TUI views.
var (
	MutedStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	BoldAccent = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	SurfaceBg  = lipgloss.NewStyle().Background(ColorSurface)
)

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

// epicPalette is the color palette used by EpicColor. It is overwritten by SetTheme.
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
