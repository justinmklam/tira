package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/editor"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/justinmklam/lazyjira/internal/tui"
	"github.com/spf13/cobra"
)

var (
	createProject string
	createType    string
	createParent  string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Jira issue",
	Long: `Create a new Jira issue using $EDITOR.

Opens a template pre-filled with project defaults. Edit the fields, save and
close the editor to create the issue. Valid types and priorities are shown as
comments inside the template.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient(cfg)
		if err != nil {
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
			return err
		}

		// Validate --type early so the user gets a clear error before the editor opens.
		if createType != "" && len(valid.IssueTypes) > 0 {
			if !tui.ContainsCI(valid.IssueTypes, createType) {
				return fmt.Errorf("invalid type %q. Valid types: %s",
					createType, strings.Join(valid.IssueTypes, ", "))
			}
		}

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

		content := editor.RenderTemplate(blank, valid)
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
	rootCmd.AddCommand(createCmd)
}
