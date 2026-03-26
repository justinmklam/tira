package app

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/display"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

// --- backlog TUI model ---

type blState int

const (
	blList    blState = iota
	blLoading         // fetching full issue for detail view
	blDetail
	blFilter
	blParentPicker     // floating parent/epic picker
	blAssignPicker     // floating assignee picker
	blStoryPointInput  // floating story point input
	blStatusPicker     // floating status picker
	blEpicFilterPicker // floating epic filter picker
	blSprintForm       // create or edit sprint (sprintFormEditID == 0 means create)
	blKeySearch        // jump-to-issue-number search (f key)
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
	editKey        string
	commentKey     string
	commentSummary string
	refresh        bool
	create         bool
	createSprintID int
	createGroupIdx int
}

type blMoveMultiDoneMsg struct {
	movedKeys      []string
	firstMovedKey  string
	targetGroupIdx int
	err            error
}

type blRankDoneMsg struct{ err error }

// blBulkDoneMsg carries results from parallel bulk operations.
// Keys and Errors are parallel slices (nil error = success for that key).
// FullRefresh is set for operations (e.g. status transitions) where a targeted
// per-issue re-fetch is insufficient — e.g. because GetIssue does not return
// StatusID, so the kanban column placement would be stale.
type blBulkDoneMsg struct {
	Keys        []string
	Errors      []error
	FullRefresh bool
}

type yankMsg struct{}

type yankDoneMsg struct{}

type blSprintDoneMsg struct {
	created *models.Sprint // non-nil when a new sprint was just created
	err     error
}

// sidebarIssueFetchedMsg is sent when the sidebar's full issue is fetched.
type sidebarIssueFetchedMsg struct {
	issue *models.Issue
	err   error
}

// blSidebarDebounceMsg is sent after a short delay to trigger a sidebar fetch.
// The fetch is skipped if sidebarPendingKey has changed since the tick was fired.
type blSidebarDebounceMsg struct{ key string }

type blModel struct {
	state   blState
	client  api.Client
	boardID int
	project string
	jiraURL string

	groups    []models.SprintGroup
	rows      []blRow
	cursor    int
	offset    int
	collapsed map[int]bool

	filter      string
	filterInput textinput.Model

	keySearchInput textinput.Model

	width  int
	height int

	loadSpinner spinner.Model
	detailIssue *models.Issue
	detailView  viewport.Model

	selected     map[string]bool // issue keys marked with spacebar
	cutKeys      map[string]bool // issue keys marked for move with 'x'
	visualMode   bool
	visualAnchor int  // row index where 'v' was pressed
	moving       bool // true while a move API call is in flight

	// parent picker state
	parentPicker     tui.PickerModel
	parentTargetKeys []string

	// assignee picker state
	assignPicker     tui.PickerModel
	assignTargetKeys []string

	// story point input state
	storyPointInput      textinput.Model
	storyPointTargetKeys []string

	// status picker state
	statusPicker     tui.PickerModel
	statusTargetKeys []string

	// epic filter state
	filterEpic       string // empty means no filter
	epicFilterPicker tui.PickerModel

	// sprint create/edit form state
	sprintFormName       textinput.Model
	sprintFormStart      textinput.Model
	sprintFormDuration   textinput.Model
	sprintFormField      int    // active field: 0=name, 1=start, 2=duration
	sprintFormEditID     int    // 0=creating new sprint, >0=editing existing sprint
	sprintFormError      string // validation or API error message
	sprintFormSubmitting bool   // true while API call is in flight

	// yank indicator state
	yankMessage string
	yankTimer   *time.Timer

	result   blResult
	quitting bool

	// Sidebar state (always visible in split-pane view)
	sidebarContent    string
	sidebarOffset     int           // scroll offset for sidebar content
	sidebarIssueKey   string        // key of issue being displayed in sidebar
	sidebarPendingKey string        // key waiting for debounce before fetch
	sidebarFullIssue  *models.Issue // full issue with description from API

	// lastIssue tracks the most recently selected issue (used when cursor is on sprint header)
	lastIssue *models.Issue
}

