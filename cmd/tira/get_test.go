package main

import "testing"

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
