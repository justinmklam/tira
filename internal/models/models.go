package models

type Issue struct {
	Key                string
	Summary            string
	Description        string
	AcceptanceCriteria string
	Status             string
	StatusID           string
	IssueType          string
	Priority           string
	Assignee           string
	AssigneeID         string
	Reporter           string
	StoryPoints        float64
	Labels             []string
	EpicKey            string
	EpicName           string
	SprintName         string
	ParentKey          string
	ParentSummary      string
	LinkedIssues       []LinkedIssue
	Comments           []Comment
	StatusChangedDate  string // ISO date when status last changed, e.g. "2026-03-01"
	ProjectKey         string
}

type LinkedIssue struct {
	Relationship string // e.g. "blocks", "is blocked by", "relates to"
	Key          string
	Summary      string
	Status       string
}

type Comment struct {
	Author  string
	Body    string
	Created string // ISO date, e.g. "2026-03-01"
}

type IssueFields struct {
	Summary            string
	IssueType          string
	Priority           string
	Assignee           string // display name (resolved to AssigneeID before API call)
	AssigneeID         string
	StoryPoints        float64
	Labels             []string
	Description        string
	AcceptanceCriteria string
	ParentKey          string
}

type ValidValues struct {
	IssueTypes []string
	Priorities []string
	Assignees  []Assignee
	Sprints    []Sprint
}

type Assignee struct {
	DisplayName string
	AccountID   string
}

type Sprint struct {
	ID        int
	Name      string
	State     string // active | future | closed
	StartDate string // ISO date, e.g. "2026-03-01"
	EndDate   string // ISO date, e.g. "2026-03-14"
}

type BoardColumn struct {
	Name      string
	StatusIDs []string
}

type SprintGroup struct {
	Sprint Sprint
	Issues []Issue
}

type Status struct {
	ID   string
	Name string
}
