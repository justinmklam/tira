# lazyjira

A lazygit-style CLI for Jira, built in Go with Charm tooling. This project provides a fast, scriptable, and extensible interface to Jira issues, sprints, and boards.

## Features
- View and edit Jira issues from the terminal
- List active sprints and backlog
- Create new issues with a markdown template
- Fast, stateless auth via environment variables
- Designed for extension into a full TUI

## Getting Started

### Prerequisites
- Go 1.21+
- A Jira Cloud account with API access

### Installation
Clone the repository and build the CLI:

```sh
git clone https://github.com/justinmklam/lazyjira.git
cd lazyjira
go build -o lazyjira ./cmd/lazyjira
```

### Configuration
Set the following environment variables (add to your shell profile):

```sh
export JIRA_URL=https://yourorg.atlassian.net
export JIRA_EMAIL=you@example.com
export JIRA_API_TOKEN=your_token_here
```

Optionally, create `~/.config/lazyjira/config.yaml` for non-sensitive defaults:

```yaml
default_project: MYPROJ
default_board_id: 42
```

### Usage

- View an issue:
  ```sh
  ./lazyjira get MP-101
  ```
- Edit an issue in your editor:
  ```sh
  ./lazyjira get MP-101 --edit
  ```
- List active sprint issues:
  ```sh
  ./lazyjira list
  ```
- List all sprints and backlog:
  ```sh
  ./lazyjira list --backlog
  ```
- Create a new issue:
  ```sh
  ./lazyjira create
  ```

### Flags
- `--debug` for verbose logging
- `--no-color` to disable color output

## Contributing
Pull requests welcome! See the plan in `docs/lazyjira-plan.md` for roadmap and structure.

## License
MIT
