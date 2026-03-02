package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}

	result, err := runSync(togglClient, tempoClient, SyncOptions{
		SyncedTag: "synced",
		AccountID: "acc-1",
		DryRun:    false,
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
	if tempoCreated[0]["issueKey"] != "PROJ-1" {
		t.Errorf("worklog issueKey = %v, want PROJ-1", tempoCreated[0]["issueKey"])
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

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}

	result, err := runSync(togglClient, tempoClient, SyncOptions{
		SyncedTag: "synced",
		AccountID: "acc-1",
		DryRun:    true,
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

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}

	result, err := runSync(togglClient, tempoClient, SyncOptions{
		SyncedTag: "synced",
		AccountID: "acc-1",
		DryRun:    false,
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

	togglClient := &TogglClient{BaseURL: togglSrv.URL, APIToken: "tok"}
	tempoClient := &TempoClient{BaseURL: tempoSrv.URL, APIToken: "tok"}

	_, err := runSync(togglClient, tempoClient, SyncOptions{
		SyncedTag: "synced",
		AccountID: "acc-1",
	})
	if err == nil {
		t.Error("expected error when Toggl fetch fails, got nil")
	}
}
