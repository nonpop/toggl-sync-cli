# Replace tag-based sync tracking with Tempo diff

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace tag-based sync deduplication with a diff against existing Tempo worklogs, eliminating the "continued entry copies synced tag" problem.

**Architecture:** Fetch Tempo worklogs for the same date range as Toggl entries, build a lookup set keyed on (issueID, date, startTime, duration) filtered to the user's account, and skip Toggl entries that match.

**Tech Stack:** Go, Tempo REST API v4 (`GET /worklogs`), existing Toggl/Jira APIs unchanged.

---

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

---

### Task 1: Add `FetchWorklogs` to TempoClient

**Files:**
- Modify: `tempo.go:10-18` (add new response types after `TempoWorklog`)
- Modify: `tempo.go:51` (add `FetchWorklogs` method after `CreateWorklog`)
- Test: `tempo_test.go`

**Step 1: Write the failing test for single-page fetch**

Add to `tempo_test.go`:

```go
func TestTempoClient_FetchWorklogs(t *testing.T) {
	response := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"tempoWorklogId":   1,
				"issue":            map[string]interface{}{"id": 1001},
				"startDate":        "2026-03-01",
				"startTime":        "09:00:00",
				"timeSpentSeconds": 3600,
				"author":           map[string]interface{}{"accountId": "acc-123"},
			},
			{
				"tempoWorklogId":   2,
				"issue":            map[string]interface{}{"id": 1002},
				"startDate":        "2026-03-01",
				"startTime":        "10:00:00",
				"timeSpentSeconds": 1800,
				"author":           map[string]interface{}{"accountId": "acc-456"},
			},
		},
		"metadata": map[string]interface{}{
			"count":  2,
			"offset": 0,
			"limit":  50,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/worklogs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("from") != "2026-03-01" {
			t.Errorf("from = %q, want %q", r.URL.Query().Get("from"), "2026-03-01")
		}
		if r.URL.Query().Get("to") != "2026-03-07" {
			t.Errorf("to = %q, want %q", r.URL.Query().Get("to"), "2026-03-07")
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-tempo-token" {
			t.Errorf("bad auth header: %s", auth)
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	client := &TempoClient{BaseURL: srv.URL, APIToken: "test-tempo-token"}

	worklogs, err := client.FetchWorklogs("2026-03-01", "2026-03-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(worklogs) != 2 {
		t.Fatalf("got %d worklogs, want 2", len(worklogs))
	}
	if worklogs[0].IssueID != 1001 {
		t.Errorf("worklogs[0].IssueID = %d, want 1001", worklogs[0].IssueID)
	}
	if worklogs[0].StartDate != "2026-03-01" {
		t.Errorf("worklogs[0].StartDate = %q, want %q", worklogs[0].StartDate, "2026-03-01")
	}
	if worklogs[0].StartTime != "09:00:00" {
		t.Errorf("worklogs[0].StartTime = %q, want %q", worklogs[0].StartTime, "09:00:00")
	}
	if worklogs[0].TimeSpentSeconds != 3600 {
		t.Errorf("worklogs[0].TimeSpentSeconds = %d, want 3600", worklogs[0].TimeSpentSeconds)
	}
	if worklogs[0].AuthorAccountID != "acc-123" {
		t.Errorf("worklogs[0].AuthorAccountID = %q, want %q", worklogs[0].AuthorAccountID, "acc-123")
	}
	if worklogs[1].IssueID != 1002 {
		t.Errorf("worklogs[1].IssueID = %d, want 1002", worklogs[1].IssueID)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestTempoClient_FetchWorklogs -v ./...`
Expected: FAIL — `FetchWorklogs` not defined

**Step 3: Write minimal implementation**

Add types and method to `tempo.go`:

