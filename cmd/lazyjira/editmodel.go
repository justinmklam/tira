package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/justinmklam/lazyjira/internal/tui"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

// Field indices.
const (
	efSummary     = 0
	efType        = 1
	efPriority    = 2
	efAssignee    = 3
	efStoryPoints = 4
	efLabels      = 5
	efDescription = 6
	efAccCriteria = 7
	efFieldCount  = 8
	efInputCount  = 6 // fields 0-5 use textinput.Model
)

// Layout constants.
const (
	emLabelW   = 14 // visual width of the label column
	emInputW   = 34 // visual width of single-line inputs (except summary)
	emPanelGap = 2  // horizontal gap between input and suggestion panel
)

var emFieldLabels = [efFieldCount]string{
	"Summary", "Type", "Priority", "Assignee",
	"Story Points", "Labels", "Description", "Acceptance Criteria",
}

type editModel struct {
	inputs [efInputCount]textinput.Model
	descTA textarea.Model
	acTA   textarea.Model

	focused int

	typeOpts     []string
	priorityOpts []string
	suggestions  []string
	suggCursor   int

	origAssignee   string
	origAssigneeID string

	initialState editFormState
	confirmAbort bool

	width    int
	height   int
	taHeight int
	completed bool
	aborted   bool
	validErr  string
}

func newEditModel(issue *models.Issue, valid *models.ValidValues, width, height int) *editModel {
	m := &editModel{
		typeOpts:       valid.IssueTypes,
		priorityOpts:   valid.Priorities,
		origAssignee:   issue.Assignee,
		origAssigneeID: issue.AssigneeID,
		suggCursor:     -1,
	}

	placeholders := [efInputCount]string{
		"", "", "", "display name or blank", "number or blank", "comma-separated",
	}
	for i := range m.inputs {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Width = emInputW
		ti.Placeholder = placeholders[i]
		m.inputs[i] = ti
	}

	sp := ""
	if issue.StoryPoints > 0 {
		sp = fmt.Sprintf("%.0f", issue.StoryPoints)
	}
	m.inputs[efSummary].SetValue(issue.Summary)
	m.inputs[efType].SetValue(issue.IssueType)
	m.inputs[efPriority].SetValue(issue.Priority)
	m.inputs[efAssignee].SetValue(issue.Assignee)
	m.inputs[efStoryPoints].SetValue(sp)
	m.inputs[efLabels].SetValue(strings.Join(issue.Labels, ", "))

	m.descTA = textarea.New()
	m.descTA.SetValue(issue.Description)
	m.descTA.ShowLineNumbers = false

	m.acTA = textarea.New()
	m.acTA.SetValue(issue.AcceptanceCriteria)
	m.acTA.ShowLineNumbers = false

	m.initialState = m.currentState()

	m.setSize(width, height)
	m.inputs[0].Focus()
	m.refreshSuggestions()
	return m
}

func (m *editModel) setSize(w, h int) {
	m.width = w
	m.height = h

	// Summary gets the full available width; other inputs use the fixed width
	// so the suggestion panel has room to appear to their right.
	summaryW := w - emLabelW - 2
	if summaryW < 20 {
		summaryW = 20
	}
	m.inputs[efSummary].Width = summaryW
	for i := 1; i < efInputCount; i++ {
		m.inputs[i].Width = emInputW
	}

	taW := max(w-4, 10)
	m.descTA.SetWidth(taW)
	m.acTA.SetWidth(taW)

	// Compute textarea height from available space.
	// Fixed rows: 6 inputs + 2 section labels + 2 blank separators + 1 hint + 1 blank before hint ≈ 13
	const overhead = 13
	taH := (h - overhead) / 2
	if taH < 4 {
		taH = 4
	}
	if taH > 14 {
		taH = 14
	}
	m.taHeight = taH
	m.descTA.SetHeight(taH)
	m.acTA.SetHeight(taH)
}

func (m *editModel) Init() tea.Cmd { return textinput.Blink }

