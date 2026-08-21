package main

import (
	"fmt"
	"os"
	"strings"

	"charm.land/log/v2"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/app"
	"github.com/spf13/cobra"
)

var (
	boardProject string
	boardID      int
	boardView    string
)

var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "Interactive board with backlog and kanban views (Tab to toggle)",
	Long: `Interactive board with backlog and kanban views (Tab to toggle).

Starts on the backlog view by default; use --view to start elsewhere:

  tira board                  # starts on backlog
  tira board --view kanban    # starts on kanban
  tira board --view backlog   # explicit, same as default`,
	RunE: func(cmd *cobra.Command, args []string) error {
		view, err := parseBoardView(boardView)
		if err != nil {
			return err
		}
		return runBoardCmd(view)
	},
}

// backlogCmd and kanbanCmd are deprecated aliases for `tira board --view=...`,
// kept working (not removed) so existing scripts/muscle memory don't break.
// Cobra prints the Deprecated message to stderr on every invocation.
var backlogCmd = &cobra.Command{
	Use:        "backlog",
	Short:      "Show the project backlog (Tab to switch to kanban)",
	Deprecated: "use 'tira board' (or 'tira board --view backlog') instead",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBoardCmd(app.ViewBacklog)
	},
}

var kanbanCmd = &cobra.Command{
	Use:        "kanban",
	Short:      "Show the active sprint as a kanban board (Tab to switch to backlog)",
	Deprecated: "use 'tira board --view kanban' instead",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBoardCmd(app.ViewKanban)
	},
}

func init() {
	rootCmd.AddCommand(boardCmd)
	rootCmd.AddCommand(backlogCmd)
	rootCmd.AddCommand(kanbanCmd)

	// Add shared flags to all board commands.
	for _, cmd := range []*cobra.Command{boardCmd, backlogCmd, kanbanCmd} {
		cmd.Flags().StringVar(&boardProject, "project", "", "override the default project from config")
		cmd.Flags().IntVar(&boardID, "board-id", 0, "override the default board ID from config")
	}
	boardCmd.Flags().StringVar(&boardView, "view", "backlog", `starting view: "backlog" or "kanban"`)
}

// parseBoardView validates and converts the --view flag value.
func parseBoardView(view string) (app.BoardView, error) {
	switch strings.ToLower(view) {
	case "", "backlog":
		return app.ViewBacklog, nil
	case "kanban":
		return app.ViewKanban, nil
	default:
		return 0, fmt.Errorf("invalid --view %q: must be \"backlog\" or \"kanban\"", view)
	}
}

func runBoardCmd(startView app.BoardView) error {
	id := cfg.BoardID
	if boardID != 0 {
		log.Debug("board ID overridden", "original", cfg.BoardID, "override", boardID)
		id = boardID
	}
	if id == 0 {
		return fmt.Errorf("board ID not configured: set default_board_id in ~/.config/tira/config.yaml or use --board-id")
	}

	// Override project from flag if provided
	project := cfg.Project
	if boardProject != "" {
		project = boardProject
		log.Debug("project overridden", "original", cfg.Project, "override", project)
	}

	rawClient, err := api.NewClient(cfg)
	if err != nil {
		return err
	}
	client := api.NewCachedClient(rawClient)

	// Validate project exists before fetching board data
	if err := client.ValidateProject(project); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil
	}

	data, err := app.FetchBoardData(client, id, project)
	if err != nil {
		return err
	}
	if len(data.Groups) == 0 {
		fmt.Fprintln(os.Stderr, "No sprints or backlog issues found.")
		return nil
	}

	return app.RunBoardTUI(client, id, cfg.JiraURL, project, cfg.ClassicProject, data, startView)
}
