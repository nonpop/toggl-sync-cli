# toggl-sync-cli Design

## Purpose

A Go CLI tool that syncs time entries from Toggl Track to Jira Tempo worklogs. Entries are processed incrementally: only unsynced entries are sent to Tempo, and synced entries are tagged in Toggl to prevent duplicates.

## CLI Interface

```
toggl-sync                  # Sync unsynced entries (default: last 7 days)
toggl-sync --dry-run        # Show what would be synced without doing it
toggl-sync --days 60        # Override sync window to 60 days
toggl-sync --config /path   # Use custom config file path
```

## Configuration

File location: `~/.config/toggl-sync/config.toml`

```toml
[toggl]
api_token = "your-toggl-api-token"
synced_tag = "synced"

[tempo]
api_token = "your-tempo-api-token"
base_url = "https://api.tempo.io/4"

[jira]
account_id = "your-jira-account-id"

[sync]
cutoff_date = "2026-03-01"
sync_window_days = 7
```

### Config fields

- `toggl.api_token` (required): Toggl Track API token
- `toggl.synced_tag` (optional, default "synced"): Tag name applied to synced entries
- `tempo.api_token` (required): Tempo API token
- `tempo.base_url` (optional, default "https://api.tempo.io/4"): Tempo API base URL
- `jira.account_id` (required): Jira account ID of the user (for Tempo worklog authoring)
- `sync.cutoff_date` (required): Absolute earliest date to consider for syncing
- `sync.sync_window_days` (optional, default 7): Only fetch entries from the last N days (overridable via `--days` flag)

## Issue Key Mapping

Jira issue keys are parsed from the start of Toggl entry descriptions using the pattern `^([A-Z][A-Z0-9]+-\d+)`. The remaining text becomes the Tempo worklog description.

Examples:
- `PROJ-123 fixed the bug` -> issue `PROJ-123`, description `fixed the bug`
- `ABC-1 review` -> issue `ABC-1`, description `review`
- `no issue key here` -> skipped with warning

## Sync Pipeline

1. **Load config** -- Read and validate TOML config file
2. **Fetch Toggl entries** -- GET from Toggl API with date range `max(cutoff_date, now - sync_window_days)` to now
3. **Filter entries** -- Exclude entries that have the "synced" tag or are currently running
4. **Parse entries** -- Extract Jira issue key from description prefix; skip entries without a valid key (warn)
5. **Create Tempo worklogs** -- POST each valid entry to Tempo API
6. **Tag in Toggl** -- Add "synced" tag to each successfully synced entry
7. **Report** -- Print each entry as it is processed (success or failure), then print summary at end

### Error handling

- Entries without a valid Jira issue key: skip with warning, continue
- Failed Tempo API call for an entry: log error, skip it, continue with remaining entries
- The skipped entry stays untagged in Toggl, so it will be retried on next run
- Config validation errors: fail immediately with a clear message

### Dry-run mode

Runs steps 1-4 (load, fetch, filter, parse) and prints what would be synced. Skips Tempo creation and Toggl tagging.

## API Integration

### Toggl API (v9)

- Auth: HTTP Basic with API token as username, "api_token" as password
- Fetch entries: `GET /me/time_entries?start_date=...&end_date=...`
- Add tag: `PUT /workspaces/{workspace_id}/time_entries/{entry_id}` with updated tags array

### Tempo API (v4)

- Auth: Bearer token in Authorization header
- Create worklog: `POST /worklogs`
  ```json
  {
    "issueKey": "PROJ-123",
    "timeSpentSeconds": 3600,
    "startDate": "2026-03-02",
    "startTime": "09:00:00",
    "description": "fixed the bug",
    "authorAccountId": "abc123"
  }
  ```

## Sync State

State is tracked via Toggl tags (stateless approach -- no local database). An entry is considered synced if it has the configured "synced" tag. This makes the tool portable and requires no local state management.

### Scaling consideration

The `sync_window_days` config (default: 7) limits how many entries are fetched from Toggl on each run. The Toggl API does not support server-side tag filtering, so filtering happens locally, but the small window keeps the data volume low even after years of use.

## Project Structure

```
toggl-sync-cli/
  main.go        # Entry point, CLI flag parsing
  config.go      # Config loading and validation
  toggl.go       # Toggl API client
  tempo.go       # Tempo API client
  sync.go        # Core sync pipeline logic
  go.mod
  go.sum
```

Flat `package main` structure. Single compiled binary.

## Dependencies

- `github.com/BurntSushi/toml` -- TOML config parsing
- Go standard library for everything else (`net/http`, `encoding/json`, `flag`, `fmt`, `regexp`, `time`)

## Testing

- **Unit tests**: Issue key parsing, config validation, date range calculation
- **Integration tests**: Full pipeline with mock HTTP servers (`net/http/httptest`)
- No external test dependencies
