// Package mock provides an in-process, fixture-backed implementation of
// api.Client used by the tira --dev mode. It is a test double, not a Jira
// emulator: it never performs HTTP and it makes no attempt to reproduce Jira's
// validation rules.
package mock

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

//go:embed fixtures/demo.yaml
var defaultFixtureYAML []byte

// Fixture is the on-disk schema for dev-mode data. Every field carries both a
// YAML and a JSON tag because the same shape is also serialised as the
// --dev-state file (see state.go).
type Fixture struct {
	Project     string                         `yaml:"project" json:"project"`
	Priorities  []string                       `yaml:"priorities,omitempty" json:"priorities,omitempty"`
	IssueTypes  []string                       `yaml:"issue_types,omitempty" json:"issue_types,omitempty"`
	Users       []UserFixture                  `yaml:"users,omitempty" json:"users,omitempty"`
	Board       BoardFixture                   `yaml:"board" json:"board"`
	Sprints     []SprintFixture                `yaml:"sprints,omitempty" json:"sprints,omitempty"`
	Backlog     []string                       `yaml:"backlog,omitempty" json:"backlog,omitempty"`
	Issues      []IssueFixture                 `yaml:"issues,omitempty" json:"issues,omitempty"`
	Transitions map[string][]TransitionFixture `yaml:"transitions,omitempty" json:"transitions,omitempty"`

	// source records where the fixture was loaded from, for the startup notice.
	// It is deliberately unexported so it is never serialised into a state file.
	source string
}

type UserFixture struct {
	DisplayName string `yaml:"display_name" json:"display_name"`
	AccountID   string `yaml:"account_id" json:"account_id"`
}

type BoardFixture struct {
	ID      int             `yaml:"id" json:"id"`
	Columns []ColumnFixture `yaml:"columns" json:"columns"`
}

// ColumnFixture lists the statuses that belong to a board column. Entries may be
// status IDs ("11") or status names ("To Do"); the client resolves names against
// the fixture's statuses.
type ColumnFixture struct {
	Name     string   `yaml:"name" json:"name"`
	Statuses []string `yaml:"statuses,omitempty" json:"statuses,omitempty"`
}

type SprintFixture struct {
	ID        int      `yaml:"id" json:"id"`
	Name      string   `yaml:"name" json:"name"`
	State     string   `yaml:"state,omitempty" json:"state,omitempty"`
	StartDate string   `yaml:"start_date,omitempty" json:"start_date,omitempty"`
	EndDate   string   `yaml:"end_date,omitempty" json:"end_date,omitempty"`
	Issues    []string `yaml:"issues,omitempty" json:"issues,omitempty"`
}

type IssueFixture struct {
	Key                string           `yaml:"key" json:"key"`
	Summary            string           `yaml:"summary" json:"summary"`
	Type               string           `yaml:"type,omitempty" json:"type,omitempty"`
	Status             string           `yaml:"status,omitempty" json:"status,omitempty"`
	StatusID           string           `yaml:"status_id,omitempty" json:"status_id,omitempty"`
	Priority           string           `yaml:"priority,omitempty" json:"priority,omitempty"`
	Assignee           string           `yaml:"assignee,omitempty" json:"assignee,omitempty"`
	Reporter           string           `yaml:"reporter,omitempty" json:"reporter,omitempty"`
	StoryPoints        float64          `yaml:"story_points,omitempty" json:"story_points,omitempty"`
	Labels             []string         `yaml:"labels,omitempty" json:"labels,omitempty"`
	Epic               string           `yaml:"epic,omitempty" json:"epic,omitempty"`
	Parent             string           `yaml:"parent,omitempty" json:"parent,omitempty"`
	Description        string           `yaml:"description,omitempty" json:"description,omitempty"`
	AcceptanceCriteria string           `yaml:"acceptance_criteria,omitempty" json:"acceptance_criteria,omitempty"`
	StatusChanged      string           `yaml:"status_changed,omitempty" json:"status_changed,omitempty"`
	Comments           []CommentFixture `yaml:"comments,omitempty" json:"comments,omitempty"`
	// Links are returned verbatim as declared. Jira stores issue links as a pair
	// of directed edges and the real client synthesises the reverse ("is blocked
	// by") when reading; the fake does not, so a fixture that wants both
	// directions must declare both.
	Links    []LinkFixture `yaml:"links,omitempty" json:"links,omitempty"`
	Subtasks []string      `yaml:"subtasks,omitempty" json:"subtasks,omitempty"`
}

