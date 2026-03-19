package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/justinmklam/lazyjira/internal/config"
	"github.com/spf13/cobra"
)

var (
	debug   bool
	profile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "lazyjira",
	Short: "A lazygit-style CLI for Jira",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if debug {
			log.SetLevel(log.DebugLevel)
		}

		var err error
		cfg, err = config.Load(profile)
		if err != nil {
			return fmt.Errorf("%w", err)
		}

		log.Debug("config loaded", "profile", profile, "url", cfg.JiraURL, "project", cfg.Project)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().StringVar(&profile, "profile", "default", "config profile to use")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
