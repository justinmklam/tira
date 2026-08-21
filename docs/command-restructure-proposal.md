# Command Structure Proposal (v2)

**Status:** Approved — phased deprecate-then-remove rollout. See §9 for resolved decisions.
**Motivation:** A recent fix added `tira update` to close an agent-usability gap (see
`feat: allow agents to update tickets`). While auditing the full command surface to land that
change, several structural inconsistencies surfaced that make the CLI harder to learn for humans
and harder to pattern-match for agents. This document catalogs them and, per the decisions in §9,
proposes a phased rollout: renamed/consolidated commands ship immediately with the new spelling as
primary and the old spelling kept as a working, cobra-`Deprecated` alias (prints a warning, still
functions) until a later removal release. `--version` and the `debug.log` relocation are additive
and ship without any deprecation period since there's nothing to alias.

---

## 1. Current command inventory

| Command | Purpose | Key flags |
|---|---|---|
| `tira get <key\|url>` | Read an issue (Markdown; pipe-safe) | `--edit` (opens `$EDITOR`) |
| `tira update <key\|url>` | Update an issue | `--template`, `--no-edit`, `--file` |
| `tira create` | Create an issue | `--project`, `--type`, `--parent`, `--file`, `--no-edit`, `--template` |
| `tira board` | Unified TUI (backlog + kanban, Tab to toggle), starts on **backlog** | `--project` |
| `tira backlog` | Unified TUI, starts on **backlog** | `--project` |
| `tira kanban` | Unified TUI, starts on **kanban** | `--project` |
| `tira completion` | Shell completion script (cobra built-in) | — |
| `tira help` | Help (cobra built-in) | — |

---

## 2. Problems identified

### 2.1 `board` and `backlog` are literally the same command

```go
// cmd/tira/board.go
var boardCmd = &cobra.Command{
    Use: "board",
    RunE: func(cmd *cobra.Command, args []string) error {
        return runBoardCmd(app.ViewBacklog)
    },
}

var backlogCmd = &cobra.Command{
    Use: "backlog",
    RunE: func(cmd *cobra.Command, args []string) error {
        return runBoardCmd(app.ViewBacklog)   // <- identical to boardCmd
    },
}
```

Both launch the exact same unified TUI, on the exact same starting view. Only `kanban` differs
(it starts on the kanban tab instead). A new user has no way to guess this from `--help` output —
three top-level commands appear to be three different views, but there are really only **one
TUI with two possible starting tabs**. This is dead duplication, not intentional API surface.

### 2.2 Two spellings for "interactively edit an issue"

`tira get <KEY> --edit` and `tira update <KEY>` (no flags, TTY stdin) both run the exact same
`runEditLoop`. Keeping both live means:
- Docs/help text have to explain the relationship every time ("for agents use `update` instead").
- Agents scanning `--help` output see edit-capable flags on *two* commands and have to guess
  which is canonical.

This was called out when `update` was added; `get --edit` was kept for backward compatibility
but the overlap itself is the source of ambiguity, not just the docs.

### 2.3 `--template` means two different things depending on the command

- `tira create --template` → prints the **generic format specification** (docs), works without
  a configured Jira project, skips config loading entirely.
- `tira update <KEY> --template` → prints the **current issue's actual field values** rendered
  into the same template shape, requires a live API call.

Same flag name, same shape of output (a template), but one is static documentation and the other
is a live read. An agent that has learned `create --template` from `tira create --help` may
reasonably assume `update --template` does the same "just show me the format" thing rather than
making a network call — or vice versa, assume `create --template` needs a valid issue key.

### 2.4 Flat namespace mixes "one-shot data operations" with "TUI launchers"

`get`, `update`, `create` are single-shot, scriptable, agent-friendly commands. `board`,
`backlog`, `kanban` are long-running interactive TUI programs. Both groups sit at the same level
in `tira --help`, with no visual or naming signal separating "things a script/agent can safely
call" from "things that take over the terminal." This isn't fatal, but it's the kind of thing
that causes an agent to try `tira board` non-interactively and hang.

### 2.5 Minor: per-command flag re-registration

```go
for _, cmd := range []*cobra.Command{boardCmd, backlogCmd, kanbanCmd} {
    cmd.Flags().StringVar(&boardProject, "project", "", "override the default project from config")
}
```

All three TUI commands share one package-level `boardProject` variable and re-declare the same
flag. Harmless today, but it's a code smell that will bite if the three commands' behavior is
ever meant to diverge (it also reinforces that these three commands are really one).

### 2.6 `RunWithSpinner` requires a real TTY, even for pipe-safe commands (root cause of the original agent report) — **FIXED**

