package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/justinmklam/tira/internal/models"
)

func TestDownloadAttachmentUsesAuthenticatedIssueClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/attachment/content/10001" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte("attachment text"))
	}))
	defer server.Close()

	client := newTestClient(server)
	got, err := client.DownloadAttachment(models.Attachment{ID: "10001", Filename: "notes.txt"})
	if err != nil {
		t.Fatalf("DownloadAttachment() error = %v", err)
	}
	if string(got) != "attachment text" {
		t.Fatalf("content = %q, want attachment text", got)
	}
}
