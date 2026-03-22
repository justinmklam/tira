package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/justinmklam/tira/internal/config"
	"github.com/justinmklam/tira/internal/debug"
	"github.com/spf13/cobra"
)

var (
	debugMode bool
	profile   string
	cfg       *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "tira",
	Short: "A lazygit-style CLI for Jira",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if debugMode {
			log.SetLevel(log.DebugLevel)
			if err := debug.Init(); err != nil {
				return fmt.Errorf("initializing debug logger: %w", err)
			}
			debug.Logf("Debug mode enabled")
		}

		// Skip config loading for commands that don't need it
		if cmd.Name() == "create" && cmd.Flag("template") != nil {
			if val := cmd.Flag("template").Value.String(); val == "true" {
				return nil
			}
		}

		var err error
		cfg, err = config.Load(profile)
		if err != nil {
			debug.LogError("config.Load", err)
			return err
		}

		log.Debug("config loaded", "profile", profile, "url", cfg.JiraURL, "project", cfg.Project)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug logging")
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
