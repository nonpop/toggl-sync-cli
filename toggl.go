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
