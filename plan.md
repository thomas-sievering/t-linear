# t-linear — Plan

CLI tool for Linear.app. Designed for use by coding agents in [Symphony](https://github.com/openai/symphony) and for human use from the terminal.

## Design Decisions

- **Go, stdlib only** — matches porkctl, grabmd, torbjorn, claudeup
- **No Cobra** — manual subcommand dispatch with `flag` package
- **JSON-only output** — all output is JSON (agents are the primary consumer)
- **Auth via env** — reads `LINEAR_API_KEY` from environment, no auth command
- **Single file to start** — `main.go` monolith, split later if needed

## Auth

Read `LINEAR_API_KEY` from env. If empty, print JSON error and exit 1. No config files, no login flow.

## GraphQL Client

Minimal stdlib HTTP client:
- POST to `https://api.linear.app/graphql`
- Header: `Authorization: <api_key>`
- Body: `{"query": "...", "variables": {...}}`
- Parse response, check for `errors` array
- Return `data` portion

## Commands

### Read Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `me` | Current authenticated user | — |
| `teams` | List all teams | — |
| `projects` | List projects | `--team` |
| `issues` | List issues | `--project`, `--state`, `--team`, `--limit` |
| `issue <ID>` | Single issue detail | — |

### Write Commands (Symphony agent use)

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `create` | Create issue | `--team`, `--title`, `--description`, `--project`, `--priority`, `--label` |
| `update <ID>` | Update issue | `--state`, `--priority`, `--assignee`, `--title` |
| `comment <ID> <text>` | Add comment to issue | — |

### Utility

| Command | Description |
|---------|-------------|
| `graphql` | Raw GraphQL query (reads from stdin or `--query` flag) |
| `version` | Print version |

## Issue Field Normalization

Per Symphony spec, issues normalize to:
```
id, identifier, title, description, priority, state, labels,
blocked_by, branch_name, url, created_at, updated_at
```

## Output Format

All output is JSON. Controlled by env vars:
- `T_LINEAR_PRETTY=1` — pretty-print JSON
- `T_LINEAR_ENVELOPE=1` — wrap in `{"ok": true, "data": ...}`

Errors always output: `{"ok": false, "error": {"code": "...", "message": "..."}}`

## File Structure

```
t-linear/
├── main.go          # all logic
├── main_test.go     # tests
├── go.mod
├── plan.md          # this file
├── README.md        # (later)
└── SKILL.md         # (later)
```

## Implementation Order

1. go.mod + main.go scaffold (version, help, subcommand dispatch, JSON helpers)
2. GraphQL client (POST, auth header, error handling)
3. `me` command (simplest — validates auth works)
4. `teams`, `projects` commands
5. `issues` command with filters
6. `issue <ID>` with full field normalization
7. `create`, `update`, `comment` write commands
8. `graphql` escape hatch
9. Test against real API
