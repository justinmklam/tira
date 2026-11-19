package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/justinmklam/tira/internal/debug"
	"github.com/spf13/cobra"
)

var commentFile string

var commentCmd = &cobra.Command{
	Use:   "comment <key|url> [text]",
	Short: "Add a comment to a Jira issue",
	Long: `Add a comment to an existing Jira issue.

Accepts an issue key or full Jira browse URL. Pass short text as an argument, or use
--file for multiline text. Use --file - to read the comment body from stdin:

  tira comment PROJ-123 "The fix is ready for review."
  tira comment PROJ-123 --file comment.md
  cat comment.md | tira comment PROJ-123 --file -`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return fmt.Errorf("accepts 1 key and optional text argument")
		}
		if commentFile != "" && len(args) == 2 {
			return fmt.Errorf("text argument cannot be used with --file")
		}
		if commentFile == "" && len(args) != 2 {
			return fmt.Errorf("text argument or --file is required")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		key := extractIssueKey(args[0])
		text := ""
		if commentFile != "" {
			var err error
			if commentFile == "-" {
				body, readErr := io.ReadAll(cmd.InOrStdin())
				text, err = string(body), readErr
			} else {
				body, readErr := os.ReadFile(commentFile)
				text, err = string(body), readErr
			}
			if err != nil {
				return fmt.Errorf("reading comment file %q: %w", commentFile, err)
			}
		} else {
			text = args[1]
		}
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
	commentCmd.Flags().StringVar(&commentFile, "file", "", "read the comment body from a file; use - for stdin")
	rootCmd.AddCommand(commentCmd)
}
