package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type TempoWorklog struct {
	IssueID          int    `json:"issueId"`
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
