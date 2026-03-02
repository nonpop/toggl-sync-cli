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
		// Extract issue key from path like /rest/api/3/issue/PROJ-1
		parts := strings.Split(r.URL.Path, "/")
		key := parts[len(parts)-1]
		// Return a fake numeric ID based on the issue number
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
			Tags:        []string{},
			WorkspaceID: 100,
		},
		{
			ID:          2,
			Description: "PROJ-2 already synced",
			Start:       "2026-03-01T10:00:00+00:00",
			Stop:        "2026-03-01T11:00:00+00:00",
			Duration:    3600,
			Tags:        []string{"synced"},
			WorkspaceID: 100,
		},
		{
			ID:          3,
			Description: "no issue key",
			Start:       "2026-03-01T11:00:00+00:00",
			Stop:        "2026-03-01T12:00:00+00:00",
			Duration:    3600,
			Tags:        []string{},
			WorkspaceID: 100,
		},
		{
			ID:          4,
			Description: "PROJ-4 running entry",
			Start:       "2026-03-01T14:00:00+00:00",
			Duration:    -1709301600,
			Tags:        []string{},
			WorkspaceID: 100,
		},
	}

	var tempoCreated []map[string]interface{}
	var taggedEntries []string

	togglSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/me/time_entries" {
			json.NewEncoder(w).Encode(togglEntries)
			return
		}
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/workspaces/") {
			taggedEntries = append(taggedEntries, r.URL.Path)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": 1})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer togglSrv.Close()

	tempoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		tempoCreated = append(tempoCreated, body)
		json.NewEncoder(w).Encode(map[string]interface{}{"tempoWorklogId": 999})
	}))
	defer tempoSrv.Close()

	jiraSrv := newJiraMock(t)
	defer jiraSrv.Close()

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}
	jiraClient := &JiraClient{BaseURL: jiraSrv.URL, Email: "e", APIToken: "t"}

	result, err := runSync(togglClient, tempoClient, jiraClient, SyncOptions{
		SyncedTag: "synced",
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

	if len(taggedEntries) != 1 {
		t.Fatalf("tagged entries = %d, want 1", len(taggedEntries))
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
			Tags:        []string{},
			WorkspaceID: 100,
		},
	}

	tempoCallCount := 0
	tagCallCount := 0
	jiraCallCount := 0

	togglSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(togglEntries)
			return
		}
		tagCallCount++
		json.NewEncoder(w).Encode(map[string]interface{}{"id": 1})
	}))
	defer togglSrv.Close()

	tempoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tempoCallCount++
		json.NewEncoder(w).Encode(map[string]interface{}{"tempoWorklogId": 999})
	}))
	defer tempoSrv.Close()

	jiraSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jiraCallCount++
	}))
	defer jiraSrv.Close()

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}
	jiraClient := &JiraClient{BaseURL: jiraSrv.URL, Email: "e", APIToken: "t"}

	result, err := runSync(togglClient, tempoClient, jiraClient, SyncOptions{
		SyncedTag: "synced",
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
	if tempoCallCount != 0 {
		t.Errorf("tempo was called %d times during dry-run", tempoCallCount)
	}
	if tagCallCount != 0 {
		t.Errorf("toggl tag was called %d times during dry-run", tagCallCount)
	}
	if jiraCallCount != 0 {
		t.Errorf("jira was called %d times during dry-run", jiraCallCount)
	}
}

func TestSync_TempoFailure(t *testing.T) {
	togglEntries := []TogglTimeEntry{
		{
			ID:          1,
			Description: "PROJ-1 will fail",
			Start:       "2026-03-01T09:00:00+00:00",
			Stop:        "2026-03-01T10:00:00+00:00",
			Duration:    3600,
			Tags:        []string{},
			WorkspaceID: 100,
		},
		{
			ID:          2,
			Description: "PROJ-2 will succeed",
			Start:       "2026-03-01T10:00:00+00:00",
			Stop:        "2026-03-01T11:00:00+00:00",
			Duration:    3600,
			Tags:        []string{},
			WorkspaceID: 100,
		},
	}

	callCount := 0
	togglSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(togglEntries)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"id": 1})
	}))
	defer togglSrv.Close()

	tempoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
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
	jiraClient := &JiraClient{BaseURL: jiraSrv.URL, Email: "e", APIToken: "t"}

	result, err := runSync(togglClient, tempoClient, jiraClient, SyncOptions{
		SyncedTag: "synced",
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

func TestSync_FetchFailure(t *testing.T) {
	togglSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer togglSrv.Close()

	tempoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("tempo should not be called when fetch fails")
	}))
	defer tempoSrv.Close()

	jiraSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("jira should not be called when fetch fails")
	}))
	defer jiraSrv.Close()

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}
	jiraClient := &JiraClient{BaseURL: jiraSrv.URL, Email: "e", APIToken: "t"}

	_, err := runSync(togglClient, tempoClient, jiraClient, SyncOptions{
		SyncedTag: "synced",
		AccountID: "acc-1",
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Error("expected error when Toggl fetch fails, got nil")
	}
}
