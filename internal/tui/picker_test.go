package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func stripANSI(s string) string { return ansi.Strip(s) }

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

func TestPickerViewRowsFillTheInnerWidthWithoutWrapping(t *testing.T) {
	picker := NewLocalPickerModel([]PickerItem{{
		Label:    "child PROJ-2",
		SubLabel: "Fix login",
	}})

	const innerW = 60
	lines := strings.Split(picker.View(innerW, 1), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want input, separator, and one row", len(lines))
	}
	for i, line := range lines {
		if got := DisplayWidth(line); got > innerW {
			t.Errorf("line %d is %d cells, want at most %d: %q", i, got, innerW, stripANSI(line))
		}
	}

	row := stripANSI(lines[2])
	if !strings.Contains(row, "child PROJ-2") || !strings.Contains(row, "Fix login") {
		t.Fatalf("row lost one of its columns: %q", row)
	}
	gap := strings.Index(row, "Fix login") - len("child PROJ-2") - len("▶ ")
	if gap < 1 || gap > 9 {
		t.Errorf("label/title gap is %d columns, want between 1 and 9: %q", gap, row)
	}
}

func TestPickerViewFullLengthRowStaysOnOneLine(t *testing.T) {
	longPicker := NewLocalPickerModel([]PickerItem{{
		Label:    "relates to PROJ-1234",
		SubLabel: strings.Repeat("x", 200),
	}})

	const innerW = 78
	lines := strings.Split(longPicker.View(innerW, 3), "\n")
	if len(lines) != 3 {
		t.Fatalf("row used %d lines, want input, separator, and one row", len(lines))
	}
	for i, line := range lines {
		if got := DisplayWidth(line); got != innerW {
			t.Errorf("line %d is %d cells, want exactly %d", i, got, innerW)
		}
	}
}

func TestPickerViewSanitizesItemText(t *testing.T) {
	picker := NewLocalPickerModel([]PickerItem{{
		Label:    "PROJ-1",
		SubLabel: "line one\nline two\ttabbed",
	}})

	lines := strings.Split(picker.View(60, 3), "\n")
	if len(lines) != 3 {
		t.Fatalf("newline in a sub-label produced %d lines, want 3", len(lines))
	}
	if row := stripANSI(lines[2]); !strings.Contains(row, "line one line two tabbed") {
		t.Errorf("sub-label was not flattened: %q", row)
	}
}

func TestPickerViewWideRunesStayInsideInnerWidth(t *testing.T) {
	picker := NewLocalPickerModel([]PickerItem{{
		Label:    "プロジェクト",
		SubLabel: "日本語のタイトルです",
	}})

	const innerW = 60
	for i, line := range strings.Split(picker.View(innerW, 3), "\n") {
		if got := DisplayWidth(line); got > innerW {
			t.Errorf("line %d is %d cells, want at most %d: %q", i, got, innerW, stripANSI(line))
		}
	}
}

func TestPickerViewCursorOutOfRangeStillRendersRows(t *testing.T) {
	picker := NewLocalPickerModel([]PickerItem{{Label: "A", SubLabel: "a"}})
	picker.Cursor = 10

	plain := stripANSI(picker.View(60, 3))
	if !strings.Contains(plain, "A") {
		t.Fatalf("an out-of-range cursor rendered an empty list: %q", plain)
	}
	if strings.Contains(plain, "No results") {
		t.Errorf("row exists but the picker reported no results: %q", plain)
	}
}

func TestPickerViewKeepsResultsWhileLoadingAndOnError(t *testing.T) {
	picker := NewLocalPickerModel([]PickerItem{{Label: "PROJ-1", SubLabel: "fix login"}})
	picker.Loading = true

	plain := stripANSI(picker.View(60, 3))
	if !strings.Contains(plain, "PROJ-1") {
		t.Errorf("loading replaced the results: %q", plain)
	}
	if !strings.Contains(plain, "searching") {
		t.Errorf("loading indicator missing: %q", plain)
	}

	picker.Loading = false
	picker.Err = "boom"
	lines := strings.Split(picker.View(60, 3), "\n")
	plain = stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "PROJ-1") {
		t.Errorf("error replaced the results: %q", plain)
	}
	if !strings.Contains(plain, "boom") {
		t.Errorf("error message missing: %q", plain)
	}
	if len(lines) > 4 { // input, separator, one row, error
		t.Errorf("error state used %d lines, want at most 4", len(lines))
	}
}

func TestPickerEnterOnEmptyListDoesNotComplete(t *testing.T) {
	picker := NewLocalPickerModel(nil)
	picker.Init()

	updated, _ := picker.Update(pickerKeyPress("enter"))
	picker = updated
	if picker.Completed {
		t.Fatal("enter completed the picker with nothing to select, which clears the value")
	}
	if picker.SelectedItem() != nil {
		t.Fatalf("selection = %+v, want nil", picker.SelectedItem())
	}
}

func TestPickerEnterWithNoneItemStillCompletes(t *testing.T) {
	picker := NewLocalPickerModel(nil)
	picker.NoneItem = &PickerItem{Label: "(none)"}
	picker.Init()

	updated, _ := picker.Update(pickerKeyPress("enter"))
	picker = updated
	if !picker.Completed {
		t.Fatal("the (none) row should be selectable")
	}
	if picker.SelectedItem() != nil {
		t.Fatalf("selection = %+v, want nil for (none)", picker.SelectedItem())
	}
}

func TestPickerInitialValueAppliedOnce(t *testing.T) {
	items := []PickerItem{
		{Label: "Sprint 1", Value: "1"},
		{Label: "Sprint 2", Value: "2"},
	}
	picker := NewPickerModel(nil)
	picker.Items = items
	picker.InitialValue = "2"

	picker.applyInitialValue()
	if picker.Cursor != 1 {
		t.Fatalf("cursor = %d, want the InitialValue row", picker.Cursor)
	}

	// A later result for a different query must not steal the cursor back.
	picker.Cursor = 0
	picker.applyInitialValue()
	if picker.Cursor != 0 {
		t.Errorf("cursor = %d, want InitialValue to be applied only once", picker.Cursor)
	}
}

func TestPickerSearchResultClampsCursor(t *testing.T) {
	picker := NewPickerModel(nil)
	picker.Cursor = 9
	picker.Items = make([]PickerItem, 10)

	updated, _ := picker.Update(pickerSearchResultMsg{items: []PickerItem{{Label: "only"}}})
	picker = updated
	if picker.Cursor != 0 {
		t.Fatalf("cursor = %d, want it clamped to the single result", picker.Cursor)
	}
	if !strings.Contains(stripANSI(picker.View(60, 3)), "only") {
		t.Error("the result is not rendered after a shrinking search")
	}
}

func TestPickerNilSearchDoesNotHang(t *testing.T) {
	picker := NewPickerModel(nil)
	if cmd := picker.dispatchSearch(""); cmd != nil {
		t.Error("dispatchSearch with no search function should be a no-op")
	}
	if picker.Loading {
		t.Error("picker should not report loading without a search function")
	}
}

func TestPickerInputWidthIsBounded(t *testing.T) {
	picker := NewPickerModel(nil)
	picker.Input.SetValue(strings.Repeat("x", 200))

	const innerW = 60
	for i, line := range strings.Split(picker.View(innerW, 3), "\n") {
		if got := DisplayWidth(line); got > innerW {
			t.Errorf("line %d is %d cells, want at most %d", i, got, innerW)
		}
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
