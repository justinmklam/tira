package app

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

// newTestEditModel builds a blank create form of the given size.
func newTestEditModel(t *testing.T, w, h int) *editModel {
	t.Helper()
	valid := &models.ValidValues{
		IssueTypes: []string{"Bug", "Story", "Task"},
		Priorities: []string{"Low", "Medium", "High"},
	}
	return newEditModel(&models.Issue{}, valid, w, h)
}

// editFormLines returns the form body rows after stripping View's single
// trailing newline.
func editFormLines(m *editModel) []string {
	return strings.Split(strings.TrimSuffix(m.View().Content, "\n"), "\n")
}

func TestEditFormViewLayout(t *testing.T) {
	for _, size := range []struct{ w, h int }{{100, 30}, {60, 20}} {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			m := newTestEditModel(t, size.w, size.h)
			lines := editFormLines(m)
			// Fixed rows are the 6 inputs, 2 section headings, 2 blank separators,
			// the blank before the footer, and the footer itself = 12. setSize's
			// overhead constant deliberately reserves one more.
			if got, want := len(lines), 12+2*m.taHeight; got != want {
				t.Errorf("form rendered %d rows, want %d", got, want)
			}
			for i, line := range lines {
				if w := tui.DisplayWidth(line); w > size.w {
					t.Errorf("row %d width %d exceeds form width %d: %q", i, w, size.w, line)
				}
			}
		})
	}
}

func TestEditFormHasNoTextareaPromptBars(t *testing.T) {
	m := newTestEditModel(t, 100, 30)
	plain := ansi.Strip(m.View().Content)
	for _, bar := range []string{"┃", "▌"} {
		if strings.Contains(plain, bar) {
			t.Errorf("form still renders the textarea prompt bar %q", bar)
		}
	}
}

func TestEditFormOnlyFocusedLabelIsAccented(t *testing.T) {
	m := newTestEditModel(t, 100, 30)
	lines := editFormLines(m)
	focused := ansiBefore(lines[efSummary], "Summary")
	unfocused := ansiBefore(lines[efStoryPoints], "Story Points")
	if focused == "" || unfocused == "" {
		t.Fatalf("missing label styling: focused=%q unfocused=%q", focused, unfocused)
	}
	if focused == unfocused {
		t.Errorf("focused and unfocused labels render the same: %q", focused)
	}
}

// formHeadingColour returns the ANSI style that introduces the named section
// heading in the current render.
func formHeadingColour(t *testing.T, m *editModel, name string) string {
	t.Helper()
	for _, line := range editFormLines(m) {
		if strings.TrimSpace(ansi.Strip(line)) == name {
			return ansiBefore(line, name)
		}
	}
	t.Fatalf("heading %q not found", name)
	return ""
}

func TestEditFormSectionHeadingTracksFocus(t *testing.T) {
	m := newTestEditModel(t, 100, 30)

	m.focused = efDescription
	m.focusFocused()
	descFocused := formHeadingColour(t, m, "Description")
	acUnfocused := formHeadingColour(t, m, "Acceptance Criteria")
	if descFocused == acUnfocused {
		t.Errorf("focused Description and unfocused Acceptance Criteria share a colour: %q", descFocused)
	}

	m.focused = efAccCriteria
	m.focusFocused()
	descBlurred := formHeadingColour(t, m, "Description")
	acFocused := formHeadingColour(t, m, "Acceptance Criteria")
	if descBlurred == descFocused {
		t.Errorf("Description heading colour did not change when it lost focus: %q", descBlurred)
	}
	if acFocused == acUnfocused {
		t.Errorf("Acceptance Criteria heading colour did not change when it gained focus: %q", acFocused)
	}
	if descBlurred == acFocused {
		t.Errorf("focused and unfocused headings share a colour: %q", descBlurred)
	}
}

func TestEditFormLabelsMeasureLabelWidth(t *testing.T) {
	populated := map[int]string{
		efType:     "Bug",
		efPriority: "High",
		efAssignee: "Ada Lovelace",
	}

	for _, values := range []bool{false, true} {
		m := newTestEditModel(t, 100, 30)
		if values {
			for i, v := range populated {
				m.inputs[i].SetValue(v)
			}
		}
		for i := 0; i < efInputCount; i++ {
			for _, focused := range []bool{false, true} {
				if focused {
					m.focused = i
				} else {
					m.focused = (i + 1) % efInputCount
				}
				if got := tui.DisplayWidth(m.fieldLabel(i)); got != emLabelW {
					t.Errorf("populated=%v focused=%v: fieldLabel(%d) width = %d, want %d: %q",
						values, focused, i, got, emLabelW, ansi.Strip(m.fieldLabel(i)))
				}
			}
		}
	}
}

func TestEditFormHeaderIsSanitised(t *testing.T) {
	m := boardModel{
		activeView: viewEdit,
		editKey:    "DEMO-1",
		editIssue:  &models.Issue{Key: "DEMO-1", Summary: "evil\nsummary \x1b[31mred"},
		editForm:   newEditModel(&models.Issue{}, &models.ValidValues{}, 50, 20),
	}
	out := m.viewEditForm(120, 40)

	if got := len(strings.Split(out, "\n")); got != 40 {
		t.Fatalf("modal rendered %d lines, want 40 — a newline escaped the header", got)
	}

	var header string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(ansi.Strip(line), "Edit DEMO-1") {
			header = line
		}
	}
	if header == "" {
		t.Fatal("sanitised header not found")
	}
	plain := ansi.Strip(header)
	for _, r := range plain {
		if unicode.IsControl(r) {
			t.Errorf("control rune %U survived in header: %q", r, plain)
		}
	}
	if !strings.Contains(plain, "evil summary") {
		t.Errorf("header lost the sanitised summary: %q", plain)
	}
	if strings.Contains(plain, "\n") {
		t.Errorf("header still contains a newline: %q", plain)
	}
}

