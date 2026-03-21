package main

import (
	"fmt"
	"os"

	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/app"
	"github.com/spf13/cobra"
)

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
}

func runBoardCmd(startView app.BoardView) error {
	if cfg.BoardID == 0 {
		return fmt.Errorf("board ID not configured: set default_board_id in ~/.config/tira/config.yaml")
	}

	client, err := api.NewClient(cfg)
	if err != nil {
		return err
	}

	data, err := app.FetchBoardData(client, cfg.BoardID)
	if err != nil {
		return err
	}
	if len(data.Groups) == 0 {
		fmt.Fprintln(os.Stderr, "No sprints or backlog issues found.")
		return nil
	}

	return app.RunBoardTUI(client, cfg.BoardID, cfg.JiraURL, cfg.Project, cfg.ClassicProject, data, startView)
}
