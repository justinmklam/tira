package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/debug"
	"github.com/justinmklam/tira/internal/editor"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
	"github.com/spf13/cobra"
)

var (
	createProject string
	createType    string
	createParent  string
	createFile    string
	createNoEdit  bool
	showTemplate  bool
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Jira issue",
	Long: `Create a new Jira issue.

Interactive mode (default):
  Opens a template pre-filled with project defaults in $EDITOR. Edit the fields,
  save and close the editor to create the issue. Valid types and priorities are
  shown as comments inside the template.

Non-interactive mode (for AI agents / automation):
  Use --file or pipe content via stdin to create an issue without opening an
  editor. The input should be in the same template format as the interactive
  mode (YAML-like front matter + Markdown body, separated by ---).

  Examples:
    # Create from a file
    tira create --file issue-template.md

    # Pipe from stdin (e.g., from an AI agent)
    echo -e "type: Task\npriority: High\n---\n# My Summary\n\n## Description\n\nDo the thing" | tira create --no-edit

    # Generate with AI and create
    ai-generate-issue | tira create --no-edit

  In non-interactive mode, validation is still performed (valid types, priorities,
  required fields) but no editor is opened.

For AI Agents:
  Use 'tira create --template' to get the exact format specification including
  all supported front matter fields and their descriptions.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle --template flag early (before config loading)
		if showTemplate {
			fmt.Println(getTemplateDocumentation())
			return nil
		}

		client, err := api.NewClient(cfg)
		if err != nil {
			debug.LogError("api.NewClient", err)
			return err
		}

		projectKey := createProject
		if projectKey == "" {
			projectKey = cfg.Project
		}
		if projectKey == "" {
			return fmt.Errorf("project key required: use --project or set default_project in config")
		}

		valid, err := loadValidValues(client, projectKey)
		if err != nil {
			debug.LogError("loadValidValues", err)
			return err
		}

		// Validate --type early so the user gets a clear error before the editor opens.
		if createType != "" && len(valid.IssueTypes) > 0 {
			if !tui.ContainsCI(valid.IssueTypes, createType) {
				return fmt.Errorf("invalid type %q. Valid types: %s",
					createType, strings.Join(valid.IssueTypes, ", "))
			}
		}

		var content string

		// Non-interactive mode: read from file or stdin
		if createFile != "" || createNoEdit || !isTerminal(os.Stdin) {
			content, err = readInput(createFile)
			if err != nil {
				return err
			}

			// Parse and validate the template
			fields, err := editor.ParseTemplate(content)
			if err != nil {
				return fmt.Errorf("parsing template: %w", err)
			}

			// Validate required fields
			if fields.Summary == "" || fields.Summary == "Summary goes here" {
				return fmt.Errorf("summary is required")
			}

			// Validate issue type if provided
			if fields.IssueType != "" && len(valid.IssueTypes) > 0 {
				if !tui.ContainsCI(valid.IssueTypes, fields.IssueType) {
					return fmt.Errorf("invalid type %q. Valid types: %s",
						fields.IssueType, strings.Join(valid.IssueTypes, ", "))
				}
			} else if len(valid.IssueTypes) > 0 {
				fields.IssueType = valid.IssueTypes[0]
			}

			// Validate priority if provided
			if fields.Priority != "" && len(valid.Priorities) > 0 {
				if !tui.ContainsCI(valid.Priorities, fields.Priority) {
					return fmt.Errorf("invalid priority %q. Valid priorities: %s",
						fields.Priority, strings.Join(valid.Priorities, ", "))
				}
			} else if len(valid.Priorities) > 0 {
				fields.Priority = valid.Priorities[len(valid.Priorities)/2]
			}

			// Apply --parent flag override
			if createParent != "" {
				fields.ParentKey = createParent
			}

			// Resolve assignee if provided
			if fields.Assignee != "" && len(valid.Assignees) > 0 {
				for _, a := range valid.Assignees {
					if strings.EqualFold(a.DisplayName, fields.Assignee) {
						fields.AssigneeID = a.AccountID
						break
					}
				}
			}

			issue, err := client.CreateIssue(projectKey, *fields)
			if err != nil {
				debug.LogError("client.CreateIssue", err)
				return fmt.Errorf("creating issue: %w", err)
			}
			fmt.Fprintf(os.Stderr, "✓ Created %s.\n", issue.Key)
			return nil
		}

		// Interactive mode: open editor
		// Build a blank issue, pre-filling type and parent.
		blank := &models.Issue{
			IssueType: createType,
			ParentKey: createParent,
		}
		if blank.IssueType == "" && len(valid.IssueTypes) > 0 {
			blank.IssueType = valid.IssueTypes[0]
		}
		if len(valid.Priorities) > 0 {
			blank.Priority = valid.Priorities[len(valid.Priorities)/2]
		}

		content = editor.RenderTemplate(blank, valid)
		fields, err := openAndValidate(content, valid)
		if err != nil || fields == nil {
			return err
		}

		if fields.Summary == "" || fields.Summary == "Summary goes here" {
			return fmt.Errorf("summary is required")
		}

		fields.ParentKey = createParent

		issue, err := client.CreateIssue(projectKey, *fields)
		if err != nil {
			debug.LogError("client.CreateIssue", err)
			return fmt.Errorf("creating issue: %w", err)
		}
		fmt.Fprintf(os.Stderr, "✓ Created %s.\n", issue.Key)
		return nil
	},
}

func init() {
	createCmd.Flags().StringVar(&createProject, "project", "", "Project key (overrides default_project from config)")
	createCmd.Flags().StringVar(&createType, "type", "", "Pre-fill issue type (valid types are listed in the editor template)")
	createCmd.Flags().StringVar(&createParent, "parent", "", "Parent issue key (e.g. MP-42)")
	createCmd.Flags().StringVarP(&createFile, "file", "f", "", "Read issue template from file (non-interactive mode)")
	createCmd.Flags().BoolVar(&createNoEdit, "no-edit", false, "Read issue template from stdin (non-interactive mode)")
	createCmd.Flags().BoolVar(&showTemplate, "template", false, "Show template format documentation (for AI agents)")
	rootCmd.AddCommand(createCmd)
}

// getTemplateDocumentation returns the template format specification for AI agents
func getTemplateDocumentation() string {
	return `# Tira Issue Template Format

## Structure

The template consists of two parts separated by "---":
1. **Front Matter** (YAML-like key: value pairs)
2. **Markdown Body** (after the "---" separator)

## Front Matter Fields

` + "```" + `
<!-- tira: do not remove this line or change field names -->
type: <IssueType>          # Required. Valid types depend on project
priority: <Priority>       # Optional. Defaults to middle priority if omitted
assignee: <DisplayName>    # Optional. Case-insensitive name match
story_points: <Number>     # Optional. Positive integer or leave blank
labels: <comma-separated>  # Optional. e.g., "backend, api, bug"
parent: <IssueKey>         # Optional. e.g., "MP-42" (read-only, shown in comment)

---
` + "```" + `

## Markdown Body Structure

` + "```markdown" + `
# <Summary>              # Required. H1 heading is the issue summary

## Description           # Optional but recommended

Your description here. Supports full Markdown:
- Headings (##, ###, etc.)
- Lists (- item or 1. item)
- Code blocks (` + "```language ... ```" + `)
- Tables, blockquotes, links, etc.

## Acceptance Criteria   # Optional

- [ ] Criterion 1
- [ ] Criterion 2
` + "```" + `

## Minimal Valid Example

` + "```" + `
type: Task

---

# Fix login bug
` + "```" + `

## Full Example

` + "```" + `
<!-- tira: do not remove this line or change field names -->
type: Story
priority: High
assignee: Jane Smith
story_points: 5
labels: backend, api

---

# Implement OAuth2 Login

## Description

Add OAuth2 login with Google provider.

## Acceptance Criteria

- [ ] User can sign in with Google
- [ ] Session persists correctly
` + "```" + `

## Validation Rules

1. **Sentinel line** must be present: ` + "`<!-- tira: do not remove this line or change field names -->`" + `
2. **Separator** "---" must appear on its own line
3. **Summary** (H1 heading) is required and cannot be "Summary goes here"
4. **Type** must be valid for the project (will default to first type if omitted)
5. **Priority** must be valid (will default to middle priority if omitted)
6. **Assignee** is resolved by display name (case-insensitive)

## Notes for AI Agents

- Fetch valid types/priorities first with: ` + "`tira create --template --project <KEY>`" + ` (requires config)
- Or let the API apply defaults by omitting type/priority
- All Markdown formatting is preserved and converted to ADF for Jira
- Empty front matter values can be omitted entirely or left blank
`
}

// isTerminal checks if the given file is a terminal
func isTerminal(f *os.File) bool {
	fileInfo, err := f.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// readInput reads content from a file or stdin
func readInput(filename string) (string, error) {
	var reader io.Reader

	if filename != "" {
		f, err := os.Open(filename)
		if err != nil {
			return "", fmt.Errorf("opening file %q: %w", filename, err)
		}
		defer func() { _ = f.Close() }()
		reader = f
	} else {
		reader = os.Stdin
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}

	return sb.String(), nil
}
