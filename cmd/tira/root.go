package main

import (
	"fmt"
	"os"
	"runtime"

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

		var err error
		cfg, err = config.Load(profile)
		if err != nil {
			debug.LogError("config.Load", err)
			return fmt.Errorf("%w", err)
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
	// Set up cleanup on exit using runtime.SetFinalizer
	if debugMode {
		runtime.SetFinalizer(new(struct{}), func(_ *struct{}) {
			if err := debug.Close(); err != nil {
				log.Error("closing debug log", "error", err)
			}
		})
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
