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
	for _, size := range []struct{ w, h int }{{100, 30}, {60, 20}, {56, 34}} {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			m := newTestEditModel(t, size.w, size.h)
			lines := editFormLines(m)
			// Fixed rows are the four section frames' borders (4x2), the blank row
			// and hint row after the last section = 16. setSize's overhead constant
			// is the exact fixed cost, not slack.
			if got, want := len(lines), 16+2*m.taHeight; got != want {
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

func TestEditFormLabelsAreStatic(t *testing.T) {
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
			m.focused = i
			focused := m.fieldLabel(i)
			m.focused = (i + 1) % efInputCount
			unfocused := m.fieldLabel(i)
			if focused != unfocused {
				t.Errorf("populated=%v: fieldLabel(%d) depends on focus: %q vs %q",
					values, i, focused, unfocused)
			}
			if got := tui.DisplayWidth(focused); got != emLabelW {
				t.Errorf("populated=%v: fieldLabel(%d) width = %d, want %d: %q",
					values, i, got, emLabelW, ansi.Strip(focused))
			}
		}
	}
}

// formSectionBorderLine returns the frame-border row carrying the named section
// title in the current render.
func formSectionBorderLine(t *testing.T, m *editModel, title string) string {
	t.Helper()
	for _, line := range editFormLines(m) {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "╭─ "+title+" ") && strings.Contains(plain, "─╮") {
			return line
		}
	}
	t.Fatalf("section %q has no frame-border row", title)
	return ""
}

func TestEditFormSectionTitlesAreStatic(t *testing.T) {
	titles := []string{"Summary", "Details", "Description", "Acceptance Criteria"}

	m := newTestEditModel(t, 100, 30)
	plain := ansi.Strip(m.View().Content)
	for _, title := range titles {
		if got := strings.Count(plain, title); got != 1 {
			t.Errorf("section title %q appears %d times in the form, want 1", title, got)
		}
	}

	before := make(map[string]string, len(titles))
	for _, title := range titles {
		before[title] = formSectionBorderLine(t, m, title)
	}

	for _, focus := range []int{efDescription, efAccCriteria} {
		m.focused = focus
		m.focusFocused()
		for _, title := range titles {
			if got := formSectionBorderLine(t, m, title); got != before[title] {
				t.Errorf("focus %d changed the %q border: %q -> %q", focus, title, before[title], got)
			}
		}
	}
}

func TestEditFormFitsHeightBudget(t *testing.T) {
	for _, size := range []struct{ w, h int }{{120, 40}, {100, 30}, {80, 30}} {
		m := newTestEditModel(t, size.w, size.h)
		if got := len(editFormLines(m)); got > size.h {
			t.Errorf("%dx%d: form rendered %d rows, budget %d", size.w, size.h, got, size.h)
		}
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

func TestEditFormBorderTitle(t *testing.T) {
	m := boardModel{
		activeView: viewEdit,
		editKey:    "DEMO-1\n\x1b[31mx",
		editIssue:  &models.Issue{Key: "DEMO-1", Summary: "evil\nsummary \x1b[31mred"},
		editForm:   newEditModel(&models.Issue{}, &models.ValidValues{}, 56, 36),
	}
	out := m.viewEditForm(120, 40)
	lines := strings.Split(out, "\n")

	if len(lines) != 40 {
		t.Fatalf("modal rendered %d lines, want 40 — a newline escaped the title", len(lines))
	}

	topIdx := -1
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), "╭─ Edit DEMO-1") {
			topIdx = i
			break
		}
	}
	if topIdx < 0 {
		t.Fatal("modal top border with the issue key not found")
	}

	top := ansi.Strip(lines[topIdx])
	if !strings.Contains(top, "x") {
		t.Errorf("top border lost the title suffix: %q", top)
	}
	for _, r := range top {
		if unicode.IsControl(r) {
			t.Errorf("control rune %U survived in the top border: %q", r, top)
		}
	}

	// The title is drawn in the modal's own frame, so the only surviving header
	// row is the first body row: the Summary section's border.
	if second := ansi.Strip(lines[topIdx+1]); !strings.Contains(second, "╭─ Summary") {
		t.Errorf("line after the top border is not the Summary frame border: %q", second)
	}

	// The top border measures exactly the modal width.
	overlayW, _ := tui.OverlaySize(120, 40)
	if got, want := tui.DisplayWidth(strings.TrimSpace(lines[topIdx])), overlayW-2; got != want {
		t.Errorf("top border width = %d, want %d", got, want)
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
