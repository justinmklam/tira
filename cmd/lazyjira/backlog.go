package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/display"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/spf13/cobra"
)

var backlogCmd = &cobra.Command{
	Use:   "backlog",
	Short: "Show the project backlog grouped by sprint",
	RunE:  runBacklogCmd,
}

func init() {
	rootCmd.AddCommand(backlogCmd)
}

func runBacklogCmd(_ *cobra.Command, _ []string) error {
	if cfg.BoardID == 0 {
		return fmt.Errorf("board ID not configured: set default_board_id in ~/.config/lazyjira/config.yaml")
	}

	client, err := api.NewClient(cfg)
	if err != nil {
		return err
	}

	groups, err := fetchSprintGroupsWithSpinner(client, cfg.BoardID)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		fmt.Fprintln(os.Stderr, "No sprints or backlog issues found.")
		return nil
	}

	for {
		result, err := runBacklogTUI(client, groups)
		if err != nil {
			return err
		}
		if result.refresh {
			groups, err = fetchSprintGroupsWithSpinner(client, cfg.BoardID)
			if err != nil {
				return err
			}
			continue
		}
		if result.editKey == "" {
			break
		}

		issue, err := fetchWithSpinner(fmt.Sprintf("Fetching %s…", result.editKey), func() (*models.Issue, error) {
			return client.GetIssue(result.editKey)
		})
		if err != nil {
			return err
		}
		if err := runEditLoop(client, issue); err != nil {
			return err
		}
		// Re-fetch to reflect updates.
		if refreshed, err := client.GetSprintGroups(cfg.BoardID); err == nil {
			groups = refreshed
		}
	}

	return nil
}

// --- sprint groups fetch spinner ---

type sprintGroupsResult struct {
	groups []models.SprintGroup
	err    error
}

type sprintGroupsSpinnerModel struct {
	spinner spinner.Model
	label   string
	result  chan sprintGroupsResult
	done    bool
	groups  []models.SprintGroup
	err     error
}

func (m sprintGroupsSpinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg { return <-m.result })
}

func (m sprintGroupsSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sprintGroupsResult:
		m.done = true
		m.groups = msg.groups
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m sprintGroupsSpinnerModel) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " " + m.label
}

func fetchSprintGroupsWithSpinner(client api.Client, boardID int) ([]models.SprintGroup, error) {
	ch := make(chan sprintGroupsResult, 1)
	go func() {
		groups, err := client.GetSprintGroups(boardID)
		ch <- sprintGroupsResult{groups, err}
	}()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	p := tea.NewProgram(sprintGroupsSpinnerModel{
		spinner: s,
		label:   "Fetching backlog…",
		result:  ch,
	}, tea.WithOutput(os.Stderr))

	fm, err := p.Run()
	if err != nil {
		return nil, err
	}
	m := fm.(sprintGroupsSpinnerModel)
	return m.groups, m.err
}

// --- backlog TUI ---

type blState int

const (
	blList    blState = iota
	blLoading         // fetching full issue for detail view
	blDetail
	blFilter
)

type blRowKind int

const (
	blRowSprint blRowKind = iota
	blRowIssue
	blRowSpacer // blank line between sprint groups
)

type blRow struct {
	kind     blRowKind
	groupIdx int
	issueIdx int // -1 for sprint header rows
}

type blResult struct {
	editKey string
	refresh bool
}

type blMoveMultiDoneMsg struct {
	movedKeys      []string
	targetGroupIdx int
	err            error
}

type blModel struct {
	state  blState
	client api.Client

	groups    []models.SprintGroup
	rows      []blRow
	cursor    int
	offset    int
	collapsed map[int]bool

	filter      string
	filterInput textinput.Model

	width  int
	height int

	loadSpinner spinner.Model
	detailIssue *models.Issue
	detailView  viewport.Model

	selected     map[string]bool // issue keys marked with spacebar
	cutKeys      map[string]bool // issue keys marked for move with 'x'
	visualMode   bool
	visualAnchor int // row index where 'v' was pressed
	moving       bool // true while a move API call is in flight

	result   blResult
	quitting bool
}

