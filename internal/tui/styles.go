package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Shared terminal color constants used across all TUI views.
var (
	SpinnerColor = lipgloss.Color("12")

	ColorRed      = lipgloss.Color("9")
	ColorGreen    = lipgloss.Color("10")
	ColorYellow   = lipgloss.Color("11")
	ColorBlue     = lipgloss.Color("12")
	ColorMagenta  = lipgloss.Color("13")
	ColorOrange   = lipgloss.Color("208")
	ColorWhite    = lipgloss.Color("15")
	ColorFg       = lipgloss.Color("252")
	ColorFgBright = lipgloss.Color("255")
	ColorDim      = lipgloss.Color("244")
	ColorDimmer   = lipgloss.Color("240")
	ColorBg       = lipgloss.Color("237")
)

// Reusable styles shared across TUI views.
var (
	DimStyle   = lipgloss.NewStyle().Foreground(ColorDim)
	BoldBlue   = lipgloss.NewStyle().Bold(true).Foreground(ColorBlue)
	SelectedBg = lipgloss.NewStyle().Background(ColorBg)
)

// IssueTypeColor returns the terminal color for a given issue type.
func IssueTypeColor(issueType string) lipgloss.Color {
	switch strings.ToLower(issueType) {
	case "bug":
		return ColorRed
	case "story":
		return ColorGreen
	case "task":
		return ColorBlue
	case "epic":
		return ColorMagenta
	case "sub-task", "subtask":
		return ColorYellow
	default:
		return ColorDim
	}
}

// EpicColor returns a consistent terminal color for an epic key by hashing it.
// Returns empty string for empty keys.
func EpicColor(epicKey string) lipgloss.Color {
	if epicKey == "" {
		return ""
	}
	palette := []lipgloss.Color{"39", "208", "141", "43", "214", "99", "203", "118", "45", "220"}
	var sum int
	for _, r := range epicKey {
		sum += int(r)
	}
	return palette[sum%len(palette)]
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
func DaysColor(days int) lipgloss.Color {
	switch {
	case days <= 2:
		return ColorGreen
	case days <= 5:
		return ColorYellow
	case days <= 9:
		return ColorOrange
	default:
		return ColorRed
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
