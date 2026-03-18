package models

type Issue struct {
	Key                string
	Summary            string
	Description        string
	AcceptanceCriteria string
	Status             string
	IssueType          string
	Priority           string
	Assignee           string
	AssigneeID         string
	Reporter           string
	StoryPoints        float64
	Labels             []string
	SprintName         string
	ParentKey          string
	ParentSummary      string
	LinkedIssues       []LinkedIssue
}

type LinkedIssue struct {
	Relationship string // e.g. "blocks", "is blocked by", "relates to"
	Key          string
	Summary      string
	Status       string
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
	ID    int
	Name  string
	State string // active | future | closed
}
