package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/debug"
	"github.com/justinmklam/tira/internal/editor"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
	"github.com/justinmklam/tira/internal/validator"
)

// editorEditExecCmd implements tea.ExecCommand to run the $EDITOR-based edit
// flow while the bubbletea program is suspended.
type editorEditExecCmd struct {
	client  api.Client
	key     string
	project string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func newEditorEditExecCmd(client api.Client, key, project string) *editorEditExecCmd {
	return &editorEditExecCmd{
		client:  client,
		key:     key,
		project: project,
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
}

func (c *editorEditExecCmd) SetStdin(r io.Reader)  { c.stdin = r }
func (c *editorEditExecCmd) SetStdout(w io.Writer) { c.stdout = w }
func (c *editorEditExecCmd) SetStderr(w io.Writer) { c.stderr = w }

func (c *editorEditExecCmd) Run() error {
	traceLog("editorExec Run: called key=%s", c.key)
	_, _ = fmt.Fprintf(c.stderr, "Fetching %s...\n", c.key)
	issue, err := c.client.GetIssue(c.key)
	if err != nil {
		debug.LogError("editorEdit: GetIssue", err)
		_, _ = fmt.Fprintf(c.stderr, "error: could not fetch %s: %v\n", c.key, err)
		return fmt.Errorf("fetching issue: %w", err)
	}

	projectKey := c.project
	if projectKey == "" {
		if idx := strings.Index(c.key, "-"); idx > 0 {
			projectKey = c.key[:idx]
		}
	}
	valid, err := c.client.GetValidValues(projectKey)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "warning: could not fetch valid values: %v\n", err)
		valid = &models.ValidValues{}
	}

	content := editor.RenderTemplate(issue, valid)
	fields, err := c.openAndValidate(content, valid)
	if err != nil || fields == nil {
		return err
	}

	if err := c.client.UpdateIssue(c.key, *fields); err != nil {
		debug.LogError("editorEdit: UpdateIssue", err)
		_, _ = fmt.Fprintf(c.stderr, "error: could not update %s: %v\n", c.key, err)
		return fmt.Errorf("updating issue: %w", err)
	}
	_, _ = fmt.Fprintf(c.stderr, "✓ %s updated.\n", c.key)
	return nil
}

// openAndValidate writes content to a temp file, opens $EDITOR, and loops
// until the file is valid or the user aborts.
func (c *editorEditExecCmd) openAndValidate(content string, valid *models.ValidValues) (*models.IssueFields, error) {
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
		if err := c.openEditor(tmpFile); err != nil {
			return nil, fmt.Errorf("editor: %w", err)
		}

		current, err := os.ReadFile(tmpFile)
		if err != nil {
			return nil, err
		}
		if string(current) == string(original) {
			_, _ = fmt.Fprintln(c.stderr, "No changes. Aborting.")
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
		c.printValidationErrors(errs)

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

// openEditor opens path in the user's preferred editor ($EDITOR → $VISUAL → vi)
// using the I/O streams wired up by tea.Exec.
func (c *editorEditExecCmd) openEditor(path string) error {
	editorProg := os.Getenv("EDITOR")
	if editorProg == "" {
		editorProg = os.Getenv("VISUAL")
	}
	if editorProg == "" {
		editorProg = "vi"
	}
	parts := strings.Fields(editorProg)
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin = c.stdin
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	return cmd.Run()
}

func (c *editorEditExecCmd) printValidationErrors(errs []validator.ValidationError) {
	style := lipgloss.NewStyle().Foreground(tui.ColorError)
	_, _ = fmt.Fprintln(c.stderr, style.Render("Validation errors:"))
	for _, e := range errs {
		_, _ = fmt.Fprintf(c.stderr, "  • %s\n", e.Message)
	}
}
