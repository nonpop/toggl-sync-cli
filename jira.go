package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type JiraClient struct {
	BaseURL  string
	Email    string
	APIToken string
}

func (c *JiraClient) GetIssueID(issueKey string) (int, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=", c.BaseURL, issueKey)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.SetBasicAuth(c.Email, c.APIToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetching issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("jira API returned status %d for issue %s", resp.StatusCode, issueKey)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding response: %w", err)
	}

	id, err := strconv.Atoi(result.ID)
	if err != nil {
		return 0, fmt.Errorf("invalid issue ID %q: %w", result.ID, err)
	}
	return id, nil
}
