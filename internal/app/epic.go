package app

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/debug"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

type epicState int

const (
	epicList epicState = iota
	epicFilter
	epicLoading
	epicDetail
	epicLabelLoading
	epicLabelInput
	epicLabelSaving
	epicLinkPicker
)

// epicItem is the board-derived projection of an epic and its represented
// children. Its position is established by the first child encountered.
type epicItem struct {
	Key              string
	Name             string
	Summary          string
	ChildCount       int
	StoryPoints      float64
	EpicStatus       string
	FirstIssueKey    string
	FirstSprintName  string
	FirstSprintState string
	FirstSprintIndex int
	FirstGroupName   string
	FirstGroupState  string
	FirstLocation    string
}

type epicResult struct {
	filterBacklogKey string
	refresh          bool
	quit             bool
}

// epicChildren holds the child work items of the selected epic together with
// the state of the fetch that produced them.
type epicChildren struct {
	items   []models.LinkedIssue
	key     string
	err     string
	loading bool
}

// epicLabelsFetchedMsg is sent when the full epic is fetched for label editing.
type epicLabelsFetchedMsg struct {
	key   string
	issue *models.Issue
	err   error
}

// epicChildrenFetchedMsg carries the child work items of an epic fetched from
// the JQL search API.
type epicChildrenFetchedMsg struct {
	key      string
	children []models.LinkedIssue
	err      error
}

type epicLabelsSavedMsg struct {
	key    string
	labels []string
	err    error
}

// epicModel is intentionally independent of boardModel so it can be wired in
// as a peer view without changing the board data or API contracts.
type epicModel struct {
	state   epicState
	client  api.Client
	jiraURL string

	items    []epicItem
	allItems []epicItem
	cursor   int
	offset   int
	width    int
	height   int

	filter      string
	filterInput textinput.Model

	loadSpinner spinner.Model
	loading     bool
	loadError   string
	quitting    bool
	result      epicResult

	sidebarContent   string
	sidebarOffset    int
	sidebarIssueKey  string
	sidebarFullIssue *models.Issue

	// Child work items of the selected epic, fetched via JQL search.
	children epicChildren

	detailIssue *models.Issue
	detailView  viewport.Model

	linkPicker       linkedItemPicker
	linkPickerReturn epicState

	labelInput     textinput.Model
	labelTargetKey string
	labelError     string
}

// buildEpicItems projects represented epics in flattened group/issue order.
func buildEpicItems(groups []models.SprintGroup) []epicItem {
	items := make([]epicItem, 0)
	byKey := make(map[string]int)

	for groupIndex, group := range groups {
		location := group.Sprint.Name
		if location == "" {
			location = group.Sprint.State
		}
		if location == "" {
			location = "Backlog"
		}

		for _, issue := range group.Issues {
			if issue.EpicKey == "" {
				continue
			}

			idx, exists := byKey[issue.EpicKey]
			if !exists {
				name := issue.EpicName
				if name == "" {
					name = issue.EpicKey
				}
				items = append(items, epicItem{
					Key:              issue.EpicKey,
					Name:             name,
					Summary:          name,
					ChildCount:       1,
					StoryPoints:      issue.StoryPoints,
					EpicStatus:       issue.EpicStatus,
					FirstIssueKey:    issue.Key,
					FirstSprintName:  group.Sprint.Name,
					FirstSprintState: group.Sprint.State,
					FirstSprintIndex: groupIndex,
					FirstGroupName:   group.Sprint.Name,
					FirstGroupState:  group.Sprint.State,
					FirstLocation:    location,
				})
				byKey[issue.EpicKey] = len(items) - 1
				continue
			}

			item := &items[idx]
			item.ChildCount++
			item.StoryPoints += issue.StoryPoints
			if item.EpicStatus == "" && issue.EpicStatus != "" {
				item.EpicStatus = issue.EpicStatus
			}
			if item.Name == item.Key && issue.EpicName != "" {
				item.Name = issue.EpicName
				item.Summary = issue.EpicName
			}
		}
	}

	openItems := items[:0]
	for _, item := range items {
		if !strings.EqualFold(item.EpicStatus, "closed") {
			openItems = append(openItems, item)
		}
	}
	return openItems
}