func blBuildRows(groups []models.SprintGroup, collapsed map[int]bool, filter string, filterEpic string) []blRow {
	var rows []blRow
	for i, g := range groups {
		if i > 0 {
			rows = append(rows, blRow{kind: blRowSpacer, groupIdx: i})
		}
		rows = append(rows, blRow{kind: blRowSprint, groupIdx: i, issueIdx: -1})
		if !collapsed[i] {
			for j, issue := range g.Issues {
				if blMatchesFilter(issue, filter, filterEpic) {
					rows = append(rows, blRow{kind: blRowIssue, groupIdx: i, issueIdx: j})
				}
			}
		}
	}
	return rows
}

func blMatchesFilter(issue models.Issue, filter string, filterEpic string) bool {
	f := strings.ToLower(filter)
	matchesText := strings.Contains(strings.ToLower(issue.Key), f) ||
		strings.Contains(strings.ToLower(issue.Summary), f)
	if !matchesText {
		return false
	}
	if filterEpic == "" {
		return true
	}
	// Check if issue matches the epic filter
	if issue.EpicKey == filterEpic || issue.EpicName == filterEpic {
		return true
	}
	return false
}

func newBacklogModel(client api.Client, boardID int, groups []models.SprintGroup, project, jiraURL string) (blModel, tea.Cmd) {
	collapsed := make(map[int]bool)

	ti := textinput.New()
	ti.Placeholder = "type to filter…"
	ti.CharLimit = 60

	spTi := textinput.New()
	spTi.Placeholder = "story points (e.g. 1, 2, 3, 5, 8)"
	spTi.CharLimit = 10

	ksTi := textinput.New()
	ksTi.Placeholder = "issue number…"
	ksTi.CharLimit = 20

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.ColorSpinner)

	m := blModel{
		state:           blList,
		client:          client,
		boardID:         boardID,
		project:         project,
		jiraURL:         jiraURL,
		groups:          groups,
		collapsed:       collapsed,
		filterInput:     ti,
		storyPointInput: spTi,
		keySearchInput:  ksTi,
		loadSpinner:     s,
		selected:        make(map[string]bool),
		cutKeys:         make(map[string]bool),
	}
	m.rows = blBuildRows(groups, collapsed, "", "")
	// Start cursor on the first issue row, skipping sprint headers.
	for i, row := range m.rows {
		if row.kind == blRowIssue {
			m.cursor = i
			break
		}
	}
	// Initialize sidebar content and trigger fetch for first issue
	issue := m.currentIssue()
	m.sidebarContent = renderSidebarContent(issue, 40) // Default width until we get window size
	m.sidebarOffset = 0
	if issue != nil {
		m.sidebarIssueKey = issue.Key
		return m, fetchSidebarIssueCmd(m.client, issue.Key)
	}
	return m, nil
}

// refreshData replaces the sprint groups and rebuilds the row list.
// Returns a command to re-fetch the sidebar issue at the correct width.
func (m *blModel) refreshData(groups []models.SprintGroup) tea.Cmd {
	m.groups = groups
	m.rows = blBuildRows(groups, m.collapsed, m.filter, m.filterEpic)
	m.cursor = tui.Clamp(m.cursor, 0, max(len(m.rows)-1, 0))
	// Reset sidebar cache and show a preview while the full issue is re-fetched.
	m.sidebarFullIssue = nil
	issue := m.currentIssue()
	m.sidebarContent = renderSidebarContent(issue, tui.DetailPaneWidth(m.width))
	m.sidebarOffset = 0
	if issue != nil {
		m.sidebarIssueKey = issue.Key
		return fetchSidebarIssueCmd(m.client, issue.Key)
	}
	m.sidebarIssueKey = ""
	return nil
}

