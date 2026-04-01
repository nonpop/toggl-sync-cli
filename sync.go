package main

import (
	"fmt"
	"time"
)

type SyncOptions struct {
	AccountID string
	DryRun    bool
	Verbose   bool
	StartDate time.Time
	EndDate   time.Time
}

type SyncResult struct {
	Synced        int
	Skipped       int
	AlreadySynced int
	Failed        int
	WouldSync     int
	TotalSeconds  int
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

	synced, byIssue := buildLookupSet(existingWorklogs, opts.AccountID, opts.Verbose)

	var result SyncResult

	for _, entry := range entries {
		// Skip running entries (negative duration)
		if entry.Duration < 0 {
			fmt.Printf("SKIP: %q (still running)\n", entry.Description)
			result.Skipped++
			continue
		}

		// Parse start time
		startTime, err := time.Parse(time.RFC3339, entry.Start)
		if err != nil {
			fmt.Printf("SKIP: %q (invalid start time: %v)\n", entry.Description, err)
			result.Skipped++
			continue
		}
		timeInfo := entryTimeInfo(startTime, entry.Duration)

		// Parse issue key
		issueKey, desc, ok := parseIssueKey(entry.Description)
		if !ok {
			fmt.Printf("SKIP: %q (no Jira issue key) [%s]\n", entry.Description, timeInfo)
			result.Skipped++
			continue
		}

		// Resolve issue key to numeric ID
		issueID, err := jira.GetIssueID(issueKey)
		if err != nil {
			fmt.Printf("FAIL: %s %q [%s]: %v\n", issueKey, desc, timeInfo, err)
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
		if opts.Verbose {
			fmt.Printf("VERBOSE: Toggl %q key={IssueID:%d, StartDate:%s, StartTime:%s, Duration:%ds}\n",
				entry.Description, key.IssueID, key.StartDate, key.StartTime, key.TimeSpentSeconds)
		}
		if _, exists := synced[key]; exists {
			if opts.Verbose {
				fmt.Printf("VERBOSE:   -> matched in Tempo\n")
			}
			result.AlreadySynced++
			continue
		}
		if opts.Verbose {
			fmt.Printf("VERBOSE:   -> no exact match")
			if candidates := byIssue[issueID]; len(candidates) > 0 {
				fmt.Printf(", Tempo worklogs for issue %d:\n", issueID)
				for _, c := range candidates {
					fmt.Printf("VERBOSE:     Tempo key={IssueID:%d, StartDate:%s, StartTime:%s, Duration:%ds}",
						c.IssueID, c.StartDate, c.StartTime, c.TimeSpentSeconds)
					var diffs []string
					if c.StartDate != key.StartDate {
						diffs = append(diffs, fmt.Sprintf("StartDate %s vs %s", key.StartDate, c.StartDate))
					}
					if c.StartTime != key.StartTime {
						diffs = append(diffs, fmt.Sprintf("StartTime %s vs %s", key.StartTime, c.StartTime))
					}
					if c.TimeSpentSeconds != key.TimeSpentSeconds {
						delta := c.TimeSpentSeconds - key.TimeSpentSeconds
						diffs = append(diffs, fmt.Sprintf("Duration %ds vs %ds (delta: %ds)", key.TimeSpentSeconds, c.TimeSpentSeconds, delta))
					}
					if len(diffs) > 0 {
						fmt.Printf(" -- diff:")
						for _, d := range diffs {
							fmt.Printf(" [%s]", d)
						}
					}
					fmt.Println()
				}
			} else {
				fmt.Printf(" (no Tempo worklogs for issue %d)\n", issueID)
			}
		}

		if opts.DryRun {
			fmt.Printf("WOULD SYNC: %s %q [%s]\n", issueKey, desc, timeInfo)
			result.WouldSync++
			result.TotalSeconds += entry.Duration
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
			fmt.Printf("FAIL: %s %q [%s]: %v\n", issueKey, desc, timeInfo, err)
			result.Failed++
			continue
		}

		fmt.Printf("OK: %s %q [%s]\n", issueKey, desc, timeInfo)
		result.Synced++
		result.TotalSeconds += entry.Duration
	}

	return result, nil
}

type worklogKey struct {
	IssueID          int
	StartDate        string
	StartTime        string
	TimeSpentSeconds int
}

func buildLookupSet(worklogs []TempoExistingWorklog, accountID string, verbose bool) (map[worklogKey]struct{}, map[int][]worklogKey) {
	set := make(map[worklogKey]struct{})
	byIssue := make(map[int][]worklogKey)
	if verbose {
		fmt.Printf("VERBOSE: Tempo worklogs fetched: %d total\n", len(worklogs))
	}
	for _, wl := range worklogs {
		if wl.AuthorAccountID != accountID {
			if verbose {
				fmt.Printf("VERBOSE:   skip (account %s != %s): IssueID:%d %s %s %ds\n",
					wl.AuthorAccountID, accountID, wl.IssueID, wl.StartDate, wl.StartTime, wl.TimeSpentSeconds)
			}
			continue
		}
		key := worklogKey{
			IssueID:          wl.IssueID,
			StartDate:        wl.StartDate,
			StartTime:        wl.StartTime,
			TimeSpentSeconds: wl.TimeSpentSeconds,
		}
		set[key] = struct{}{}
		byIssue[wl.IssueID] = append(byIssue[wl.IssueID], key)
		if verbose {
			fmt.Printf("VERBOSE:   loaded: {IssueID:%d, StartDate:%s, StartTime:%s, Duration:%ds}\n",
				key.IssueID, key.StartDate, key.StartTime, key.TimeSpentSeconds)
		}
	}
	return set, byIssue
}

func entryTimeInfo(startTime time.Time, durationSeconds int) string {
	local := startTime.Local()
	end := local.Add(time.Duration(durationSeconds) * time.Second)
	return fmt.Sprintf("%s %s-%s (%s)",
		local.Format("2006-01-02"),
		local.Format("15:04"),
		end.Format("15:04"),
		formatDuration(durationSeconds),
	)
}

func formatDuration(seconds int) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