func blBuildRows(groups []models.SprintGroup, collapsed map[int]bool, filter string) []blRow {
	var rows []blRow
	for i, g := range groups {
		if i > 0 {
			rows = append(rows, blRow{kind: blRowSpacer, groupIdx: i})
		}
		rows = append(rows, blRow{kind: blRowSprint, groupIdx: i, issueIdx: -1})
		if !collapsed[i] {
			for j, issue := range g.Issues {
				if filter == "" || blMatchesFilter(issue, filter) {
					rows = append(rows, blRow{kind: blRowIssue, groupIdx: i, issueIdx: j})
				}
			}
		}
	}
	return rows
}

// blEpicColor returns a consistent terminal color for an epic key by hashing it.
// Returns empty string for empty keys (caller should use dim/default style).
func blEpicColor(epicKey string) string {
	if epicKey == "" {
		return ""
	}
	palette := []string{"39", "208", "141", "43", "214", "99", "203", "118", "45", "220"}
	var sum int
	for _, r := range epicKey {
		sum += int(r)
	}
	return palette[sum%len(palette)]
}

// blTypeColor returns the terminal color number for an issue type.
func blTypeColor(issueType string) string {
	switch strings.ToLower(issueType) {
	case "bug":
		return "9" // red
	case "story":
		return "10" // green
	case "task":
		return "12" // blue
	case "epic":
		return "13" // magenta
	case "sub-task", "subtask":
		return "11" // yellow
	default:
		return "244"
	}
}

func blMatchesFilter(issue models.Issue, filter string) bool {
	f := strings.ToLower(filter)
	return strings.Contains(strings.ToLower(issue.Key), f) ||
		strings.Contains(strings.ToLower(issue.Summary), f)
}

// blFixedWidth returns s padded or truncated to exactly n runes.
func blFixedWidth(s string, n int) string {
	r := []rune(s)
	if len(r) == n {
		return s
	}
	if len(r) > n {
		if n <= 1 {
			return string(r[:n])
		}
		return string(r[:n-1]) + "…"
	}
	return s + strings.Repeat(" ", n-len(r))
}

func blClamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func blScrollToFit(m blModel) blModel {
	vh := m.viewHeight()
	if vh <= 0 {
		return m
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+vh {
		m.offset = m.cursor - vh + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
	return m
}

func newBacklogModel(client api.Client, groups []models.SprintGroup) blModel {
	collapsed := make(map[int]bool)
	_ = groups // all sprints start expanded

	ti := textinput.New()
	ti.Placeholder = "type to filter…"
	ti.CharLimit = 60

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	m := blModel{
		state:       blList,
		client:      client,
		groups:      groups,
		collapsed:   collapsed,
		filterInput: ti,
		loadSpinner: s,
		selected: make(map[string]bool),
		cutKeys:  make(map[string]bool),
	}
	m.rows = blBuildRows(groups, collapsed, "")
	return m
}

func (m blModel) viewHeight() int {
	if m.height < 5 {
		return 1
	}
	return m.height - 3 // top bar + column header + footer
}

// visualIssueKeys returns the set of issue keys spanned by the visual selection range.
func (m blModel) visualIssueKeys() map[string]bool {
	if !m.visualMode {
		return nil
	}
	lo, hi := m.visualAnchor, m.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	keys := make(map[string]bool)
	for i := lo; i <= hi; i++ {
		if i < len(m.rows) && m.rows[i].kind == blRowIssue {
			row := m.rows[i]
			issue := m.groups[row.groupIdx].Issues[row.issueIdx]
			keys[issue.Key] = true
		}
	}
	return keys
}

// allSelected returns the effective selection: base selection toggled by any
// active visual range (so reselecting an already-selected item deselects it).
func (m blModel) allSelected() map[string]bool {
	if !m.visualMode && len(m.selected) == 0 {
		return nil
	}
	combined := make(map[string]bool, len(m.selected))
	for k := range m.selected {
		combined[k] = true
	}
	for k := range m.visualIssueKeys() {
		if combined[k] {
			delete(combined, k)
		} else {
			combined[k] = true
		}
	}
	return combined
}

// moveKeys returns the issue keys to operate on: the combined selection if any,
// otherwise just the current issue.
func (m blModel) moveKeys() []string {
	if combined := m.allSelected(); len(combined) > 0 {
		keys := make([]string, 0, len(combined))
		for k := range combined {
			keys = append(keys, k)
		}
		return keys
	}
	if issue := m.currentIssue(); issue != nil {
		return []string{issue.Key}
	}
	return nil
}

func (m blModel) currentIssue() *models.Issue {
	if m.cursor >= len(m.rows) {
		return nil
	}
	row := m.rows[m.cursor]
	if row.kind != blRowIssue {
		return nil
	}
	return &m.groups[row.groupIdx].Issues[row.issueIdx]
}

func (m blModel) Init() tea.Cmd { return nil }

func (m blModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle global messages first.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.state == blDetail {
			m.detailView.Width = msg.Width
			m.detailView.Height = msg.Height - 3
		}
		return m, nil

	case issueFetchedMsg:
		if msg.err != nil {
			m.state = blList
			return m, nil
		}
		m.detailIssue = msg.issue
		md, _ := display.RenderIssue(msg.issue)
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.width),
		)
		content := md
		if err == nil {
			if rendered, err2 := renderer.Render(md); err2 == nil {
				content = rendered
			}
		}
		vp := viewport.New(m.width, m.height-3)
		vp.SetContent(content)
		m.detailView = vp
		m.state = blDetail
		return m, nil

	case spinner.TickMsg:
		if m.state == blLoading || m.moving {
			var cmd tea.Cmd
			m.loadSpinner, cmd = m.loadSpinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case blMoveMultiDoneMsg:
		m.moving = false
		if msg.err == nil {
			// Remove moved issues from all source groups, append to target.
			movedSet := make(map[string]bool, len(msg.movedKeys))
			for _, k := range msg.movedKeys {
				movedSet[k] = true
			}
			var movedIssues []models.Issue
			for i := range m.groups {
				var remaining []models.Issue
				for _, issue := range m.groups[i].Issues {
					if movedSet[issue.Key] && i != msg.targetGroupIdx {
						movedIssues = append(movedIssues, issue)
					} else {
						remaining = append(remaining, issue)
					}
				}
				m.groups[i].Issues = remaining
			}
			m.groups[msg.targetGroupIdx].Issues = append(m.groups[msg.targetGroupIdx].Issues, movedIssues...)
			m.selected = make(map[string]bool)
			m.cutKeys = make(map[string]bool)
			m.visualMode = false
			m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
			m.cursor = blClamp(m.cursor, 0, len(m.rows)-1)
			return blScrollToFit(m), nil
		}
		return m, nil
	}

	switch m.state {
	case blList:
		return m.updateList(msg)
	case blFilter:
		return m.updateFilter(msg)
	case blDetail:
		return m.updateDetail(msg)
	}
	return m, nil
}