// patchIssue updates a single issue in place within the sprint groups and
// rebuilds derived state (rows, sidebar). Preserves agile-only fields that
// GetIssue does not return (StatusID, EpicKey, EpicName, ProjectKey,
// StatusChangedDate) from the existing record.
func (m *blModel) patchIssue(fresh models.Issue) {
	for gi := range m.groups {
		for ii := range m.groups[gi].Issues {
			if m.groups[gi].Issues[ii].Key != fresh.Key {
				continue
			}
			existing := m.groups[gi].Issues[ii]
			// Preserve fields not returned by GetIssue.
			fresh.StatusID = existing.StatusID
			fresh.EpicKey = existing.EpicKey
			fresh.EpicName = existing.EpicName
			fresh.ProjectKey = existing.ProjectKey
			if fresh.StoryPoints == 0 {
				fresh.StoryPoints = existing.StoryPoints
			}
			if fresh.StatusChangedDate == "" {
				fresh.StatusChangedDate = existing.StatusChangedDate
			}
			m.groups[gi].Issues[ii] = fresh
		}
	}
	m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
	m.cursor = tui.Clamp(m.cursor, 0, max(len(m.rows)-1, 0))
	// Update sidebar if it is already showing this issue.
	if m.sidebarFullIssue != nil && m.sidebarFullIssue.Key == fresh.Key {
		m.sidebarFullIssue = &fresh
		m.sidebarContent = renderSidebarContent(&fresh, tui.DetailPaneWidth(m.width))
	}
}

// appendGroups adds lazy-loaded sprint groups (remaining sprints + backlog)
// to the backlog model and rebuilds the row list. Preserves cursor position.
func (m *blModel) appendGroups(groups []models.SprintGroup) {
	m.groups = append(m.groups, groups...)
	m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
	// Cursor stays where it is — new groups are appended below the viewport.
}

