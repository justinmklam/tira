package validator

import "testing"

func TestDefaultIssueType(t *testing.T) {
	valid := []string{"Bug", "Story", "Task"}

	tests := []struct {
		name       string
		configured string
		valid      []string
		want       string
		wantReject bool
	}{
		{name: "empty configured uses first valid", configured: "", valid: valid, want: "Bug"},
		{name: "matching configured is used", configured: "Task", valid: valid, want: "Task"},
		{name: "case-insensitive match normalises", configured: "task", valid: valid, want: "Task"},
		{name: "unmatched configured falls back and reports", configured: "Feature", valid: valid, want: "Bug", wantReject: true},
		{name: "empty valid list returns configured", configured: "Task", valid: nil, want: "Task"},
		{name: "both empty", configured: "", valid: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rejected := DefaultIssueType(tt.configured, tt.valid)
			if got != tt.want || rejected != tt.wantReject {
				t.Errorf("DefaultIssueType(%q, %v) = (%q, %v), want (%q, %v)",
					tt.configured, tt.valid, got, rejected, tt.want, tt.wantReject)
			}
		})
	}
}
