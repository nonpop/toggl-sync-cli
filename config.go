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
	APIToken string `toml:"api_token"`
}

type TempoConfig struct {
	APIToken string `toml:"api_token"`
	BaseURL  string `toml:"base_url"`
}

type JiraConfig struct {
	BaseURL   string `toml:"base_url"`
	Email     string `toml:"email"`
	APIToken  string `toml:"api_token"`
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
	if cfg.Tempo.BaseURL == "" {
		cfg.Tempo.BaseURL = "https://api.tempo.io/4"
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
	if cfg.Jira.BaseURL == "" {
		return Config{}, fmt.Errorf("jira.base_url is required")
	}
	if cfg.Jira.Email == "" {
		return Config{}, fmt.Errorf("jira.email is required")
	}
	if cfg.Jira.APIToken == "" {
		return Config{}, fmt.Errorf("jira.api_token is required")
	}
	if cfg.Jira.AccountID == "" {
		return Config{}, fmt.Errorf("jira.account_id is required")
	}
	// Validate cutoff_date format if provided
	if cfg.Sync.CutoffDate != "" {
		if _, err := time.Parse("2006-01-02", cfg.Sync.CutoffDate); err != nil {
			return Config{}, fmt.Errorf("sync.cutoff_date must be YYYY-MM-DD format: %w", err)
		}
	}

	return cfg, nil
}