```go
type TempoExistingWorklog struct {
	IssueID          int
	StartDate        string
	StartTime        string
	TimeSpentSeconds int
	AuthorAccountID  string
}

type tempoSearchResponse struct {
	Results []struct {
		Issue            struct{ ID int }    `json:"issue"`
		StartDate        string              `json:"startDate"`
		StartTime        string              `json:"startTime"`
		TimeSpentSeconds int                 `json:"timeSpentSeconds"`
		Author           struct{ AccountID string `json:"accountId"` } `json:"author"`
	} `json:"results"`
	Metadata struct {
		Count  int `json:"count"`
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	} `json:"metadata"`
}

func (c *TempoClient) FetchWorklogs(from, to string) ([]TempoExistingWorklog, error) {
	var all []TempoExistingWorklog
	offset := 0
	limit := 1000

	for {
		url := fmt.Sprintf("%s/worklogs?from=%s&to=%s&offset=%d&limit=%d",
			c.BaseURL, from, to, offset, limit)

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching worklogs: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("tempo API returned status %d: %s", resp.StatusCode, string(body))
		}

		var page tempoSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		for _, r := range page.Results {
			all = append(all, TempoExistingWorklog{
				IssueID:          r.Issue.ID,
				StartDate:        r.StartDate,
				StartTime:        r.StartTime,
				TimeSpentSeconds: r.TimeSpentSeconds,
				AuthorAccountID:  r.Author.AccountID,
			})
		}

		if len(page.Results) < page.Metadata.Limit {
			break
		}
		offset += len(page.Results)
	}

	return all, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestTempoClient_FetchWorklogs -v ./...`
Expected: PASS

**Step 5: Write pagination test**

Add to `tempo_test.go`:

```go
func TestTempoClient_FetchWorklogs_Pagination(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		offset := r.URL.Query().Get("offset")
		var resp map[string]interface{}
		if offset == "0" || offset == "" {
			resp = map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"tempoWorklogId":   1,
						"issue":            map[string]interface{}{"id": 1001},
						"startDate":        "2026-03-01",
						"startTime":        "09:00:00",
						"timeSpentSeconds": 3600,
						"author":           map[string]interface{}{"accountId": "acc-1"},
					},
				},
				"metadata": map[string]interface{}{"count": 1, "offset": 0, "limit": 1},
			}
		} else {
			resp = map[string]interface{}{
				"results":  []map[string]interface{}{},
				"metadata": map[string]interface{}{"count": 0, "offset": 1, "limit": 1},
			}
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &TempoClient{BaseURL: srv.URL, APIToken: "tok"}
	worklogs, err := client.FetchWorklogs("2026-03-01", "2026-03-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(worklogs) != 1 {
		t.Fatalf("got %d worklogs, want 1", len(worklogs))
	}
	if callCount != 2 {
		t.Errorf("API called %d times, want 2 (page 1 + empty page 2)", callCount)
	}
}
```

**Step 6: Run test to verify it passes** (implementation already handles pagination)

Run: `go test -run TestTempoClient_FetchWorklogs -v ./...`
Expected: PASS

**Step 7: Commit**

```
git add tempo.go tempo_test.go
git commit -m "feat: add FetchWorklogs to TempoClient with pagination"
```

---

### Task 2: Add `worklogKey` and `buildLookupSet`

**Files:**
- Modify: `sync.go` (add types and function)
- Modify: `sync_test.go` (add unit test)

**Step 1: Write the failing test**

Add to `sync_test.go`:

```go
func TestBuildLookupSet(t *testing.T) {
	worklogs := []TempoExistingWorklog{
		{IssueID: 1001, StartDate: "2026-03-01", StartTime: "09:00:00", TimeSpentSeconds: 3600, AuthorAccountID: "me"},
		{IssueID: 1002, StartDate: "2026-03-01", StartTime: "10:00:00", TimeSpentSeconds: 1800, AuthorAccountID: "other"},
		{IssueID: 1003, StartDate: "2026-03-01", StartTime: "11:00:00", TimeSpentSeconds: 900, AuthorAccountID: "me"},
	}

	set := buildLookupSet(worklogs, "me")

	// Should include my worklogs
	if _, ok := set[worklogKey{1001, "2026-03-01", "09:00:00", 3600}]; !ok {
		t.Error("expected worklog 1001 in set")
	}
	if _, ok := set[worklogKey{1003, "2026-03-01", "11:00:00", 900}]; !ok {
		t.Error("expected worklog 1003 in set")
	}

	// Should exclude other user's worklog
	if _, ok := set[worklogKey{1002, "2026-03-01", "10:00:00", 1800}]; ok {
		t.Error("worklog 1002 (other user) should not be in set")
	}

	if len(set) != 2 {
		t.Errorf("set size = %d, want 2", len(set))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestBuildLookupSet -v ./...`
Expected: FAIL — `buildLookupSet` and `worklogKey` not defined

**Step 3: Write minimal implementation**

Add to `sync.go`:

```go
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
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestBuildLookupSet -v ./...`
Expected: PASS