func epicMatchesFilter(item epicItem, filter string) bool {
	f := strings.ToLower(filter)
	return strings.Contains(strings.ToLower(item.Key), f) ||
		strings.Contains(strings.ToLower(item.Name), f) ||
		strings.Contains(strings.ToLower(item.Summary), f)
}

func filterEpicItems(items []epicItem, filter string) []epicItem {
	if filter == "" {
		return items
	}

	filtered := make([]epicItem, 0, len(items))
	for _, item := range items {
		if epicMatchesFilter(item, filter) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func newEpicModel(client api.Client, groups []models.SprintGroup, jiraURL string, loading bool) (epicModel, tea.Cmd) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.ColorSpinner)

	items := buildEpicItems(groups)
	filterInput := textinput.New()
	filterInput.Placeholder = "type to filter…"
	filterInput.CharLimit = 60

	m := epicModel{
		state:       epicList,
		client:      client,
		jiraURL:     strings.TrimRight(jiraURL, "/"),
		items:       items,
		allItems:    items,
		filterInput: filterInput,
		loadSpinner: s,
		loading:     loading,
	}
	m.updateSidebar()
	return m, m.selectionCommand()
}

// refreshData replaces the projection while preserving the selected epic by
// key where possible. It returns a command only when a new sidebar fetch is
// needed.
func (m *epicModel) refreshData(groups []models.SprintGroup, loading bool, loadErr error) tea.Cmd {
	selectedKey := m.selectedKey()
	m.allItems = buildEpicItems(groups)
	m.items = filterEpicItems(m.allItems, m.filter)
	m.loading = loading
	m.loadError = ""
	if loadErr != nil {
		m.loadError = loadErr.Error()
	}

	m.cursor = 0
	if selectedKey != "" {
		for i, item := range m.items {
			if item.Key == selectedKey {
				m.cursor = i
				break
			}
		}
	}
	m.cursor = tui.Clamp(m.cursor, 0, max(len(m.items)-1, 0))
	m.offset = tui.Clamp(m.offset, 0, max(m.cursor, 0))
	m.ensureVisible()

	if m.selectedKey() != m.sidebarIssueKey {
		m.sidebarFullIssue = nil
		m.clearChildren()
		m.updateSidebar()
		return m.selectionCommand()
	}
	m.updateSidebar()
	return nil
}

func (m *epicModel) applyFilter() tea.Cmd {
	selectedKey := m.selectedKey()
	if m.allItems == nil {
		m.allItems = m.items
	}
	source := m.allItems
	m.items = filterEpicItems(source, m.filter)
	m.cursor = tui.Clamp(m.cursor, 0, max(len(m.items)-1, 0))
	m.ensureVisible()

	if selectedKey == m.selectedKey() {
		return nil
	}
	m.sidebarIssueKey = ""
	m.sidebarFullIssue = nil
	m.clearChildren()
	m.updateSidebar()
	return m.selectionCommand()
}

func (m epicModel) selectedKey() string {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return ""
	}
	return m.items[m.cursor].Key
}

// clearChildren drops cached child work items so the next render fetches them
// for the newly selected epic.
func (m *epicModel) clearChildren() {
	m.children = epicChildren{}
}

// renderDetail rebuilds the detail viewport for the open epic, keeping the
// current scroll position so late-arriving child work items do not reset it.
func (m *epicModel) renderDetail() {
	if m.detailIssue == nil {
		return
	}
	vpW, vpH := tui.OverlayViewportSize(m.width, m.height)
	offset := m.detailView.YOffset()
	vp := viewport.New(viewport.WithWidth(vpW), viewport.WithHeight(vpH))
	vp.SetContent(renderEpicIssueContent(m.detailIssue, m.children.items, vpW))
	vp.SetYOffset(offset)
	m.detailView = vp
}

