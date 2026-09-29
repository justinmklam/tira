package main

import "testing"

func TestResolveCreateIssueType(t *testing.T) {
	valid := []string{"Bug", "Story", "Task"}

	tests := []struct {
		name        string
		flagType    string
		configured  string
		want        string
		wantWarning bool
		wantErr     bool
	}{
		{name: "flag wins over configured", flagType: "Story", configured: "Task", want: "Story"},
		{name: "invalid flag errors", flagType: "Feature", configured: "Task", wantErr: true},
		{name: "configured used when no flag", configured: "Task", want: "Task"},
		{name: "invalid configured warns and falls back", configured: "Feature", want: "Bug", wantWarning: true},
		{name: "first valid when both empty", want: "Bug"},
		{name: "configured match is case-insensitive", configured: "task", want: "Task"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warn, err := resolveCreateIssueType(tt.flagType, tt.configured, valid)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error for invalid flag type")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("type = %q, want %q", got, tt.want)
			}
			if (warn != "") != tt.wantWarning {
				t.Errorf("warning = %q, wantWarning %v", warn, tt.wantWarning)
			}
		})
	}
}

// TestResolveCreateIssueTypeAcceptsFlagWithoutMetadata covers a project whose
// metadata fetch failed: an explicit --type cannot be validated, so it is
// accepted rather than rejected.
func TestResolveCreateIssueTypeAcceptsFlagWithoutMetadata(t *testing.T) {
	got, warn, err := resolveCreateIssueType("Story", "Task", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Story" || warn != "" {
		t.Errorf("got (%q, %q), want (\"Story\", \"\")", got, warn)
	}
}
