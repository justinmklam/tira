package main

import (
	"fmt"
	"strings"

	"github.com/justinmklam/tira/internal/debug"
	"github.com/spf13/cobra"
)

var commentCmd = &cobra.Command{
	Use:   "comment <key|url> <text>",
	Short: "Add a comment to a Jira issue",
	Long: `Add a comment to an existing Jira issue.

Accepts an issue key or full Jira browse URL. Quote comment text containing
spaces so it is passed as one argument:

  tira comment PROJ-123 "The fix is ready for review."`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := extractIssueKey(args[0])
		text := args[1]
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("comment text cannot be empty")
		}

		client, err := newAPIClient()
		if err != nil {
			debug.LogError("newAPIClient", err)
			return fmt.Errorf("creating API client: %w", err)
		}

		if err := client.AddComment(key, text); err != nil {
			debug.LogError("AddComment", err)
			return fmt.Errorf("adding comment to %s: %w", key, err)
		}

		cmd.PrintErrf("✓ Comment added to %s.\n", key)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(commentCmd)
}
