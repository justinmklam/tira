package main

import (
	"fmt"
	"os"

	"charm.land/log/v2"
	"github.com/justinmklam/tira/internal/config"
	"github.com/justinmklam/tira/internal/debug"
	"github.com/justinmklam/tira/internal/tui"
	"github.com/spf13/cobra"
)

var (
	debugMode bool
	debugFile string
	profile   string
	cfg       *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "tira",
	Short: "A blazing fast terminal interface for Jira",
	Long: `tira — a fast terminal interface for Jira.

Quick reference for AI agents and automation:

  Read an issue (outputs Markdown; pipe-safe):
    tira get <KEY>
    tira get <KEY> | cat

  Create an issue from stdin (non-interactive):
    cat <<'EOF' | tira create --no-edit
    <!-- tira: do not remove this line or change field names -->
    type: Task

    ---

    # Summary of the work

    ## Description

    What needs to be done.

    ## Acceptance Criteria

    - Criterion 1
    EOF

  Get the full template format specification:
    tira create --template

  Update an existing issue (non-interactive, recommended for agents):
    tira update <KEY> --show > /tmp/issue.md   # capture current values
    # edit /tmp/issue.md, changing only what you need, then:
    cat /tmp/issue.md | tira update <KEY> --no-edit

  Edit an existing issue interactively:
    tira update <KEY>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if debugFile != "" {
			debugMode = true
		}
		if debugMode {
			log.SetLevel(log.DebugLevel)
			if err := debug.Init(debugFile); err != nil {
				return fmt.Errorf("initializing debug logger: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Debug logging to %s\n", debug.LogPath())
			debug.Logf("Debug mode enabled")
		}

		// Skip config loading for commands that don't need it
		if cmd.Name() == "create" && cmd.Flag("template") != nil {
			if val := cmd.Flag("template").Value.String(); val == "true" {
				return nil
			}
		}
		if cmd.Name() == "version" {
			return nil
		}

		var err error
		cfg, err = config.Load(profile)
		if err != nil {
			debug.LogError("config.Load", err)
			return err
		}

		if cfg.Theme != "" {
			if err := tui.SetTheme(cfg.Theme); err != nil {
				return fmt.Errorf("invalid theme %q: %w", cfg.Theme, err)
			}
		}

		log.Debug("config loaded", "profile", profile, "url", cfg.JiraURL, "project", cfg.Project)
		return nil
	},
}

func init() {
	rootCmd.Version = version
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().StringVar(&debugFile, "debug-file", "", "enable debug logging to a specific path (implies --debug; default path: $XDG_STATE_HOME/tira/debug.log)")
	rootCmd.PersistentFlags().StringVar(&profile, "profile", "default", "config profile to use")
}

func Execute() {
	err := rootCmd.Execute()
	if closeErr := debug.Close(); closeErr != nil {
		log.Error("closing debug log", "error", closeErr)
	}
	if err != nil {
		os.Exit(1)
	}
}
