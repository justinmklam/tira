package main

import (
	"fmt"
	"os"

	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/debug"
	"github.com/justinmklam/tira/internal/editor"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
	"github.com/spf13/cobra"
)

var (
	updateFile   string
	updateNoEdit bool
	updateShow   bool
)

var updateCmd = &cobra.Command{
	Use:   "update <key|url>",
	Short: "Update an existing Jira issue",
	Long: `Update an existing Jira issue.

Accepts a bare issue key or a full Jira browse URL.

Non-interactive mode (recommended for AI agents / automation):

  Step 1 — fetch the current issue as an editable template:

    tira update HIVE-3774 --show > /tmp/hive-3774.md

  Step 2 — edit /tmp/hive-3774.md (change only the fields/sections you want
  to update; leave the rest as-is), then pipe it back:

    cat /tmp/hive-3774.md | tira update HIVE-3774 --no-edit

  Or in one step, without a temp file, by piping a full template directly:

    cat <<'EOF' | tira update HIVE-3774 --no-edit
    <!-- tira: do not remove this line or change field names -->
    type: Task

    ---

    # HIVE-3774: Updated summary

    ## Description

    Updated description text. Full Markdown supported, including links to
    code, e.g. [pkg/foo.go](https://github.com/org/repo/blob/main/pkg/foo.go).

    ## Acceptance Criteria

    - Criterion 1
    - Criterion 2
    EOF

  Only non-empty fields are applied — you do not need to reproduce every
  field, but the template MUST include the sentinel line and the "# Summary"
  heading. Omitted/blank fields (type, priority, assignee, story_points,
  labels) are left unchanged on the ticket. Description and Acceptance
  Criteria are only replaced if their sections are present with content.

  Piping via stdin (without --no-edit) also works automatically when stdout
  is not a terminal — --no-edit is accepted for clarity/explicitness.

Interactive mode:
  Run with no --file/--no-edit and no piped stdin to open the current
  template in $EDITOR, make changes, save and close to write them back.

  tira update HIVE-3774

For AI Agents:
  Always call 'tira update <KEY> --show' first to capture the current
  field values before editing — this avoids clobbering fields you didn't
  intend to change and guarantees the sentinel line and summary heading are
  present. See 'tira create --template' for the full template format spec
  (front matter fields, body sections, validation rules) — it applies here
  too.`,
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
			return fmt.Errorf("fetching %s: %w", key, err)
		}

		valid, err := loadValidValues(client, projectKeyFromIssueKey(issue.Key))
		if err != nil {
			debug.LogError("loadValidValues", err)
			return err
		}

		if updateShow {
			fmt.Print(editor.RenderTemplate(issue, valid))
			return nil
		}

		if updateFile != "" || updateNoEdit || !isTerminal(os.Stdin) {
			content, err := readInput(updateFile)
			if err != nil {
				return err
			}
			return applyTemplateUpdate(client, issue, valid, content)
		}

		return runEditLoop(client, issue, valid)
	},
}

func init() {
	updateCmd.Flags().StringVarP(&updateFile, "file", "f", "", "Read updated template from file (non-interactive mode)")
	updateCmd.Flags().BoolVar(&updateNoEdit, "no-edit", false, "Read updated template from stdin (non-interactive mode)")
	updateCmd.Flags().BoolVar(&updateShow, "show", false, "Print the current issue as an editable template and exit")
	// --template is a deprecated alias for --show, bound to the same variable.
	updateCmd.Flags().BoolVar(&updateShow, "template", false, "Print the current issue as an editable template and exit")
	if err := updateCmd.Flags().MarkDeprecated("template", "use --show instead"); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(updateCmd)
}
