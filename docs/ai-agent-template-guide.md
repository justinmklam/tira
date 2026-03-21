# Tira Issue Template Examples for AI Agents

## Quick Reference

### Minimal Valid Template
```
type: Task

---

# Summary goes here
```

### Standard Template
```
<!-- tira: do not remove this line or change field names -->
type: Story
priority: High
assignee: Jane Smith
story_points: 5
labels: backend, api

---

# Issue Summary

## Description

Description in Markdown.

## Acceptance Criteria

- [ ] Criterion 1
- [ ] Criterion 2
```

## Front Matter Fields

| Field | Required | Description | Example |
|-------|----------|-------------|---------|
| `type` | Yes* | Issue type (*or omit to use default) | `Story`, `Bug`, `Task` |
| `priority` | No | Priority level | `High`, `Medium`, `Low` |
| `assignee` | No | Assignee display name | `Jane Smith` |
| `story_points` | No | Numeric estimate | `5` |
| `labels` | No | Comma-separated tags | `backend, api, bug` |
| `parent` | No | Parent issue key | `MP-42` |

## Markdown Body

After the `---` separator:

- **H1 heading** (`#`) = Issue summary (required)
- **H2 headings** (`##`) = Sections like Description, Acceptance Criteria
- **Full Markdown support**: lists, code blocks, tables, blockquotes, etc.

## Generation Pattern for AI Agents

```python
def generate_issue_template(summary, description, acceptance_criteria, issue_type="Task", priority="Medium"):
    template = f"""type: {issue_type}
priority: {priority}

---

# {summary}

## Description

{description}

## Acceptance Criteria

"""
    for criterion in acceptance_criteria:
        template += f"- [ ] {criterion}\n"
    
    return template
```

## Full Example with Rich Formatting

```
<!-- tira: do not remove this line or change field names -->
type: Bug
priority: Critical
assignee: John Doe
labels: backend, critical, auth

---

# Login fails after password reset

## Description

Users report receiving "invalid session token" error after resetting password.

### Steps to Reproduce

1. Go to login page
2. Click "Forgot Password"
3. Reset password
4. Try to login

### Expected Behavior

User should be logged in automatically.

### Actual Behavior

Error: "Invalid session token"

## Acceptance Criteria

- [ ] Password reset completes without errors
- [ ] User is auto-logged in after reset
- [ ] Session token is valid

## Technical Notes

> Check `AuthService.resetPassword()` method

```go
func (s *AuthService) resetPassword(token, newPassword string) error {
    // Implementation here
}
```
```

## Commands

```bash
# Get template documentation
./tira create --template

# Create from file
./tira create --file issue.md

# Create from stdin (AI agent pattern)
generate-issue | ./tira create --no-edit

# Create with heredoc
./tira create --no-edit << 'EOF'
type: Task

---

# My Issue

Description here.
EOF
```
