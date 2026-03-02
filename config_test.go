package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_ValidFull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
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
`), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

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
	if err := os.WriteFile(path, []byte(`
[toggl]
api_token = "tok"

[tempo]
api_token = "tok"

[jira]
account_id = "acc"

[sync]
cutoff_date = "2026-01-01"
`), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

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
			if err := os.WriteFile(path, []byte(tt.config), 0644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}
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
	if err := os.WriteFile(path, []byte(`
[toggl]
api_token = "t"
[tempo]
api_token = "t"
[jira]
account_id = "a"
[sync]
cutoff_date = "not-a-date"
`), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := loadConfig(path)
	if err == nil {
		t.Error("expected error for invalid cutoff_date, got nil")
	}
}