**Step 5: Commit**

```
git add sync.go sync_test.go
git commit -m "feat: add worklogKey type and buildLookupSet function"
```

---

### Task 3: Rewrite `runSync` to use Tempo diff

**Files:**
- Modify: `sync.go:8-14` (change `SyncOptions`)
- Modify: `sync.go:24-108` (rewrite `runSync`)
- Modify: `sync_test.go` (rewrite all sync tests)

**Step 1: Rewrite `SyncOptions` and `runSync`**

Replace `SyncOptions` in `sync.go`:

```go
type SyncOptions struct {
	AccountID string
	DryRun    bool
	StartDate time.Time
	EndDate   time.Time
}
```

Replace `runSync` in `sync.go`:

```go
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
```

**Step 2: Delete `hasTag` function from `sync.go`** (lines 110-117)

Remove entirely — no longer used.

**Step 3: Rewrite sync tests**

Replace entire `sync_test.go` content. Key changes:
- Tempo mock now serves both `GET /worklogs` (returns existing worklogs) and `POST /worklogs` (creates new ones)
- Toggl mock no longer handles PUT (tag) requests
- `SyncOptions` no longer has `SyncedTag`
- "Already synced" is determined by existing Tempo worklogs, not tags
- Dry-run test: Jira *is* called now (issue ID needed for diff), but Tempo POST is not

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newJiraMock(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		key := parts[len(parts)-1]
		ids := map[string]string{
			"PROJ-1": "1001",
			"PROJ-2": "1002",
			"PROJ-4": "1004",
		}
		id, ok := ids[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "key": key})
	}))
}