`get`, `update`, and `board` all wrap their initial API fetch in `tui.RunWithSpinner`, which starts
a `tea.Program`. Bubbletea needs a real terminal on stdin to read raw input, even though the
spinner only *writes* to stderr — so in any sandbox/CI/agent shell with no TTY at all (not just
piped output), `RunWithSpinner` failed immediately with `bubbletea: error opening TTY: ... open
/dev/tty: device not configured`, regardless of `--edit`/`--no-edit`/`--show`/piping/redirecting.
This is exactly what the original agent log hit: `tira get HIVE-3774 | cat` and even `tira get
HIVE-3774 > /tmp/hive3774.md` both failed the same way, sending the agent down a rabbit hole of
`TERM=dumb`, `script`, custom `$EDITOR` shims, and `/dev/tty` isolation — none of which were the
actual problem, and none of which would have helped, since the command hadn't even reached the
template/editor logic yet.

**Fix:** `internal/tui.RunWithSpinner` now checks `term.IsTerminal` on stdin and stderr first
(`isInteractive()`); if either isn't a TTY, it calls the wrapped function synchronously with no
spinner UI at all, instead of invoking bubbletea. This makes `get`/`update`'s data-fetching step
behave identically whether run interactively or piped/redirected/in an agent sandbox — no flags,
env vars, or workarounds required. `board`/`backlog`/`kanban` still require a real TTY overall
(they hand off to a full-screen TUI after the fetch), but the initial fetch no longer needlessly
fails first.



1. **One job, one command.** No two ways to do the same thing without a clear "this one is the
   deprecated alias" signal.
2. **Consistent flag semantics across sibling commands** (`create`/`update`) so agents can
   pattern-match: the same flag name should always mean the same kind of thing.
3. **Separate "scriptable" commands from "interactive TUI" commands** at the help-text level,
   without necessarily changing the top-level namespace (a full `tira issues <verb>` /
   `tira board <verb>` regrouping was evaluated separately and declined as too disruptive for
   the benefit — see §6).
4. **No breaking changes in a single release.** Every removed/renamed spelling gets a deprecation
   period via cobra's `Deprecated` field (prints a warning, still works) before removal.

---

## 4. Proposed changes

### 4.1 Collapse `board` / `backlog` / `kanban` into one command + `--view` flag

```bash
tira board                  # unified TUI, starts on backlog (default)
tira board --view kanban    # unified TUI, starts on kanban
tira board --view backlog   # explicit, same as default
```

- Keep `backlog` and `kanban` as **deprecated aliases**:
  ```go
  backlogCmd.Deprecated = "use 'tira board' (or 'tira board --view backlog') instead"
  kanbanCmd.Deprecated = "use 'tira board --view kanban' instead"
  ```
  Cobra prints the deprecation notice on stderr and the command still runs — zero breakage for
  existing muscle memory, shell aliases, or scripts.
- Delete the duplicate `boardCmd`/`backlogCmd` RunE bodies in favor of a single implementation
  parameterized by `--view`.
- **Files:** `cmd/tira/board.go` only.

### 4.2 Deprecate `get --edit` in favor of `update`

```go
getCmd.Flags().BoolVar(&editFlag, "edit", false, "...")
_ = getCmd.Flags().MarkDeprecated("edit", "use 'tira update <key>' instead")
```

`get` becomes purely a read command; `update` is the sole entry point for interactive *and*
non-interactive edits. Prints a deprecation warning but keeps working for one release, then the
flag and its code path (`runEditLoop` invocation from `get.go`) are removed.

- **Files:** `cmd/tira/get.go` (remove the `--edit` branch and its now-unused imports once the
  deprecation window ends).

### 4.3 Disambiguate `--template` on `update`

