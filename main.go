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
	jiraClient := &JiraClient{
		BaseURL:  cfg.Jira.BaseURL,
		Email:    cfg.Jira.Email,
		APIToken: cfg.Jira.APIToken,
	}
	if err := jiraClient.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to Jira: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Println("=== DRY RUN ===")
	}
	fmt.Printf("Fetching Toggl entries from %s to %s...\n",
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	result, err := runSync(togglClient, tempoClient, jiraClient, SyncOptions{
		AccountID: cfg.Jira.AccountID,
		DryRun:    *dryRun,
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

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
