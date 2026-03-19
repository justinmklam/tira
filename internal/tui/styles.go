package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shared terminal color constants used across all TUI views.
var (
	SpinnerColor = lipgloss.Color("12")

	ColorRed     = lipgloss.Color("9")
	ColorGreen   = lipgloss.Color("10")
	ColorYellow  = lipgloss.Color("11")
	ColorBlue    = lipgloss.Color("12")
	ColorMagenta = lipgloss.Color("13")
	ColorOrange  = lipgloss.Color("208")
	ColorWhite   = lipgloss.Color("15")
	ColorFg      = lipgloss.Color("252")
	ColorFgBright = lipgloss.Color("255")
	ColorDim     = lipgloss.Color("244")
	ColorDimmer  = lipgloss.Color("240")
	ColorBg      = lipgloss.Color("237")
)

// Reusable styles shared across TUI views.
var (
	DimStyle     = lipgloss.NewStyle().Foreground(ColorDim)
	BoldBlue     = lipgloss.NewStyle().Bold(true).Foreground(ColorBlue)
	SelectedBg   = lipgloss.NewStyle().Background(ColorBg)
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
