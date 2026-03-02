package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJiraClient_Init(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_edge/tenant_info" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{"cloudId": "abc-123-def"})
	}))
	defer srv.Close()

	client := &JiraClient{BaseURL: srv.URL, Email: "e", APIToken: "t"}
	if err := client.Init(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://api.atlassian.com/ex/jira/abc-123-def"
	if client.gatewayBaseURL != want {
		t.Errorf("gatewayBaseURL = %q, want %q", client.gatewayBaseURL, want)
	}
}

func TestJiraClient_Init_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &JiraClient{BaseURL: srv.URL, Email: "e", APIToken: "t"}
	if err := client.Init(); err == nil {
		t.Error("expected error when tenant info fails, got nil")
	}
}

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
		Email:          "test@example.com",
		APIToken:       "test-token",
		gatewayBaseURL: srv.URL,
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
		Email:          "test@example.com",
		APIToken:       "test-token",
		gatewayBaseURL: srv.URL,
	}

	_, err := client.GetIssueID("NOPE-999")
	if err == nil {
		t.Error("expected error for not found issue, got nil")
	}
}
