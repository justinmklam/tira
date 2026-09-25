package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// PickerItem is a single selectable entry.
type PickerItem struct {
	Label    string // primary display text (e.g. issue key or display name)
	SubLabel string // secondary display text (e.g. summary or email)
	Value    string // opaque value returned on selection
}

// SearchFunc performs a server-side search and returns matching items.
// It is called in a goroutine so it may block on network I/O.
type SearchFunc func(query string) ([]PickerItem, error)

// pickerSearchResultMsg carries results back to the picker.
type pickerSearchResultMsg struct {
	searchToken int
	items       []PickerItem
	err         error
}

// pickerDebounceMsg fires after the debounce delay to trigger an actual search.
type pickerDebounceMsg struct {
	debounceToken int
	query         string
}

const DefaultPickerDebounce = 300 * time.Millisecond

// PickerModel is a reusable search picker with debounced server-side queries.
// Embed it in a parent model and delegate Update calls to it.
// Check Completed/Aborted after each update; use SelectedItem() to read the result.
type PickerModel struct {
	Input   textinput.Model
	Items   []PickerItem
	Cursor  int
	Loading bool
	Err     string

	// NoneItem, when non-nil, is always prepended as the first row (cursor=0).
	// Selecting it returns nil from SelectedItem(), signalling "clear the value".
	NoneItem *PickerItem

	// InitialValue, when non-empty, positions the cursor on the first item
	// whose Value matches when search results are loaded.
	InitialValue string

	Completed bool
	Aborted   bool

	search        SearchFunc
	debounce      time.Duration
	debounceToken int // incremented on each input change
	searchToken   int // incremented on each actual search dispatch

	// initialValueApplied records that the cursor has already been placed on
	// InitialValue, so a later search result cannot yank the cursor back while
	// the user is navigating.
	initialValueApplied bool

	// local pickers filter localItems in memory instead of calling search.
	local      bool
	localItems []PickerItem
}

// NewPickerModel creates a picker backed by the given search function.
// Call Init() to fire the initial (empty-query) search.
func NewPickerModel(search SearchFunc) PickerModel {
	ti := textinput.New()
	ti.Placeholder = "type to search…"
	ti.CharLimit = 100
	return PickerModel{
		search:   search,
		debounce: DefaultPickerDebounce,
		Input:    ti,
	}
}

// NewLocalPickerModel creates a picker that filters a fixed item list in memory.
// There is no debounce or loading state: typing narrows the list immediately, so
// it suits choices that are already held by the caller.
func NewLocalPickerModel(items []PickerItem) PickerModel {
	ti := textinput.New()
	ti.Placeholder = "type to filter…"
	ti.CharLimit = 100
	return PickerModel{
		Input:      ti,
		Items:      items,
		local:      true,
		localItems: items,
	}
}

// Local reports whether the picker filters a fixed item list in memory.
func (m PickerModel) Local() bool { return m.local }

// Init focuses the input. Server-backed pickers also fire the initial search;
// local pickers already hold their items.
func (m *PickerModel) Init() tea.Cmd {
	m.applyInitialValue()
	if m.Local() {
		return m.Input.Focus()
	}
	return tea.Batch(m.Input.Focus(), m.dispatchSearch(""))
}

// filterLocal narrows the fixed item list to entries matching the query,
// case-insensitively across the label, sub-label, and value.
func (m *PickerModel) filterLocal(query string) {
	m.Items = filterPickerItems(m.localItems, query)
	m.Loading = false
	m.Err = ""
}

// filterPickerItems returns the items matching query. An empty query matches
// everything.
func filterPickerItems(items []PickerItem, query string) []PickerItem {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return items
	}

	matches := make([]PickerItem, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Label), q) ||
			strings.Contains(strings.ToLower(item.SubLabel), q) ||
			strings.Contains(strings.ToLower(item.Value), q) {
			matches = append(matches, item)
		}
	}
	return matches
}