func TestSync_FullPipeline(t *testing.T) {
	togglEntries := []TogglTimeEntry{
		{
			ID:          1,
			Description: "PROJ-1 did work",
			Start:       "2026-03-01T09:00:00+00:00",
			Stop:        "2026-03-01T10:00:00+00:00",
			Duration:    3600,
			WorkspaceID: 100,
		},
		{
			ID:          2,
			Description: "PROJ-2 already in tempo",
			Start:       "2026-03-01T10:00:00+00:00",
			Stop:        "2026-03-01T11:00:00+00:00",
			Duration:    3600,
			WorkspaceID: 100,
		},
		{
			ID:          3,
			Description: "no issue key",
			Start:       "2026-03-01T11:00:00+00:00",
			Stop:        "2026-03-01T12:00:00+00:00",
			Duration:    3600,
			WorkspaceID: 100,
		},
		{
			ID:          4,
			Description: "PROJ-4 running entry",
			Start:       "2026-03-01T14:00:00+00:00",
			Duration:    -1709301600,
			WorkspaceID: 100,
		},
	}

	// PROJ-2 already exists in Tempo (start times converted to local)
	tempoExisting := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"tempoWorklogId":   99,
				"issue":            map[string]interface{}{"id": 1002},
				"startDate":        time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC).Local().Format("2006-01-02"),
				"startTime":        time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC).Local().Format("15:04:05"),
				"timeSpentSeconds": 3600,
				"author":           map[string]interface{}{"accountId": "acc-1"},
			},
		},
		"metadata": map[string]interface{}{"count": 1, "offset": 0, "limit": 1000},
	}

	var tempoCreated []map[string]interface{}

	togglSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/me/time_entries" {
			json.NewEncoder(w).Encode(togglEntries)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer togglSrv.Close()

	tempoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/worklogs" {
			json.NewEncoder(w).Encode(tempoExisting)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/worklogs" {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			tempoCreated = append(tempoCreated, body)
			json.NewEncoder(w).Encode(map[string]interface{}{"tempoWorklogId": 999})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tempoSrv.Close()

	jiraSrv := newJiraMock(t)
	defer jiraSrv.Close()

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}
	jiraClient := &JiraClient{Email: "e", APIToken: "t", gatewayBaseURL: jiraSrv.URL}

	result, err := runSync(togglClient, tempoClient, jiraClient, SyncOptions{
		AccountID: "acc-1",
		DryRun:    false,
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Synced != 1 {
		t.Errorf("synced = %d, want 1", result.Synced)
	}
	if result.Skipped != 2 {
		t.Errorf("skipped = %d, want 2 (1 no key + 1 running)", result.Skipped)
	}
	if result.AlreadySynced != 1 {
		t.Errorf("already_synced = %d, want 1", result.AlreadySynced)
	}
	if result.Failed != 0 {
		t.Errorf("failed = %d, want 0", result.Failed)
	}

	if len(tempoCreated) != 1 {
		t.Fatalf("tempo worklogs created = %d, want 1", len(tempoCreated))
	}
	if int(tempoCreated[0]["issueId"].(float64)) != 1001 {
		t.Errorf("worklog issueId = %v, want 1001", tempoCreated[0]["issueId"])
	}
}

func TestSync_DryRun(t *testing.T) {
	togglEntries := []TogglTimeEntry{
		{
			ID:          1,
			Description: "PROJ-1 work",
			Start:       "2026-03-01T09:00:00+00:00",
			Stop:        "2026-03-01T10:00:00+00:00",
			Duration:    3600,
			WorkspaceID: 100,
		},
	}

	tempoExisting := map[string]interface{}{
		"results":  []map[string]interface{}{},
		"metadata": map[string]interface{}{"count": 0, "offset": 0, "limit": 1000},
	}

	tempoPostCount := 0

	togglSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(togglEntries)
	}))
	defer togglSrv.Close()

	tempoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(tempoExisting)
			return
		}
		tempoPostCount++
		json.NewEncoder(w).Encode(map[string]interface{}{"tempoWorklogId": 999})
	}))
	defer tempoSrv.Close()

	jiraSrv := newJiraMock(t)
	defer jiraSrv.Close()

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}
	jiraClient := &JiraClient{Email: "e", APIToken: "t", gatewayBaseURL: jiraSrv.URL}

	result, err := runSync(togglClient, tempoClient, jiraClient, SyncOptions{
		AccountID: "acc-1",
		DryRun:    true,
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Synced != 0 {
		t.Errorf("dry-run synced = %d, want 0", result.Synced)
	}
	if result.WouldSync != 1 {
		t.Errorf("dry-run would_sync = %d, want 1", result.WouldSync)
	}
	if tempoPostCount != 0 {
		t.Errorf("tempo POST called %d times during dry-run", tempoPostCount)
	}
}

func TestSync_TempoCreateFailure(t *testing.T) {
	togglEntries := []TogglTimeEntry{
		{
			ID:          1,
			Description: "PROJ-1 will fail",
			Start:       "2026-03-01T09:00:00+00:00",
			Stop:        "2026-03-01T10:00:00+00:00",
			Duration:    3600,
			WorkspaceID: 100,
		},
		{
			ID:          2,
			Description: "PROJ-2 will succeed",
			Start:       "2026-03-01T10:00:00+00:00",
			Stop:        "2026-03-01T11:00:00+00:00",
			Duration:    3600,
			WorkspaceID: 100,
		},
	}

	tempoExisting := map[string]interface{}{
		"results":  []map[string]interface{}{},
		"metadata": map[string]interface{}{"count": 0, "offset": 0, "limit": 1000},
	}

	createCallCount := 0
	togglSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(togglEntries)
	}))
	defer togglSrv.Close()

	tempoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(tempoExisting)
			return
		}
		createCallCount++
		if createCallCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"server error"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"tempoWorklogId": 999})
	}))
	defer tempoSrv.Close()

	jiraSrv := newJiraMock(t)
	defer jiraSrv.Close()

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}
	jiraClient := &JiraClient{Email: "e", APIToken: "t", gatewayBaseURL: jiraSrv.URL}

	result, err := runSync(togglClient, tempoClient, jiraClient, SyncOptions{
		AccountID: "acc-1",
		DryRun:    false,
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Synced != 1 {
		t.Errorf("synced = %d, want 1", result.Synced)
	}
	if result.Failed != 1 {
		t.Errorf("failed = %d, want 1", result.Failed)
	}
}

func TestSync_TogglFetchFailure(t *testing.T) {
	togglSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer togglSrv.Close()

	tempoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("tempo should not be called when Toggl fetch fails")
	}))
	defer tempoSrv.Close()

	jiraSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("jira should not be called when Toggl fetch fails")
	}))
	defer jiraSrv.Close()

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}
	jiraClient := &JiraClient{Email: "e", APIToken: "t", gatewayBaseURL: jiraSrv.URL}

	_, err := runSync(togglClient, tempoClient, jiraClient, SyncOptions{
		AccountID: "acc-1",
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Error("expected error when Toggl fetch fails, got nil")
	}
}

