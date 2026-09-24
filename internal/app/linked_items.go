package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

// linkedItemPicker is the "Linked Items" overlay shared by the backlog, kanban,
// and epics views. It lists the work items related to the selected issue and
// opens the highlighted one in the browser, since related items are frequently
// not on the board and so cannot be reached by moving the cursor.
//
// The list is filterable: typing narrows it, arrow keys move the selection.
type linkedItemPicker struct {
	picker tui.PickerModel
	items  []models.LinkedIssue
}

// linkedPickerAction reports the outcome of a picker key press back to the
// owning view, which owns the states to return to.
type linkedPickerAction int

const (
	linkedPickerNone linkedPickerAction = iota
	linkedPickerConfirmed
	linkedPickerAborted
)

// open replaces the picker contents with the given related work items and
// returns the command that focuses the filter input. The picker opens even when
// there are none, where it reports that nothing matched.
func (p *linkedItemPicker) open(items []models.LinkedIssue) tea.Cmd {
	p.items = items
	p.picker = tui.NewLocalPickerModel(linkedPickerRows(items))
	return p.picker.Init()
}

// reopen replaces the contents while keeping the highlighted row where
// possible, so items that arrive after the picker opened do not move the
// selection out from under the user.
func (p *linkedItemPicker) reopen(items []models.LinkedIssue) tea.Cmd {
	cursor := p.picker.Cursor
	cmd := p.open(items)
	p.picker.Cursor = tui.Clamp(cursor, 0, max(len(items)-1, 0))
	return cmd
}

// selectedKey returns the key of the highlighted work item, or "" when nothing
// is highlighted (no items, or the filter excluded everything).
func (p linkedItemPicker) selectedKey() string {
	item := p.picker.SelectedItem()
	if item == nil {
		return ""
	}
	return item.Value
}

// handleKey updates the picker for the given message and reports whether the
// user confirmed or aborted. ctrl+c is left to the caller, because quitting is
// view-specific.
func (p *linkedItemPicker) handleKey(msg tea.Msg) (linkedPickerAction, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		p.picker, _ = p.picker.Update(msg)
		return linkedPickerNone, nil
	}
	switch key.String() {
	case "esc":
		return linkedPickerAborted, nil
	case "enter":
		return linkedPickerConfirmed, nil
	}

	var cmd tea.Cmd
	p.picker, cmd = p.picker.Update(msg)
	return linkedPickerNone, cmd
}

// viewLinkedItemsPicker renders the shared related work items overlay.
func viewLinkedItemsPicker(picker tui.PickerModel, width, height int) string {
	return tui.RenderPickerOverlay(picker.View, "Linked Items", width, height)
}

// linkedPickerRows maps related work items onto picker rows: the relationship
// and key form the label, and the summary with its attributes the sub-label.
func linkedPickerRows(items []models.LinkedIssue) []tui.PickerItem {
	rows := make([]tui.PickerItem, 0, len(items))
	for _, item := range items {
		label := item.Key
		if item.Relationship != "" {
			label = item.Relationship + " " + item.Key
		}
		rows = append(rows, tui.PickerItem{
			Label:    label,
			SubLabel: linkedItemSubLabel(item),
			Value:    item.Key,
		})
	}
	return rows
}

// linkedItemSubLabel summarises a related work item's summary, issue type,
// status, and subtask count for display, skipping any that are empty.
func linkedItemSubLabel(item models.LinkedIssue) string {
	attributes := make([]string, 0, 3)
	if item.IssueType != "" {
		attributes = append(attributes, item.IssueType)
	}
	if item.Status != "" {
		attributes = append(attributes, item.Status)
	}
	if item.SubTaskCount > 0 {
		attributes = append(attributes, fmt.Sprintf("%d subtasks", item.SubTaskCount))
	}

	subLabel := item.Summary
	if detail := strings.Join(attributes, " · "); detail != "" {
		if subLabel != "" {
			subLabel += " "
		}
		subLabel += "(" + detail + ")"
	}
	return subLabel
}

// linkedItemsForIssue returns the related work items carried by a fully fetched
// issue: explicit issue links, subtasks, and the parent.
func linkedItemsForIssue(issue *models.Issue) []models.LinkedIssue {
	if issue == nil {
		return nil
	}
	items := make([]models.LinkedIssue, 0, len(issue.LinkedIssues)+len(issue.SubTasks)+1)
	items = append(items, issue.LinkedIssues...)
	items = append(items, issue.SubTasks...)
	if issue.ParentKey != "" {
		items = append(items, models.LinkedIssue{
			Relationship: "parent",
			Key:          issue.ParentKey,
			Summary:      issue.ParentSummary,
		})
	}
	return items
}

// collectLinkedItems flattens the groups into one list, keeping only the first
// entry for each key so a work item is never listed twice.
func collectLinkedItems(groups ...[]models.LinkedIssue) []models.LinkedIssue {
	total := 0
	for _, group := range groups {
		total += len(group)
	}

	items := make([]models.LinkedIssue, 0, total)
	seen := make(map[string]bool, total)
	for _, group := range groups {
		for _, item := range group {
			if item.Key == "" || seen[item.Key] {
				continue
			}
			seen[item.Key] = true
			items = append(items, item)
		}
	}
	return items
}
