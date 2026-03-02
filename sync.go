package main

import (
	"fmt"
	"time"
)

type SyncOptions struct {
	SyncedTag string
	AccountID string
	DryRun    bool
	StartDate time.Time
	EndDate   time.Time
}

type SyncResult struct {
	Synced        int
	Skipped       int
	AlreadySynced int
	Failed        int
	WouldSync     int
}

func runSync(toggl *TogglClient, tempo *TempoClient, jira *JiraClient, opts SyncOptions) (SyncResult, error) {
	entries, err := toggl.FetchEntries(opts.StartDate, opts.EndDate)
	if err != nil {
		return SyncResult{}, fmt.Errorf("fetching Toggl entries: %w", err)
	}

	var result SyncResult

	for _, entry := range entries {
		// Skip already synced
		if hasTag(entry.Tags, opts.SyncedTag) {
			result.AlreadySynced++
			continue
		}

		// Skip running entries (negative duration)
		if entry.Duration < 0 {
			fmt.Printf("SKIP: [%d] %q (still running)\n", entry.ID, entry.Description)
			result.Skipped++
			continue
		}

		// Parse issue key
		issueKey, desc, ok := parseIssueKey(entry.Description)
		if !ok {
			fmt.Printf("SKIP: [%d] %q (no Jira issue key)\n", entry.ID, entry.Description)
			result.Skipped++
			continue
		}

		// Parse start time
		startTime, err := time.Parse(time.RFC3339, entry.Start)
		if err != nil {
			fmt.Printf("SKIP: [%d] %q (invalid start time: %v)\n", entry.ID, entry.Description, err)
			result.Skipped++
			continue
		}

		if opts.DryRun {
			fmt.Printf("WOULD SYNC: [%d] %s %q (%s)\n",
				entry.ID, issueKey, desc, formatDuration(entry.Duration))
			result.WouldSync++
			continue
		}

		// Resolve issue key to numeric ID
		issueID, err := jira.GetIssueID(issueKey)
		if err != nil {
			fmt.Printf("FAIL: [%d] %s %q: %v\n", entry.ID, issueKey, desc, err)
			result.Failed++
			continue
		}

		// Create Tempo worklog
		localTime := startTime.Local()
		wl := TempoWorklog{
			IssueID:          issueID,
			TimeSpentSeconds: entry.Duration,
			StartDate:        localTime.Format("2006-01-02"),
			StartTime:        localTime.Format("15:04:05"),
			Description:      desc,
			AuthorAccountID:  opts.AccountID,
		}

		if err := tempo.CreateWorklog(wl); err != nil {
			fmt.Printf("FAIL: [%d] %s %q: %v\n", entry.ID, issueKey, desc, err)
			result.Failed++
			continue
		}

		// Tag as synced in Toggl
		if err := toggl.AddTag(entry.WorkspaceID, entry.ID, opts.SyncedTag); err != nil {
			fmt.Printf("WARN: [%d] worklog created but tagging failed: %v\n", entry.ID, err)
			// Still count as synced since the Tempo worklog was created
			result.Synced++
			continue
		}

		fmt.Printf("OK: [%d] %s %q (%s)\n",
			entry.ID, issueKey, desc, formatDuration(entry.Duration))
		result.Synced++
	}

	return result, nil
}

type worklogKey struct {
	IssueID          int
	StartDate        string
	StartTime        string
	TimeSpentSeconds int
}

func buildLookupSet(worklogs []TempoExistingWorklog, accountID string) map[worklogKey]struct{} {
	set := make(map[worklogKey]struct{})
	for _, wl := range worklogs {
		if wl.AuthorAccountID != accountID {
			continue
		}
		key := worklogKey{
			IssueID:          wl.IssueID,
			StartDate:        wl.StartDate,
			StartTime:        wl.StartTime,
			TimeSpentSeconds: wl.TimeSpentSeconds,
		}
		set[key] = struct{}{}
	}
	return set
}

func hasTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

func formatDuration(seconds int) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