// insertIssue appends a newly created issue to the matching sprint group (or
// the backlog group when sprintID == 0) and rebuilds the row list.
func (m *blModel) insertIssue(issue models.Issue, sprintID int) {
	for gi := range m.groups {
		g := &m.groups[gi]
		match := (sprintID == 0 && g.Sprint.State == "backlog") ||
			(sprintID != 0 && g.Sprint.ID == sprintID)
		if match {
			g.Issues = append(g.Issues, issue)
			break
		}
	}
	m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
	m.cursor = tui.Clamp(m.cursor, 0, max(len(m.rows)-1, 0))
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
// otherwise just the current issue. Keys are returned in display order.
func (m blModel) moveKeys() []string {
	if combined := m.allSelected(); len(combined) > 0 {
		return m.keysInDisplayOrder(combined)
	}
	if issue := m.currentIssue(); issue != nil {
		return []string{issue.Key}
	}
	return nil
}

// lastIssueKey returns the key of the last issue in the group that is NOT in
// excludeSet (used to find the rank-after anchor for bottom-of-sprint placement).
func lastIssueKey(issues []models.Issue, excludeSet map[string]bool) string {
	for i := len(issues) - 1; i >= 0; i-- {
		if !excludeSet[issues[i].Key] {
			return issues[i].Key
		}
	}
	return ""
}

// keysInDisplayOrder returns the subset of keys that appear in the given set,
// ordered by their position in the current groups/issues list.
func (m blModel) keysInDisplayOrder(keySet map[string]bool) []string {
	keys := make([]string, 0, len(keySet))
	for _, g := range m.groups {
		for _, issue := range g.Issues {
			if keySet[issue.Key] {
				keys = append(keys, issue.Key)
			}
		}
	}
	return keys
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

// navigateToKey moves the cursor to the first row matching key.
func (m *blModel) navigateToKey(key string) {
	for i, row := range m.rows {
		if row.kind == blRowIssue && m.groups[row.groupIdx].Issues[row.issueIdx].Key == key {
			m.cursor = i
			*m = blScrollToFit(*m)
			return
		}
	}
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

// commitVisualSelection merges the visual range into the base selection.
func (m *blModel) commitVisualSelection() {
	for k := range m.visualIssueKeys() {
		if m.selected[k] {
			delete(m.selected, k)
		} else {
			m.selected[k] = true
		}
	}
	m.visualMode = false
}

func (m blModel) Init() tea.Cmd { return nil }

func (m blModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.state == blDetail {
			vpW, vpH := tui.OverlayViewportSize(m.width, m.height)
			m.detailView.Width = vpW
			m.detailView.Height = vpH
		}
		// Re-render sidebar at the actual terminal width.
		// The initial render uses a default width since the terminal size isn't known yet.
		if m.width > 0 {
			detailW := tui.DetailPaneWidth(m.width)
			if m.sidebarFullIssue != nil {
				m.sidebarContent = renderSidebarContent(m.sidebarFullIssue, detailW)
			} else if issue := m.currentIssue(); issue != nil {
				m.sidebarContent = renderSidebarContent(issue, detailW)
			} else if m.lastIssue != nil {
				m.sidebarContent = renderSidebarContent(m.lastIssue, detailW)
			}
		}
		// Trigger initial fetch if we haven't done so yet
		if m.width > 0 && m.sidebarIssueKey == "" {
			if cur := m.currentIssue(); cur != nil {
				m.sidebarIssueKey = cur.Key
				return m, fetchSidebarIssueCmd(m.client, cur.Key)
			}
		}
		return m, nil

	case issueFetchedMsg:
		if msg.err != nil {
			m.state = blList
			return m, nil
		}
		m.detailIssue = msg.issue
		vpW, vpH := tui.OverlayViewportSize(m.width, m.height)
		vp := viewport.New(vpW, vpH)
		vp.SetContent(msg.content)
		m.detailView = vp
		m.state = blDetail
		return m, nil

	case blSidebarDebounceMsg:
		if msg.key == m.sidebarPendingKey {
			m.sidebarIssueKey = msg.key
			m.sidebarFullIssue = nil
			return m, fetchSidebarIssueCmd(m.client, msg.key)
		}
		return m, nil

	case sidebarIssueFetchedMsg:
		if msg.err == nil && msg.issue != nil {
			m.sidebarFullIssue = msg.issue
			// Always re-render at the current width (don't use pre-rendered content)
			width := tui.DetailPaneWidth(m.width)
			if width < 20 {
				width = 40 // Fallback if window size not known
			}
			m.sidebarContent = renderSidebarContent(msg.issue, width)
			m.sidebarOffset = 0
		}
		return m, nil

	case spinner.TickMsg:
		if m.state == blLoading || m.moving || m.sprintFormSubmitting {
			var cmd tea.Cmd
			m.loadSpinner, cmd = m.loadSpinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case blMoveMultiDoneMsg:
		m.moving = false
		if msg.err == nil {
			movedSet := make(map[string]bool, len(msg.movedKeys))
			for _, k := range msg.movedKeys {
				movedSet[k] = true
			}
			// Remove moved issues from all groups, append to target.
			var movedIssues []models.Issue
			for i := range m.groups {
				var remaining []models.Issue
				for _, issue := range m.groups[i].Issues {
					if movedSet[issue.Key] {
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
			m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
			// Navigate cursor to the first moved issue's new position.
			if msg.firstMovedKey != "" {
				for i, row := range m.rows {
					if row.kind == blRowIssue && row.groupIdx == msg.targetGroupIdx &&
						m.groups[msg.targetGroupIdx].Issues[row.issueIdx].Key == msg.firstMovedKey {
						m.cursor = i
						break
					}
				}
			} else {
				m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
			}
			return blScrollToFit(m), nil
		}
		return m, nil

	case blRankDoneMsg:
		// Rank API failures are non-fatal; local state is already updated.
		return m, nil

	case blBulkDoneMsg:
		m.selected = make(map[string]bool)
		m.visualMode = false
		if msg.FullRefresh {
			// Status transitions need a full refresh: GetIssue does not return
			// StatusID, so a targeted patch would leave the kanban column stale.
			m.result.refresh = true
			return m, nil
		}
		// For all other bulk ops, re-fetch only the affected issues.
		var cmds []tea.Cmd
		for i, key := range msg.Keys {
			if i < len(msg.Errors) && msg.Errors[i] == nil {
				cmds = append(cmds, issueRefreshCmd(m.client, key))
			}
		}
		return m, tea.Batch(cmds...)

	case blSprintDoneMsg:
		m.sprintFormSubmitting = false
		if msg.err != nil {
			m.sprintFormError = msg.err.Error()
			m.state = blSprintForm
			// Re-focus the active field so the user can correct and resubmit.
			var cmd tea.Cmd
			switch m.sprintFormField {
			case 0:
				cmd = m.sprintFormName.Focus()
			case 1:
				cmd = m.sprintFormStart.Focus()
			case 2:
				cmd = m.sprintFormDuration.Focus()
			}
			return m, cmd
		}
		m.state = blList
		m.result.refresh = true
		return m, nil

	case yankMsg:
		m.yankMessage = "YANKED"
		if m.yankTimer != nil {
			m.yankTimer.Stop()
		}
		m.yankTimer = time.AfterFunc(2*time.Second, func() {
			// This will be called after 2 seconds to clear the message
		})
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return yankDoneMsg{}
		})

	case yankDoneMsg:
		m.yankMessage = ""
		return m, nil
	}

	switch m.state {
	case blList:
		return m.updateList(msg)
	case blFilter:
		return m.updateFilter(msg)
	case blDetail:
		return m.updateDetail(msg)
	case blParentPicker:
		return m.updateParentPicker(msg)
	case blAssignPicker:
		return m.updateAssignPicker(msg)
	case blStoryPointInput:
		return m.updateStoryPointInput(msg)
	case blStatusPicker:
		return m.updateStatusPicker(msg)
	case blEpicFilterPicker:
		return m.updateEpicFilterPicker(msg)
	case blSprintForm:
		return m.updateSprintForm(msg)
	case blKeySearch:
		return m.updateKeySearch(msg)
	}
	return m, nil
}

// blMoveMultiCmd moves keys to a sprint (or backlog) and, when rankAfterKey is
// non-empty, explicitly ranks them after that issue so they land at the bottom.
func blMoveMultiCmd(client api.Client, keys []string, targetSprintID, targetGroupIdx int, rankAfterKey string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if targetSprintID == 0 {
			err = client.MoveIssuesToBacklog(keys)
		} else {
			err = client.MoveIssuesToSprint(targetSprintID, keys)
			if err == nil && rankAfterKey != "" {
				err = client.RankIssues(keys, rankAfterKey, "")
			}
		}
		firstKey := ""
		if len(keys) > 0 {
			firstKey = keys[0]
		}
		return blMoveMultiDoneMsg{movedKeys: keys, firstMovedKey: firstKey, targetGroupIdx: targetGroupIdx, err: err}
	}
}

func blRankCmd(client api.Client, keys []string, rankAfterKey, rankBeforeKey string) tea.Cmd {
	return func() tea.Msg {
		return blRankDoneMsg{err: client.RankIssues(keys, rankAfterKey, rankBeforeKey)}
	}
}

// copyToClipboardCmd copies the given text to the system clipboard and sends yankMsg on completion.
func copyToClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("pbcopy")
		default:
			cmd = exec.Command("xclip", "-selection", "clipboard")
		}
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
		return yankMsg{}
	}
}