// noneVisible reports whether the NoneItem should currently be shown.
// It is hidden whenever the user has typed a query.
func (m PickerModel) noneVisible() bool {
	return m.NoneItem != nil && m.Input.Value() == ""
}

// SelectedItem returns the highlighted item, or nil if NoneItem is selected.
func (m PickerModel) SelectedItem() *PickerItem {
	offset := 0
	if m.noneVisible() {
		offset = 1
	}
	idx := m.Cursor - offset
	if idx < 0 || idx >= len(m.Items) {
		return nil
	}
	item := m.Items[idx]
	return &item
}

// applyInitialValue positions the cursor on the first item whose value matches
// InitialValue. It runs at most once per picker, so typing a new query never
// jumps the highlight back to the original value.
func (m *PickerModel) applyInitialValue() {
	if m.InitialValue == "" || m.initialValueApplied {
		return
	}
	m.initialValueApplied = true
	offset := 0
	if m.noneVisible() {
		offset = 1
	}
	for i, item := range m.Items {
		if item.Value == m.InitialValue {
			m.Cursor = offset + i
			return
		}
	}
}

// clampCursor keeps Cursor on a selectable row. Every path that replaces Items
// calls it, so a shrinking list can never leave the highlight (or the scroll
// window) pointing past the end, which would render an empty list.
func (m *PickerModel) clampCursor() {
	maxRow := m.totalRows() - 1
	if maxRow < 0 {
		maxRow = 0
	}
	m.Cursor = Clamp(m.Cursor, 0, maxRow)
}

func (m *PickerModel) totalRows() int {
	n := len(m.Items)
	if m.noneVisible() {
		n++
	}
	return n
}

func (m *PickerModel) dispatchSearch(query string) tea.Cmd {
	m.Loading = true
	m.Err = ""
	m.searchToken++
	if m.search == nil { // no search function: nothing to fetch
		m.Loading = false
		return nil
	}
	tok := m.searchToken
	fn := m.search
	return func() tea.Msg {
		items, err := fn(query)
		return pickerSearchResultMsg{searchToken: tok, items: items, err: err}
	}
}

// Update handles picker-internal messages and key input.
// All unrecognised messages are forwarded to the text input.
func (m PickerModel) Update(msg tea.Msg) (PickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case pickerSearchResultMsg:
		if msg.searchToken != m.searchToken {
			return m, nil // stale result from a superseded search
		}
		m.Loading = false
		if msg.err != nil {
			m.Err = msg.err.Error()
			return m, nil
		}
		m.Items = msg.items
		m.applyInitialValue()
		m.clampCursor()
		return m, nil

	case pickerDebounceMsg:
		if msg.debounceToken != m.debounceToken {
			return m, nil // keystroke was superseded
		}
		return m, m.dispatchSearch(msg.query)
	}

	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.Input, cmd = m.Input.Update(msg)
		return m, cmd
	}

	switch key.String() {
	case "esc":
		m.Aborted = true
		return m, nil

	case "enter":
		if m.totalRows() == 0 {
			return m, nil // nothing selectable; enter must not clear the value
		}
		m.Completed = true
		return m, nil

	case "down", "ctrl+n":
		if total := m.totalRows(); m.Cursor < total-1 {
			m.Cursor++
		}
		return m, nil

	case "up", "ctrl+p":
		if m.Cursor > 0 {
			m.Cursor--
		}
		return m, nil
	}

	// All other keys (including letters) go to the text input.
	prev := m.Input.Value()
	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	if newVal := m.Input.Value(); newVal != prev {
		m.Cursor = 0
		if m.Local() {
			m.filterLocal(newVal)
			m.clampCursor()
			return m, cmd
		}
		m.debounceToken++
		debTok := m.debounceToken
		query := newVal
		delay := m.debounce
		cmd = tea.Tick(delay, func(time.Time) tea.Msg {
			return pickerDebounceMsg{debounceToken: debTok, query: query}
		})
	}
	return m, cmd
}

// pickerRow is a single rendered list row: a label column and a sub-label.
type pickerRow struct{ label, subLabel string }