func (m epicModel) selectedItem() *epicItem {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	return &m.items[m.cursor]
}

func (m epicModel) previewIssue() *models.Issue {
	item := m.selectedItem()
	if item == nil {
		return nil
	}
	return &models.Issue{
		Key:         item.Key,
		Summary:     item.Name,
		IssueType:   "Epic",
		EpicKey:     item.Key,
		EpicName:    item.Name,
		Status:      item.EpicStatus,
		SprintName:  item.FirstLocation,
		StoryPoints: item.StoryPoints,
	}
}

func (m *epicModel) updateSidebar() {
	width := tui.DetailPaneWidth(m.width)
	if width < 20 {
		width = 40
	}
	issue := m.previewIssue()
	if m.sidebarFullIssue != nil && m.sidebarFullIssue.Key == m.selectedKey() {
		issue = m.sidebarFullIssue
	}
	m.sidebarContent = renderEpicSidebarContent(issue, m.selectedItem(), m.children, width)
	m.sidebarOffset = 0
}

func (m *epicModel) sidebarCommand() tea.Cmd {
	key := m.selectedKey()
	if key == "" || key == m.sidebarIssueKey || m.client == nil {
		return nil
	}
	return fetchSidebarIssueCmd(m.client, key)
}

// childrenCommand fetches the child work items for the selected epic when they
// are not already loaded.
func (m *epicModel) childrenCommand() tea.Cmd {
	key := m.selectedKey()
	if key == "" || key == m.children.key || m.client == nil {
		return nil
	}
	// Record the key immediately so a second call does not fetch the same epic
	// twice while the first request is still in flight.
	m.children = epicChildren{key: key, loading: true}
	// Re-render so the sidebar shows the in-flight state immediately.
	m.updateSidebar()
	return fetchEpicChildrenCmd(m.client, key)
}

// selectionCommand batches the sidebar and child-work-item fetches needed for
// the currently selected epic.
func (m *epicModel) selectionCommand() tea.Cmd {
	return tea.Batch(m.sidebarCommand(), m.childrenCommand())
}

func (m epicModel) viewHeight() int {
	if m.height < 5 {
		return 1
	}
	return m.height - 4
}

func (m *epicModel) updateSelection(next int) tea.Cmd {
	m.cursor = tui.Clamp(next, 0, max(len(m.items)-1, 0))
	m.offset = tui.Clamp(m.offset, 0, max(m.cursor, 0))
	if m.selectedKey() == m.sidebarIssueKey {
		m.updateSidebar()
		return nil
	}
	m.sidebarIssueKey = ""
	m.sidebarFullIssue = nil
	m.clearChildren()
	m.updateSidebar()
	return m.selectionCommand()
}

