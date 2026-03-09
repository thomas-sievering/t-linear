# t-linear

CLI for [Linear.app](https://linear.app) — query and manage issues, projects, teams, and workflow states from the terminal. All output is JSON.

## Install

```bash
go install github.com/thomas-sievering/t-linear@latest
```

Requires Go 1.23+.

## Auth

Set `LINEAR_API_KEY` environment variable. Generate a personal API key at **Linear → Settings → API → Personal API keys**.

## Usage

```
t-linear <command> [flags]
```

| Command | Description |
|---------|-------------|
| `me` | Show current user |
| `teams` | List teams |
| `projects [--team KEY]` | List projects |
| `issues [--team KEY] [--project SLUG] [--state S] [--limit N]` | List issues |
| `issue <ID>` | Show issue detail with relations |
| `create --team KEY --title T [--description D] [--project SLUG] [--priority N] [--label L]` | Create issue |
| `update <ID> [--state S] [--priority N] [--title T] [--assignee EMAIL]` | Update issue |
| `comment <ID> <text>` | Add comment to issue |
| `comments <ID>` | List comments on issue |
| `comment-update <CID> <body>` | Update a comment |
| `states --team KEY` | List workflow states |
| `state-create --team KEY --name N --type T [--color HEX] [--description D] [--position N]` | Create workflow state |
| `graphql [--query Q] [--vars JSON]` | Run raw GraphQL (reads stdin if no `--query`) |
| `version` | Print version |

## Examples

```bash
# List issues in "In Progress" state for team ENG
t-linear issues --team ENG --state "In Progress"

# Create an issue
t-linear create --team ENG --title "Fix login bug" --priority 2

# Move an issue to Done
t-linear update ENG-42 --state Done

# Add a comment
t-linear comment ENG-42 "Deployed to staging"

# List comments on an issue
t-linear comments ENG-42

# Raw GraphQL
echo '{ viewer { id name } }' | t-linear graphql
```

## Environment Variables

| Variable | Effect |
|----------|--------|
| `LINEAR_API_KEY` | API key (required) |
| `T_LINEAR_PRETTY=1` | Pretty-print JSON output |
| `T_LINEAR_ENVELOPE=1` | Wrap output in `{"ok":true,"data":...}` envelope |

## License

[MIT](LICENSE)
