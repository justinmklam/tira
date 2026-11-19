package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/models"
	"github.com/spf13/cobra"
)

var (
	exportOutput  string
	exportProject string
)

var exportCSVHeader = []string{
	"Issue Key", "Issue URL", "Summary", "Epic Key", "Epic URL", "Type", "Story Points",
	"Assignee", "Priority", "Status", "Reporter", "Labels", "Parent Key", "Parent URL",
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export project issues to CSV",
	Long: `Export every issue in a Jira project to a CSV file.

By default, the project configured for the active profile is exported. Use
--project to override it. The output file is overwritten if it already exists.
Issue data comes from Jira's paginated bulk search API; fields unavailable in
that response are left blank, and individual issues are not fetched separately.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		project, jiraURL := "", ""
		if cfg != nil {
			project, jiraURL = cfg.Project, cfg.JiraURL
		}
		return runProjectExport(exportOutput, exportProject, project, jiraURL, newAPIClient)
	},
}

func init() {
	exportCmd.Flags().StringVar(&exportOutput, "output", "", "CSV output path (required; overwrites an existing file)")
	exportCmd.Flags().StringVar(&exportProject, "project", "", "project key (overrides the configured project)")
	if err := exportCmd.MarkFlagRequired("output"); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(exportCmd)
}

func runProjectExport(outputPath, projectOverride, configuredProject, jiraURL string, clientFactory func() (api.Client, error)) error {
	if strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("output path is required; use --output <path>")
	}
	projectKey := strings.TrimSpace(projectOverride)
	if projectKey == "" {
		projectKey = strings.TrimSpace(configuredProject)
	}
	if projectKey == "" {
		return fmt.Errorf("project key is not configured; set project in your profile or use --project <key>")
	}

	client, err := clientFactory()
	if err != nil {
		return fmt.Errorf("creating API client: %w", err)
	}
	issues, err := client.GetProjectIssues(projectKey)
	if err != nil {
		return fmt.Errorf("fetching project %s issues: %w", projectKey, err)
	}
	return writeProjectIssuesCSV(outputPath, strings.TrimRight(strings.TrimSpace(jiraURL), "/"), issues)
}

func writeProjectIssuesCSV(outputPath, jiraURL string, issues []models.Issue) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating CSV file %q: %w", outputPath, err)
	}

	writer := csv.NewWriter(file)
	_ = writer.Write(exportCSVHeader)
	for _, issue := range issues {
		if err := writer.Write(exportIssueCSVRow(jiraURL, issue)); err != nil {
			break
		}
	}
	writer.Flush()
	writeErr := writer.Error()
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("writing CSV file %q: %w", outputPath, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing CSV file %q: %w", outputPath, closeErr)
	}
	return nil
}

func exportIssueCSVRow(jiraURL string, issue models.Issue) []string {
	issueURL := jiraURL + "/browse/" + issue.Key
	epicURL := ""
	if issue.EpicKey != "" {
		epicURL = jiraURL + "/browse/" + issue.EpicKey
	}
	parentURL := ""
	if issue.ParentKey != "" {
		parentURL = jiraURL + "/browse/" + issue.ParentKey
	}
	storyPoints := ""
	if issue.StoryPoints != 0 {
		storyPoints = strconv.FormatFloat(issue.StoryPoints, 'f', -1, 64)
	}
	return []string{
		issue.Key, issueURL, issue.Summary, issue.EpicKey, epicURL, issue.IssueType, storyPoints,
		issue.Assignee, issue.Priority, issue.Status, issue.Reporter, strings.Join(issue.Labels, "; "),
		issue.ParentKey, parentURL,
	}
}