// rows returns the display entries: the optional NoneItem followed by results.
func (m PickerModel) rows() []pickerRow {
	rows := make([]pickerRow, 0, len(m.Items)+1)
	if m.noneVisible() {
		rows = append(rows, pickerRow{m.NoneItem.Label, m.NoneItem.SubLabel})
	}
	for _, item := range m.Items {
		rows = append(rows, pickerRow{item.Label, item.SubLabel})
	}
	return rows
}

// View renders the picker content (input + separator + rows) sized to innerW
// columns and at most maxListRows list rows. Every returned line is at most
// innerW display cells wide, so the caller can wrap it in a border without any
// line wrapping. Does not include a border; the caller wraps it.
func (m PickerModel) View(innerW, maxListRows int) string {
	if innerW < 1 {
		innerW = 1
	}
	if maxListRows < 0 {
		maxListRows = 0
	}
	m.clampCursor()

	lines := make([]string, 0, maxListRows+3)
	lines = append(lines, " "+FixedWidth(FitInput(m.Input, innerW-1), innerW-1))
	lines = append(lines, m.separatorLine(innerW))

	rows, status := m.listLines(innerW, maxListRows)
	lines = append(lines, rows...)
	if status != "" {
		lines = append(lines, status)
	}
	return strings.Join(lines, "\n")
}

// separatorLine renders the rule under the input. While a search is in flight
// the indicator sits on its right edge, so the current results stay on screen
// instead of being replaced for the duration of the request.
func (m PickerModel) separatorLine(innerW int) string {
	const status = " searching… "
	if !m.Loading || DisplayWidth(status) >= innerW {
		return MutedStyle.Render(strings.Repeat("─", innerW))
	}
	bar := strings.Repeat("─", innerW-DisplayWidth(status))
	return MutedStyle.Render(bar) + MutedStyle.Render(status)
}

// listLines renders at most maxRows list rows plus an optional status line (a
// search error). It never returns an empty block while rows exist, so the
// highlight is always visible.
func (m PickerModel) listLines(innerW, maxRows int) (rows []string, status string) {
	if m.Err != "" {
		status = lipgloss.NewStyle().Foreground(ColorError).
			Render(FixedWidth("  ! "+SanitizeRow(m.Err), innerW))
		maxRows--
	}
	if maxRows < 0 {
		maxRows = 0
	}

	entries := m.rows()
	if len(entries) == 0 {
		if m.Loading {
			return []string{MutedStyle.Render(FixedWidth("  Searching…", innerW))}, status
		}
		return []string{MutedStyle.Render(FixedWidth("  No results", innerW))}, status
	}
	if maxRows == 0 {
		return nil, status
	}

	// Scroll window that always keeps the cursor on screen.
	start := 0
	if m.Cursor >= maxRows {
		start = m.Cursor - maxRows + 1
	}
	if last := len(entries) - maxRows; start > last {
		start = last
	}
	if start < 0 {
		start = 0
	}
	end := min(start+maxRows, len(entries))

	// Label gets 1/3 of the row, sub-label the rest. The row is exactly innerW
	// cells: "▶ " prefix (2) + label + " " separator (1) + sub-label.
	usable := innerW - 3
	if usable < 1 {
		usable = 1
	}
	keyW := usable / 3
	if keyW < 8 {
		keyW = 8
	}
	if keyW > usable {
		keyW = usable
	}
	subW := usable - keyW

	rows = make([]string, 0, end-start)
	for i := start; i < end; i++ {
		e := entries[i]
		label := FixedWidth(SanitizeRow(e.label), keyW)
		sub := FixedWidth(SanitizeRow(e.subLabel), subW)
		if i == m.Cursor {
			row := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("▶ "+label) +
				" " + lipgloss.NewStyle().Foreground(ColorForegroundBright).Render(sub)
			rows = append(rows, row)
		} else {
			rows = append(rows, "  "+MutedStyle.Render(label)+" "+MutedStyle.Render(sub))
		}
	}
	return rows, status
}
