package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/debug"
	"github.com/justinmklam/tira/internal/display"
	"github.com/justinmklam/tira/internal/editor"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
	"github.com/justinmklam/tira/internal/validator"
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
			debug.LogError("api.NewClient", err)
			return err
		}

		issue, err := tui.RunWithSpinner(fmt.Sprintf("Fetching %s…", key), func() (*models.Issue, error) {
			return client.GetIssue(key)
		})
		if err != nil {
			debug.LogError("client.GetIssue", err)
			return err
		}

		if !editFlag {
			output := display.RenderIssue(issue)
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

	valid, err := loadValidValues(client, projectKey)
	if err != nil {
		debug.LogError("loadValidValues", err)
		return err
	}

	content := editor.RenderTemplate(issue, valid)
	fields, err := openAndValidate(content, valid)
	if err != nil || fields == nil {
		return err
	}

	printFieldDiff(issue, fields)

	if err := client.UpdateIssue(issue.Key, *fields); err != nil {
		debug.LogError("UpdateIssue", err)
		return fmt.Errorf("updating issue: %w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ %s updated.\n", issue.Key)
	return nil
}

// loadValidValues fetches valid field values with a spinner, falling back to
// an empty ValidValues on error so the edit flow can still proceed.
func loadValidValues(client api.Client, projectKey string) (*models.ValidValues, error) {
	valid, err := tui.RunWithSpinner("Fetching valid values…", func() (*models.ValidValues, error) {
		return client.GetValidValues(projectKey)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch valid values: %v\n", err)
		return &models.ValidValues{}, nil
	}
	return valid, nil
}

// openAndValidate writes content to a temp file, opens $EDITOR, and loops
// until the file is valid or the user aborts. Returns nil fields (no error)
// if the user made no changes or chose to abort after validation failure.
func openAndValidate(content string, valid *models.ValidValues) (*models.IssueFields, error) {
	tmpFile, err := editor.WriteTempFile(content)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tmpFile) }()

	original, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, err
	}

	for {
		if err := editor.OpenEditor(tmpFile); err != nil {
			return nil, fmt.Errorf("editor: %w", err)
		}

		current, err := os.ReadFile(tmpFile)
		if err != nil {
			return nil, err
		}
		if string(current) == string(original) {
			fmt.Fprintln(os.Stderr, "No changes. Aborting.")
			return nil, nil
		}

		fields, err := editor.ParseTemplate(string(current))
		if err != nil {
			return nil, fmt.Errorf("could not parse file: %w", err)
		}

		errs := validator.Validate(fields, valid)
		if len(errs) == 0 {
			fields.AssigneeID = validator.ResolveAssigneeID(fields, valid)
			return fields, nil
		}

		annotated := validator.AnnotateTemplate(string(current), errs)
		if err := os.WriteFile(tmpFile, []byte(annotated), 0600); err != nil {
			return nil, err
		}
		printValidationErrors(errs)

		retry := true
		if err := huh.NewConfirm().
			Title("Validation failed. Re-open editor?").
			Value(&retry).
			Run(); err != nil {
			return nil, err
		}
		if !retry {
			return nil, nil
		}
		original = []byte(annotated)
	}
}

// printValidationErrors renders a styled error summary to stderr.
func printValidationErrors(errs []validator.ValidationError) {
	style := lipgloss.NewStyle().Foreground(tui.ColorRed)
	fmt.Fprintln(os.Stderr, style.Render("Validation errors:"))
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "  • %s\n", e.Message)
	}
}

// printFieldDiff shows which fields changed.
func printFieldDiff(issue *models.Issue, fields *models.IssueFields) {
	label := lipgloss.NewStyle().Bold(true)
	old := lipgloss.NewStyle().Foreground(tui.ColorRed)
	new_ := lipgloss.NewStyle().Foreground(tui.ColorGreen)

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
