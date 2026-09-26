package main

import (
	"strings"
	"testing"

	"github.com/justinmklam/tira/internal/app"
	"github.com/spf13/cobra"
)

func TestParseBoardView(t *testing.T) {
	tests := []struct {
		name    string
		view    string
		want    app.BoardView
		wantErr bool
	}{
		{name: "empty defaults to backlog", view: "", want: app.ViewBacklog},
		{name: "backlog", view: "backlog", want: app.ViewBacklog},
		{name: "kanban", view: "kanban", want: app.ViewKanban},
		{name: "epics", view: "epics", want: app.ViewEpics},
		{name: "case insensitive", view: "KANBAN", want: app.ViewKanban},
		{name: "invalid value errors", view: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBoardView(tt.view)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseBoardView(%q) error = nil, want error", tt.view)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBoardView(%q) unexpected error: %v", tt.view, err)
			}
			if got != tt.want {
				t.Errorf("parseBoardView(%q) = %v, want %v", tt.view, got, tt.want)
			}
		})
	}
}

// TestBacklogAndKanbanCommands_AreDeprecated locks in that `backlog`/`kanban`
// are kept working as deprecated aliases for `board --view=...` (see
// docs/command-restructure-proposal.md §5) rather than removed outright.
func TestBacklogAndKanbanCommands_AreDeprecated(t *testing.T) {
	if backlogCmd.Deprecated == "" {
		t.Error("backlogCmd should be marked Deprecated")
	}
	if kanbanCmd.Deprecated == "" {
		t.Error("kanbanCmd should be marked Deprecated")
	}
	if boardCmd.Deprecated != "" {
		t.Error("boardCmd (the canonical command) should not be marked Deprecated")
	}
}

// TestBoardCommands_HaveConsistentFlags verifies all three board-launching
// commands share --project and --board-id, and only `board` has --view.
func TestBoardCommands_HaveConsistentFlags(t *testing.T) {
	for _, cmd := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{"board", boardCmd},
		{"backlog", backlogCmd},
		{"kanban", kanbanCmd},
	} {
		if cmd.cmd.Flags().Lookup("project") == nil {
			t.Errorf("%s missing --project flag", cmd.name)
		}
		if cmd.cmd.Flags().Lookup("board-id") == nil {
			t.Errorf("%s missing --board-id flag", cmd.name)
		}
	}

	if boardCmd.Flags().Lookup("view") == nil {
		t.Error("boardCmd missing --view flag")
	}
	if boardCmd.Flags().Lookup("snapshot") == nil {
		t.Error("boardCmd missing --snapshot flag")
	}
	if boardCmd.Flags().Lookup("snapshot-size") == nil {
		t.Error("boardCmd missing --snapshot-size flag")
	} else if got := boardCmd.Flags().Lookup("snapshot-size").DefValue; got != "120x40" {
		t.Errorf("--snapshot-size default = %q, want %q", got, "120x40")
	}
	if backlogCmd.Flags().Lookup("view") != nil {
		t.Error("backlogCmd should not have its own --view flag (view is implied)")
	}
	if kanbanCmd.Flags().Lookup("view") != nil {
		t.Error("kanbanCmd should not have its own --view flag (view is implied)")
	}
}

func TestParseSnapshotSize(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantW      int
		wantH      int
		wantErrMsg string
	}{
		{name: "default", raw: "120x40", wantW: 120, wantH: 40},
		{name: "minimum accepted", raw: "20x20", wantW: 20, wantH: 20},
		{name: "surrounding spaces are tolerated", raw: " 80 x 24 ", wantW: 80, wantH: 24},
		{name: "missing height", raw: "120", wantErrMsg: "expected WxH"},
		{name: "empty height", raw: "120x", wantErrMsg: "expected WxH"},
		{name: "empty width", raw: "x40", wantErrMsg: "expected WxH"},
		{name: "non numeric", raw: "widex40", wantErrMsg: "expected WxH"},
		{name: "uppercase separator", raw: "120X40", wantErrMsg: "expected WxH"},
		{name: "width below minimum", raw: "19x40", wantErrMsg: "at least 20"},
		{name: "height below minimum", raw: "120x19", wantErrMsg: "at least 20"},
		{name: "negative", raw: "-1x40", wantErrMsg: "at least 20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height, err := parseSnapshotSize(tt.raw)
			if tt.wantErrMsg != "" {
				if err == nil {
					t.Fatalf("parseSnapshotSize(%q) error = nil, want error", tt.raw)
				}
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("parseSnapshotSize(%q) error = %q, want it to contain %q", tt.raw, err, tt.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSnapshotSize(%q) unexpected error: %v", tt.raw, err)
			}
			if width != tt.wantW || height != tt.wantH {
				t.Errorf("parseSnapshotSize(%q) = %dx%d, want %dx%d", tt.raw, width, height, tt.wantW, tt.wantH)
			}
		})
	}
}
