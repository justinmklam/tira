---
description: Create, read, or update Jira tickets using the tira CLI. Use when the user wants to create a Jira issue/ticket/story/task/bug, look up a ticket by key (e.g. PROJ-123), update/edit an existing ticket, or mentions "tira".
name: tira
---
# Tira — Jira Ticket Management

Use the `tira` CLI to create and read Jira tickets. The binary is at `~/go/bin/tira` (also on `$PATH`).

## Reading a ticket

```bash
tira get <KEY>          # e.g. tira get PROJ-123
```

Output is plain text — ticket summary, description, status, assignee, and comments.

## Updating a ticket

Use `tira update <KEY> --no-edit` with a heredoc piped to stdin. **Never open an interactive editor.**
Only non-empty fields/sections are applied — Jira fields you omit are left unchanged.

**Always capture the current template first**, then edit only what changed, then pipe it back:

```bash
# 1. Capture current state (does not modify the ticket)
tira update PROJ-123 --template > /tmp/proj-123.md

# 2. Edit /tmp/proj-123.md — change only the fields/sections that need updating

# 3. Pipe the edited template back (never opens an editor)
cat /tmp/proj-123.md | tira update PROJ-123 --no-edit
```

Or, if you already know the full current field values (e.g. from a prior `tira get`), you can
skip the temp file and pipe a heredoc directly — as long as you reproduce the sentinel line and
`# Summary` heading:

```bash
cat <<'EOF' | tira update PROJ-123 --no-edit
<!-- tira: do not remove this line or change field names -->
type: Task

---

# PROJ-123: Updated summary

## Description

Updated description text.

## Acceptance Criteria

- Criterion 1
- Criterion 2
EOF
```

tira prints `✓ <KEY> updated.` on success along with a field-level diff.

## Creating a ticket

Use `tira create --no-edit` with a heredoc piped to stdin. **Never open an interactive editor.**

### Template format

```
<!-- tira: do not remove this line or change field names -->
type: <IssueType>
assignee: <DisplayName>
story_points: <Number>

---

# <Summary — one concise line in sentence case>

## Description

Prose description. Supports full Markdown: headings, lists, code blocks, tables, links.

## Acceptance Criteria

- Criterion 1
- Criterion 2
```

### Rules

- The sentinel comment `<!-- tira: do not remove this line or change field names -->` **must** be the first line.
- The `---` separator must appear on its own line.
- The H1 `# Summary` is required and becomes the ticket title. **Use sentence case** (capitalise only the first word and proper nouns).
- Do **not** set `priority` or `labels` — omit these fields entirely.
- Omit other optional fields rather than leaving them blank.
- Always pipe via `--no-edit`; never open an interactive editor.
- Jira does **not** support `- [ ]` checkbox syntax in acceptance criteria — use plain bullet points.

### Valid types

Depends on your Jira project configuration. Common values: `Story`, `Task`, `Bug`, `Sub-task`, `Epic`.

Run `tira create --template` to see valid types and fields for your configured project.

### Shell invocation patterns

**Heredoc piped to stdin (preferred for agents):**

```bash
cat <<'EOF' | tira create --no-edit
<!-- tira: do not remove this line or change field names -->
type: Task

---

# Short summary of the work

## Description

What needs to be done and why.

## Acceptance Criteria

- Thing is done
EOF
```

**Heredoc redirected directly (equivalent):**

```bash
tira create --no-edit <<'EOF'
<!-- tira: do not remove this line or change field names -->
type: Story
assignee: Jane Smith
story_points: 3

---

# Implement OAuth2 login

## Description

Add OAuth2 login flow with Google provider.

## Acceptance Criteria

- User can sign in with Google
- Session is persisted correctly
- Logout clears the session
EOF
```

**From a file:**

```bash
tira create --file issue-template.md
```

**Get the full template spec (useful for agents building templates programmatically):**

```bash
tira create --template
```

tira prints `✓ Created <KEY>.` on success — echo that key back to the user with a link:
`https://<your-domain>.atlassian.net/browse/<KEY>`

## Including code references

Use standard Markdown link syntax `[text](url)` — tira converts this to ADF for Jira automatically.

Example:
```markdown
[src/auth/login.go](https://github.com/your-org/your-repo/blob/main/src/auth/login.go)
```

## Configuration

Config lives at `~/.config/tira/config.yaml`. Run `tira --help` or see [docs/configuration.md](../../docs/configuration.md) for setup details.

Use `--profile <name>` to select a non-default config profile.

## Workflow

### Creating a ticket

1. **Understand the task** — read relevant files, identify affected code, collect GitHub links.
2. **Draft the ticket** — write a clear summary, full description with code references, and acceptance criteria.
3. **Show the draft** — display the full ticket contents to the user in a code block and ask for confirmation before creating it. Use the `ask_user` tool with choices `["Yes, create it", "No, cancel"]`.
4. **Create via stdin** — only if the user confirms, pipe the template to `tira create --no-edit`.
5. **Report back** — share the created ticket key and its Jira URL with the user.

### Updating a ticket

1. **Capture current state** — run `tira update <KEY> --template` and read the output; never guess at existing field values.
2. **Understand the task** — read relevant files, identify affected code, collect GitHub links.
3. **Draft the change** — modify only the fields/sections that need updating; leave everything else exactly as captured.
4. **Show the diff** — display what will change (new description, new acceptance criteria, etc.) to the user in a code block and ask for confirmation before updating. Use the `ask_user` tool with choices `["Yes, update it", "No, cancel"]`.
5. **Update via stdin** — only if the user confirms, pipe the edited template to `tira update <KEY> --no-edit`.
6. **Report back** — share the updated ticket key and its Jira URL with the user.
