package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/justinmklam/tira/internal/models"
)

func TestBlankIssueFromValidUsesDefaultType(t *testing.T) {
	valid := &models.ValidValues{
		IssueTypes: []string{"Bug", "Story", "Task"},
		Priorities: []string{"Low", "Medium", "High"},
	}

	got := blankIssueFromValid(valid, "Story")
	if got.IssueType != "Story" {
		t.Errorf("IssueType = %q, want configured default %q", got.IssueType, "Story")
	}
	if got.Priority != "Medium" {
		t.Errorf("Priority = %q, want middle priority %q", got.Priority, "Medium")
	}

	got = blankIssueFromValid(valid, "Nonexistent")
	if got.IssueType != "Bug" {
		t.Errorf("invalid default IssueType = %q, want first valid %q", got.IssueType, "Bug")
	}
}

func TestNewLocalOptionPickerFiltersAndSelects(t *testing.T) {
	p := newLocalOptionPicker([]string{"Epic", "Story", "Task", "Bug", "Subtask"}, "Task")
	p.Init()

	if item := p.SelectedItem(); item == nil || item.Value != "Task" {
		t.Fatalf("initial selection = %v, want Task", item)
	}

	for _, r := range "sto" {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	if item := p.SelectedItem(); item == nil || item.Value != "Story" {
		t.Fatalf("after typing %q selection = %v, want Story", "sto", item)
	}

	updated, _ := p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !updated.Completed {
		t.Error("enter did not complete the picker")
	}
}

func TestNewLocalOptionPickerEmptyList(t *testing.T) {
	p := newLocalOptionPicker(nil, "")
	p.Init()
	if item := p.SelectedItem(); item != nil {
		t.Errorf("empty picker SelectedItem() = %v, want nil", item)
	}
}
