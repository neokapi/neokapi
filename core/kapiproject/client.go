package kapiproject

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// BowrainClient is a REST client for the Bowrain server sync API.
type BowrainClient struct {
	baseURL    string
	projectID  string
	httpClient *http.Client
}

// NewBowrainClient creates a new client for the given server URL and project.
func NewBowrainClient(serverURL, projectID string) *BowrainClient {
	return &BowrainClient{
		baseURL:    strings.TrimRight(serverURL, "/"),
		projectID:  projectID,
		httpClient: &http.Client{},
	}
}

// SyncPushRequest is the request body for pushing blocks.
type SyncPushRequest struct {
	Blocks []BlockInput `json:"blocks"`
}

// BlockInput represents a block in the API.
type BlockInput struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// SyncPushResponse is the response from a push.
type SyncPushResponse struct {
	Stored    int   `json:"stored"`
	NewCursor int64 `json:"new_cursor"`
}

// ChangeEntry represents a single change log entry from the server.
type ChangeEntry struct {
	Seq         int64  `json:"seq"`
	BlockID     string `json:"block_id"`
	ChangeType  string `json:"change_type"`
	Locale      string `json:"locale"`
	ContentHash string `json:"content_hash"`
}

// SyncPullResponse is the response from a pull.
type SyncPullResponse struct {
	Changes   []ChangeEntry `json:"changes"`
	NewCursor int64         `json:"new_cursor"`
	HasMore   bool          `json:"has_more"`
}

// Push sends blocks to the server.
func (c *BowrainClient) Push(ctx context.Context, blocks []BlockInput) (*SyncPushResponse, error) {
	body, err := json.Marshal(SyncPushRequest{Blocks: blocks})
	if err != nil {
		return nil, fmt.Errorf("marshal push request: %w", err)
	}

	u := fmt.Sprintf("%s/api/v1/projects/%s/sync/push", c.baseURL, c.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("push request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("push failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result SyncPushResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode push response: %w", err)
	}
	return &result, nil
}

// Pull fetches changes from the server since the given cursor.
func (c *BowrainClient) Pull(ctx context.Context, cursor int64, locales []string, limit int) (*SyncPullResponse, error) {
	u, err := url.Parse(fmt.Sprintf("%s/api/v1/projects/%s/sync/pull", c.baseURL, c.projectID))
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	q := u.Query()
	q.Set("cursor", strconv.FormatInt(cursor, 10))
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if len(locales) > 0 {
		q.Set("locales", strings.Join(locales, ","))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pull failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result SyncPullResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode pull response: %w", err)
	}
	return &result, nil
}