func TestSync_TempoFetchFailure(t *testing.T) {
	togglEntries := []TogglTimeEntry{
		{ID: 1, Description: "PROJ-1 work", Start: "2026-03-01T09:00:00+00:00", Duration: 3600, WorkspaceID: 100},
	}

	togglSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(togglEntries)
	}))
	defer togglSrv.Close()

	tempoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer tempoSrv.Close()

	jiraSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("jira should not be called when Tempo fetch fails")
	}))
	defer jiraSrv.Close()

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}
	jiraClient := &JiraClient{Email: "e", APIToken: "t", gatewayBaseURL: jiraSrv.URL}

	_, err := runSync(togglClient, tempoClient, jiraClient, SyncOptions{
		AccountID: "acc-1",
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Error("expected error when Tempo fetch fails, got nil")
	}
}
```

**Step 4: Run all tests to verify they pass**

Run: `go test -v ./...`
Expected: PASS

**Step 5: Commit**

```
git add sync.go sync_test.go
git commit -m "feat: replace tag-based sync with Tempo worklog diff"
```

---

### Task 4: Remove tag code from Toggl client and config

**Files:**
- Modify: `toggl.go:1-9` (remove `bytes` import)
- Modify: `toggl.go:58-87` (delete `AddTag` method entirely)
- Modify: `toggl_test.go:73-105` (delete `TestTogglClient_AddTag`)
- Modify: `config.go:18-21` (remove `SyncedTag` from `TogglConfig`)
- Modify: `config.go:52-54` (remove `SyncedTag` default)
- Modify: `config_test.go:15,41-43` (remove `synced_tag` from full config test)
- Modify: `config_test.go:87-89` (remove `SyncedTag` default assertion)
- Modify: `main.go:63` (remove `SyncedTag` from `SyncOptions`)

**Step 1: Delete `AddTag` from `toggl.go`**

Remove `AddTag` method (lines 58-87) and remove `"bytes"` from imports (no longer needed).

**Step 2: Delete `TestTogglClient_AddTag` from `toggl_test.go`** (lines 73-105)

**Step 3: Remove `SyncedTag` from config**

In `config.go`:
- Remove `SyncedTag string \`toml:"synced_tag"\`` from `TogglConfig`
- Remove the `if cfg.Toggl.SyncedTag == "" { ... }` default block

**Step 4: Update config tests**

In `config_test.go`:
- Remove `synced_tag = "done"` from the test config string in `TestLoadConfig_ValidFull`
- Remove the `cfg.Toggl.SyncedTag` assertion in `TestLoadConfig_ValidFull` (lines 41-43)
- Remove the `cfg.Toggl.SyncedTag` assertion in `TestLoadConfig_Defaults` (lines 87-89)

**Step 5: Update `main.go`**

Remove `SyncedTag: cfg.Toggl.SyncedTag,` from the `SyncOptions` struct literal (line 63).

**Step 6: Run all tests**

Run: `go test -v ./...`
Expected: PASS

**Step 7: Commit**

```
git add toggl.go toggl_test.go config.go config_test.go main.go
git commit -m "refactor: remove tag-based sync tracking code"
```

---

### Task 5: Update README

**Files:**
- Modify: `README.md`

**Step 1: Update "How it works" section**

Replace steps 3-4:
```
3. Creates worklogs in Tempo for each unsynced entry
4. Tags synced entries in Toggl to prevent duplicates
```
With:
```
3. Fetches existing Tempo worklogs for the same period
4. Creates worklogs in Tempo only for entries not already present
```

**Step 2: Remove the sentence about sync state**

Remove: `Sync state is tracked via Toggl tags — no local database needed.`
Replace with: `Deduplication is based on matching Toggl entries against existing Tempo worklogs (by issue, date, start time, and duration).`

**Step 3: Remove `toggl.synced_tag` from the optional fields table**

Delete the row: `| toggl.synced_tag | synced | Tag name applied to synced entries |`

**Step 4: Commit**

```
git add README.md
git commit -m "docs: update README for Tempo diff-based sync"
```

---

### Task 6: Final verification

**Step 1: Run full test suite**

Run: `go test -v ./...`
Expected: All tests PASS

**Step 2: Build**

Run: `go build ./...`
Expected: No errors

**Step 3: Verify no leftover tag references**

Run: `grep -r "synced_tag\|SyncedTag\|AddTag\|hasTag\|tag_action" --include='*.go' .`
Expected: No matches
