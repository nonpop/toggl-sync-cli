package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTogglClient_FetchEntries(t *testing.T) {
	entries := []TogglTimeEntry{
		{
			ID:          1,
			Description: "PROJ-1 task one",
			Start:       "2026-03-01T09:00:00+00:00",
			Stop:        "2026-03-01T10:00:00+00:00",
			Duration:    3600,
			Tags:        []string{},
			WorkspaceID: 100,
		},
		{
			ID:          2,
			Description: "PROJ-2 task two",
			Start:       "2026-03-01T10:00:00+00:00",
			Stop:        "2026-03-01T11:30:00+00:00",
			Duration:    5400,
			Tags:        []string{"synced"},
			WorkspaceID: 100,
		},
		{
			ID:          3,
			Description: "PROJ-3 running",
			Start:       "2026-03-01T14:00:00+00:00",
			Duration:    -1709301600,
			Tags:        []string{},
			WorkspaceID: 100,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/time_entries" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "test-token" || pass != "api_token" {
			t.Error("bad auth")
		}
		json.NewEncoder(w).Encode(entries)
	}))
	defer srv.Close()

	client := &TogglClient{
		BaseURL:  srv.URL,
		APIToken: "test-token",
	}

	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)

	result, err := client.FetchEntries(start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d entries, want 3", len(result))
	}
	if result[0].ID != 1 || result[1].ID != 2 || result[2].ID != 3 {
		t.Error("entries not returned correctly")
	}
}
