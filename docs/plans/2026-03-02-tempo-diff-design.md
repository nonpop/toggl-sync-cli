# Replace tag-based sync tracking with Tempo diff

## Problem

When continuing an existing time entry in Toggl (play button), Toggl copies all
tags — including the "synced" tag — to the new entry. This causes the new entry
to be incorrectly skipped during sync.

## Solution

Instead of tagging Toggl entries, fetch existing Tempo worklogs for the same
date range and diff against Toggl entries. An entry that already exists in Tempo
is skipped; otherwise it's synced. No local state, no Toggl modifications.

## Matching strategy

A Toggl entry is considered already synced if a Tempo worklog exists with all
four of these fields matching:

- Jira issue ID (numeric)
- Start date (`YYYY-MM-DD`)
- Start time (`HH:MM:SS`)
- Duration in seconds (`timeSpentSeconds`)

Additionally, only worklogs authored by the configured `jira.account_id` are
considered, so another user's worklog on the same issue/time won't cause a false
match.

### Time resolution

Both sides use second-resolution values. Toggl start times are formatted via
Go's `time.Format("15:04:05")`, which truncates sub-second precision (does not
round). Since we create the Tempo worklog with the same truncated value, the
stored Tempo `startTime` will always match. This is explicitly relied upon — do
not change either side to round independently.

## Tempo API: fetch worklogs

Add `TempoClient.FetchWorklogs(from, to string)` calling
`GET /worklogs?from=...&to=...` with pagination (up to 1000 per page, loop
until exhausted).

Response fields used per worklog:
- `issue.id` (int)
- `startDate` (string)
- `startTime` (string)
- `timeSpentSeconds` (int)
- `author.accountId` (string)

## Lookup set

Build a `map[worklogKey]struct{}` from fetched Tempo worklogs, filtering to
entries where `author.accountId` matches the configured account ID.

```go
type worklogKey struct {
    IssueID          int
    StartDate        string // "2006-01-02"
    StartTime        string // "15:04:05"
    TimeSpentSeconds int
}
```

## Sync flow changes

1. Fetch Toggl entries (unchanged)
2. **New:** Fetch Tempo worklogs for the same date range, build lookup set
3. For each Toggl entry:
   - Skip running entries (unchanged)
   - Parse issue key (unchanged)
   - Resolve Jira issue ID — **moved before** the "already synced" check
   - Build worklog key, check lookup set — skip if matched
   - Dry-run: stop here, report "would sync"
   - Create Tempo worklog (unchanged), **no AddTag call**

## Code removal

- `TogglClient.AddTag` method and its test
- `SyncedTag` from `SyncOptions`, `TogglConfig`, config loading/defaults
- `hasTag` function
- `toggl.synced_tag` documentation in README
- Tag-related mock handlers in tests

## Kept

- `SyncResult.AlreadySynced` — still meaningful, now determined by Tempo diff

## Error handling

If the Tempo worklogs fetch fails, `runSync` returns an error immediately (same
pattern as the Toggl fetch failure).
