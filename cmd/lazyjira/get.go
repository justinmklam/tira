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
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/display"
	"github.com/justinmklam/lazyjira/internal/editor"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/justinmklam/lazyjira/internal/validator"
	"github.com/spf13/cobra"
)

var editFlag bool

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

		if !editFlag {
			output, err := display.RenderIssue(issue)
			if err != nil {
				return err
			}
			return page(output)
		}

		return runEditLoop(client, issue)
	},
}

func init() {
	getCmd.Flags().BoolVar(&editFlag, "edit", false, "Open issue in $EDITOR and write changes back to Jira")
	rootCmd.AddCommand(getCmd)
}

// runEditLoop implements the full get --edit flow.
func runEditLoop(client api.Client, issue *models.Issue) error {
	// Derive project key from issue key (e.g. "MP-101" → "MP").
	projectKey := cfg.Project
	if idx := strings.Index(issue.Key, "-"); idx > 0 {
		projectKey = issue.Key[:idx]
	}

	// Fetch valid values (non-fatal if it fails — we still allow editing).
	valid, err := fetchValidValues(client, projectKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch valid values: %v\n", err)
		valid = &models.ValidValues{}
	}

	// Write template to a temp file.
	content := editor.RenderTemplate(issue, valid)
	tmpFile, err := editor.WriteTempFile(content)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	original, err := os.ReadFile(tmpFile)
	if err != nil {
		return err
	}

	for {
		if err := editor.OpenEditor(tmpFile); err != nil {
			return fmt.Errorf("editor: %w", err)
		}

		current, err := os.ReadFile(tmpFile)
		if err != nil {
			return err
		}
		if string(current) == string(original) {
			fmt.Fprintln(os.Stderr, "No changes. Aborting.")
			return nil
		}

		fields, err := editor.ParseTemplate(string(current))
		if err != nil {
			return fmt.Errorf("could not parse file: %w", err)
		}

		errs := validator.Validate(fields, valid)
		if len(errs) == 0 {
			// Resolve assignee display name → account ID.
			fields.AssigneeID = validator.ResolveAssigneeID(fields, valid)
			break
		}

		// Annotate file with errors and ask to re-open.
		annotated := validator.AnnotateTemplate(string(current), errs)
		if err := os.WriteFile(tmpFile, []byte(annotated), 0600); err != nil {
			return err
		}
		printValidationErrors(errs)

		retry := true
		if err := huh.NewConfirm().
			Title("Validation failed. Re-open editor?").
			Value(&retry).
			Run(); err != nil {
			return err
		}
		if !retry {
			return nil
		}
		// Update original so another no-change check works correctly.
		original = []byte(annotated)
	}

	// Re-read current content for the diff.
	current, _ := os.ReadFile(tmpFile)
	fields, _ := editor.ParseTemplate(string(current))
	fields.AssigneeID = validator.ResolveAssigneeID(fields, valid)

	printFieldDiff(issue, fields)

	if err := client.UpdateIssue(issue.Key, *fields); err != nil {
		return fmt.Errorf("updating issue: %w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ %s updated.\n", issue.Key)
	return nil
}

// fetchValidValues wraps client.GetValidValues with a spinner.
func fetchValidValues(client api.Client, projectKey string) (*models.ValidValues, error) {
	ch := make(chan validResult, 1)
	go func() {
		v, err := client.GetValidValues(projectKey)
		ch <- validResult{v, err}
	}()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	prog := tea.NewProgram(validSpinnerModel{
		spinner: s,
		label:   "Fetching valid values…",
		result:  ch,
	}, tea.WithOutput(os.Stderr))

	fm, err := prog.Run()
	if err != nil {
		return nil, err
	}
	vsm := fm.(validSpinnerModel)
	return vsm.v, vsm.err
}

type validResult struct {
	v   *models.ValidValues
	err error
}

type validSpinnerModel struct {
	spinner spinner.Model
	label   string
	result  chan validResult
	done    bool
	v       *models.ValidValues
	err     error
}

func (m validSpinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg { return <-m.result })
}
func (m validSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case validResult:
		m.done = true
		m.v = msg.v
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}
func (m validSpinnerModel) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " " + m.label
}

// printValidationErrors renders a styled error summary to stderr.
func printValidationErrors(errs []validator.ValidationError) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	fmt.Fprintln(os.Stderr, style.Render("Validation errors:"))
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "  • %s\n", e.Message)
	}
}

// printFieldDiff shows which fields changed.
func printFieldDiff(issue *models.Issue, fields *models.IssueFields) {
	label := lipgloss.NewStyle().Bold(true)
	old := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	new_ := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	type change struct{ field, from, to string }
	var changes []change

	if fields.Summary != "" && fields.Summary != issue.Summary {
		changes = append(changes, change{"summary", issue.Summary, fields.Summary})
	}
	if fields.IssueType != "" && !strings.EqualFold(fields.IssueType, issue.IssueType) {
		changes = append(changes, change{"type", issue.IssueType, fields.IssueType})
	}
	if fields.Priority != "" && !strings.EqualFold(fields.Priority, issue.Priority) {
		changes = append(changes, change{"priority", issue.Priority, fields.Priority})
	}
	if fields.Assignee != "" && !strings.EqualFold(fields.Assignee, issue.Assignee) {
		changes = append(changes, change{"assignee", issue.Assignee, fields.Assignee})
	}
	if fields.StoryPoints != issue.StoryPoints {
		changes = append(changes, change{"story_points",
			fmt.Sprintf("%.0f", issue.StoryPoints),
			fmt.Sprintf("%.0f", fields.StoryPoints),
		})
	}
	if len(changes) == 0 && fields.Description == issue.Description {
		fmt.Fprintln(os.Stderr, "No field changes detected.")
		return
	}

	fmt.Fprintln(os.Stderr, label.Render("Changes:"))
	for _, c := range changes {
		fmt.Fprintf(os.Stderr, "  %s: %s → %s\n",
			label.Render(c.field),
			old.Render(c.from),
			new_.Render(c.to),
		)
	}
	if fields.Description != issue.Description {
		fmt.Fprintln(os.Stderr, "  "+label.Render("description")+" (modified)")
	}
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
