package app

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

type epicState int

const (
	epicList epicState = iota
	epicLoading
	epicDetail
)

// epicItem is the board-derived projection of an epic and its represented
// children. Its position is established by the first child encountered.
type epicItem struct {
	Key              string
	Name             string
	Summary          string
	ChildCount       int
	StoryPoints      float64
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

// epicModel is intentionally independent of boardModel so it can be wired in
// as a peer view without changing the board data or API contracts.
type epicModel struct {
	state   epicState
	client  api.Client
	jiraURL string

	items  []epicItem
	cursor int
	offset int
	width  int
	height int

	loadSpinner spinner.Model
	loading     bool
	loadError   string
	quitting    bool
	result      epicResult

	sidebarContent   string
	sidebarOffset    int
	sidebarIssueKey  string
	sidebarFullIssue *models.Issue

	detailIssue *models.Issue
	detailView  viewport.Model
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
			if item.Name == item.Key && issue.EpicName != "" {
				item.Name = issue.EpicName
				item.Summary = issue.EpicName
			}
		}
	}

	return items
}

func newEpicModel(client api.Client, groups []models.SprintGroup, jiraURL string, loading bool) (epicModel, tea.Cmd) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.ColorSpinner)

	m := epicModel{
		state:       epicList,
		client:      client,
		jiraURL:     strings.TrimRight(jiraURL, "/"),
		items:       buildEpicItems(groups),
		loadSpinner: s,
		loading:     loading,
	}
	m.updateSidebar()
	return m, m.sidebarCommand()
}

// refreshData replaces the projection while preserving the selected epic by
// key where possible. It returns a command only when a new sidebar fetch is
// needed.
func (m *epicModel) refreshData(groups []models.SprintGroup, loading bool, loadErr error) tea.Cmd {
	selectedKey := m.selectedKey()
	m.items = buildEpicItems(groups)
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
		m.updateSidebar()
		return m.sidebarCommand()
	}
	m.updateSidebar()
	return nil
}

func (m epicModel) selectedKey() string {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return ""
	}
	return m.items[m.cursor].Key
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
	m.sidebarContent = renderEpicSidebarContent(issue, m.selectedItem(), width)
	m.sidebarOffset = 0
}

func (m epicModel) sidebarCommand() tea.Cmd {
	key := m.selectedKey()
	if key == "" || key == m.sidebarIssueKey || m.client == nil {
		return nil
	}
	return fetchSidebarIssueCmd(m.client, key)
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
	m.updateSidebar()
	return m.sidebarCommand()
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
		m.updateSidebar()
		if m.sidebarIssueKey == "" {
			return m, m.sidebarCommand()
		}
		return m, nil

	case issueFetchedMsg:
		if msg.err != nil {
			m.state = epicList
			m.loadError = msg.err.Error()
			return m, nil
		}
		m.detailIssue = msg.issue
		vpW, vpH := tui.OverlayViewportSize(m.width, m.height)
		vp := viewport.New(viewport.WithWidth(vpW), viewport.WithHeight(vpH))
		vp.SetContent(msg.content)
		m.detailView = vp
		m.state = epicDetail
		return m, nil

	case sidebarIssueFetchedMsg:
		if msg.err == nil && msg.issue != nil && msg.issue.Key == m.selectedKey() {
			m.sidebarIssueKey = msg.issue.Key
			m.sidebarFullIssue = msg.issue
			m.updateSidebar()
		}
		return m, nil

	case spinner.TickMsg:
		if m.state == epicLoading {
			var cmd tea.Cmd
			m.loadSpinner, cmd = m.loadSpinner.Update(msg)
			return m, cmd
		}
		return m, nil
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
		vpW, _ := tui.OverlayViewportSize(m.width, m.height)
		return m, tea.Batch(m.loadSpinner.Tick, fetchIssueCmd(m.client, item.Key, vpW))
	case "o":
		if item := m.selectedItem(); item != nil {
			return m, openInBrowserCmd(m.issueURL(item.Key))
		}
	case "b":
		if item := m.selectedItem(); item != nil {
			m.result.filterBacklogKey = item.Key
		}
		return m, nil
	case "R":
		m.result.refresh = true
		return m, nil
	}

	return m, nil
}

func (m *epicModel) clampSidebarOffset() {
	totalLines := strings.Count(m.sidebarContent, "\n") + 1
	maxOffset := totalLines - m.viewHeight()
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.sidebarOffset = tui.Clamp(m.sidebarOffset, 0, maxOffset)
}