// issueURL returns the absolute URL for an issue key.
func (m blModel) issueURL(key string) string {
	baseURL := strings.TrimRight(m.jiraURL, "/")
	return fmt.Sprintf("%s/browse/%s", baseURL, key)
}

// nextSprintName derives the name for a new sprint from the last non-backlog sprint.
// It finds a trailing integer and increments it; if none is found, appends " 1".
func nextSprintName(groups []models.SprintGroup) string {
	var lastName string
	for _, g := range groups {
		if g.Sprint.State != "backlog" && g.Sprint.Name != "" {
			lastName = g.Sprint.Name
		}
	}
	if lastName == "" {
		return "Sprint 1"
	}
	re := regexp.MustCompile(`^(.*?)(\d+)\s*$`)
	if m := re.FindStringSubmatch(lastName); m != nil {
		n, _ := strconv.Atoi(m[2])
		return m[1] + strconv.Itoa(n+1)
	}
	return lastName + " 1"
}

// computeEndDate returns startDate + durationWeeks*7 days formatted as YYYY-MM-DD.
// Returns "" if startDate is not a valid YYYY-MM-DD or durationWeeks < 1.
func computeEndDate(startDate string, durationWeeks int) string {
	if durationWeeks < 1 {
		return ""
	}
	t, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, durationWeeks*7).Format("2006-01-02")
}

