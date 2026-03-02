package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

const atlassianGatewayURL = "https://api.atlassian.com"

type JiraClient struct {
	BaseURL  string
	Email    string
	APIToken string

	// gatewayBaseURL is the resolved API gateway URL.
	// Set automatically via Init(), or directly in tests.
	gatewayBaseURL string
}

// Init resolves the cloud ID from the Jira instance and configures the
// API gateway base URL. Must be called before GetIssueID.
func (c *JiraClient) Init() error {
	cloudID, err := c.fetchCloudID()
	if err != nil {
		return fmt.Errorf("resolving Jira cloud ID: %w", err)
	}
	c.gatewayBaseURL = fmt.Sprintf("%s/ex/jira/%s", atlassianGatewayURL, cloudID)
	return nil
}

func (c *JiraClient) fetchCloudID() (string, error) {
	url := c.BaseURL + "/_edge/tenant_info"

	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching tenant info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tenant info returned status %d", resp.StatusCode)
	}

	var result struct {
		CloudID string `json:"cloudId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding tenant info: %w", err)
	}
	if result.CloudID == "" {
		return "", fmt.Errorf("empty cloud ID in tenant info response")
	}
	return result.CloudID, nil
}

func (c *JiraClient) GetIssueID(issueKey string) (int, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=", c.gatewayBaseURL, issueKey)

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
