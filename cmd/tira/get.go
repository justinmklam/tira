package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/term"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
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
	Use:   "get <key|url>",
	Short: "Fetch and display a Jira issue",
	Long: `Fetch and display a Jira issue as Markdown.

Accepts a bare issue key or a full Jira browse URL — the issue key is
extracted automatically either way.

When stdout is a terminal, the output is paged via glow (if installed) or less.
When stdout is piped, raw Markdown is written directly — useful for agents:

  tira get PROJ-123
  tira get https://your-domain.atlassian.net/browse/PROJ-123
  tira get PROJ-123 | cat          # pipe-safe: writes raw Markdown
  tira get PROJ-123 | grep Status  # extract specific fields

Use --edit to open the issue in $EDITOR and write changes back to Jira
(interactive only — for non-interactive/agent updates, use 'tira update' instead).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := extractIssueKey(args[0])

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

		valid, err := loadValidValues(client, projectKeyFromIssueKey(issue.Key))
		if err != nil {
			debug.LogError("loadValidValues", err)
			return err
		}

		return runEditLoop(client, issue, valid)
	},
}

func init() {
	getCmd.Flags().BoolVar(&editFlag, "edit", false, "Open issue in $EDITOR and write changes back to Jira (interactive; for agents use 'tira update')")
	rootCmd.AddCommand(getCmd)
}

// projectKeyFromIssueKey derives the project key from an issue key
// (e.g. "MP-101" → "MP"), falling back to the configured default project.
func projectKeyFromIssueKey(issueKey string) string {
	if idx := strings.Index(issueKey, "-"); idx > 0 {
		return issueKey[:idx]
	}
	return cfg.Project
}

// issueKeyRe matches a Jira issue key such as "PROJ-123" or "TEST-456".
var issueKeyRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*-\d+`)

// extractIssueKey returns the Jira issue key from arg, which may be a bare
// key (e.g. "PROJ-123") or a full browse URL
// (e.g. "https://example.atlassian.net/browse/PROJ-123"). If no key
// pattern is found, arg is returned unchanged so the API call surfaces a
// clear error.
func extractIssueKey(arg string) string {
	arg = strings.TrimSpace(arg)
	if match := issueKeyRe.FindString(arg); match != "" {
		return strings.ToUpper(match)
	}
	return arg
}

// runEditLoop implements the interactive get --edit flow: opens the issue
// template in $EDITOR, validates, and writes changes back to Jira.
func runEditLoop(client api.Client, issue *models.Issue, valid *models.ValidValues) error {
	content := editor.RenderTemplate(issue, valid)
	fields, err := openAndValidate(content, valid)
	if err != nil || fields == nil {
		return err
	}

	return applyUpdate(client, issue, fields)
}

// applyTemplateUpdate parses raw template content (from stdin, --file, or a
// captured/edited template), validates it, and writes changes back to Jira.
// Used by the non-interactive update flow (`tira update --no-edit`).
func applyTemplateUpdate(client api.Client, issue *models.Issue, valid *models.ValidValues, content string) error {
	fields, err := editor.ParseTemplate(content)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	errs := validator.Validate(fields, valid)
	if len(errs) > 0 {
		printValidationErrors(errs)
		return fmt.Errorf("template failed validation (%d error(s)); run 'tira update %s --template' to get a fresh, current template to edit", len(errs), issue.Key)
	}
	fields.AssigneeID = validator.ResolveAssigneeID(fields, valid)

	return applyUpdate(client, issue, fields)
}

// applyUpdate diffs fields against issue, calls UpdateIssue, and reports the
// result. It is a no-op (with a message) if nothing changed.
func applyUpdate(client api.Client, issue *models.Issue, fields *models.IssueFields) error {
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
	style := lipgloss.NewStyle().Foreground(tui.ColorError)
	fmt.Fprintln(os.Stderr, style.Render("Validation errors:"))
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "  • %s\n", e.Message)
	}
}

// printFieldDiff shows which fields changed.
func printFieldDiff(issue *models.Issue, fields *models.IssueFields) {
	label := lipgloss.NewStyle().Bold(true)
	old := lipgloss.NewStyle().Foreground(tui.ColorError)
	new_ := lipgloss.NewStyle().Foreground(tui.ColorSuccess)

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
