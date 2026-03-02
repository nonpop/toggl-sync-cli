# toggl-sync-cli

A CLI tool that syncs time entries from [Toggl Track](https://toggl.com/) to [Jira Tempo](https://www.tempo.io/) worklogs.

## How it works

1. Fetches recent time entries from Toggl (default: last 7 days)
2. Parses Jira issue keys from the entry description (e.g. `PROJ-123 fixed the bug`)
3. Creates worklogs in Tempo for each unsynced entry
4. Tags synced entries in Toggl to prevent duplicates

Sync state is tracked via Toggl tags — no local database needed.

## Installation

```
go install github.com/nonpop/toggl-sync-cli@latest
```

Or build from source:

```
go build -o toggl-sync .
```

## Configuration

Create `~/.config/toggl-sync/config.toml`:

```toml
[toggl]
api_token = "your-toggl-api-token"

[tempo]
api_token = "your-tempo-api-token"

[jira]
account_id = "your-jira-account-id"

[sync]
cutoff_date = "2026-03-01"
```

### Required fields

| Field | Description |
|---|---|
| `toggl.api_token` | Toggl Track API token (Profile > API Token) |
| `tempo.api_token` | Tempo API token (Tempo Settings > API Integration) |
| `jira.account_id` | Your Jira/Atlassian account ID |
| `sync.cutoff_date` | Ignore entries before this date (YYYY-MM-DD) |

### Optional fields

| Field | Default | Description |
|---|---|---|
| `toggl.synced_tag` | `synced` | Tag name applied to synced entries |
| `tempo.base_url` | `https://api.tempo.io/3` | Tempo API base URL |
| `sync.sync_window_days` | `7` | Only fetch entries from the last N days |

## Usage

```
toggl-sync                  # Sync unsynced entries
toggl-sync --dry-run        # Preview what would be synced
toggl-sync --days 30        # Look back 30 days instead of the default
toggl-sync --config /path   # Use a custom config file
```

## Toggl entry format

Put the Jira issue key at the start of your Toggl entry description:

```
PROJ-123 fixed the login bug
ABC-42 code review
DATA-7
```

The issue key is extracted and the remaining text becomes the Tempo worklog description. Entries without a valid issue key are skipped with a warning.

## Example output

```
Fetching Toggl entries from 2026-02-23 to 2026-03-03...
OK: [12345] PROJ-123 "fixed the login bug" (1h30m)
SKIP: [12346] "lunch break" (no Jira issue key)
OK: [12347] ABC-42 "code review" (45m)
SKIP: [12348] "PROJ-99 still running" (still running)

=== Summary ===
Synced:         2
Failed:         0
Skipped:        2
Already synced: 5
```

## Running tests

```
go test -v ./...
```
