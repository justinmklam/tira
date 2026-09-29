package main

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/debug"
	"github.com/justinmklam/tira/internal/models"
)

const (
	attachmentModeAuto     = "auto"
	attachmentModeNone     = "none"
	attachmentModeMetadata = "metadata"
	attachmentModeContent  = "content"
	maxInlineAttachment    = 1 << 20 // 1 MiB
)

func prepareIssueAttachments(client any, issue *models.Issue, mode, requestedDir string) error {
	if mode != attachmentModeAuto && mode != attachmentModeNone && mode != attachmentModeMetadata && mode != attachmentModeContent {
		return fmt.Errorf("invalid --attachments mode %q (want auto, none, metadata, or content)", mode)
	}
	if mode == attachmentModeNone || mode == attachmentModeMetadata || len(issue.Attachments) == 0 {
		return nil
	}

	downloader, ok := client.(api.AttachmentDownloader)
	if !ok {
		for i := range issue.Attachments {
			issue.Attachments[i].ContentError = "attachment downloads are unavailable for this client"
		}
		return nil
	}

	attachmentDir, err := attachmentDirectory(issue.Key, requestedDir)
	if err != nil {
		return err
	}
	for i := range issue.Attachments {
		attachment := &issue.Attachments[i]
		body, err := downloader.DownloadAttachment(*attachment)
		if err != nil {
			attachment.ContentError = err.Error()
			debug.LogError("DownloadAttachment", err)
			continue
		}
		path, err := uniqueAttachmentPath(attachmentDir, attachment.Filename, attachment.ID)
		if err != nil {
			attachment.ContentError = err.Error()
			debug.LogError("attachment path", err)
			continue
		}
		if err := os.WriteFile(path, body, 0600); err != nil {
			attachment.ContentError = fmt.Sprintf("saving attachment: %v", err)
			debug.LogError("saving attachment", err)
			continue
		}
		attachment.LocalPath = path
		if isTextAttachment(*attachment, body) {
			if len(body) <= maxInlineAttachment {
				attachment.TextContent = string(body)
			} else if mode == attachmentModeContent {
				attachment.ContentError = "text attachment exceeds the 1 MiB inline limit"
			}
		}
	}
	return nil
}

func attachmentDirectory(issueKey, requestedDir string) (string, error) {
	base := requestedDir
	if base == "" {
		var err error
		base, err = os.MkdirTemp("", "tira-attachments-")
		if err != nil {
			return "", fmt.Errorf("creating temporary attachment directory: %w", err)
		}
	}
	dir := filepath.Join(base, sanitizeFilename(issueKey))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating attachment directory: %w", err)
	}
	return dir, nil
}

func uniqueAttachmentPath(dir, filename, id string) (string, error) {
	name := sanitizeFilename(filename)
	if name == "" || name == "." {
		name = "attachment-" + sanitizeFilename(id)
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path, nil
	}
	for n := 2; ; n++ {
		ext := filepath.Ext(name)
		stem := strings.TrimSuffix(name, ext)
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, n, ext))
		_, statErr := os.Stat(candidate)
		if os.IsNotExist(statErr) {
			return candidate, nil
		}
		if statErr != nil {
			return "", statErr
		}
	}
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "\x00", "")
	return name
}

func isTextAttachment(attachment models.Attachment, body []byte) bool {
	mediaType := attachment.MimeType
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = parsed
	}
	if strings.HasPrefix(mediaType, "text/") || strings.Contains(mediaType, "json") || strings.Contains(mediaType, "xml") {
		return utf8.Valid(body)
	}
	ext := strings.ToLower(filepath.Ext(attachment.Filename))
	switch ext {
	case ".txt", ".md", ".csv", ".log", ".json", ".xml", ".yaml", ".yml", ".toml", ".ini", ".go", ".js", ".ts", ".tsx", ".jsx", ".py", ".rb", ".java", ".sh", ".sql", ".html", ".css":
		return utf8.Valid(body)
	default:
		return false
	}
}