func (m *editModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.confirmAbort {
			switch key.String() {
			case "y", "enter":
				m.aborted = true
				return m, nil
			case "n", "esc":
				m.confirmAbort = false
				return m, nil
			}
			return m, nil
		}

		switch key.String() {
		case "esc":
			if m.isDirty() {
				m.confirmAbort = true
			} else {
				m.aborted = true
			}
			return m, nil

		case "shift+tab":
			m.blurFocused()
			if m.focused == 0 {
				m.focused = efFieldCount - 1
			} else {
				m.focused--
			}
			m.focusFocused()
			m.refreshSuggestions()
			return m, nil

		case "tab":
			// Accept highlighted suggestion (or first) if any.
			if m.hasSuggestions() && len(m.suggestions) > 0 {
				idx := m.suggCursor
				if idx < 0 {
					idx = 0
				}
				val := m.suggestions[idx]
				if m.inputs[m.focused].Value() != val {
					m.inputs[m.focused].SetValue(val)
					m.suggCursor = -1
					m.refreshSuggestions()
					return m, nil
				}
			}
			m.blurFocused()
			m.focused = (m.focused + 1) % efFieldCount
			m.focusFocused()
			m.refreshSuggestions()
			return m, nil

		case "up":
			if m.hasSuggestions() && len(m.suggestions) > 0 {
				if m.suggCursor <= 0 {
					m.suggCursor = len(m.suggestions) - 1
				} else {
					m.suggCursor--
				}
				return m, nil
			}

		case "down":
			if m.hasSuggestions() && len(m.suggestions) > 0 {
				if m.suggCursor < 0 {
					m.suggCursor = 0
				} else {
					m.suggCursor = (m.suggCursor + 1) % len(m.suggestions)
				}
				return m, nil
			}

		case "enter":
			if m.focused < efInputCount {
				// Accept highlighted suggestion; otherwise advance field.
				if m.hasSuggestions() && m.suggCursor >= 0 && m.suggCursor < len(m.suggestions) {
					m.inputs[m.focused].SetValue(m.suggestions[m.suggCursor])
					m.suggCursor = -1
					m.refreshSuggestions()
				} else {
					m.blurFocused()
					m.focused = (m.focused + 1) % efFieldCount
					m.focusFocused()
					m.refreshSuggestions()
				}
				return m, nil
			}
			// Textareas: fall through so enter inserts a newline.

		case "ctrl+s":
			if errMsg := m.validate(); errMsg != "" {
				m.validErr = errMsg
				return m, nil
			}
			m.completed = true
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.focused < efInputCount {
		prev := m.inputs[m.focused].Value()
		m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
		if m.inputs[m.focused].Value() != prev {
			m.refreshSuggestions()
		}
	} else if m.focused == efDescription {
		m.descTA, cmd = m.descTA.Update(msg)
	} else {
		m.acTA, cmd = m.acTA.Update(msg)
	}
	return m, cmd
}

func (m *editModel) blurFocused() {
	if m.focused < efInputCount {
		m.inputs[m.focused].Blur()
	} else if m.focused == efDescription {
		m.descTA.Blur()
	} else {
		m.acTA.Blur()
	}
	m.suggestions = nil
	m.suggCursor = -1
}

func (m *editModel) focusFocused() {
	if m.focused < efInputCount {
		m.inputs[m.focused].Focus()
	} else if m.focused == efDescription {
		m.descTA.Focus()
	} else {
		m.acTA.Focus()
	}
}

func (m *editModel) hasSuggestions() bool {
	return m.focused == efType || m.focused == efPriority
}

func (m *editModel) refreshSuggestions() {
	if !m.hasSuggestions() {
		m.suggestions = nil
		m.suggCursor = -1
		return
	}
	var opts []string
	switch m.focused {
	case efType:
		opts = m.typeOpts
	case efPriority:
		opts = m.priorityOpts
	}
	query := m.inputs[m.focused].Value()
	if query == "" {
		m.suggestions = make([]string, len(opts))
		copy(m.suggestions, opts)
		m.suggCursor = -1
	} else {
		ranks := fuzzy.RankFindFold(query, opts)
		sort.Sort(ranks)
		m.suggestions = make([]string, len(ranks))
		for i, r := range ranks {
			m.suggestions[i] = r.Target
		}
		if m.suggCursor >= len(m.suggestions) {
			m.suggCursor = len(m.suggestions) - 1
		}
	}
}

func (m *editModel) isDirty() bool {
	curr := m.currentState()
	init := m.initialState
	return curr.summary != init.summary ||
		curr.issueType != init.issueType ||
		curr.priority != init.priority ||
		curr.assignee != init.assignee ||
		curr.storyPoints != init.storyPoints ||
		curr.labels != init.labels ||
		curr.description != init.description ||
		curr.acceptanceCriteria != init.acceptanceCriteria
}

func (m *editModel) validate() string {
	if strings.TrimSpace(m.inputs[efSummary].Value()) == "" {
		return "Summary cannot be empty"
	}
	if s := strings.TrimSpace(m.inputs[efStoryPoints].Value()); s != "" {
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			return "Story Points must be a number"
		}
	}
	return ""
}

