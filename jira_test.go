package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJiraClient_GetIssueID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/PROJ-123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "test@example.com" || pass != "test-token" {
			t.Error("bad auth")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":  "12345",
			"key": "PROJ-123",
		})
	}))
	defer srv.Close()

	client := &JiraClient{
		BaseURL:  srv.URL,
		Email:    "test@example.com",
		APIToken: "test-token",
	}

	id, err := client.GetIssueID("PROJ-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 12345 {
		t.Errorf("id = %d, want 12345", id)
	}
}

func TestJiraClient_GetIssueID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := &JiraClient{
		BaseURL:  srv.URL,
		Email:    "test@example.com",
		APIToken: "test-token",
	}

	_, err := client.GetIssueID("NOPE-999")
	if err == nil {
		t.Error("expected error for not found issue, got nil")
	}
}
