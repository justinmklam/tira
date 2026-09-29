package app

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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
	secW      int // outer width of each nested section frame
	secInner  int // content width of each nested section frame (secW-2)
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

	// The form body sits inside the modal's frame, indented by one cell on each
	// side. secW is the nested section frame's outer width and secInner its
	// content width; both setSize and View derive from these two values so the
	// layout arithmetic lives in one place.
	m.secW = max(w-2, 20)
	m.secInner = m.secW - 2

	// Summary gets the section's full content width. textinput.View reserves one
	// cell for the cursor beyond SetWidth, so it is sized one short of its slot;
	// otherwise the cursor would spill past the section frame's right border.
	m.inputs[efSummary].SetWidth(m.secInner - 2)
	for i := 1; i < efInputCount; i++ {
		m.inputs[i].SetWidth(emInputW)
	}

	// The textarea's SetWidth counts the prompt inside its total width. With the
	// prompt cleared, secInner-1 leaves exactly secInner cells once View indents
	// the block by one cell, matching the field rows and the section titles.
	taW := max(m.secInner-1, 10)
	m.descTA.SetWidth(taW)
	m.acTA.SetWidth(taW)

	// Compute textarea height from available space. The form renders
	// 20 + 2*taHeight rows: two border rows and one title padding row per section
	// frame (4 sections = 12), one Summary value row, five Details value rows, and
	// one blank plus one hint row after the last section. 20 is the exact fixed
	// cost, not slack; TestEditFormViewLayout asserts the formula.
	const overhead = 20
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

// fieldLabel renders one label cell as plain muted text. Focus is shown only by
// the input's own cursor, so the label is independent of m.focused and of the
// field's value. The result always measures exactly emLabelW cells, so the value
// column stays aligned.
func (m *editModel) fieldLabel(i int) string {
	return tui.MutedStyle.Render(tui.FixedWidth(emFieldLabels[i], emLabelW))
}

// section renders one nested section frame with its title in the top border and
// indents every row by one cell so the section sits inside the modal's frame.
// Frame itself supplies the blank padding row under the title.
func (m *editModel) section(title, body string) []string {
	rows := strings.Split(tui.Frame(title, body, m.secW, 0, tui.ColorSubtle, tui.ColorForegroundBright), "\n")
	for i, row := range rows {
		rows[i] = " " + row
	}
	return rows
}

// indent prepends one cell to every line of a rendered block.
func indent(block string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		lines[i] = " " + line
	}
	return strings.Join(lines, "\n")
}

func (m *editModel) View() tea.View {
	var lines []string

	lines = append(lines, m.section("Summary", " "+m.inputs[efSummary].View())...)

	details := make([]string, 0, efLabels-efType+1)
	for i := efType; i <= efLabels; i++ {
		details = append(details, " "+m.fieldLabel(i)+" "+m.inputs[i].View())
	}
	lines = append(lines, m.section("Details", strings.Join(details, "\n"))...)

	lines = append(lines, m.section("Description", indent(m.descTA.View()))...)
	lines = append(lines, m.section("Acceptance Criteria", indent(m.acTA.View()))...)

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