func (m blModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit

	case "j":
		next := blClamp(m.cursor+1, 0, len(m.rows)-1)
		if next < len(m.rows) && m.rows[next].kind == blRowSpacer {
			next = blClamp(next+1, 0, len(m.rows)-1)
		}
		m.cursor = next
		return blScrollToFit(m), nil

	case "k":
		prev := blClamp(m.cursor-1, 0, len(m.rows)-1)
		if prev >= 0 && m.rows[prev].kind == blRowSpacer {
			prev = blClamp(prev-1, 0, len(m.rows)-1)
		}
		m.cursor = prev
		return blScrollToFit(m), nil

	case "J", "}":
		// Jump to next sprint header.
		for i := m.cursor + 1; i < len(m.rows); i++ {
			if m.rows[i].kind == blRowSprint {
				m.cursor = i
				break
			}
		}
		return blScrollToFit(m), nil

	case "K", "{":
		// Jump to previous sprint header.
		for i := m.cursor - 1; i >= 0; i-- {
			if m.rows[i].kind == blRowSprint {
				m.cursor = i
				break
			}
		}
		return blScrollToFit(m), nil

	case "g":
		m.cursor = 0
		m.offset = 0
		return m, nil

	case "G":
		m.cursor = len(m.rows) - 1
		return blScrollToFit(m), nil

	case "ctrl+d":
		m.cursor = blClamp(m.cursor+m.viewHeight()/2, 0, len(m.rows)-1)
		if m.rows[m.cursor].kind == blRowSpacer {
			m.cursor = blClamp(m.cursor+1, 0, len(m.rows)-1)
		}
		return blScrollToFit(m), nil

	case "ctrl+u":
		m.cursor = blClamp(m.cursor-m.viewHeight()/2, 0, len(m.rows)-1)
		if m.rows[m.cursor].kind == blRowSpacer {
			m.cursor = blClamp(m.cursor-1, 0, len(m.rows)-1)
		}
		return blScrollToFit(m), nil

	case "z":
		row := m.rows[m.cursor]
		gIdx := row.groupIdx
		m.collapsed[gIdx] = !m.collapsed[gIdx]
		m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
		m.cursor = blClamp(m.cursor, 0, len(m.rows)-1)
		return blScrollToFit(m), nil

	case "Z":
		// Collapse all if any expanded; otherwise expand all.
		anyExpanded := false
		for i := range m.groups {
			if !m.collapsed[i] {
				anyExpanded = true
				break
			}
		}
		for i := range m.groups {
			m.collapsed[i] = anyExpanded
		}
		m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
		m.cursor = blClamp(m.cursor, 0, len(m.rows)-1)
		return blScrollToFit(m), nil

	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.filterInput.SetValue("")
			m.rows = blBuildRows(m.groups, m.collapsed, "")
			m.cursor = blClamp(m.cursor, 0, len(m.rows)-1)
		} else if m.visualMode {
			// Exit visual mode — toggle range in base selection (reselecting deselects).
			for k := range m.visualIssueKeys() {
				if m.selected[k] {
					delete(m.selected, k)
				} else {
					m.selected[k] = true
				}
			}
			m.visualMode = false
		} else if len(m.selected) > 0 {
			m.selected = make(map[string]bool)
		}
		return m, nil

	case "/":
		m.state = blFilter
		m.filterInput.SetValue(m.filter)
		return m, m.filterInput.Focus()

	case "enter":
		if m.cursor >= len(m.rows) {
			return m, nil
		}
		row := m.rows[m.cursor]
		if row.kind == blRowSpacer {
			return m, nil
		}
		if row.kind == blRowSprint {
			// Toggle collapse on the sprint header.
			m.collapsed[row.groupIdx] = !m.collapsed[row.groupIdx]
			m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
			m.cursor = blClamp(m.cursor, 0, len(m.rows)-1)
			return blScrollToFit(m), nil
		}
		// Load and show detail view.
		issue := m.groups[row.groupIdx].Issues[row.issueIdx]
		m.state = blLoading
		return m, tea.Batch(m.loadSpinner.Tick, fetchIssueCmd(m.client, issue.Key))

	case "e":
		if issue := m.currentIssue(); issue != nil {
			m.result = blResult{editKey: issue.Key}
			return m, tea.Quit
		}

	case "R":
		m.result = blResult{refresh: true}
		return m, tea.Quit

	case " ":
		if issue := m.currentIssue(); issue != nil {
			if m.selected[issue.Key] {
				delete(m.selected, issue.Key)
			} else {
				m.selected[issue.Key] = true
			}
		}
		return m, nil

	case "v":
		if m.visualMode {
			// Exit visual mode — toggle range in base selection (reselecting deselects).
			for k := range m.visualIssueKeys() {
				if m.selected[k] {
					delete(m.selected, k)
				} else {
					m.selected[k] = true
				}
			}
			m.visualMode = false
		} else {
			m.visualMode = true
			m.visualAnchor = m.cursor
		}
		return m, nil

	case "S":
		if issue := m.currentIssue(); issue != nil {
			if m.selected[issue.Key] {
				delete(m.selected, issue.Key)
			} else {
				m.selected[issue.Key] = true
			}
			// Move cursor up.
			prev := blClamp(m.cursor-1, 0, len(m.rows)-1)
			if prev >= 0 && m.rows[prev].kind == blRowSpacer {
				prev = blClamp(prev-1, 0, len(m.rows)-1)
			}
			m.cursor = prev
			return blScrollToFit(m), nil
		}
		return m, nil

	case "x":
		// Cut: mark selected (or current) issues for move, clear selection.
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		m.cutKeys = make(map[string]bool, len(keys))
		for _, k := range keys {
			m.cutKeys[k] = true
		}
		m.selected = make(map[string]bool)
		return m, nil

	case "p":
		// Paste: move cut issues to the sprint under the cursor.
		if len(m.cutKeys) == 0 {
			return m, nil
		}
		if m.cursor >= len(m.rows) {
			return m, nil
		}
		targetGroupIdx := m.rows[m.cursor].groupIdx
		target := m.groups[targetGroupIdx]
		keys := make([]string, 0, len(m.cutKeys))
		for k := range m.cutKeys {
			keys = append(keys, k)
		}
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, target.Sprint.ID, targetGroupIdx))

	case ">":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		row := m.rows[m.cursor]
		nextIdx := row.groupIdx + 1
		if nextIdx >= len(m.groups) {
			return m, nil
		}
		target := m.groups[nextIdx]
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, target.Sprint.ID, nextIdx))

	case "<":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		row := m.rows[m.cursor]
		prevIdx := row.groupIdx - 1
		if prevIdx < 0 {
			return m, nil
		}
		target := m.groups[prevIdx]
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, target.Sprint.ID, prevIdx))

	case "B":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		row := m.rows[m.cursor]
		backlogIdx := -1
		for i, g := range m.groups {
			if g.Sprint.State == "backlog" {
				backlogIdx = i
				break
			}
		}
		if backlogIdx < 0 || backlogIdx == row.groupIdx {
			return m, nil
		}
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, 0, backlogIdx))
	}

	return m, nil
}

