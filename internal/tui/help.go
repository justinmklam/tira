package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	Width        int
	Height       int
	ScrollOffset int // Current scroll position
}

// NewHelpModel creates a new help model.
func NewHelpModel() HelpModel {
	return HelpModel{}
}

// HelpSections returns all help sections for the backlog and kanban views.
func HelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Navigation (Backlog)",
			Keybindings: []HelpKeybinding{
				{Key: "j / k", Description: "Move down / up within a sprint"},
				{Key: "J / }", Description: "Jump to next sprint header"},
				{Key: "K / {", Description: "Jump to previous sprint header"},
				{Key: "g / G", Description: "Jump to first / last ticket in the list"},
				{Key: "z", Description: "Toggle collapse current sprint"},
				{Key: "Z", Description: "Toggle collapse all sprints"},
				{Key: "/", Description: "Filter tickets (fuzzy search by summary or key)"},
				{Key: "Enter", Description: "Toggle expand/collapse sprint or open ticket detail"},
				{Key: "Esc", Description: "Clear filter / cancel current action / clear selection"},
			},
		},
		{
			Title: "Issue Details Sidebar (Backlog)",
			Keybindings: []HelpKeybinding{
				{Key: "ctrl+d / ctrl+u", Description: "Scroll details down / up by 1/4 page"},
			},
		},
		{
			Title: "Navigation (Kanban)",
			Keybindings: []HelpKeybinding{
				{Key: "h / j / k / l", Description: "Move left / down / up / right between columns and issues"},
				{Key: "Enter", Description: "Open ticket detail pane"},
				{Key: "Esc", Description: "Close detail pane / cancel action"},
			},
		},
		{
			Title: "Selection (Backlog)",
			Keybindings: []HelpKeybinding{
				{Key: "Space", Description: "Toggle select ticket under cursor"},
				{Key: "v", Description: "Enter visual mode — extend with j/k, confirm with Enter"},
				{Key: "Esc", Description: "Clear all selections (when not in visual mode)"},
			},
		},
		{
			Title: "Moving Tickets (Backlog)",
			Keybindings: []HelpKeybinding{
				{Key: "C-j / C-k", Description: "Move ticket one position down / up within its sprint"},
				{Key: "> / <", Description: "Move ticket to next / previous sprint directly"},
				{Key: "B", Description: "Move ticket to backlog (no sprint)"},
				{Key: "x", Description: "Cut selected ticket(s) for move"},
				{Key: "p", Description: "Paste cut ticket(s) to current sprint"},
			},
		},
		{
			Title: "Editing (Backlog)",
			Keybindings: []HelpKeybinding{
				{Key: "e", Description: "Edit ticket in $EDITOR (full template flow)"},
				{Key: "c", Description: "Add comment — inline text input (ctrl+s to save, esc to cancel)"},
				{Key: "S", Description: "Set story points — inline numeric input"},
				{Key: "s", Description: "Change status — picker"},
				{Key: "a", Description: "Create new ticket in current sprint"},
				{Key: "C", Description: "Create new ticket in backlog"},
				{Key: "P", Description: "Set parent — fuzzy picker (works on selection or cursor ticket)"},
				{Key: "A", Description: "Set assignee — fuzzy picker (works on selection or cursor ticket)"},
				{Key: "F", Description: "Filter by epic — fuzzy picker"},
				{Key: "o", Description: "Open ticket in browser"},
				{Key: "O", Description: "Open all selected tickets in browser"},
				{Key: "y", Description: "Copy ticket URL to clipboard (cursor issue)"},
				{Key: "C-n", Description: "Create new sprint — form with name, start date, duration"},
				{Key: "E", Description: "Edit sprint under cursor (name, start date, duration)"},
			},
		},
		{
			Title: "Editing (Kanban)",
			Keybindings: []HelpKeybinding{
				{Key: "e", Description: "Edit ticket in $EDITOR (full template flow)"},
				{Key: "c", Description: "Add comment — inline text input (ctrl+s to save, esc to cancel)"},
				{Key: "s", Description: "Change status — picker"},
				{Key: "A", Description: "Set assignee — fuzzy picker"},
				{Key: "o", Description: "Open ticket in browser"},
			},
		},
		{
			Title: "View",
			Keybindings: []HelpKeybinding{
				{Key: "1", Description: "Switch to backlog view"},
				{Key: "2", Description: "Switch to kanban board view"},
				{Key: "Tab", Description: "Toggle between backlog and board"},
				{Key: "R", Description: "Refresh from Jira API"},
				{Key: "?", Description: "Show this help overlay"},
				{Key: "q", Description: "Quit"},
			},
		},
	}
}

// getContentLines returns the help content as a slice of lines.
func (m HelpModel) getContentLines(innerW int) []string {
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

	return lines
}

// Init initializes the help model.
func (m HelpModel) Init() tea.Cmd {
	return nil
}

// Update handles scrolling within the help overlay.
func (m HelpModel) Update(msg tea.Msg, innerH int) HelpModel {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m
	}

	// Get total content height
	lines := m.getContentLines(m.Width)
	totalLines := len(lines)

	switch key.String() {
	case "j", "down":
		if m.ScrollOffset < totalLines-innerH {
			m.ScrollOffset++
		}
	case "k", "up":
		if m.ScrollOffset > 0 {
			m.ScrollOffset--
		}
	case "ctrl+d":
		halfPage := innerH / 2
		if halfPage < 1 {
			halfPage = 1
		}
		m.ScrollOffset += halfPage
		if m.ScrollOffset > totalLines-innerH {
			m.ScrollOffset = totalLines - innerH
		}
	case "ctrl+u":
		halfPage := innerH / 2
		if halfPage < 1 {
			halfPage = 1
		}
		m.ScrollOffset -= halfPage
		if m.ScrollOffset < 0 {
			m.ScrollOffset = 0
		}
	case "g":
		m.ScrollOffset = 0
	case "G":
		m.ScrollOffset = totalLines - innerH
		if m.ScrollOffset < 0 {
			m.ScrollOffset = 0
		}
	}

	// Clamp scroll offset
	if m.ScrollOffset < 0 {
		m.ScrollOffset = 0
	}
	if m.ScrollOffset > totalLines-innerH {
		m.ScrollOffset = totalLines - innerH
	}

	return m
}

// View renders the help overlay content (without border).
func (m HelpModel) View(innerW, innerH int) string {
	lines := m.getContentLines(innerW)
	totalLines := len(lines)

	// Apply scroll offset
	start := m.ScrollOffset
	if start < 0 {
		start = 0
	}
	end := start + innerH
	if end > totalLines {
		end = totalLines
	}

	// Get visible lines
	visibleLines := lines[start:end]

	return strings.Join(visibleLines, "\n")
}

// HelpOverlaySize returns the dimensions for the floating help overlay.
func HelpOverlaySize(totalWidth, totalHeight int) (w, h int) {
	// Use ~70% of screen width, ~90% of height for more vertical space
	w = totalWidth * 70 / 100
	if w > 100 {
		w = 100
	}
	if w < 60 {
		w = 60
	}

	h = totalHeight * 90 / 100
	if h > 50 {
		h = 50
	}
	if h < 25 {
		h = 25
	}

	return w, h
}
