package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/app"
	"github.com/spf13/cobra"
)

var boardProject string

var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "Interactive board with backlog and kanban views (Tab to toggle)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBoardCmd(app.ViewBacklog)
	},
}

var backlogCmd = &cobra.Command{
	Use:   "backlog",
	Short: "Show the project backlog (Tab to switch to kanban)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBoardCmd(app.ViewBacklog)
	},
}

var kanbanCmd = &cobra.Command{
	Use:   "kanban",
	Short: "Show the active sprint as a kanban board (Tab to switch to backlog)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBoardCmd(app.ViewKanban)
	},
}

func init() {
	rootCmd.AddCommand(boardCmd)
	rootCmd.AddCommand(backlogCmd)
	rootCmd.AddCommand(kanbanCmd)

	// Add project flag to all board commands
	for _, cmd := range []*cobra.Command{boardCmd, backlogCmd, kanbanCmd} {
		cmd.Flags().StringVar(&boardProject, "project", "", "override the default project from config")
	}
}

func runBoardCmd(startView app.BoardView) error {
	if cfg.BoardID == 0 {
		return fmt.Errorf("board ID not configured: set default_board_id in ~/.config/tira/config.yaml")
	}

	// Override project from flag if provided
	project := cfg.Project
	if boardProject != "" {
		project = boardProject
		log.Debug("project overridden", "original", cfg.Project, "override", project)
	}

	client, err := api.NewClient(cfg)
	if err != nil {
		return err
	}

	// Validate project exists before fetching board data
	if err := client.ValidateProject(project); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil
	}

	data, err := app.FetchBoardData(client, cfg.BoardID, project)
	if err != nil {
		return err
	}
	if len(data.Groups) == 0 {
		fmt.Fprintln(os.Stderr, "No sprints or backlog issues found.")
		return nil
	}

	return app.RunBoardTUI(client, cfg.BoardID, cfg.JiraURL, project, cfg.ClassicProject, data, startView)
}