func (m *epicModel) ensureVisible() {
	vh := m.viewHeight()
	if vh <= 0 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+vh {
		m.offset = m.cursor - vh + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m epicModel) issueURL(key string) string {
	return fmt.Sprintf("%s/browse/%s", strings.TrimRight(m.jiraURL, "/"), key)
}

func (m *epicModel) beginLabelEdit() tea.Cmd {
	key := m.selectedKey()
	if key == "" || m.client == nil {
		return nil
	}

	m.labelTargetKey = key
	m.labelError = ""
	m.loadError = ""
	if m.sidebarFullIssue != nil && m.sidebarFullIssue.Key == key {
		return m.openLabelInput(m.sidebarFullIssue)
	}

	m.state = epicLabelLoading
	return tea.Batch(m.loadSpinner.Tick, fetchEpicLabelsCmd(m.client, key))
}

func (m *epicModel) openLabelInput(issue *models.Issue) tea.Cmd {
	if issue == nil || issue.Key != m.labelTargetKey {
		return nil
	}

	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "comma-separated labels"
	input.CharLimit = 200
	input.SetValue(strings.Join(issue.Labels, ", "))
	m.labelInput = input
	m.setLabelInputSize()
	m.state = epicLabelInput
	return m.labelInput.Focus()
}

func (m *epicModel) setLabelInputSize() {
	width := m.width
	if width == 0 {
		width = 120
	}
	overlayW, _ := tui.OverlaySize(width, m.height)
	inputW := overlayW - 8
	if inputW < 24 {
		inputW = 24
	}
	m.labelInput.SetWidth(inputW)
}

func (m epicModel) Init() tea.Cmd { return nil }

func (m epicModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.state == epicDetail {
			vpW, vpH := tui.OverlayViewportSize(m.width, m.height)
			m.detailView.SetWidth(vpW)
			m.detailView.SetHeight(vpH)
		}
		if m.state == epicLabelInput {
			m.setLabelInputSize()
		}
		m.updateSidebar()
		if m.sidebarIssueKey == "" {
			return m, m.selectionCommand()
		}
		return m, nil

	case issueFetchedMsg:
		if msg.err != nil {
			m.state = epicList
			m.loadError = msg.err.Error()
			return m, nil
		}
		m.detailIssue = msg.issue
		m.state = epicDetail
		m.renderDetail()
		if m.children.key == msg.issue.Key {
			return m, nil
		}
		return m, m.childrenCommand()

	case epicLabelsFetchedMsg:
		if m.state != epicLabelLoading || msg.key != m.labelTargetKey {
			return m, nil
		}
		if msg.err != nil {
			debug.LogError("client.GetIssue for labels", msg.err)
			m.state = epicList
			m.labelTargetKey = ""
			m.loadError = msg.err.Error()
			return m, nil
		}
		if msg.issue == nil {
			err := fmt.Errorf("fetching labels for %s returned no issue", msg.key)
			debug.LogError("client.GetIssue for labels", err)
			m.state = epicList
			m.labelTargetKey = ""
			m.loadError = err.Error()
			return m, nil
		}
		m.sidebarIssueKey = msg.issue.Key
		m.sidebarFullIssue = msg.issue
		m.updateSidebar()
		return m, m.openLabelInput(msg.issue)

	case epicLabelsSavedMsg:
		if m.state != epicLabelSaving || msg.key != m.labelTargetKey {
			return m, nil
		}
		if msg.err != nil {
			m.state = epicLabelInput
			m.labelError = msg.err.Error()
			return m, nil
		}
		if m.sidebarFullIssue != nil && m.sidebarFullIssue.Key == msg.key {
			m.sidebarFullIssue.Labels = append([]string(nil), msg.labels...)
			m.updateSidebar()
		}
		m.state = epicList
		m.labelTargetKey = ""
		m.labelError = ""
		return m, nil

	case sidebarIssueFetchedMsg:
		if msg.err == nil && msg.issue != nil && msg.issue.Key == m.selectedKey() {
			m.sidebarIssueKey = msg.issue.Key
			m.sidebarFullIssue = msg.issue
			m.updateSidebar()
			// The picker reads its items from the full issue, so an open picker
			// has to be refreshed when the fetch lands.
			if m.state == epicLinkPicker {
				m.linkPicker.reopen(m.linkPickerItems())
			}
		}
		return m, nil

	case epicChildrenFetchedMsg:
		if msg.key != m.selectedKey() {
			return m, nil
		}
		if msg.err != nil {
			m.children = epicChildren{key: msg.key, err: msg.err.Error()}
		} else {
			m.children = epicChildren{key: msg.key, items: msg.children}
		}
		m.updateSidebar()
		if m.state == epicDetail {
			m.renderDetail()
		}
		if m.state == epicLinkPicker {
			m.linkPicker.reopen(m.linkPickerItems())
		}
		return m, nil

	case spinner.TickMsg:
		if m.state == epicLoading || m.state == epicLabelLoading || m.state == epicLabelSaving {
			var cmd tea.Cmd
			m.loadSpinner, cmd = m.loadSpinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if m.state == epicFilter {
		return m.updateFilter(msg)
	}

	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if m.state == epicDetail {
			var cmd tea.Cmd
			m.detailView, cmd = m.detailView.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if m.state == epicLabelLoading {
		if key.String() == "esc" {
			m.state = epicList
			m.labelTargetKey = ""
			m.labelError = ""
		}
		return m, nil
	}

	if m.state == epicLabelInput {
		switch key.String() {
		case "ctrl+c":
			m.quitting = true
			m.result.quit = true
			return m, nil
		case "esc":
			m.state = epicList
			m.labelTargetKey = ""
			m.labelError = ""
			return m, nil
		case "enter":
			labels := parseLabels(m.labelInput.Value())
			m.labelError = ""
			m.state = epicLabelSaving
			return m, tea.Batch(m.loadSpinner.Tick, setEpicLabelsCmd(m.client, m.labelTargetKey, labels))
		}
		var cmd tea.Cmd
		m.labelInput, cmd = m.labelInput.Update(msg)
		return m, cmd
	}

	if m.state == epicLabelSaving {
		if key.String() == "ctrl+c" {
			m.quitting = true
			m.result.quit = true
		}
		return m, nil
	}

	if m.state == epicLinkPicker {
		if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
			m.quitting = true
			m.result.quit = true
			return m, nil
		}
		action, cmd := m.linkPicker.handleKey(msg)
		switch action {
		case linkedPickerAborted:
			m.state = m.linkPickerReturn
		case linkedPickerConfirmed:
			key := m.linkPicker.selectedKey()
			m.state = m.linkPickerReturn
			if key != "" {
				return m, openInBrowserCmd(m.issueURL(key))
			}
		}
		return m, cmd
	}

	if m.state == epicDetail {
		switch key.String() {
		case "esc", "q":
			m.state = epicList
			m.detailIssue = nil
			return m, nil
		case "o":
			if m.detailIssue != nil {
				return m, openInBrowserCmd(m.issueURL(m.detailIssue.Key))
			}
		case "L":
			return m, m.openLinkPicker()
		}
		var cmd tea.Cmd
		m.detailView, cmd = m.detailView.Update(msg)
		return m, cmd
	}

	switch key.String() {
	case "ctrl+c", "q":
		m.quitting = true
		m.result.quit = true
		return m, nil
	case "j", "down":
		var cmd tea.Cmd
		cmd = m.updateSelection(m.cursor + 1)
		m.ensureVisible()
		return m, cmd
	case "k", "up":
		var cmd tea.Cmd
		cmd = m.updateSelection(m.cursor - 1)
		m.ensureVisible()
		return m, cmd
	case "g":
		return m, m.updateSelection(0)
	case "G":
		cmd := m.updateSelection(len(m.items) - 1)
		m.ensureVisible()
		return m, cmd
	case "d", "pgdown", "ctrl+f":
		cmd := m.updateSelection(m.cursor + max(m.viewHeight()/4, 1))
		m.ensureVisible()
		return m, cmd
	case "u", "pgup", "ctrl+b":
		cmd := m.updateSelection(m.cursor - max(m.viewHeight()/4, 1))
		m.ensureVisible()
		return m, cmd
	case "ctrl+d":
		m.sidebarOffset += max(m.viewHeight()/4, 1)
		m.clampSidebarOffset()
		return m, nil
	case "ctrl+u":
		m.sidebarOffset -= max(m.viewHeight()/4, 1)
		m.clampSidebarOffset()
		return m, nil
	case "enter":
		item := m.selectedItem()
		if item == nil || m.client == nil {
			return m, nil
		}
		m.state = epicLoading
		m.detailIssue = nil
		return m, tea.Batch(m.loadSpinner.Tick, fetchEpicIssueCmd(m.client, item.Key))
	case "o":
		if item := m.selectedItem(); item != nil {
			return m, openInBrowserCmd(m.issueURL(item.Key))
		}
	case "b":
		if item := m.selectedItem(); item != nil {
			m.result.filterBacklogKey = item.Key
		}
		return m, nil
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.filterInput.SetValue("")
			return m, m.applyFilter()
		}
		return m, nil
	case "/":
		m.state = epicFilter
		m.filterInput.SetValue(m.filter)
		return m, m.filterInput.Focus()
	case "l":
		return m, m.beginLabelEdit()
	case "L":
		return m, m.openLinkPicker()
	case "R":
		m.result.refresh = true
		return m, nil
	}

	return m, nil
}

func (m epicModel) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			m.filter = ""
			m.filterInput.SetValue("")
			m.filterInput.Blur()
			m.state = epicList
			return m, m.applyFilter()
		case "enter":
			m.filter = m.filterInput.Value()
			m.filterInput.Blur()
			m.state = epicList
			return m, m.applyFilter()
		case "ctrl+c":
			m.quitting = true
			m.result.quit = true
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filter = m.filterInput.Value()
	filterCmd := m.applyFilter()
	if cmd == nil {
		return m, filterCmd
	}
	if filterCmd == nil {
		return m, cmd
	}
	return m, tea.Batch(cmd, filterCmd)
}

