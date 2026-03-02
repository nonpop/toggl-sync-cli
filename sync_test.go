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
