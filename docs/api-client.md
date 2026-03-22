# API Client

**File:** `internal/api/client.go`

The API client is the interface between tira and the Jira Cloud REST API.

---

## Client Interface

```go
type Client interface {
    GetIssue(key string) (*models.Issue, error)
    UpdateIssue(key string, fields models.IssueFields) error
    CreateIssue(projectKey string, fields models.IssueFields) (*models.Issue, error)
    GetValidValues(projectKey string) (*models.ValidValues, error)
    GetIssueMetadata(projectKey string) (*models.ValidValues, error)
    GetBoardColumns(boardID int) ([]models.BoardColumn, error)
    GetActiveSprint(boardID int) ([]models.Issue, error)
    GetSprintGroups(boardID int) ([]models.SprintGroup, error)
    GetSprintList(boardID int) ([]models.Sprint, error)
    GetSprintGroupsBatch(boardID int, sprints []models.Sprint) ([]models.SprintGroup, error)
    GetBacklogIssues(boardID int) ([]models.Issue, error)
    GetBacklog(projectKey string) ([]models.Sprint, error)  // NOT IMPLEMENTED
    GetEpics(projectKey, query string) ([]models.Issue, error)
    MoveIssuesToSprint(sprintID int, keys []string) error
    MoveIssuesToBacklog(keys []string) error
    RankIssues(keys []string, rankAfterKey, rankBeforeKey string) error
    SetParent(issueKey, parentKey string) error
    SearchAssignees(projectKey, query string) ([]models.Assignee, error)
    SetAssignee(issueKey, accountID string) error
    GetStatuses(issueKey string) ([]models.Status, error)
    TransitionStatus(issueKey, statusID string) error
    AddComment(issueKey, text string) error
}
```

The single concrete implementation is `jiraClient`. All TUI models receive a `Client` interface — never a concrete type — enabling mock substitution in tests.

---

## Implementation: jiraClient

```go
type jiraClient struct {
    client  *jira.Client   // go-jira/v2/cloud client
    baseURL string          // trimmed JiraURL
    http    *http.Client    // shared HTTP client with BasicAuth transport
}
```

**Authentication:** `jira.BasicAuthTransport{Username: email, APIToken: token}` wraps the HTTP client. In debug mode, a `debug.Transport` wraps it further to log requests.

---

## Hybrid API Approach

The client uses two strategies:

### 1. Raw HTTP (`c.http.Get/Do`)

Used for:
- Endpoints that need manual JSON parsing
- ADF (Atlassian Document Format) field access
- Endpoints not natively supported by go-jira

### 2. go-jira `NewRequest`/`Do`

Used for Agile endpoints (`rest/agile/1.0/...`) that go-jira properly handles:
- `MoveIssuesToSprint`
- `MoveIssuesToBacklog`
- `RankIssues`
- `AddComment`

**Example:**
```go
req, err := c.client.NewRequest(ctx, http.MethodPut, "rest/agile/1.0/issue/rank", payload)
if err != nil { return err }
_, err = c.client.Do(req, nil)
return err
```

`NewRequest` handles JSON encoding and base URL resolution; `Do` handles response checking.

---

## Key Methods

### GetIssue — Three Concurrent Goroutines

```go
wg.Add(3)
go func() { result, fetchErr = c.fetchFullIssue(key) }()
go func() { comments = c.fetchComments(key) }()
go func() { statusDate = c.fetchStatusChangeDate(key) }()
wg.Wait()
```

**fetchFullIssue** makes **one** raw HTTP request to `/rest/api/3/issue/{key}?expand=names` and double-unmarshals the `fields` JSON:
1. Into a typed struct for standard fields (summary, status, issuetype, priority, etc.)
2. Into `map[string]json.RawMessage` for custom/ADF fields

The `names` expansion returns a `fieldID → fieldName` map, inverted to `name → fieldID` for lookups.

### GetSprintGroups — Concurrent Sprint + Backlog Fetch

Composed from three lower-level methods:

1. **`GetSprintList(boardID)`** — fetches sprint metadata (ID, name, state, dates) via one API call. No issue data.
2. **`GetSprintGroupsBatch(boardID, sprints)`** — fires one goroutine per sprint to fetch issues concurrently.
3. **`GetBacklogIssues(boardID)`** — fetches backlog issues in a single API call.

`GetSprintGroups` calls all three sequentially (sprints first, then batch + backlog). The TUI uses the decomposed methods directly for progressive loading (see below).

### Progressive Loading

The board TUI minimizes time-to-first-render by splitting the load:

1. **Initial load**: `fetchBoardDataCore` fetches `GetSprintList` + `GetBoardColumns` concurrently, then `GetSprintGroupsBatch` for the first 3 sprints only. The TUI renders immediately with partial data.
2. **Lazy load**: `lazyLoadCmd` fires in the background to fetch remaining sprints via `GetSprintGroupsBatch` and `GetBacklogIssues`. Results arrive as `blLazyLoadDoneMsg` and are appended to the model.
3. **Manual refresh** (`R` key): Uses `fetchAllBoardDataCore` → `GetSprintGroups` to reload everything at once.