func (m blModel) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.filter = ""
			m.filterInput.SetValue("")
			m.filterInput.Blur()
			m.state = blList
			m.rows = blBuildRows(m.groups, m.collapsed, "")
			m.cursor = blClamp(m.cursor, 0, len(m.rows)-1)
			return m, nil
		case "enter":
			m.filter = m.filterInput.Value()
			m.filterInput.Blur()
			m.state = blList
			m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
			m.cursor = blClamp(m.cursor, 0, len(m.rows)-1)
			return m, nil
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	// Live update rows as the user types.
	m.filter = m.filterInput.Value()
	m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
	m.cursor = blClamp(m.cursor, 0, len(m.rows)-1)
	return m, cmd
}

func (m blModel) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			m.state = blList
			m.detailIssue = nil
			return m, nil
		case "e":
			if m.detailIssue != nil {
				m.result = blResult{editKey: m.detailIssue.Key}
				return m, tea.Quit
			}
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.detailView, cmd = m.detailView.Update(msg)
	return m, cmd
}

func (m blModel) View() string {
	switch m.state {
	case blLoading:
		return "\n  " + m.loadSpinner.View() + " Loading issue…"
	case blDetail:
		return m.viewDetail()
	default:
		return m.viewList()
	}
}

func (m blModel) viewDetail() string {
	if m.detailIssue == nil {
		return ""
	}
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Padding(0, 1).
		Render(m.detailIssue.Key + "  " + m.detailIssue.Summary)
	footer := "\n" + dim.Render("  e: edit   esc/q: back to list   j/k: scroll")
	return header + "\n" + m.detailView.View() + footer
}

// blColumnHeader returns a dim header row aligned with issue row columns.
func blColumnHeader(width int) string {
	const (
		keyW    = 10
		epicW   = 16
		typeW   = 8
		spW     = 5
		assignW = 14
	)
	summaryW := width - 2 - keyW - 2 - epicW - 1 - typeW - 1 - spW - 1 - assignW - 2
	if summaryW < 8 {
		summaryW = 8
	}
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	return dim.Render(
		"  " +
			blFixedWidth("KEY", keyW) + "  " +
			blFixedWidth("SUMMARY", summaryW) + "  " +
			blFixedWidth("EPIC", epicW) + " " +
			blFixedWidth("TYPE", typeW) + " " +
			blFixedWidth("SP", spW) + " " +
			blFixedWidth("ASSIGNEE", assignW),
	)
}