func (m *epicModel) clampSidebarOffset() {
	totalLines := strings.Count(m.sidebarContent, "\n") + 1
	maxOffset := totalLines - m.viewHeight()
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.sidebarOffset = tui.Clamp(m.sidebarOffset, 0, maxOffset)
}

func fetchEpicLabelsCmd(client api.Client, key string) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.GetIssue(key)
		return epicLabelsFetchedMsg{key: key, issue: issue, err: err}
	}
}

// fetchEpicIssueCmd fetches an epic for the detail overlay. Unlike
// fetchIssueCmd it does not pre-render the body, because the epic detail also
// renders child work items that arrive from a separate request.
func fetchEpicIssueCmd(client api.Client, key string) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.GetIssue(key)
		return issueFetchedMsg{issue: issue, err: err}
	}
}

// fetchEpicChildrenCmd fetches the child work items of an epic via JQL search.
func fetchEpicChildrenCmd(client api.Client, key string) tea.Cmd {
	return func() tea.Msg {
		children, err := client.GetEpicChildren(key)
		if err != nil {
			debug.LogError("client.GetEpicChildren", err)
		}
		return epicChildrenFetchedMsg{key: key, children: epicChildLinks(children), err: err}
	}
}

// epicChildLinks projects fetched child issues into the shared link representation.
func epicChildLinks(children []models.Issue) []models.LinkedIssue {
	links := make([]models.LinkedIssue, 0, len(children))
	for _, child := range children {
		links = append(links, models.LinkedIssue{
			Relationship: "child",
			Key:          child.Key,
			Summary:      child.Summary,
			Status:       child.Status,
			IssueType:    child.IssueType,
			Priority:     child.Priority,
			SubTaskCount: child.SubTaskCount,
		})
	}
	return links
}

// openLinkPicker opens a picker of the selected epic's related work items.
// Related items may live outside the board, so the picker opens in the browser
// rather than moving the cursor.
func (m *epicModel) openLinkPicker() tea.Cmd {
	m.linkPickerReturn = m.state
	m.state = epicLinkPicker
	return m.linkPicker.open(m.linkPickerItems())
}

// linkPickerItems returns the related work items for the selected epic:
// explicit issue links, subtasks, and child work items, deduplicated by key.
func (m *epicModel) linkPickerItems() []models.LinkedIssue {
	var issue *models.Issue
	if m.sidebarFullIssue != nil && m.sidebarFullIssue.Key == m.selectedKey() {
		issue = m.sidebarFullIssue
	}
	return collectLinkedItems(linkedItemsForIssue(issue), m.children.items)
}

func setEpicLabelsCmd(client api.Client, key string, labels []string) tea.Cmd {
	return func() tea.Msg {
		err := client.SetLabels(key, labels)
		if err != nil {
			debug.LogError("client.SetLabels", err)
		}
		return epicLabelsSavedMsg{key: key, labels: labels, err: err}
	}
}
