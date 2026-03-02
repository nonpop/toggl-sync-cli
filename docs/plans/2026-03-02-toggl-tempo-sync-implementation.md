# toggl-sync-cli Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Go CLI that syncs unsynced Toggl time entries to Jira Tempo worklogs, using Toggl tags for state tracking.

**Architecture:** A simple sequential pipeline — fetch unsynced Toggl entries, parse Jira issue keys from descriptions, create Tempo worklogs, tag entries as synced. Flat `package main` structure with separate files per concern.

**Tech Stack:** Go, `github.com/BurntSushi/toml`, standard library (`net/http`, `encoding/json`, `flag`, `regexp`, `time`, `net/http/httptest` for tests).

**Design doc:** `docs/plans/2026-03-02-toggl-tempo-sync-design.md`

---

### Task 1: Project Setup

**Files:**
- Create: `go.mod`

**Step 1: Initialize Go module**

Run: `go mod init github.com/nonpop/toggl-sync-cli`
Expected: `go.mod` created

**Step 2: Add TOML dependency**

Run: `go get github.com/BurntSushi/toml`
Expected: `go.mod` and `go.sum` updated with the dependency

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: initialize Go module with TOML dependency"
```

---

### Task 2: Config Loading and Validation

**Files:**
- Create: `config.go`
- Test: `config_test.go`

**Step 1: Write the failing test**

Create `config_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_ValidFull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`
[toggl]
api_token = "toggl-token-123"
synced_tag = "done"

[tempo]
api_token = "tempo-token-456"
base_url = "https://custom.tempo.io/3"

[jira]
account_id = "jira-acc-789"

[sync]
cutoff_date = "2026-01-15"
sync_window_days = 14
`), 0644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Toggl.APIToken != "toggl-token-123" {
		t.Errorf("toggl api_token = %q, want %q", cfg.Toggl.APIToken, "toggl-token-123")
	}
	if cfg.Toggl.SyncedTag != "done" {
		t.Errorf("toggl synced_tag = %q, want %q", cfg.Toggl.SyncedTag, "done")
	}
	if cfg.Tempo.APIToken != "tempo-token-456" {
		t.Errorf("tempo api_token = %q, want %q", cfg.Tempo.APIToken, "tempo-token-456")
	}
	if cfg.Tempo.BaseURL != "https://custom.tempo.io/3" {
		t.Errorf("tempo base_url = %q, want %q", cfg.Tempo.BaseURL, "https://custom.tempo.io/3")
	}
	if cfg.Jira.AccountID != "jira-acc-789" {
		t.Errorf("jira account_id = %q, want %q", cfg.Jira.AccountID, "jira-acc-789")
	}
	if cfg.Sync.CutoffDate != "2026-01-15" {
		t.Errorf("sync cutoff_date = %q, want %q", cfg.Sync.CutoffDate, "2026-01-15")
	}
	if cfg.Sync.SyncWindowDays != 14 {
		t.Errorf("sync sync_window_days = %d, want %d", cfg.Sync.SyncWindowDays, 14)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`
[toggl]
api_token = "tok"

[tempo]
api_token = "tok"

[jira]
account_id = "acc"

[sync]
cutoff_date = "2026-01-01"
`), 0644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Toggl.SyncedTag != "synced" {
		t.Errorf("default synced_tag = %q, want %q", cfg.Toggl.SyncedTag, "synced")
	}
	if cfg.Tempo.BaseURL != "https://api.tempo.io/3" {
		t.Errorf("default base_url = %q, want %q", cfg.Tempo.BaseURL, "https://api.tempo.io/3")
	}
	if cfg.Sync.SyncWindowDays != 7 {
		t.Errorf("default sync_window_days = %d, want %d", cfg.Sync.SyncWindowDays, 7)
	}
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{
			name: "missing toggl api_token",
			config: `
[toggl]
[tempo]
api_token = "t"
[jira]
account_id = "a"
[sync]
cutoff_date = "2026-01-01"
`,
		},
		{
			name: "missing tempo api_token",
			config: `
[toggl]
api_token = "t"
[tempo]
[jira]
account_id = "a"
[sync]
cutoff_date = "2026-01-01"
`,
		},
		{
			name: "missing jira account_id",
			config: `
[toggl]
api_token = "t"
[tempo]
api_token = "t"
[jira]
[sync]
cutoff_date = "2026-01-01"
`,
		},
		{
			name: "missing sync cutoff_date",
			config: `
[toggl]
api_token = "t"
[tempo]
api_token = "t"
[jira]
account_id = "a"
[sync]
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			os.WriteFile(path, []byte(tt.config), 0644)
			_, err := loadConfig(path)
			if err == nil {
				t.Error("expected error for missing required field, got nil")
			}
		})
	}
}

func TestLoadConfig_InvalidCutoffDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`
[toggl]
api_token = "t"
[tempo]
api_token = "t"
[jira]
account_id = "a"
[sync]
cutoff_date = "not-a-date"
`), 0644)

	_, err := loadConfig(path)
	if err == nil {
		t.Error("expected error for invalid cutoff_date, got nil")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestLoadConfig -v`
Expected: FAIL — `loadConfig` not defined

**Step 3: Write implementation**

Create `config.go`:

```go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Toggl TogglConfig `toml:"toggl"`
	Tempo TempoConfig `toml:"tempo"`
	Jira  JiraConfig  `toml:"jira"`
	Sync  SyncConfig  `toml:"sync"`
}

type TogglConfig struct {
	APIToken  string `toml:"api_token"`
	SyncedTag string `toml:"synced_tag"`
}

type TempoConfig struct {
	APIToken string `toml:"api_token"`
	BaseURL  string `toml:"base_url"`
}

type JiraConfig struct {
	AccountID string `toml:"account_id"`
}

type SyncConfig struct {
	CutoffDate     string `toml:"cutoff_date"`
	SyncWindowDays int    `toml:"sync_window_days"`
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file: %w", err)
	}

	// Apply defaults
	if cfg.Toggl.SyncedTag == "" {
		cfg.Toggl.SyncedTag = "synced"
	}
	if cfg.Tempo.BaseURL == "" {
		cfg.Tempo.BaseURL = "https://api.tempo.io/3"
	}
	if cfg.Sync.SyncWindowDays == 0 {
		cfg.Sync.SyncWindowDays = 7
	}

	// Validate required fields
	if cfg.Toggl.APIToken == "" {
		return Config{}, fmt.Errorf("toggl.api_token is required")
	}
	if cfg.Tempo.APIToken == "" {
		return Config{}, fmt.Errorf("tempo.api_token is required")
	}
	if cfg.Jira.AccountID == "" {
		return Config{}, fmt.Errorf("jira.account_id is required")
	}
	if cfg.Sync.CutoffDate == "" {
		return Config{}, fmt.Errorf("sync.cutoff_date is required")
	}

	// Validate cutoff_date format
	if _, err := time.Parse("2006-01-02", cfg.Sync.CutoffDate); err != nil {
		return Config{}, fmt.Errorf("sync.cutoff_date must be YYYY-MM-DD format: %w", err)
	}

	return cfg, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestLoadConfig -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add config.go config_test.go
git commit -m "feat: add config loading with TOML parsing and validation"
```

---

### Task 3: Issue Key Parser

**Files:**
- Create: `parser.go`
- Test: `parser_test.go`

**Step 1: Write the failing test**

Create `parser_test.go`:

```go
package main

import "testing"

func TestParseIssueKey(t *testing.T) {
	tests := []struct {
		description string
		wantKey     string
		wantDesc    string
		wantOK      bool
	}{
		{"PROJ-123 fixed the bug", "PROJ-123", "fixed the bug", true},
		{"ABC-1 review", "ABC-1", "review", true},
		{"DATA2-99 migration script", "DATA2-99", "migration script", true},
		{"PROJ-123", "PROJ-123", "", true},
		{"no issue key here", "", "", false},
		{"proj-123 lowercase", "", "", false},
		{"123-ABC wrong format", "", "", false},
		{"", "", "", false},
		{"A-1 single letter project", "A-1", "single letter project", true},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			key, desc, ok := parseIssueKey(tt.description)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
			if desc != tt.wantDesc {
				t.Errorf("desc = %q, want %q", desc, tt.wantDesc)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestParseIssueKey -v`
Expected: FAIL — `parseIssueKey` not defined

**Step 3: Write implementation**

Create `parser.go`:

```go
package main

import (
	"regexp"
	"strings"
)

var issueKeyRe = regexp.MustCompile(`^([A-Z][A-Z0-9]*-\d+)(?:\s+(.*))?$`)

func parseIssueKey(description string) (issueKey, remaining string, ok bool) {
	m := issueKeyRe.FindStringSubmatch(description)
	if m == nil {
		return "", "", false
	}
	return m[1], strings.TrimSpace(m[2]), true
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestParseIssueKey -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add parser.go parser_test.go
git commit -m "feat: add Jira issue key parser for Toggl descriptions"
```

---

### Task 4: Toggl API Client

**Files:**
- Create: `toggl.go`
- Test: `toggl_test.go`

**Step 1: Write the failing tests**

Create `toggl_test.go`:

```go
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

func TestTogglClient_AddTag(t *testing.T) {
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/workspaces/100/time_entries/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]interface{}{"id": 42})
	}))
	defer srv.Close()

	client := &TogglClient{
		BaseURL:  srv.URL,
		APIToken: "test-token",
	}

	err := client.AddTag(100, 42, "synced")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotBody["tag_action"] != "add" {
		t.Errorf("tag_action = %v, want %q", gotBody["tag_action"], "add")
	}
	tags, ok := gotBody["tags"].([]interface{})
	if !ok || len(tags) != 1 || tags[0] != "synced" {
		t.Errorf("tags = %v, want [\"synced\"]", gotBody["tags"])
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run TestTogglClient -v`
Expected: FAIL — `TogglClient` not defined

**Step 3: Write implementation**

Create `toggl.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const togglBaseURL = "https://api.track.toggl.com/api/v9"

type TogglTimeEntry struct {
	ID          int      `json:"id"`
	Description string   `json:"description"`
	Start       string   `json:"start"`
	Stop        string   `json:"stop"`
	Duration    int      `json:"duration"`
	Tags        []string `json:"tags"`
	WorkspaceID int      `json:"workspace_id"`
}

type TogglClient struct {
	BaseURL  string
	APIToken string
}

func (c *TogglClient) FetchEntries(startDate, endDate time.Time) ([]TogglTimeEntry, error) {
	url := fmt.Sprintf("%s/me/time_entries?start_date=%s&end_date=%s",
		c.BaseURL,
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"),
	)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.SetBasicAuth(c.APIToken, "api_token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching entries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("toggl API returned status %d", resp.StatusCode)
	}

	var entries []TogglTimeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return entries, nil
}

func (c *TogglClient) AddTag(workspaceID, entryID int, tag string) error {
	url := fmt.Sprintf("%s/workspaces/%d/time_entries/%d", c.BaseURL, workspaceID, entryID)

	body := map[string]interface{}{
		"tag_action": "add",
		"tags":       []string{tag},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.SetBasicAuth(c.APIToken, "api_token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("updating entry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("toggl API returned status %d", resp.StatusCode)
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestTogglClient -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add toggl.go toggl_test.go
git commit -m "feat: add Toggl API client for fetching entries and adding tags"
```

---

### Task 5: Tempo API Client

**Files:**
- Create: `tempo.go`
- Test: `tempo_test.go`

**Step 1: Write the failing test**

Create `tempo_test.go`:

```go
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
		IssueKey:        "PROJ-123",
		TimeSpentSeconds: 3600,
		StartDate:       "2026-03-02",
		StartTime:       "09:00:00",
		Description:     "fixed the bug",
		AuthorAccountID: "acc-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotBody["issueKey"] != "PROJ-123" {
		t.Errorf("issueKey = %v, want PROJ-123", gotBody["issueKey"])
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
		IssueKey:        "PROJ-123",
		TimeSpentSeconds: 3600,
		StartDate:       "2026-03-02",
		StartTime:       "09:00:00",
		AuthorAccountID: "acc-123",
	})
	if err == nil {
		t.Error("expected error for bad request, got nil")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestTempoClient -v`
Expected: FAIL — `TempoClient` not defined

**Step 3: Write implementation**

Create `tempo.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type TempoWorklog struct {
	IssueKey         string `json:"issueKey"`
	TimeSpentSeconds int    `json:"timeSpentSeconds"`
	StartDate        string `json:"startDate"`
	StartTime        string `json:"startTime"`
	Description      string `json:"description,omitempty"`
	AuthorAccountID  string `json:"authorAccountId"`
}

type TempoClient struct {
	BaseURL  string
	APIToken string
}

func (c *TempoClient) CreateWorklog(wl TempoWorklog) error {
	url := fmt.Sprintf("%s/worklogs", c.BaseURL)

	data, err := json.Marshal(wl)
	if err != nil {
		return fmt.Errorf("encoding worklog: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("creating worklog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tempo API returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestTempoClient -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add tempo.go tempo_test.go
git commit -m "feat: add Tempo API client for creating worklogs"
```

---

### Task 6: Sync Pipeline

**Files:**
- Create: `sync.go`
- Test: `sync_test.go`

**Step 1: Write the failing tests**

Create `sync_test.go`:

```go
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

	result := runSync(togglClient, tempoClient, SyncOptions{
		SyncedTag: "synced",
		AccountID: "acc-1",
		DryRun:    false,
	})

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

	result := runSync(togglClient, tempoClient, SyncOptions{
		SyncedTag: "synced",
		AccountID: "acc-1",
		DryRun:    true,
	})

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

	result := runSync(togglClient, tempoClient, SyncOptions{
		SyncedTag: "synced",
		AccountID: "acc-1",
		DryRun:    false,
	})

	if result.Synced != 1 {
		t.Errorf("synced = %d, want 1", result.Synced)
	}
	if result.Failed != 1 {
		t.Errorf("failed = %d, want 1", result.Failed)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run TestSync -v`
Expected: FAIL — `runSync`, `SyncOptions`, `SyncResult` not defined

**Step 3: Write implementation**

Create `sync.go`:

```go
package main

import (
	"fmt"
	"time"
)

type SyncOptions struct {
	SyncedTag string
	AccountID string
	DryRun    bool
}

type SyncResult struct {
	Synced        int
	Skipped       int
	AlreadySynced int
	Failed        int
	WouldSync     int
}

func runSync(toggl *TogglClient, tempo *TempoClient, opts SyncOptions) SyncResult {
	entries, err := toggl.FetchEntries(time.Time{}, time.Now())
	if err != nil {
		fmt.Printf("ERROR: failed to fetch Toggl entries: %v\n", err)
		return SyncResult{}
	}

	var result SyncResult

	for _, entry := range entries {
		// Skip already synced
		if hasTag(entry.Tags, opts.SyncedTag) {
			result.AlreadySynced++
			continue
		}

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

		if opts.DryRun {
			fmt.Printf("WOULD SYNC: [%d] %s %q (%s)\n",
				entry.ID, issueKey, desc, formatDuration(entry.Duration))
			result.WouldSync++
			continue
		}

		// Create Tempo worklog
		wl := TempoWorklog{
			IssueKey:         issueKey,
			TimeSpentSeconds: entry.Duration,
			StartDate:        startTime.Format("2006-01-02"),
			StartTime:        startTime.Format("15:04:05"),
			Description:      desc,
			AuthorAccountID:  opts.AccountID,
		}

		if err := tempo.CreateWorklog(wl); err != nil {
			fmt.Printf("FAIL: [%d] %s %q: %v\n", entry.ID, issueKey, desc, err)
			result.Failed++
			continue
		}

		// Tag as synced in Toggl
		if err := toggl.AddTag(entry.WorkspaceID, entry.ID, opts.SyncedTag); err != nil {
			fmt.Printf("WARN: [%d] worklog created but tagging failed: %v\n", entry.ID, err)
			// Still count as synced since the Tempo worklog was created
			result.Synced++
			continue
		}

		fmt.Printf("OK: [%d] %s %q (%s)\n",
			entry.ID, issueKey, desc, formatDuration(entry.Duration))
		result.Synced++
	}

	return result
}

func hasTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

func formatDuration(seconds int) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestSync -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add sync.go sync_test.go
git commit -m "feat: add core sync pipeline with dry-run support"
```

---

### Task 7: Main Entry Point and CLI

**Files:**
- Create: `main.go`

**Step 1: Write the implementation**

Create `main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	defaultConfigPath := filepath.Join(os.Getenv("HOME"), ".config", "toggl-sync", "config.toml")

	configPath := flag.String("config", defaultConfigPath, "path to config file")
	dryRun := flag.Bool("dry-run", false, "show what would be synced without doing it")
	days := flag.Int("days", 0, "override sync window (days to look back)")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	windowDays := cfg.Sync.SyncWindowDays
	if *days > 0 {
		windowDays = *days
	}

	cutoff, _ := time.Parse("2006-01-02", cfg.Sync.CutoffDate)
	windowStart := time.Now().AddDate(0, 0, -windowDays)
	startDate := cutoff
	if windowStart.After(cutoff) {
		startDate = windowStart
	}
	endDate := time.Now().AddDate(0, 0, 1) // tomorrow to include today

	togglClient := &TogglClient{
		BaseURL:  togglBaseURL,
		APIToken: cfg.Toggl.APIToken,
	}
	tempoClient := &TempoClient{
		BaseURL:  cfg.Tempo.BaseURL,
		APIToken: cfg.Tempo.APIToken,
	}

	if *dryRun {
		fmt.Println("=== DRY RUN ===")
	}
	fmt.Printf("Fetching Toggl entries from %s to %s...\n",
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	result := runSync(togglClient, tempoClient, SyncOptions{
		SyncedTag: cfg.Toggl.SyncedTag,
		AccountID: cfg.Jira.AccountID,
		DryRun:    *dryRun,
		StartDate: startDate,
		EndDate:   endDate,
	})

	fmt.Println()
	fmt.Println("=== Summary ===")
	if *dryRun {
		fmt.Printf("Would sync: %d\n", result.WouldSync)
	} else {
		fmt.Printf("Synced:         %d\n", result.Synced)
		fmt.Printf("Failed:         %d\n", result.Failed)
	}
	fmt.Printf("Skipped:        %d\n", result.Skipped)
	fmt.Printf("Already synced: %d\n", result.AlreadySynced)

	if result.Failed > 0 {
		os.Exit(1)
	}
}
```

Note: This requires a small update to `SyncOptions` and `runSync` to accept `StartDate`/`EndDate` instead of using `time.Time{}`/`time.Now()`. The update to `sync.go` is:

Add fields to `SyncOptions`:
```go
type SyncOptions struct {
	SyncedTag string
	AccountID string
	DryRun    bool
	StartDate time.Time
	EndDate   time.Time
}
```

Update `runSync` to use them:
```go
entries, err := toggl.FetchEntries(opts.StartDate, opts.EndDate)
```

Update existing tests to set `StartDate`/`EndDate` in `SyncOptions`:
```go
SyncOptions{
	SyncedTag: "synced",
	AccountID: "acc-1",
	DryRun:    false,
	StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
}
```

**Step 2: Verify it compiles**

Run: `go build -o toggl-sync .`
Expected: Binary created successfully

**Step 3: Run all tests**

Run: `go test -v ./...`
Expected: All PASS

**Step 4: Commit**

```bash
git add main.go sync.go sync_test.go
git commit -m "feat: add CLI entry point with flag parsing and sync window"
```

---

### Task 8: Manual Testing and Polish

**Step 1: Verify `--help` output**

Run: `./toggl-sync --help`
Expected: Shows usage with `--config`, `--dry-run`, `--days` flags

**Step 2: Verify error on missing config**

Run: `./toggl-sync --config /nonexistent`
Expected: Error message about config file, exit code 1

**Step 3: Add .gitignore**

Create `.gitignore`:
```
toggl-sync
```

**Step 4: Commit**

```bash
git add .gitignore
git commit -m "chore: add gitignore for compiled binary"
```

---

### Task 9: Final Test Run and Cleanup

**Step 1: Run full test suite**

Run: `go test -v -count=1 ./...`
Expected: All tests pass

**Step 2: Run go vet**

Run: `go vet ./...`
Expected: No issues

**Step 3: Verify build**

Run: `go build -o toggl-sync . && echo "Build OK"`
Expected: "Build OK"

**Step 4: Final commit if any cleanup was needed**

```bash
git add -A
git commit -m "chore: final cleanup"
```
