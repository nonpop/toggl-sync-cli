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

type TempoExistingWorklog struct {
	IssueID          int
	StartDate        string
	StartTime        string
	TimeSpentSeconds int
	AuthorAccountID  string
}

type tempoSearchResponse struct {
	Results []struct {
		Issue            struct{ ID int }                              `json:"issue"`
		StartDate        string                                       `json:"startDate"`
		StartTime        string                                       `json:"startTime"`
		TimeSpentSeconds int                                          `json:"timeSpentSeconds"`
		Author           struct{ AccountID string `json:"accountId"` } `json:"author"`
	} `json:"results"`
	Metadata struct {
		Count  int `json:"count"`
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	} `json:"metadata"`
}

func (c *TempoClient) FetchWorklogs(from, to string) ([]TempoExistingWorklog, error) {
	var all []TempoExistingWorklog
	offset := 0
	limit := 1000

	for {
		url := fmt.Sprintf("%s/worklogs?from=%s&to=%s&offset=%d&limit=%d",
			c.BaseURL, from, to, offset, limit)

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching worklogs: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("tempo API returned status %d: %s", resp.StatusCode, string(body))
		}

		var page tempoSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		resp.Body.Close()

		for _, r := range page.Results {
			all = append(all, TempoExistingWorklog{
				IssueID:          r.Issue.ID,
				StartDate:        r.StartDate,
				StartTime:        r.StartTime,
				TimeSpentSeconds: r.TimeSpentSeconds,
				AuthorAccountID:  r.Author.AccountID,
			})
		}

		if len(page.Results) < page.Metadata.Limit {
			break
		}
		offset += len(page.Results)
	}

	return all, nil
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
