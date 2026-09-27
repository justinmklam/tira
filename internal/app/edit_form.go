package app

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		ti.SetWidth(emInputW)
		ti.Placeholder = placeholders[i]
		styles := ti.Styles()
		styles.Focused.Placeholder = tui.MutedStyle.Italic(true)
		styles.Blurred.Placeholder = tui.MutedStyle.Italic(true)
		ti.SetStyles(styles)
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
	// Prompt must be cleared before setSize: the textarea memoises promptWidth
	// at SetWidth time, so the default "┃ " would otherwise leave a two-cell
	// inset behind on every line.
	m.descTA.Prompt = ""
	m.descTA.Placeholder = "Write a description…"
	m.descTA.ShowLineNumbers = false
	m.descTA.SetValue(issue.Description)
	descStyles := m.descTA.Styles()
	descStyles.Focused.Placeholder = tui.MutedStyle.Italic(true)
	descStyles.Blurred.Placeholder = tui.MutedStyle.Italic(true)
	m.descTA.SetStyles(descStyles)

	m.acTA = textarea.New()
	m.acTA.Prompt = ""
	m.acTA.Placeholder = "Add acceptance criteria…"
	m.acTA.ShowLineNumbers = false
	m.acTA.SetValue(issue.AcceptanceCriteria)
	acStyles := m.acTA.Styles()
	acStyles.Focused.Placeholder = tui.MutedStyle.Italic(true)
	acStyles.Blurred.Placeholder = tui.MutedStyle.Italic(true)
	m.acTA.SetStyles(acStyles)

	m.initialState = m.currentState()

	m.setSize(width, height)
	m.inputs[0].Focus()
	return m
}

func (m *editModel) setSize(w, h int) {
	m.width = w
	m.height = h

	// Summary gets the full available width; other inputs use the fixed width.
	// textinput.View reserves one cell for the cursor beyond SetWidth, so the
	// summary's editable width is one short of its slot; otherwise the cursor
	// would spill past the form's right edge.
	summaryW := w - emLabelW - 2
	if summaryW < 20 {
		summaryW = 20
	}
	m.inputs[efSummary].SetWidth(summaryW - 1)
	for i := 1; i < efInputCount; i++ {
		m.inputs[i].SetWidth(emInputW)
	}

	// The textarea's SetWidth counts the prompt inside its total width. With the
	// prompt cleared, w-1 leaves exactly w cells once View indents the block by
	// one cell, matching the field rows and the section headings.
	taW := max(w-1, 10)
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

// fieldGlyph returns the semantic one-cell glyph and its colour for the fields
// that carry one (type, priority, assignee). A field without a glyph returns the
// empty string, so its label keeps the plain muted treatment.
func (m *editModel) fieldGlyph(i int) (string, color.Color) {
	switch i {
	case efType:
		v := m.inputs[efType].Value()
		if c := tui.IssueTypeColor(v); c != nil {
			return tui.TypeGlyph(v), c
		}
		return tui.TypeGlyph(v), tui.ColorMuted
	case efPriority:
		v := m.inputs[efPriority].Value()
		g := tui.PriorityGlyph(v)
		if g == "" {
			return "", tui.ColorMuted
		}
		if c := tui.PriorityColor(v); c != nil {
			return g, c
		}
		return g, tui.ColorMuted
	case efAssignee:
		v := m.inputs[efAssignee].Value()
		if v == "" {
			return "", tui.ColorMuted
		}
		if c := tui.PersonColor(v); c != nil {
			return "•", c
		}
		return "•", tui.ColorMuted
	}
	return "", tui.ColorMuted
}

// fieldLabel renders one label cell. The focused field is bold accent and carries
// no glyph; every other label is muted, prefixed by a semantic glyph where the
// field has one. The result always measures exactly emLabelW cells, so the value
// column stays aligned.
func (m *editModel) fieldLabel(i int) string {
	name := emFieldLabels[i]
	if i == m.focused {
		return lipgloss.NewStyle().Bold(true).Foreground(tui.ColorAccent).
			Render(tui.FixedWidth(name, emLabelW))
	}
	glyph, glyphColor := m.fieldGlyph(i)
	if glyph == "" {
		return tui.MutedStyle.Render(tui.FixedWidth(name, emLabelW))
	}
	prefix := lipgloss.NewStyle().Foreground(glyphColor).Render(glyph) + " "
	rest := emLabelW - tui.DisplayWidth(prefix)
	if rest < 0 {
		rest = 0
	}
	return prefix + tui.MutedStyle.Render(tui.FixedWidth(name, rest))
}

func (m *editModel) View() tea.View {
	var lines []string

	for i := 0; i < efInputCount; i++ {
		lines = append(lines, " "+m.fieldLabel(i)+" "+m.inputs[i].View())
	}

	descFg := tui.ColorMuted
	if m.focused == efDescription {
		descFg = tui.ColorAccent
	}
	lines = append(lines, "")
	lines = append(lines, tui.SectionHeader(" Description", descFg, m.width))
	for _, line := range strings.Split(m.descTA.View(), "\n") {
		lines = append(lines, " "+line)
	}

	acFg := tui.ColorMuted
	if m.focused == efAccCriteria {
		acFg = tui.ColorAccent
	}
	lines = append(lines, "")
	lines = append(lines, tui.SectionHeader(" Acceptance Criteria", acFg, m.width))
	for _, line := range strings.Split(m.acTA.View(), "\n") {
		lines = append(lines, " "+line)
	}

	if m.validErr != "" {
		lines = append(lines, "")
		lines = append(lines, " "+tui.Badge("✗ "+m.validErr, tui.ColorOnChrome, tui.ColorError))
	}

	lines = append(lines, "")
	if m.confirmAbort {
		lines = append(lines, " "+tui.Badge("! Discard unsaved changes? (y/n)", tui.ColorOnChrome, tui.ColorWarning))
	} else {
		lines = append(lines, " "+tui.FooterHints([]string{
			"enter open picker / next",
			"tab next",
			"shift+tab back",
			"ctrl+s save",
			"esc cancel",
		}, m.width))
	}

	return tea.NewView(strings.Join(lines, "\n") + "\n")
}
