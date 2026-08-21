package main

import "testing"

// TestUpdateTemplateFlag_IsDeprecatedAliasForShow locks in that --template is
// kept working (bound to the same variable as --show, just deprecated)
// rather than removed outright — see docs/command-restructure-proposal.md §5.
func TestUpdateTemplateFlag_IsDeprecatedAliasForShow(t *testing.T) {
	showFlag := updateCmd.Flags().Lookup("show")
	if showFlag == nil {
		t.Fatal(`update missing "--show" flag`)
	}
	if showFlag.Deprecated != "" {
		t.Errorf("--show should not be deprecated, got Deprecated=%q", showFlag.Deprecated)
	}

	templateFlag := updateCmd.Flags().Lookup("template")
	if templateFlag == nil {
		t.Fatal(`update missing deprecated "--template" flag`)
	}
	if templateFlag.Deprecated == "" {
		t.Error("--template should be marked Deprecated")
	}

	// Both flags must be bound to the same underlying variable so that
	// setting either one produces identical behavior.
	updateShow = false
	if err := templateFlag.Value.Set("true"); err != nil {
		t.Fatalf("setting --template: %v", err)
	}
	if !updateShow {
		t.Error("setting --template did not update the shared updateShow variable")
	}

	updateShow = false
	if err := showFlag.Value.Set("true"); err != nil {
		t.Fatalf("setting --show: %v", err)
	}
	if !updateShow {
		t.Error("setting --show did not update the shared updateShow variable")
	}
}
