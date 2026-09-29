package api

import (
	"fmt"
	"io"
	"strings"

	"github.com/justinmklam/tira/internal/models"
)

const maxAttachmentDownloadSize int64 = 50 << 20 // 50 MiB

// DownloadAttachment downloads an issue attachment using Jira's authenticated
// HTTP transport. The limit prevents an accidental `get` from consuming an
// unbounded amount of memory.
func (c *jiraClient) DownloadAttachment(attachment models.Attachment) ([]byte, error) {
	contentURL := attachment.ContentURL
	if contentURL == "" && attachment.ID != "" {
		contentURL = fmt.Sprintf("%s/rest/api/3/attachment/content/%s", c.baseURL, attachment.ID)
	}
	if contentURL == "" {
		return nil, fmt.Errorf("attachment %q has no content URL", attachment.Filename)
	}
	if !strings.HasPrefix(contentURL, c.baseURL+"/") && contentURL != c.baseURL {
		return nil, fmt.Errorf("attachment %q has an unexpected content URL", attachment.Filename)
	}

	resp, err := c.http.Get(contentURL)
	if err != nil {
		return nil, fmt.Errorf("downloading attachment %q: %w", attachment.Filename, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("downloading attachment %q: HTTP %d", attachment.Filename, resp.StatusCode)
	}
	if resp.ContentLength > maxAttachmentDownloadSize {
		return nil, fmt.Errorf("attachment %q exceeds the 50 MiB download limit", attachment.Filename)
	}

	limited := io.LimitReader(resp.Body, maxAttachmentDownloadSize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading attachment %q: %w", attachment.Filename, err)
	}
	if int64(len(body)) > maxAttachmentDownloadSize {
		return nil, fmt.Errorf("attachment %q exceeds the 50 MiB download limit", attachment.Filename)
	}
	return body, nil
}
