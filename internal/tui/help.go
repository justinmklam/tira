package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HelpSection represents a section of keybindings in the help overlay.
type HelpSection struct {
	Title       string
	Keybindings []HelpKeybinding
}

// HelpKeybinding represents a single keybinding entry.
type HelpKeybinding struct {
	Key         string
	Description string
}

// HelpModel holds the state for the help overlay.
type HelpModel struct {
	Width  int
	Height int
}

// NewHelpModel creates a new help model.
func NewHelpModel() HelpModel {
	return HelpModel{}
}

// HelpSections returns all help sections for the backlog and kanban views.
func HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Navigation",
			Keybindings: []HelpKeybinding{
				{Key: "j / k", Description: "Move down / up within a sprint"},
				{Key: "J / K", Description: "Jump to next / previous sprint header"},
				{Key: "g / G", Description: "Jump to first / last ticket in the list"},
				{Key: "{ / }", Description: "Previous / next sprint"},
				{Key: "C-d / C-u", Description: "Half-page down / up"},
				{Key: "z", Description: "Toggle collapse current sprint"},
				{Key: "Z", Description: "Toggle collapse all sprints"},
				{Key: "/", Description: "Filter tickets (fuzzy search)"},
				{Key: "n / N", Description: "Next / previous filter match"},
				{Key: "Esc", Description: "Clear filter / cancel current action"},
			},
		},
		{
			Title: "Selection",
			Keybindings: []HelpKeybinding{
				{Key: "Space", Description: "Toggle select ticket under cursor"},
				{Key: "v", Description: "Enter visual mode — extend with j/k, confirm with Enter"},
				{Key: "V", Description: "Select all tickets in current sprint"},
				{Key: "*", Description: "Invert selection across all sprints"},
				{Key: "Esc", Description: "Clear all selections"},
			},
		},
		{
			Title: "Moving Tickets",
			Keybindings: []HelpKeybinding{
				{Key: "m", Description: "Move selected ticket(s) to sprint"},
				{Key: "C-j / C-k", Description: "Move ticket one position down / up"},
				{Key: "> / <", Description: "Move ticket to next / previous sprint"},
				{Key: "B", Description: "Move ticket to backlog (no sprint)"},
			},
		},
		{
			Title: "Editing",
			Keybindings: []HelpKeybinding{
				{Key: "e", Description: "Edit ticket in $EDITOR (full template flow)"},
				{Key: "r", Description: "Rename — inline edit of summary only"},
				{Key: "t", Description: "Change type — picker"},
				{Key: "p", Description: "Change priority — picker"},
				{Key: "s", Description: "Set story points — inline numeric input"},
				{Key: "l", Description: "Edit labels — inline comma-separated input"},
				{Key: "a", Description: "Create new ticket in current sprint"},
				{Key: "C", Description: "Create new ticket in backlog"},
				{Key: "P", Description: "Set parent — fuzzy picker"},
				{Key: "A", Description: "Set assignee — fuzzy picker"},
				{Key: "x", Description: "Delete ticket — requires y confirmation"},
				{Key: "Enter", Description: "Open ticket detail pane"},
				{Key: "o", Description: "Open ticket in browser"},
				{Key: "O", Description: "Open ticket in browser (canonical URL)"},
				{Key: "y", Description: "Copy ticket URL to clipboard"},
			},
		},
		{
			Title: "View",
			Keybindings: []HelpKeybinding{
				{Key: "1", Description: "Switch to backlog view"},
				{Key: "2", Description: "Switch to kanban board view"},
				{Key: "Tab", Description: "Toggle between backlog and board"},
				{Key: "f", Description: "Cycle filter presets: all → mine → unassigned"},
				{Key: "S", Description: "Cycle sort: default → priority → points → assignee"},
				{Key: "R", Description: "Refresh from Jira API"},
				{Key: "?", Description: "Show this help overlay"},
				{Key: "q", Description: "Quit"},
			},
		},
	}
}

// View renders the help overlay content (without border).
func (m HelpModel) View(innerW, innerH int) string {
	sections := HelpSections()
	var lines []string

	// Title
	title := BoldBlue.Render("Keybindings Help")
	lines = append(lines, title)
	lines = append(lines, "")

	// Render each section
	for _, section := range sections {
		// Section title
		sectionTitle := lipgloss.NewStyle().Bold(true).Foreground(ColorMagenta).Render(section.Title)
		lines = append(lines, sectionTitle)

		// Keybindings
		maxKeyW := 0
		for _, kb := range section.Keybindings {
			if len(kb.Key) > maxKeyW {
				maxKeyW = len(kb.Key)
			}
		}

		for _, kb := range section.Keybindings {
			keyStr := FixedWidth(kb.Key, maxKeyW)
			keyStyled := lipgloss.NewStyle().Foreground(ColorYellow).Render(keyStr)
			line := keyStyled + "  " + kb.Description
			lines = append(lines, line)
		}

		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")

	// Handle scrolling if content is taller than available height
	contentLines := strings.Split(content, "\n")
	if len(contentLines) > innerH {
		// Show as much as fits
		if len(contentLines) > innerH {
			contentLines = contentLines[:innerH]
		}
	}

	return strings.Join(contentLines, "\n")
}

// HelpOverlaySize returns the dimensions for the floating help overlay.
func HelpOverlaySize(totalWidth, totalHeight int) (w, h int) {
	// Use ~70% of screen width, ~80% of height
	w = totalWidth * 70 / 100
	if w > 100 {
		w = 100
	}
	if w < 60 {
		w = 60
	}

	h = totalHeight * 80 / 100
	if h > 40 {
		h = 40
	}
	if h < 20 {
		h = 20
	}

	return w, h
}
