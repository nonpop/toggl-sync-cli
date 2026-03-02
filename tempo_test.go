package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTempoClient_CreateWorklog(t *testing.T) {
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/worklogs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-tempo-token" {
			t.Errorf("bad auth header: %s", auth)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"tempoWorklogId": 999})
	}))
	defer srv.Close()

	client := &TempoClient{
		BaseURL:  srv.URL,
		APIToken: "test-tempo-token",
	}

	err := client.CreateWorklog(TempoWorklog{
		IssueID:          12345,
		TimeSpentSeconds: 3600,
		StartDate:        "2026-03-02",
		StartTime:        "09:00:00",
		Description:      "fixed the bug",
		AuthorAccountID:  "acc-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if int(gotBody["issueId"].(float64)) != 12345 {
		t.Errorf("issueId = %v, want 12345", gotBody["issueId"])
	}
	if gotBody["description"] != "fixed the bug" {
		t.Errorf("description = %v, want 'fixed the bug'", gotBody["description"])
	}
	if int(gotBody["timeSpentSeconds"].(float64)) != 3600 {
		t.Errorf("timeSpentSeconds = %v, want 3600", gotBody["timeSpentSeconds"])
	}
}

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

func TestTempoClient_CreateWorklog_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errors":[{"message":"bad request"}]}`))
	}))
	defer srv.Close()

	client := &TempoClient{
		BaseURL:  srv.URL,
		APIToken: "test-tempo-token",
	}

	err := client.CreateWorklog(TempoWorklog{
		IssueID:          12345,
		TimeSpentSeconds: 3600,
		StartDate:        "2026-03-02",
		StartTime:        "09:00:00",
		AuthorAccountID:  "acc-123",
	})
	if err == nil {
		t.Error("expected error for bad request, got nil")
	}
}
