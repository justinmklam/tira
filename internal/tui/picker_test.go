package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func pickerKeyPress(text string) tea.KeyPressMsg {
	runes := []rune(text)
	var code rune
	if len(runes) > 0 {
		code = runes[0]
	}
	return tea.KeyPressMsg(tea.Key{Code: code, Text: text})
}

func newTestItems() []PickerItem {
	return []PickerItem{
		{Label: "blocks PROJ-2", SubLabel: "Fix login (Bug)", Value: "PROJ-2"},
		{Label: "relates to PROJ-3", SubLabel: "Add signup (Story)", Value: "PROJ-3"},
		{Label: "parent EPIC-9", SubLabel: "Checkout epic", Value: "EPIC-9"},
	}
}

func TestPickerViewKeepsIssueTitleCloseToKey(t *testing.T) {
	picker := NewLocalPickerModel([]PickerItem{{
		Label:    "child PROJ-2",
		SubLabel: "Fix login",
	}})

	plain := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(picker.View(60, 1), "")
	if !strings.Contains(plain, "child PROJ-2       Fix login") {
		t.Fatalf("picker row has unexpected spacing:\n%q", plain)
	}
	if strings.Contains(plain, "child PROJ-2          Fix login") {
		t.Fatalf("picker row still has the old wide gap:\n%q", plain)
	}

	longPicker := NewLocalPickerModel([]PickerItem{{
		Label:    "child PROJ-2",
		SubLabel: "A title that should be clipped instead of wrapping to another row",
	}})
	if got := strings.Count(longPicker.View(60, 1), "\n"); got != 2 {
		t.Fatalf("long picker row used %d lines, want 1 row plus input and separator", got+1)
	}
}

func TestFilterPickerItemsMatchesLabelSubLabelAndValue(t *testing.T) {
	items := newTestItems()

	tests := []struct {
		query string
		key   string
	}{
		{query: "blocks", key: "PROJ-2"},
		{query: "SIGNUP", key: "PROJ-3"},
		{query: "epic-9", key: "EPIC-9"},
		{query: "  login  ", key: "PROJ-2"},
	}

	for _, tt := range tests {
		got := filterPickerItems(items, tt.query)
		if len(got) != 1 {
			t.Fatalf("filterPickerItems(%q) = %d items, want 1", tt.query, len(got))
		}
		if got[0].Value != tt.key {
			t.Errorf("filterPickerItems(%q) = %q, want %q", tt.query, got[0].Value, tt.key)
		}
	}

	if got := filterPickerItems(items, "nothing-matches"); len(got) != 0 {
		t.Errorf("items = %d, want 0", len(got))
	}
	if got := filterPickerItems(items, ""); len(got) != len(items) {
		t.Errorf("empty query = %d items, want all %d", len(got), len(items))
	}
}

func TestLocalPickerModelFiltersWithoutDebounce(t *testing.T) {
	items := newTestItems()
	picker := NewLocalPickerModel(items)
	if !picker.Local() {
		t.Fatal("Local() = false, want true for a local picker")
	}
	if len(picker.Items) != len(items) {
		t.Fatalf("items = %d, want the list supplied at construction", len(picker.Items))
	}

	if cmd := picker.Init(); cmd == nil {
		t.Fatal("Init should return a focus command")
	}
	if !picker.Input.Focused() {
		t.Fatal("Init should focus the filter input")
	}

	// Typing filters immediately: no debounce tick is scheduled.
	updated, cmd := picker.Update(pickerKeyPress("signup"))
	picker = updated
	if cmd != nil {
		if _, isDebounce := cmd().(pickerDebounceMsg); isDebounce {
			t.Error("local typing should not schedule a debounced search")
		}
	}
	if len(picker.Items) != 1 || picker.Items[0].Value != "PROJ-3" {
		t.Fatalf("items = %+v, want the matching item", picker.Items)
	}
	if picker.Loading {
		t.Error("a local picker should never report a loading state")
	}
	if picker.SelectedItem() == nil || picker.SelectedItem().Value != "PROJ-3" {
		t.Errorf("selection = %+v, want the filtered item", picker.SelectedItem())
	}

	// Arrow keys navigate within the filtered results.
	updated, _ = picker.Update(pickerKeyPress("down"))
	picker = updated
	if picker.Cursor != 0 {
		t.Errorf("cursor = %d, want it clamped to the single result", picker.Cursor)
	}

	// Clearing the query restores the full list.
	for range 6 {
		updated, _ = picker.Update(pickerKeyPress("backspace"))
		picker = updated
	}
	if len(picker.Items) != len(items) {
		t.Fatalf("items = %d, want the full list once the query is cleared", len(picker.Items))
	}
}

func TestLocalPickerModelSelectionAndCancel(t *testing.T) {
	picker := NewLocalPickerModel(newTestItems())
	picker.Init()

	updated, _ := picker.Update(pickerKeyPress("down"))
	picker = updated
	selected := picker.SelectedItem()
	if selected == nil || selected.Value != "PROJ-3" {
		t.Fatalf("selection = %+v, want the second item", selected)
	}

	updated, _ = picker.Update(pickerKeyPress("enter"))
	picker = updated
	if !picker.Completed {
		t.Error("enter should complete the picker")
	}

	aborted := NewLocalPickerModel(newTestItems())
	aborted.Init()
	updated, _ = aborted.Update(pickerKeyPress("esc"))
	aborted = updated
	if !aborted.Aborted {
		t.Error("esc should abort the picker")
	}
}
