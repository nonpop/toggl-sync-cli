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
