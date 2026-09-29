package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/justinmklam/tira/internal/models"
)

type attachmentTestClient struct {
	content []byte
}

func (c attachmentTestClient) DownloadAttachment(models.Attachment) ([]byte, error) {
	return c.content, nil
}

func TestPrepareIssueAttachmentsSavesAndInlinesText(t *testing.T) {
	dir := t.TempDir()
	issue := &models.Issue{Key: "PROJ-1", Attachments: []models.Attachment{{ID: "1", Filename: "notes.txt", MimeType: "text/plain"}}}

	if err := prepareIssueAttachments(attachmentTestClient{content: []byte("hello\n")}, issue, attachmentModeAuto, dir); err != nil {
		t.Fatalf("prepareIssueAttachments() error = %v", err)
	}
	if issue.Attachments[0].TextContent != "hello\n" {
		t.Fatalf("TextContent = %q", issue.Attachments[0].TextContent)
	}
	if filepath.Dir(issue.Attachments[0].LocalPath) != filepath.Join(dir, "PROJ-1") {
		t.Fatalf("LocalPath = %q", issue.Attachments[0].LocalPath)
	}
	if _, err := os.Stat(issue.Attachments[0].LocalPath); err != nil {
		t.Fatalf("saved attachment: %v", err)
	}
}

func TestPrepareIssueAttachmentsSanitizesFilename(t *testing.T) {
	dir := t.TempDir()
	issue := &models.Issue{Key: "PROJ-1", Attachments: []models.Attachment{{ID: "1", Filename: "../secret.txt", MimeType: "text/plain"}}}

	if err := prepareIssueAttachments(attachmentTestClient{content: []byte("safe")}, issue, attachmentModeAuto, dir); err != nil {
		t.Fatalf("prepareIssueAttachments() error = %v", err)
	}
	if filepath.Dir(issue.Attachments[0].LocalPath) != filepath.Join(dir, "PROJ-1") {
		t.Fatalf("attachment escaped directory: %q", issue.Attachments[0].LocalPath)
	}
	if filepath.Base(issue.Attachments[0].LocalPath) != "secret.txt" {
		t.Fatalf("sanitized filename = %q", filepath.Base(issue.Attachments[0].LocalPath))
	}
}