func (m *editModel) currentState() editFormState {
	return editFormState{
		summary:            m.inputs[efSummary].Value(),
		issueType:          m.inputs[efType].Value(),
		priority:           m.inputs[efPriority].Value(),
		assignee:           m.inputs[efAssignee].Value(),
		origAssignee:       m.origAssignee,
		origAssigneeID:     m.origAssigneeID,
		storyPoints:        m.inputs[efStoryPoints].Value(),
		labels:             m.inputs[efLabels].Value(),
		description:        m.descTA.Value(),
		acceptanceCriteria: m.acTA.Value(),
	}
}

func (m *editModel) View() string {
	// Width available for the suggestion panel (to the right of label+input).
	panelW := m.width - emLabelW - 1 - emInputW - emPanelGap
	if panelW < 16 {
		panelW = 0
	}

	// inputRowVisualW is used to right-pad rows before attaching the panel.
	// Computed from the fixed-width inputs (not summary) since the panel only
	// appears for Type, Priority, and Assignee.
	inputRowVisualW := 1 + emLabelW + 1 + emInputW

	// Build all content lines into a single slice so the suggestion panel can
	// extend beyond the 6 input rows if needed.
	var lines []string

	for i := 0; i < efInputCount; i++ {
		label := tui.DimStyle.Render(tui.FixedWidth(emFieldLabels[i], emLabelW))
		lines = append(lines, " "+label+" "+m.inputs[i].View())
	}

	lines = append(lines, "")
	lines = append(lines, " "+tui.DimStyle.Render("Description"))
	lines = append(lines, strings.Split(m.descTA.View(), "\n")...)

	lines = append(lines, "")
	lines = append(lines, " "+tui.DimStyle.Render("Acceptance Criteria"))
	lines = append(lines, strings.Split(m.acTA.View(), "\n")...)

	if m.validErr != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(tui.ColorRed).Render("  "+m.validErr))
	}

	lines = append(lines, "")
	if m.confirmAbort {
		msg := lipgloss.NewStyle().Foreground(tui.ColorRed).Bold(true).Render("  Discard unsaved changes? (y/n)")
		lines = append(lines, msg)
	} else {
		lines = append(lines, tui.DimStyle.Render("  tab: next/complete  shift+tab: back  ↑↓: select  ctrl+s: save  esc: cancel"))
	}

	// Overlay suggestion panel starting at the focused row.
	if !m.confirmAbort && m.hasSuggestions() && len(m.suggestions) > 0 && panelW > 0 {
		show := min(10, len(m.suggestions))
		innerW := panelW - 2 // content width inside border

		inner := make([]string, show)
		for i := 0; i < show; i++ {
			label := tui.FixedWidth(m.suggestions[i], innerW-2) // -2 for "▶ " / "  "
			if i == m.suggCursor {
				inner[i] = lipgloss.NewStyle().Foreground(tui.ColorBlue).Bold(true).Render("▶ " + label)
			} else {
				inner[i] = tui.DimStyle.Render("  " + label)
			}
		}
		panel := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorBlue).
			Width(innerW).
			Render(strings.Join(inner, "\n"))

		panelLines := strings.Split(panel, "\n")
		for i, pl := range panelLines {
			rowIdx := m.focused + i
			if rowIdx < len(lines) {
				lines[rowIdx] = emPadToVisual(lines[rowIdx], inputRowVisualW+emPanelGap) + pl
			}
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

// emPadToVisual pads s with spaces until its visual width reaches w.
func emPadToVisual(s string, w int) string {
	cur := lipgloss.Width(s)
	if cur >= w {
		return s
	}
	return s + strings.Repeat(" ", w-cur)
}