**Status change dates** are NOT fetched during board load. They are fetched lazily when an issue is selected for the sidebar (via `GetIssue`, which fetches the full changelog).

### Custom Field Resolution

**Story points** field name varies by Jira instance:
- Tries `"story points"` then `"story point estimate"` in `nameToID`
- Falls back to `customfield_10016` (the most common custom field ID for story points)
- In `fetchAgileIssues`, also tries `story_points` and `customfield_10016` directly

**Acceptance Criteria** field name varies:
- Tries `"acceptance criteria"` then `"acceptance criterion"` in `nameToID`

### UpdateIssue — Field ID Resolution

Before updating, `fetchFieldIDs(key)` makes a request to `?expand=names` on the specific issue to get accurate field IDs for that issue type. This is required because custom field IDs differ between projects.

### CreateIssue — Field ID Resolution

`fetchAllFieldIDs()` queries `/rest/api/3/field` to get all field IDs for the Jira instance.

---

## ADF (Atlassian Document Format)

**File:** `internal/api/adf.go`

Jira stores rich text in ADF (a JSON tree format). `ADFToMarkdown(node map[string]any) string` is a recursive tree walker that converts ADF nodes to Markdown.

### Supported Node Types

| Node Type | Markdown Output |
|-----------|-----------------|
| `doc` | Root container |
| `paragraph` | `\n\n` after content |
| `heading` | `#` prefix based on `attrs.level` |
| `text` | Literal text with marks applied |
| `hardBreak` | `\n` |
| `rule` | `\n---\n\n` |
| `blockquote` | `> ` prefix on each line |
| `bulletList` | `- ` prefix, nested with indent |
| `orderedList` | `N. ` prefix, nested with indent |
| `codeBlock` | ` ```lang ... ``` ` |
| `inlineCard`, `blockCard` | `<url>` format |
| `mention` | `@name` or `@id` |

### Text Marks

| Mark | Markdown |
|------|----------|
| `strong` | `**bold**` |
| `em` | `*italic*` |
| `code` | `` `inline code` `` |
| `link` | `[text](url)` |
| `strike` | `~~strikethrough~~` |
| `underline` | (no markdown equivalent, left as-is) |

Unknown node types are silently ignored.

### markdownToADF

A minimal converter that wraps text in a single ADF paragraph block. Does **NOT** parse Markdown into ADF nodes — it sends the raw markdown text as a single plain text node. This is a known limitation.

---

## Status Transitions

### GetStatuses

Fetches available transitions via `/rest/api/3/issue/{key}/transitions?expand=transitions`.

```go
type Status struct {
    ID   string  // transition ID (not status ID)
    Name string
}
```

**Important:** `Status.ID` is the **transition ID** (from `/transitions` endpoint), not the Jira status ID. `Issue.StatusID` is the actual Jira status ID used for kanban column mapping.

### TransitionStatus

POSTs to `/rest/api/3/issue/{key}/transitions` with the transition ID.

---

## Move and Rank Operations

These use `c.client.NewRequest` + `c.client.Do` (go-jira's Agile support):

| Method | Endpoint |
|--------|----------|
| `MoveIssuesToSprint` | `rest/agile/1.0/sprint/{id}/issue` (POST) |
| `MoveIssuesToBacklog` | `rest/agile/1.0/backlog/issue` (POST) |
| `RankIssues` | `rest/agile/1.0/issue/rank` (PUT) |

---

## Concurrency Patterns

### Pattern 1: sync.WaitGroup for Parallel Fetches

`GetIssue` fires 3 goroutines concurrently (issue data, comments, status date). `GetSprintGroupsBatch` fires N goroutines (one per sprint).

### Pattern 2: Progressive Loading

`fetchBoardDataCore` fetches the first 3 sprints for fast initial render, then `lazyLoadCmd` fetches remaining sprints + backlog in the background. See "Progressive Loading" section above.

### Pattern 3: Error Aggregation

Errors from parallel fetches are collected and returned together where possible.

---

## Known Limitations

### 1. GetBacklog Not Implemented

`func (c *jiraClient) GetBacklog(projectKey string) ([]models.Sprint, error)` always returns `fmt.Errorf("not implemented")`. The actual backlog data comes from `GetBacklogIssues(boardID)` (used during progressive loading) or `GetSprintGroups` (which includes a "Backlog" sprint group).

### 2. markdownToADF is a Stub

`markdownToADF(text)` wraps the entire text in a single ADF paragraph plain-text node. It does NOT parse Markdown. This means that when updating Description or Acceptance Criteria, bold/italic/lists in the user's Markdown are sent as literal text to Jira and will appear unformatted.

### 3. Comments Limited to 50

`fetchComments` requests `/comment?maxResults=50&orderBy=-created`. This limits comments to the 50 most recent, newest first.

### 4. Status Change Date from Changelog

`fetchStatusChangeDate` fetches up to 100 changelog entries and returns the date of the **first** entry in the response that changed `field == "status"`. Since Jira returns changelog entries newest-first, this is the most recent status change.

---

## See Also

- [Configuration](configuration.md) — How credentials are loaded
- [Internal Packages](internal-packages.md) — Data models and display rendering
- [Glossary](glossary.md) — ADF and API terminology
