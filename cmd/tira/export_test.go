package main

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/models"
)

type exportTestClient struct {
	api.Client
	issues []models.Issue
	err    error
	calls  []string
}

func (c *exportTestClient) GetProjectIssues(projectKey string) ([]models.Issue, error) {
	c.calls = append(c.calls, projectKey)
	return c.issues, c.err
}

func TestRunProjectExportWritesStableCSV(t *testing.T) {
	issues := []models.Issue{
		{
			Key: "PROJ-1", Summary: "comma, quote \" and\nnewline", EpicKey: "PROJ-10",
			IssueType: "Story", StoryPoints: 3.5, Assignee: "Ada Lovelace", Priority: "High",
			Status: "In Progress", Reporter: "Grace Hopper", Labels: []string{"comma,label", "quote\"label", "line\nbreak"}, ParentKey: "PROJ-10",
		},
		{Key: "PROJ-2", Summary: "Minimal", IssueType: "Task"},
	}
	client := &exportTestClient{issues: issues}
	output := filepath.Join(t.TempDir(), "issues.csv")
	factoryCalls := 0
	factory := func() (api.Client, error) {
		factoryCalls++
		return client, nil
	}

	if err := runProjectExport(output, "", "PROJ", "https://jira.example///", factory); err != nil {
		t.Fatalf("runProjectExport() error = %v", err)
	}
	if factoryCalls != 1 || !reflect.DeepEqual(client.calls, []string{"PROJ"}) {
		t.Fatalf("factory calls = %d, project calls = %v", factoryCalls, client.calls)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatalf("opening CSV: %v", err)
	}
	rows, readErr := csv.NewReader(file).ReadAll()
	closeErr := file.Close()
	if readErr != nil {
		t.Fatalf("parsing CSV: %v", readErr)
	}
	if closeErr != nil {
		t.Fatalf("closing CSV: %v", closeErr)
	}
	wantHeader := []string{"Issue Key", "Issue URL", "Summary", "Epic Key", "Epic URL", "Type", "Story Points", "Assignee", "Priority", "Status", "Reporter", "Labels", "Parent Key", "Parent URL"}
	if !reflect.DeepEqual(rows[0], wantHeader) {
		t.Fatalf("header = %#v, want %#v", rows[0], wantHeader)
	}
	if len(rows) != 3 {
		t.Fatalf("CSV rows = %d, want header and two issues", len(rows))
	}
	wantFirst := []string{
		"PROJ-1", "https://jira.example/browse/PROJ-1", "comma, quote \" and\nnewline",
		"PROJ-10", "https://jira.example/browse/PROJ-10", "Story", "3.5", "Ada Lovelace", "High",
		"In Progress", "Grace Hopper", "comma,label; quote\"label; line\nbreak", "PROJ-10", "https://jira.example/browse/PROJ-10",
	}
	if !reflect.DeepEqual(rows[1], wantFirst) {
		t.Errorf("first row = %#v, want %#v", rows[1], wantFirst)
	}
	wantSecond := []string{"PROJ-2", "https://jira.example/browse/PROJ-2", "Minimal", "", "", "Task", "", "", "", "", "", "", "", ""}
	if !reflect.DeepEqual(rows[2], wantSecond) {
		t.Errorf("second row = %#v, want %#v", rows[2], wantSecond)
	}
}

func TestRunProjectExportUsesProjectOverrideAndValidatesBeforeClient(t *testing.T) {
	client := &exportTestClient{}
	factoryCalls := 0
	factory := func() (api.Client, error) {
		factoryCalls++
		return client, nil
	}
	output := filepath.Join(t.TempDir(), "issues.csv")

	if err := runProjectExport(output, "OVERRIDE", "CONFIG", "https://jira.example", factory); err != nil {
		t.Fatalf("override export error = %v", err)
	}
	if !reflect.DeepEqual(client.calls, []string{"OVERRIDE"}) {
		t.Errorf("project calls = %v, want OVERRIDE", client.calls)
	}

	factoryCalls = 0
	if err := runProjectExport("", "OVERRIDE", "CONFIG", "https://jira.example", factory); err == nil {
		t.Fatal("missing output path error = nil")
	}
	if err := runProjectExport(output, "", "", "https://jira.example", factory); err == nil {
		t.Fatal("missing project error = nil")
	}
	if factoryCalls != 0 {
		t.Errorf("client factory called %d times for invalid arguments, want zero", factoryCalls)
	}
}

func TestRunProjectExportFetchFailureDoesNotTruncateOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "issues.csv")
	if err := os.WriteFile(output, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	client := &exportTestClient{err: errors.New("jira unavailable")}
	factory := func() (api.Client, error) { return client, nil }

	err := runProjectExport(output, "", "PROJ", "https://jira.example", factory)
	if err == nil || !strings.Contains(err.Error(), "fetching project PROJ issues") {
		t.Fatalf("error = %v, want contextual fetch error", err)
	}
	data, readErr := os.ReadFile(output)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "keep me" {
		t.Errorf("output after failed fetch = %q, want original contents", data)
	}
}

func TestExportCommandRejectsPositionalArguments(t *testing.T) {
	if err := exportCmd.Args(exportCmd, []string{"unexpected"}); err == nil {
		t.Fatal("Args() error = nil, want positional arguments rejected")
	}
	if err := exportCmd.Args(exportCmd, nil); err != nil {
		t.Fatalf("Args() with no positional arguments = %v", err)
	}
}
