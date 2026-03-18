package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/display"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Fetch and display a Jira issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		client, err := api.NewClient(cfg)
		if err != nil {
			return err
		}

		issue, err := fetchWithSpinner(fmt.Sprintf("Fetching %s…", key), func() (*models.Issue, error) {
			return client.GetIssue(key)
		})
		if err != nil {
			return err
		}

		output, err := display.RenderIssue(issue)
		if err != nil {
			return err
		}

		return page(output)
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}

// --- spinner ---

type fetchResult struct {
	issue *models.Issue
	err   error
}

type spinnerModel struct {
	spinner spinner.Model
	label   string
	result  chan fetchResult
	done    bool
	issue   *models.Issue
	err     error
}

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitForResult(m.result))
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fetchResult:
		m.done = true
		m.issue = msg.issue
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m spinnerModel) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " " + m.label
}

func waitForResult(ch chan fetchResult) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func fetchWithSpinner(label string, fn func() (*models.Issue, error)) (*models.Issue, error) {
	ch := make(chan fetchResult, 1)
	go func() {
		issue, err := fn()
		ch <- fetchResult{issue, err}
	}()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	p := tea.NewProgram(spinnerModel{
		spinner: s,
		label:   label,
		result:  ch,
	}, tea.WithOutput(os.Stderr))

	m, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("spinner: %w", err)
	}

	sm := m.(spinnerModel)
	return sm.issue, sm.err
}

// --- pager ---

func page(content string) error {
	// If stdout is not a TTY (piped to cat, glow, etc.) write raw markdown.
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		_, err := io.WriteString(os.Stdout, content)
		return err
	}

	// Render via glow, falling back to less -R if glow is not installed.
	for _, pager := range []string{"glow --pager --style=dracula --width=120 -", "less -R"} {
		parts := strings.Fields(pager)
		c := exec.Command(parts[0], parts[1:]...)
		c.Stdin = strings.NewReader(content)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err == nil {
			return nil
		}
	}

	_, err := io.WriteString(os.Stdout, content)
	return err
}