// sprintDurationWeeks returns the number of weeks between two YYYY-MM-DD date strings.
// Returns 2 as a safe default if either date is missing or unparseable.
func sprintDurationWeeks(startDate, endDate string) int {
	s := startDate
	if len(s) > 10 {
		s = s[:10]
	}
	e := endDate
	if len(e) > 10 {
		e = e[:10]
	}
	t1, err1 := time.Parse("2006-01-02", s)
	t2, err2 := time.Parse("2006-01-02", e)
	if err1 != nil || err2 != nil {
		return 2
	}
	days := int(t2.Sub(t1).Hours() / 24)
	weeks := (days + 6) / 7
	if weeks < 1 {
		return 2
	}
	return weeks
}

func parseFloat(s string) (float64, error) {
	var result float64
	_, err := fmt.Sscanf(s, "%f", &result)
	return result, err
}

// renderMarkdownWithGlamour renders a markdown string through glamour at the given wrap width.
func renderMarkdownWithGlamour(md string, wrapWidth int) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(tui.GlamourStyleConfig),
		glamour.WithWordWrap(wrapWidth),
	)
	if err != nil {
		return md
	}
	content, err := renderer.Render(md)
	if err != nil {
		return md
	}
	return strings.TrimLeft(content, "\n")
}

// renderIssueContent renders an issue's markdown through glamour at the given wrap width.
// Used by the detail overlay and sidebar.
func renderIssueContent(issue *models.Issue, wrapWidth int) string {
	return renderMarkdownWithGlamour(display.RenderIssue(issue), wrapWidth)
}

// renderIssueDetailView renders a common issue detail view with border and footer.
// Used by both backlog and kanban detail overlays.
func renderIssueDetailView(issue *models.Issue, detailView viewport.Model, width, height, overlayW, innerW int) string {
	footer := tui.MutedStyle.Render("  e: edit   c: comment   o: open in browser   esc/q: back   j/k: scroll")
	body := detailView.View() + "\n" + footer

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorAccent).
		Width(innerW).
		Render(body)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

// renderSidebarContent returns the sidebar content for the given issue.
// It uses the same rendering as the detail overlay.
func renderSidebarContent(issue *models.Issue, width int) string {
	if issue == nil {
		return tui.MutedStyle.Render("No issue selected")
	}
	return renderIssueContent(issue, width-4)
}

// fetchSidebarIssueCmd fetches the full issue from the Jira API for sidebar display.
// Rendering happens in the Update handler using the current terminal width.
func fetchSidebarIssueCmd(client api.Client, key string) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.GetIssue(key)
		return sidebarIssueFetchedMsg{issue: issue, err: err}
	}
}

// fetchIssueCmd fetches the full issue from the Jira API for detail view overlay.
// This is a shared function used by both backlog and kanban views.
func fetchIssueCmd(client api.Client, key string, vpW int) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.GetIssue(key)
		if err != nil {
			return issueFetchedMsg{err: err}
		}
		return issueFetchedMsg{issue: issue, content: renderIssueContent(issue, vpW)}
	}
}
