package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/justinmklam/tira/internal/mock"
)

func TestApplyTemplateUpdateIgnoresParent(t *testing.T) {
	client, err := mock.New(mock.Options{})
	if err != nil {
		t.Fatalf("mock.New error: %v", err)
	}
	issue, err := client.GetIssue("DEMO-11")
	if err != nil {
		t.Fatalf("GetIssue error: %v", err)
	}
	originalParent := issue.ParentKey

	valid, err := client.GetValidValues("DEMO")
	if err != nil {
		t.Fatalf("GetValidValues error: %v", err)
	}
	content := `<!-- tira: do not remove this line or change field names -->
parent: DEMO-2
---
# DEMO-11: Updated child summary`
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe error: %v", err)
	}
	os.Stderr = writer
	updateErr := applyTemplateUpdate(client, issue, valid, content)
	_ = writer.Close()
	os.Stderr = oldStderr
	warning, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("reading captured stderr: %v", err)
	}
	if updateErr != nil {
		t.Fatalf("applyTemplateUpdate error: %v", updateErr)
	}
	if !strings.Contains(string(warning), `warning: parent is read-only in update templates; ignoring "DEMO-2"`) {
		t.Errorf("stderr = %q, want read-only parent warning", string(warning))
	}

	updated, err := client.GetIssue("DEMO-11")
	if err != nil {
		t.Fatalf("GetIssue after update error: %v", err)
	}
	if updated.Summary != "Updated child summary" {
		t.Errorf("Summary = %q, want %q", updated.Summary, "Updated child summary")
	}
	if updated.ParentKey != originalParent {
		t.Errorf("ParentKey = %q, want unchanged %q", updated.ParentKey, originalParent)
	}
}

func TestExtractIssueKey(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "bare key",
			arg:  "PROJ-123",
			want: "PROJ-123",
		},
		{
			name: "lowercase bare key is upper-cased",
			arg:  "proj-123",
			want: "PROJ-123",
		},
		{
			name: "full browse URL",
			arg:  "https://example.atlassian.net/browse/PROJ-123",
			want: "PROJ-123",
		},
		{
			name: "browse URL with trailing slash",
			arg:  "https://example.atlassian.net/browse/PROJ-123/",
			want: "PROJ-123",
		},
		{
			name: "browse URL with query string",
			arg:  "https://example.atlassian.net/browse/TEST-456?filter=1",
			want: "TEST-456",
		},
		{
			name: "surrounding whitespace is trimmed",
			arg:  "  PROJ-123  ",
			want: "PROJ-123",
		},
		{
			name: "unrecognized input is returned unchanged",
			arg:  "not-a-key",
			want: "not-a-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractIssueKey(tt.arg); got != tt.want {
				t.Errorf("extractIssueKey(%q) = %q, want %q", tt.arg, got, tt.want)
			}
		})
	}
}
