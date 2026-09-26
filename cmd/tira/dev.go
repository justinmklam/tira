package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/mock"
)

var (
	devMode     bool
	devFixtures string
	devState    string
	devClient   *mock.Client // non-nil only in dev mode
)

// devModeEnabled reports whether --dev was passed or TIRA_DEV_MODE is truthy.
// Dev mode is never enabled by config file contents, so a stray config value can
// never silently mock a real board.
func devModeEnabled() bool {
	if devMode {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TIRA_DEV_MODE"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// initDevClient builds the fixture-backed client and patches cfg from it so
// custom fixtures work with no config file at all. Flags win over the
// TIRA_DEV_* environment variables.
func initDevClient() error {
	fixtures := devFixtures
	if fixtures == "" {
		fixtures = os.Getenv("TIRA_DEV_FIXTURES")
	}
	state := devState
	if state == "" {
		state = os.Getenv("TIRA_DEV_STATE")
	}

	client, err := mock.New(mock.Options{FixturePath: fixtures, StatePath: state})
	if err != nil {
		return err
	}
	devClient = client

	fmt.Fprintf(os.Stderr, "dev mode: fixture %s (project %s, board %d)\n",
		client.Source(), client.Project(), client.BoardID())

	if cfg.Project == "" {
		cfg.Project = client.Project()
	}
	if cfg.BoardID == 0 {
		cfg.BoardID = client.BoardID()
	}
	if cfg.JiraURL == "" {
		// Used only to build browser links (the "o" key); nothing is fetched.
		cfg.JiraURL = "https://demo.atlassian.net"
	}
	if os.Getenv("TIRA_CLASSIC_PROJECT") == "" {
		cfg.ClassicProject = true
	}
	return nil
}

// newAPIClient returns the fixture-backed client in dev mode and the real Jira
// client otherwise.
func newAPIClient() (api.Client, error) {
	if devClient != nil {
		return devClient, nil
	}
	return api.NewClient(cfg)
}
