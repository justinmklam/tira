package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// Init focuses the input and fires the initial empty search.
func (m *PickerModel) Init() tea.Cmd {
	return tea.Batch(m.Input.Focus(), m.dispatchSearch(""))
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
		// Position cursor on InitialValue match if set.
		if m.InitialValue != "" {
			offset := 0
			if m.noneVisible() {
				offset = 1
			}
			for i, item := range msg.items {
				if item.Value == m.InitialValue {
					m.Cursor = offset + i
					break
				}
			}
		} else if m.Cursor >= m.totalRows() {
			m.Cursor = 0
		}
		return m, nil

	case pickerDebounceMsg:
		if msg.debounceToken != m.debounceToken {
			return m, nil // keystroke was superseded
		}
		return m, m.dispatchSearch(msg.query)
	}

	key, ok := msg.(tea.KeyMsg)
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

// View renders the picker content (input + list) sized to innerW columns and
// at most maxListRows list rows. Does not include a border; the caller wraps it.
func (m PickerModel) View(innerW, maxListRows int) string {
	var lines []string

	lines = append(lines, " "+m.Input.View())

	sep := MutedStyle.Render(strings.Repeat("─", innerW))
	lines = append(lines, sep)

	switch {
	case m.Loading:
		lines = append(lines, MutedStyle.Render("  Searching…"))

	case m.Err != "":
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorError).Render("  Error: "+m.Err))

	default:
		// Build display entries: optional NoneItem followed by results.
		type entry struct{ label, subLabel string }
		var entries []entry
		if m.noneVisible() {
			entries = append(entries, entry{m.NoneItem.Label, m.NoneItem.SubLabel})
		}
		for _, item := range m.Items {
			entries = append(entries, entry{item.Label, item.SubLabel})
		}

		if len(entries) == 0 {
			lines = append(lines, MutedStyle.Render("  No results"))
		} else {
			// Scroll window so the cursor stays visible.
			start := 0
			if m.Cursor >= maxListRows {
				start = m.Cursor - maxListRows + 1
			}
			end := start + maxListRows
			if end > len(entries) {
				end = len(entries)
			}

			// Label gets 2/3 of usable width, subLabel gets the rest.
			// "  " prefix (2) + " " separator (1) = 3 chars overhead per row.
			usable := innerW - 3
			keyW := usable * 2 / 5
			subW := usable - keyW
			if keyW < 8 {
				keyW = 8
			}
			if subW < 4 {
				subW = 4
			}

			for i := start; i < end; i++ {
				e := entries[i]
				label := FixedWidth(e.label, keyW)
				sub := FixedWidth(e.subLabel, subW)
				if i == m.Cursor {
					row := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("▶ "+label) +
						" " + lipgloss.NewStyle().Foreground(ColorForegroundBright).Render(sub)
					lines = append(lines, row)
				} else {
					lines = append(lines, "  "+MutedStyle.Render(label)+" "+MutedStyle.Render(sub))
				}
			}
		}
	}

	return strings.Join(lines, "\n")
}
