package agenticmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GitHubIssueTracker files issues in a GitHub repository.
type GitHubIssueTracker struct {
	// Owner is the repository owner (e.g., "neokapi").
	Owner string

	// Repo is the repository name (e.g., "agentic-fleet").
	Repo string

	// Token is a GitHub PAT with issues write permission.
	Token string
}

type ghIssueRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels,omitempty"`
}

type ghIssueResponse struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

func (t *GitHubIssueTracker) FileIssue(ctx context.Context, title, body string, labels []string) (string, int, error) {
	payload, err := json.Marshal(ghIssueRequest{Title: title, Body: body, Labels: labels})
	if err != nil {
		return "", 0, err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", t.Owner, t.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+t.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("github API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("github API %d: %s", resp.StatusCode, string(respBody))
	}

	var result ghIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, fmt.Errorf("decode response: %w", err)
	}

	return result.HTMLURL, result.Number, nil
}
