package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/justinmklam/tira/internal/mock"
	"github.com/spf13/cobra"
)

func withCommentTestClient(t *testing.T) *mock.Client {
	t.Helper()
	client, err := mock.New(mock.Options{})
	if err != nil {
		t.Fatalf("mock.New() error = %v", err)
	}
	previous := devClient
	devClient = client
	t.Cleanup(func() { devClient = previous })
	return client
}

func TestCommentCommandArgs(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"DEMO-3"},
		{"DEMO-3", "one", "two"},
	} {
		if err := commentCmd.Args(commentCmd, args); err == nil {
			t.Errorf("commentCmd.Args(%q) error = nil, want an argument-count error", args)
		}
	}
	if err := commentCmd.Args(commentCmd, []string{"DEMO-3", "hello"}); err != nil {
		t.Errorf("commentCmd.Args() error = %v, want nil", err)
	}
}

func TestCommentCommandAddsComment(t *testing.T) {
	client := withCommentTestClient(t)
	before, err := client.GetIssue("DEMO-3")
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	wantText := "  first line\nsecond line  "

	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	browseURL := "https://demo.atlassian.net/browse/demo-3?focusedCommentId=1"
	if err := commentCmd.RunE(cmd, []string{browseURL, wantText}); err != nil {
		t.Fatalf("comment command error = %v", err)
	}

	after, err := client.GetIssue("DEMO-3")
	if err != nil {
		t.Fatalf("GetIssue() after comment error = %v", err)
	}
	if len(after.Comments) != len(before.Comments)+1 {
		t.Fatalf("comments = %d, want %d", len(after.Comments), len(before.Comments)+1)
	}
	if got := after.Comments[0].Body; got != wantText {
		t.Errorf("new comment body = %q, want unchanged text %q", got, wantText)
	}
	if got, want := stderr.String(), "✓ Comment added to DEMO-3.\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestCommentCommandRejectsBlankText(t *testing.T) {
	client := withCommentTestClient(t)
	before, err := client.GetIssue("DEMO-3")
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}

	err = commentCmd.RunE(&cobra.Command{}, []string{"DEMO-3", " \n\t "})
	if err == nil || !strings.Contains(err.Error(), "comment text cannot be empty") {
		t.Fatalf("comment command error = %v, want empty-text validation error", err)
	}
	after, err := client.GetIssue("DEMO-3")
	if err != nil {
		t.Fatalf("GetIssue() after rejected comment error = %v", err)
	}
	if len(after.Comments) != len(before.Comments) {
		t.Errorf("blank comment changed comment count from %d to %d", len(before.Comments), len(after.Comments))
	}
}

func TestCommentCommandReturnsPostingError(t *testing.T) {
	withCommentTestClient(t)
	err := commentCmd.RunE(&cobra.Command{}, []string{"DEMO-999", "hello"})
	if err == nil || !strings.Contains(err.Error(), "adding comment to DEMO-999") {
		t.Fatalf("comment command error = %v, want contextual posting error", err)
	}
}