type CommentFixture struct {
	Author  string `yaml:"author,omitempty" json:"author,omitempty"`
	Created string `yaml:"created,omitempty" json:"created,omitempty"`
	Body    string `yaml:"body" json:"body"`
}

type LinkFixture struct {
	Relationship string `yaml:"relationship" json:"relationship"`
	Key          string `yaml:"key" json:"key"`
}

// TransitionFixture models an available issue transition. The ID doubles as the
// target status ID, so a fixture issue with no explicit status_id inherits the
// ID of the transition whose Name matches its Status.
type TransitionFixture struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
}

// Load reads a fixture from path. An empty path loads the embedded demo fixture.
// A non-empty path must point at a single YAML (or JSON) file.
func Load(path string) (*Fixture, error) {
	data := defaultFixtureYAML
	source := "embedded demo fixture"

	if path != "" {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("reading dev fixtures %q: %w", path, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("dev fixtures path %q is a directory; pass a single YAML file", path)
		}
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading dev fixtures %q: %w", path, err)
		}
		source = path
	}

	fixture, err := decode(data)
	if err != nil {
		return nil, fmt.Errorf("dev fixtures from %s: %w", source, err)
	}
	fixture.source = source

	if err := fixture.validate(); err != nil {
		return nil, fmt.Errorf("invalid dev fixtures from %s: %w", source, err)
	}
	return fixture, nil
}

// decode parses fixture bytes with unknown-key detection enabled so a typo in an
// agent-authored fixture fails loudly and names the offending key.
func decode(data []byte) (*Fixture, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var f Fixture
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("fixture is empty")
		}
		return nil, err
	}
	return &f, nil
}

// validate reports the first referential-integrity or completeness problem it
// finds, naming the offending value.
func (f *Fixture) validate() error {
	if f.Project == "" {
		return errors.New("project is required")
	}
	if f.Board.ID <= 0 {
		return fmt.Errorf("board.id must be a positive integer, got %d", f.Board.ID)
	}
	if len(f.Board.Columns) == 0 {
		return errors.New("board.columns must contain at least one column")
	}

	keys := make(map[string]bool, len(f.Issues))
	for _, issue := range f.Issues {
		if issue.Key == "" {
			return errors.New("every issue must have a key")
		}
		if keys[issue.Key] {
			return fmt.Errorf("duplicate issue key %q", issue.Key)
		}
		if !strings.HasPrefix(issue.Key, f.Project+"-") {
			return fmt.Errorf("issue key %q does not start with project prefix %q", issue.Key, f.Project+"-")
		}
		keys[issue.Key] = true
	}

	check := func(where, key string) error {
		if key == "" {
			return nil
		}
		if !keys[key] {
			return fmt.Errorf("%s references unknown issue key %q", where, key)
		}
		return nil
	}

	for i, sprint := range f.Sprints {
		for _, key := range sprint.Issues {
			if err := check(fmt.Sprintf("sprints[%d] (%s)", i, sprint.Name), key); err != nil {
				return err
			}
		}
	}
	for _, key := range f.Backlog {
		if err := check("backlog", key); err != nil {
			return err
		}
	}
	for _, issue := range f.Issues {
		if err := check(fmt.Sprintf("issue %s epic", issue.Key), issue.Epic); err != nil {
			return err
		}
		if err := check(fmt.Sprintf("issue %s parent", issue.Key), issue.Parent); err != nil {
			return err
		}
		for _, sub := range issue.Subtasks {
			if err := check(fmt.Sprintf("issue %s subtasks", issue.Key), sub); err != nil {
				return err
			}
		}
		for _, link := range issue.Links {
			if err := check(fmt.Sprintf("issue %s links", issue.Key), link.Key); err != nil {
				return err
			}
		}
	}
	return nil
}