func TestToIssueFields_AllFields(t *testing.T) {
	state := editFormState{
		summary:            "Test Summary",
		issueType:          "Story",
		priority:           "High",
		assignee:           "John Doe",
		origAssignee:       "Jane Doe",
		origAssigneeID:     "account-123",
		storyPoints:        "5",
		labels:             "label1, label2, label3",
		description:        "Test description",
		acceptanceCriteria: "Test acceptance criteria",
	}

	valid := &models.ValidValues{
		IssueTypes: []string{"Bug", "Story", "Task"},
		Priorities: []string{"Low", "Medium", "High"},
		Assignees:  []models.Assignee{{DisplayName: "John Doe", AccountID: "account-456"}},
	}

	fields := state.toIssueFields(valid)

	if fields.Summary != "Test Summary" {
		t.Errorf("Summary = %q, want %q", fields.Summary, "Test Summary")
	}
	if fields.IssueType != "Story" {
		t.Errorf("IssueType = %q, want %q", fields.IssueType, "Story")
	}
	if fields.Priority != "High" {
		t.Errorf("Priority = %q, want %q", fields.Priority, "High")
	}
	if fields.Assignee != "John Doe" {
		t.Errorf("Assignee = %q, want %q", fields.Assignee, "John Doe")
	}
	if fields.StoryPoints != 5 {
		t.Errorf("StoryPoints = %v, want 5", fields.StoryPoints)
	}
	if len(fields.Labels) != 3 {
		t.Errorf("Labels = %v, want 3 labels", len(fields.Labels))
	}
	if fields.Description != "Test description" {
		t.Errorf("Description = %q, want %q", fields.Description, "Test description")
	}
	if fields.AcceptanceCriteria != "Test acceptance criteria" {
		t.Errorf("AcceptanceCriteria = %q, want %q", fields.AcceptanceCriteria, "Test acceptance criteria")
	}
}

func TestToIssueFields_AssigneeUnchanged_ReusesID(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "John Doe",
		origAssignee:       "John Doe",
		origAssigneeID:     "original-account-id",
		storyPoints:        "3",
		labels:             "",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	// When assignee is unchanged, should reuse original AccountID
	if fields.AssigneeID != "original-account-id" {
		t.Errorf("AssigneeID = %q, want %q", fields.AssigneeID, "original-account-id")
	}
}

func TestToIssueFields_AssigneeChanged_ResolvesID(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "New User",
		origAssignee:       "Old User",
		origAssigneeID:     "old-account-id",
		storyPoints:        "3",
		labels:             "",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{
		Assignees: []models.Assignee{
			{DisplayName: "New User", AccountID: "new-account-id"},
			{DisplayName: "Old User", AccountID: "old-account-id"},
		},
	}

	fields := state.toIssueFields(valid)

	// When assignee changes, should resolve new AccountID
	if fields.AssigneeID != "new-account-id" {
		t.Errorf("AssigneeID = %q, want %q", fields.AssigneeID, "new-account-id")
	}
}

func TestToIssueFields_EmptyStoryPoints(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "",
		origAssignee:       "",
		origAssigneeID:     "",
		storyPoints:        "",
		labels:             "",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	if fields.StoryPoints != 0 {
		t.Errorf("StoryPoints = %v, want 0 for empty input", fields.StoryPoints)
	}
}

func TestToIssueFields_InvalidStoryPoints(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "",
		origAssignee:       "",
		origAssigneeID:     "",
		storyPoints:        "not-a-number",
		labels:             "",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	if fields.StoryPoints != 0 {
		t.Errorf("StoryPoints = %v, want 0 for invalid input", fields.StoryPoints)
	}
}

func TestToIssueFields_LabelsCommaSeparated(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "",
		origAssignee:       "",
		origAssigneeID:     "",
		storyPoints:        "",
		labels:             "  label1  , label2 ,  label3  ",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	expectedLabels := []string{"label1", "label2", "label3"}
	if len(fields.Labels) != len(expectedLabels) {
		t.Fatalf("Labels = %v, want %v", fields.Labels, expectedLabels)
	}
	for i, label := range fields.Labels {
		if label != expectedLabels[i] {
			t.Errorf("Labels[%d] = %q, want %q", i, label, expectedLabels[i])
		}
	}
}

func TestToIssueFields_LabelsEmpty(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "",
		origAssignee:       "",
		origAssigneeID:     "",
		storyPoints:        "",
		labels:             "",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	if len(fields.Labels) != 0 {
		t.Errorf("Labels = %v, want empty slice", fields.Labels)
	}
}

func TestToIssueFields_TrimWhitespace(t *testing.T) {
	state := editFormState{
		summary:            "  Test Summary  ",
		issueType:          "  Story  ",
		priority:           "  High  ",
		assignee:           "  John Doe  ",
		origAssignee:       "  John Doe  ",
		origAssigneeID:     "account-123",
		storyPoints:        "  5  ",
		labels:             "",
		description:        "  Description  ",
		acceptanceCriteria: "  AC  ",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	if fields.Summary != "Test Summary" {
		t.Errorf("Summary not trimmed: %q", fields.Summary)
	}
	if fields.Description != "Description" {
		t.Errorf("Description not trimmed: %q", fields.Description)
	}
	if fields.AcceptanceCriteria != "AC" {
		t.Errorf("AcceptanceCriteria not trimmed: %q", fields.AcceptanceCriteria)
	}
}