Rename `update`'s flag from `--template` to `--show` (prints current field values, read-only,
no mutation) and keep `create --template` as-is (it's the more established, doc-only meaning).
Keep `--template` as a hidden deprecated alias on `update` pointing at `--show` for one release.

```bash
tira update HIVE-3774 --show > /tmp/hive-3774.md   # was: --template
```

- **Files:** `cmd/tira/update.go`, `docs/cli-commands.md`, `skills/tira.md`, `README.md`.

### 4.4 Group help output by audience

No code restructuring needed — just reorder/label the root `Long` help text and add short
descriptions that make the split explicit:

```
Scriptable (safe for agents/CI, single command + exit):
  get, create, update

Interactive (opens a full-screen TUI):
  board
```

- **Files:** `cmd/tira/root.go` only (text change).

---

## 5. Migration plan — **DECIDED: phased deprecate-then-remove** (see §9)

Cobra has first-class deprecation support (`Command.Deprecated`, `Flags().MarkDeprecated()`) that
prints a warning to stderr while the old spelling keeps working — the LOE to keep old spellings
alive is ~30–45 minutes total across all three renames, since it reuses the exact same
implementation rather than a parallel code path. Given that, all three renames ship the new
spelling as primary immediately, with the old spelling kept as a working deprecated alias:

| Change | Old spelling | Deprecation mechanism |
|---|---|---|
| `board`/`backlog`/`kanban` → `board [--view backlog\|kanban] [--board-id N]` | `backlog`, `kanban` | `backlogCmd.Deprecated`/`kanbanCmd.Deprecated` set to a message pointing at `board --view=...`; both remain thin wrappers calling the same consolidated impl |
| `get --edit` → `update` | `get --edit` | `getCmd.Flags().MarkDeprecated("edit", "use 'tira update <key>' instead")`; `runEditLoop` code path untouched |
| `update --template` → `update --show` | `update --template` | New `--show` flag bound to the same variable as the existing `--template` flag; `MarkDeprecated("template", "use --show instead")` |

`--version`/`tira version` and the `debug.log` relocation are additive/new-default changes with
no old spelling to alias, so they ship without a deprecation period.

**Removal** of the deprecated spellings is deferred to a later release once usage/CHANGELOG
history shows it's safe; no removal date is fixed by this document.

---

## 6. Considered and declined: full resource-grouped namespace

A `tira issues new|edit|get` (or similar) regrouping was evaluated in a prior discussion and
declined: the actual pain point causing agent confusion was the *missing* `update` command and
`get --edit`'s hard dependency on an interactive editor, not the flatness of the namespace itself.
Given `get`/`create`/`update` are already unambiguous verbs once §4.2–4.3 land, a full rename
would add breaking-change churn (every doc, script, and skill needs `issues` inserted) without a
corresponding clarity gain. Not recommended unless a fourth+ resource type (e.g. sprints, epics)
is added and the flat namespace actually becomes crowded.

---

## 7. Summary of files touched

| File | Change |
|---|---|
| `cmd/tira/board.go` | Consolidate `board`/`backlog`/`kanban` into one impl + `--view` and `--board-id` flags; keep `backlog`/`kanban` as deprecated wrapper commands |
| `cmd/tira/get.go` | Mark `--edit` deprecated (keep working) |
| `cmd/tira/update.go` | Add `--show`, mark `--template` deprecated (keep working, same underlying variable) |
| `cmd/tira/root.go` | Reorganize `Long` help text by audience; wire `rootCmd.Version` |
| `cmd/tira/main.go` | Add `var version = "dev"`; add `tira version` command |
| `internal/debug/logger.go` | Move default log path to `$XDG_STATE_HOME/tira/debug.log`; add `--debug-file` override |
| `internal/tui/spinner.go` | `RunWithSpinner` skips bubbletea/spinner UI and calls fn synchronously when stdin/stderr isn't a TTY (see §2.6) — fixes the root-cause "error opening TTY" failures agents hit |
| `cmd/tira/update_test.go`, `cmd/tira/board_test.go` (new) | Cover `--view`/`--board-id` parsing and deprecated-flag alias binding |
| `internal/tui/spinner_test.go` (new) | Cover `RunWithSpinner`'s non-interactive fallback path |
| `docs/cli-commands.md`, `README.md`, `skills/tira.md` | Reflect new flags/commands; note deprecated spellings |

**Out of scope for this release** (tracked as separate follow-up proposals per §9 decision):
inline per-field flags on `create`/`update` (§8.3), JSON output (§8.4).


---

## 8. Additional findings for a v2 (major, breaking-change-tolerant) release

Since a v2 bump tolerates breaking changes, the phased/deprecation approach in §5 can likely be
skipped for a single coordinated release. While auditing the command surface, a few issues
outside the command-naming scope surfaced that are natural to bundle into the same major version
since they're also user-facing/behavioral changes:

### 8.1 `--version` / `tira version` doesn't exist — and the release pipeline's version injection is silently dead

`.goreleaser.yml` sets:
```yaml
ldflags:
  - -s -w -X main.version={{.Version}}
```
but there is no `var version string` declared anywhere in `package main` — `main.go` is 3 lines
(`func main() { Execute() }`). The `-X` flag has nothing to bind to, so it's silently a no-op
(`go build` doesn't even warn). **There is currently no way to ask an installed `tira` binary
what version it is.** For a CLI distributed via GoReleaser/Homebrew-style installs, this is a real
gap for bug reports and for agents/scripts checking capability before using a feature flag like
`--show` (§4.3) that may not exist in older installs.

**Proposed fix:** add `var version = "dev"` in `main.go`, wire it to `-X main.version=...` (already
declared in `.goreleaser.yml`, just needs the symbol to exist), and expose it via cobra's built-in
`rootCmd.Version = version` (gives `-v`/`--version` for free) plus `tira version` for scripting
(`tira version --format json` if §8.4 lands).

### 8.2 `debug.log` is written to the current working directory

Already flagged as a known gotcha (`debug.Init()` calls `os.Create("debug.log")` — see
`internal/debug/logger.go:26`). This litters whatever directory the user happens to be in when
they pass `--debug`, and there's no override flag. A v2 bump is a reasonable place to change the
default to a proper location (`$XDG_STATE_HOME/tira/debug.log`, falling back to
`~/.local/state/tira/debug.log` or `~/.cache/tira/debug.log`) and add a `--debug-file <path>`
override for anyone who relied on the old CWD behavior.

### 8.3 Inline flags are inconsistent across `create` (partial) and `update` (none)

`create` supports `--project`, `--type`, `--parent` as flags but requires the full template (via
`--file`/`--no-edit`/`$EDITOR`) for `assignee`, `priority`, `story_points`, `labels`,
`description`, and `acceptance_criteria`. `update` has **no field flags at all** — every update,
even a one-field change like reassigning an issue, requires the full template round-trip
(`--show` → edit → `--no-edit`). For agents this is workable but verbose for trivial edits; for
humans scripting a quick reassignment it's the same friction. A v2 candidate:

```bash
tira update HIVE-42 --priority High
tira update HIVE-42 --assignee "Jane Smith" --points 3
tira create --project HIVE --type Bug --summary "Title" --assignee "Jane Smith"
```

This is a meaningfully larger scope item (new flags on two commands, flag/template precedence
rules when both are supplied) — recommend scoping as its own follow-up proposal rather than
folding into the command-renaming work in §4, but worth deciding now since it affects whether
`--show`/`--no-edit` remain the *only* non-interactive path or become the *fallback* for
multi-field/prose changes only.

### 8.4 No machine-readable output

`get`, `create`, and `update` all produce human-oriented Markdown/text. There's no `--format json`
anywhere, so an agent or script that wants the created/updated issue's key, URL, or fields
structured has to scrape stdout/stderr text (`✓ Created HIVE-123.`, `✓ HIVE-123 updated.`,
rendered Markdown). This is a substantial feature addition (JSON schemas for issue/field-diff
output on all three commands), not a rename — flagging it here because it's the kind of thing
that's much cheaper to design consistently once, at the same time as touching all three commands'
CLI surface for §4, rather than retrofitted per-command later.

### 8.5 `board`/`backlog`/`kanban` can't override `board_id`, only `project`

```go
if cfg.BoardID == 0 {
    return fmt.Errorf("board ID not configured: set default_board_id in ~/.config/tira/config.yaml")
}
```
There's a `--project` override flag but no equivalent `--board-id`/`--board` override — anyone
wanting to view a different board than their configured default must switch `--profile` or edit
config. Minor, but inconsistent with the `--project` override that already exists on the same
commands. Cheap to add alongside §4.1's `--view` flag work since it's the same file.

### 8.6 Test coverage gap for the new/changed commands

`cmd/tira/get_test.go` only covers `extractIssueKey`. There is no test file for `update.go` (added
without its own tests) and none of the proposed §4 changes (deprecated aliases, `--view`,
`--show` rename) currently have a place they'd be tested. Recommend adding `cmd/tira/update_test.go`
and `cmd/tira/board_test.go` covering flag parsing/deprecation wiring as part of whichever phase
lands them (this repo has no existing pattern for testing full `RunE` execution against a fake
Jira server — §8.4/T1 in `docs/go-idioms-review.md` notes the same gap at the `internal/api`
layer, so a shared `httptest`-based fixture would serve both).

---

## 9. Decisions (resolved 2026-07-21, revised 2026-07-22)

1. **Phased deprecate-then-remove, not a hard break** (revised 2026-07-22). Cobra's
   `Deprecated`/`MarkDeprecated` make keeping old spellings alive nearly free (~30–45 min total),
   so `backlog`/`kanban`, `get --edit`, and `update --template` all keep working (with a stderr
   warning) alongside their new spellings. Removal is deferred to a later release.
2. **Bundle §8.1 (`--version`) and §8.2 (`debug.log` relocation) into this release.** Both are
   small, low-risk, and additive/behavioral (no old spelling to alias), so they ship immediately.
3. **§8.3 (inline per-field flags) and §8.4 (JSON output) are out of scope.** Each is tracked as
   its own follow-up design proposal rather than folded in now.
4. **`--board-id` flag (§8.5): included**, added alongside `--view` in the same `board.go` change.
5. **Version wiring (§8.1): both.** `rootCmd.Version` (free `-v`/`--version` via cobra) *and* a
   `tira version` subcommand (for future scripting/`--format json` without overloading a flag).
6. **`debug.log` relocation (§8.2): `$XDG_STATE_HOME/tira/debug.log`**, falling back to
   `~/.local/state/tira/debug.log` when `$XDG_STATE_HOME` is unset, plus a `--debug-file <path>`
   override flag.
