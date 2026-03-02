# toggl-sync-cli

> **Warning:** This code is 100% AI-generated and has not been manually reviewed. Use at your own risk.

> This is a quick personal helper tool. I am not accepting pull requests or responding to issues. If you want changes, please fork the repository.

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
base_url = "https://your-org.atlassian.net"
email = "you@example.com"
api_token = "your-jira-api-token"
account_id = "your-jira-account-id"

[sync]
cutoff_date = "2026-03-01"
```

### Finding your API keys and account ID

**`toggl.api_token`** — Go to [Toggl Track](https://track.toggl.com/), click your profile picture (bottom left) > **Profile Settings**, scroll down to **API Token**. Copy the token shown there.

**`tempo.api_token`** — In Jira, open the Tempo sidebar and go to **Settings** > **API Integration** (under Data Access). Click **New Token**, give it a name, and set the access scope to allow worklog creation. Copy the token immediately — it is only shown once.

**`jira.base_url`** — Your Jira Cloud instance URL, e.g. `https://your-org.atlassian.net`. You can find this in your browser address bar when visiting Jira.

**`jira.email`** — The email address associated with your Atlassian account. Go to [Atlassian account settings](https://id.atlassian.com/manage-profile/email) to check.

**`jira.api_token`** — Go to [Atlassian API tokens](https://id.atlassian.com/manage-profile/security/api-tokens), click **Create API token with scopes**, give it a name, add the `read:jira-work` scope, and copy the token. This is the only scope the tool needs (it reads issue IDs to create Tempo worklogs).

**`jira.account_id`** — In Jira Cloud, click your avatar (top right) > **Profile**. Your account ID is the string in the URL after `/people/` (e.g. `https://your-org.atlassian.net/people/5abcdef1234567890abcdef` — the ID is `5abcdef1234567890abcdef`).

### Required fields

| Field | Description |
|---|---|
| `toggl.api_token` | Toggl Track API token |
| `tempo.api_token` | Tempo API token (needs worklog write access) |
| `jira.base_url` | Your Jira Cloud instance URL (e.g. `https://your-org.atlassian.net`) |
| `jira.email` | Email for your Atlassian account |
| `jira.api_token` | Jira/Atlassian API token |
| `jira.account_id` | Your Atlassian account ID |
| `sync.cutoff_date` | Ignore entries before this date (YYYY-MM-DD). Interpreted as midnight in your Toggl profile timezone. |

### Optional fields

| Field | Default | Description |
|---|---|---|
| `toggl.synced_tag` | `synced` | Tag name applied to synced entries |
| `tempo.base_url` | `https://api.tempo.io/4` | Tempo API base URL |
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
