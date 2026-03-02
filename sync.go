package main

import (
	"fmt"
	"time"
)

type SyncOptions struct {
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

	existingWorklogs, err := tempo.FetchWorklogs(
		opts.StartDate.Format("2006-01-02"),
		opts.EndDate.Format("2006-01-02"),
	)
	if err != nil {
		return SyncResult{}, fmt.Errorf("fetching Tempo worklogs: %w", err)
	}

	synced := buildLookupSet(existingWorklogs, opts.AccountID)

	var result SyncResult

	for _, entry := range entries {
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

		// Resolve issue key to numeric ID
		issueID, err := jira.GetIssueID(issueKey)
		if err != nil {
			fmt.Printf("FAIL: [%d] %s %q: %v\n", entry.ID, issueKey, desc, err)
			result.Failed++
			continue
		}

		// Check if already synced via Tempo diff
		localTime := startTime.Local()
		key := worklogKey{
			IssueID:          issueID,
			StartDate:        localTime.Format("2006-01-02"),
			StartTime:        localTime.Format("15:04:05"),
			TimeSpentSeconds: entry.Duration,
		}
		if _, exists := synced[key]; exists {
			result.AlreadySynced++
			continue
		}

		if opts.DryRun {
			fmt.Printf("WOULD SYNC: [%d] %s %q (%s)\n",
				entry.ID, issueKey, desc, formatDuration(entry.Duration))
			result.WouldSync++
			continue
		}

		// Create Tempo worklog
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

func formatDuration(seconds int) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