func (m blModel) viewList() string {
	if m.quitting {
		return ""
	}

	width := m.width
	if width == 0 {
		width = 120
	}

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	// Top bar.
	topBar := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Padding(0, 1).Render("Backlog")
	if m.visualMode {
		topBar += " " + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")).Render("VISUAL")
	} else if m.filter != "" {
		topBar += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("/ "+m.filter)
	}
	if len(m.cutKeys) > 0 {
		topBar += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render(fmt.Sprintf("✂ %d cut", len(m.cutKeys)))
	}

	// Column header.
	colHeader := blColumnHeader(width)

	// Visible rows.
	vh := m.viewHeight()
	end := m.offset + vh
	if end > len(m.rows) {
		end = len(m.rows)
	}
	lines := make([]string, 0, vh)
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.renderRow(i, width))
	}
	for len(lines) < vh {
		lines = append(lines, "")
	}

	// Footer.
	var footer string
	if m.state == blFilter {
		footer = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render("/") +
			" " + m.filterInput.View() +
			"  " + dim.Render("esc: clear  enter: apply")
	} else {
		hints := []string{
			"j/k: navigate", "J/K/{/}: sprint", "z/Z: collapse",
			"space/S: select", "v: visual", "enter: view", "e: edit",
			"x: cut", "p: paste", ">/<: adj sprint", "B: backlog",
			"/: filter", "R: refresh", "q: quit",
		}
		left := "  " + strings.Join(hints, "   ")
		if n := len(m.allSelected()); n > 0 {
			left = fmt.Sprintf("  %d selected   ", n) + strings.Join(hints, "   ")
		}
		if m.moving {
			spinnerStr := m.loadSpinner.View() + dim.Render(" Moving…")
			leftWidth := width - lipgloss.Width(spinnerStr) - 2
			footer = dim.Render(blFixedWidth(left, leftWidth)) + "  " + spinnerStr
		} else {
			footer = dim.Render(left)
		}
	}

	return topBar + "\n" + colHeader + "\n" + strings.Join(lines, "\n") + "\n" + footer
}

