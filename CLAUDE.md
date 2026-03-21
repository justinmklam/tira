## Build & Quality

- `make build` — compile the binary
- `make test` — run all tests
- `make test-race` — run tests with race detector
- `make fmt` — format code in-place
- `make fmt-check` — check formatting without modifying files
- `make vet` — run go vet
- `make lint` — run golangci-lint (requires [golangci-lint](https://golangci-lint.run/welcome/install/))
- `make check` — run all checks (fmt, vet, lint, test) — mirrors CI
- Always run `make check` before pushing

## Architecture

- See `docs/architecture.md` for the full architecture overview
- The `internal/tui` package has NO dependencies on other internal packages — keep it that way
- TUI models live in `internal/app/`, `cmd/tira/` is a thin CLI layer (Cobra commands + config)
- `internal/app` contains all Bubbletea models — keep Cobra command wiring in `cmd/tira/`
- `api.Client` is an interface — all API access goes through it for testability

## API client conventions

- For Jira API endpoints not natively supported by the go-jira library, use `c.client.NewRequest` and `c.client.Do` instead of raw `http.Client` requests. Example:
  ```go
  req, err := c.client.NewRequest(ctx, http.MethodPut, "rest/agile/1.0/issue/rank", payload)
  if err != nil { return err }
  _, err = c.client.Do(req, nil)
  return err
  ```
  `NewRequest` handles JSON encoding and base URL resolution; `Do` handles response checking.

## Code conventions

- Use `internal/tui` color constants (e.g. `tui.ColorBlue`) instead of raw string literals like `"12"`
- Use `tui.RunWithSpinner[T]` for any blocking operation that needs a loading indicator — do not create one-off spinner models
- Use `tui.FixedWidth`, `tui.Clamp`, `tui.SplitPanes` and other helpers from `internal/tui/helpers.go` instead of reimplementing
- Backlog is split: `backlog.go` (model + update dispatch), `backlog_update.go` (sub-handlers), and `backlog_view.go` (rendering) — all in `internal/app/`
- The `board` command runs a unified TUI that wraps both backlog and kanban views — Tab toggles between them
- Editor/validator packages are pure string/struct logic with no I/O — keep them testable
- Any new keybindings should be updated in [this doc](./docs/keybindings-backlog.md) and optionally [the help window](./internal/tui/help.go) and/or the bottom persistent help
