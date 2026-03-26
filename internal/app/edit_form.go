package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
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
	emLabelW = 14 // visual width of the label column
	emInputW = 34 // visual width of single-line inputs (except summary)
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

	origAssignee   string
	origAssigneeID string

	initialState editFormState
	confirmAbort bool

	width     int
	height    int
	taHeight  int
	completed bool
	aborted   bool
	validErr  string

	wantAssigneePicker bool
	wantTypePicker     bool
	wantPriorityPicker bool
}

func newEditModel(issue *models.Issue, valid *models.ValidValues, width, height int) *editModel {
	m := &editModel{
		origAssignee:   issue.Assignee,
		origAssigneeID: issue.AssigneeID,
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
	return m
}

func (m *editModel) setSize(w, h int) {
	m.width = w
	m.height = h

	// Summary gets the full available width; other inputs use the fixed width.
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
			return m, nil

		case "tab":
			m.blurFocused()
			m.focused = (m.focused + 1) % efFieldCount
			m.focusFocused()
			return m, nil

		case "enter":
			if m.focused < efInputCount {
				// Assignee field: open picker instead of advancing.
				if m.focused == efAssignee {
					m.wantAssigneePicker = true
					return m, nil
				}
				// Type field: open option picker instead of advancing.
				if m.focused == efType {
					m.wantTypePicker = true
					return m, nil
				}
				// Priority field: open option picker instead of advancing.
				if m.focused == efPriority {
					m.wantPriorityPicker = true
					return m, nil
				}
				m.blurFocused()
				m.focused = (m.focused + 1) % efFieldCount
				m.focusFocused()
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
		m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
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

func (m *editModel) setAssignee(displayName, accountID string) {
	m.inputs[efAssignee].SetValue(displayName)
	m.origAssignee = displayName
	m.origAssigneeID = accountID
}

func (m *editModel) View() string {
	var lines []string

	for i := 0; i < efInputCount; i++ {
		label := tui.MutedStyle.Render(tui.FixedWidth(emFieldLabels[i], emLabelW))
		lines = append(lines, " "+label+" "+m.inputs[i].View())
	}

	lines = append(lines, "")
	lines = append(lines, " "+tui.MutedStyle.Render("Description"))
	lines = append(lines, strings.Split(m.descTA.View(), "\n")...)

	lines = append(lines, "")
	lines = append(lines, " "+tui.MutedStyle.Render("Acceptance Criteria"))
	lines = append(lines, strings.Split(m.acTA.View(), "\n")...)

	if m.validErr != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(tui.ColorError).Render("  "+m.validErr))
	}

	lines = append(lines, "")
	if m.confirmAbort {
		msg := lipgloss.NewStyle().Foreground(tui.ColorError).Bold(true).Render("  Discard unsaved changes? (y/n)")
		lines = append(lines, msg)
	} else {
		lines = append(lines, tui.MutedStyle.Render("  enter: open picker / next  tab: next  shift+tab: back  ctrl+s: save  esc: cancel"))
	}

	return strings.Join(lines, "\n") + "\n"
}