func (m blModel) renderRow(idx, width int) string {
	row := m.rows[idx]
	isSelected := idx == m.cursor

	// activeGroupIdx is the sprint group the cursor currently lives in.
	activeGroupIdx := -1
	if m.cursor < len(m.rows) {
		activeGroupIdx = m.rows[m.cursor].groupIdx
	}

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	if row.kind == blRowSpacer {
		return ""
	}

	if row.kind == blRowSprint {
		group := m.groups[row.groupIdx]
		icon := "▼"
		if m.collapsed[row.groupIdx] {
			icon = "▶"
		}

		// Left accent bar color indicates sprint state.
		stateColor := "240"
		stateLabel := group.Sprint.State
		switch group.Sprint.State {
		case "active":
			stateColor = "10"
		case "future":
			stateColor = "12"
		}
		accentColor := stateColor
		if row.groupIdx == activeGroupIdx {
			accentColor = "11" // yellow — marks the currently selected sprint
		}
		accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(accentColor)).Bold(true)
		accent := accentStyle.Render("▌")
		stateBadge := lipgloss.NewStyle().
			Foreground(lipgloss.Color(stateColor)).
			Render(stateLabel)

		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
		namePart := nameStyle.Render(icon + " " + group.Sprint.Name)

		countStr := fmt.Sprintf("%d issues", len(group.Issues))
		count := dim.Render(countStr)

		left := accent + " " + namePart + "  " + stateBadge
		leftLen := lipgloss.Width(left)
		rightLen := len(countStr)
		fillLen := width - leftLen - rightLen - 2
		if fillLen < 1 {
			fillLen = 1
		}
		fill := dim.Render(strings.Repeat("─", fillLen))
		line := left + " " + fill + " " + count

		if isSelected {
			return lipgloss.NewStyle().Background(lipgloss.Color("237")).Width(width).Render(line)
		}
		return line
	}

	// Issue row.
	issue := m.groups[row.groupIdx].Issues[row.issueIdx]

	const (
		keyW    = 10
		epicW   = 16
		typeW   = 8
		spW     = 5
		assignW = 14
	)
	summaryW := width - 2 - keyW - 2 - epicW - 1 - typeW - 1 - spW - 1 - assignW - 2
	if summaryW < 8 {
		summaryW = 8
	}

	key := blFixedWidth(issue.Key, keyW)
	summary := blFixedWidth(issue.Summary, summaryW)

	epicText := issue.EpicName
	if epicText == "" {
		epicText = issue.EpicKey
	}
	if epicText == "" {
		epicText = "—"
	}
	epic := blFixedWidth(epicText, epicW)

	issueType := blFixedWidth(issue.IssueType, typeW)

	var spText string
	if issue.StoryPoints > 0 {
		if issue.StoryPoints == float64(int(issue.StoryPoints)) {
			spText = fmt.Sprintf("%d", int(issue.StoryPoints))
		} else {
			spText = fmt.Sprintf("%.1f", issue.StoryPoints)
		}
	} else {
		spText = "—"
	}
	sp := blFixedWidth(spText, spW)

	assignee := issue.Assignee
	if assignee == "" {
		assignee = "—"
	}
	assignee = blFixedWidth(assignee, assignW)

	epicColor := blEpicColor(issue.EpicKey)

	isChecked := m.allSelected()[issue.Key]
	isCut := m.cutKeys[issue.Key]

	if isSelected {
		bg := lipgloss.NewStyle().Background(lipgloss.Color("237"))
		var cursorStr string
		switch {
		case isCut:
			cursorStr = bg.Bold(true).Foreground(lipgloss.Color("208")).Render("✂ ")
		default:
			cursorStr = bg.Bold(true).Foreground(lipgloss.Color("12")).Render("▶ ")
		}
		keyColor := lipgloss.Color("15")
		if isChecked {
			keyColor = lipgloss.Color("11") // yellow when selected+cursor
		} else if isCut {
			keyColor = lipgloss.Color("208") // orange when cut+cursor
		}
		keyPart := bg.Bold(true).Foreground(keyColor).Render(key)
		summaryPart := bg.Foreground(lipgloss.Color("15")).Render("  " + summary + "  ")
		epicStyle := bg.Foreground(lipgloss.Color("244"))
		if epicColor != "" {
			epicStyle = bg.Foreground(lipgloss.Color(epicColor))
		}
		epicPart := epicStyle.Render(epic + " ")
		typePart := bg.Bold(true).Foreground(lipgloss.Color(blTypeColor(issue.IssueType))).Render(issueType + " ")
		spPart := bg.Foreground(lipgloss.Color("252")).Render(sp + " ")
		assigneePart := bg.Foreground(lipgloss.Color("252")).Render(assignee)
		return cursorStr + keyPart + summaryPart + epicPart + typePart + spPart + assigneePart
	}

	var cursorStr string
	var keyPart string
	switch {
	case isCut:
		cursorStr = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render("✂ ")
		keyPart = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208")).Render(key)
	case isChecked:
		cursorStr = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("● ")
		keyPart = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render(key)
	default:
		cursorStr = "  "
		keyPart = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render(key)
	}
	summaryPart := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render("  " + summary + "  ")
	epicStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	if epicColor != "" {
		epicStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(epicColor))
	}
	epicPart := epicStyle.Render(epic + " ")
	typePart := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(blTypeColor(issue.IssueType))).Render(issueType + " ")
	spPart := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(sp + " ")
	assigneePart := dim.Render(assignee)
	return cursorStr + keyPart + summaryPart + epicPart + typePart + spPart + assigneePart
}

func blMoveMultiCmd(client api.Client, keys []string, targetSprintID, targetGroupIdx int) tea.Cmd {
	return func() tea.Msg {
		var err error
		if targetSprintID == 0 {
			err = client.MoveIssuesToBacklog(keys)
		} else {
			err = client.MoveIssuesToSprint(targetSprintID, keys)
		}
		return blMoveMultiDoneMsg{
			movedKeys:      keys,
			targetGroupIdx: targetGroupIdx,
			err:            err,
		}
	}
}

func runBacklogTUI(client api.Client, groups []models.SprintGroup) (blResult, error) {
	m := newBacklogModel(client, groups)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return blResult{}, fmt.Errorf("backlog: %w", err)
	}
	return final.(blModel).result, nil
}
