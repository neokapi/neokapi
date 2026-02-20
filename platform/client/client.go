package client

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
	"time"
)

// BowrainClient is a REST client for the Bowrain server sync API.
// It supports two auth modes:
//   - ClaimToken: unclaimed project, flat routes /api/v1/projects/:id/sync/*
//   - JWT + workspace: workspace project, routes /api/v1/workspaces/:ws/projects/:id/sync/*
type BowrainClient struct {
	baseURL    string
	projectID  string
	workspace  string // workspace slug; empty for unclaimed (ClaimToken) projects
	authToken  string // JWT bearer token for workspace projects
	claimToken string // ClaimToken for unclaimed projects
	httpClient *http.Client

	refreshToken   string                             // opaque refresh token for auto-refresh
	onTokenRefresh func(newAccess, newRefresh string) // callback after successful refresh
}

// NewWorkspaceBowrainClient creates a client that uses workspace-scoped routes with auth.
func NewWorkspaceBowrainClient(serverURL, workspace, projectID, authToken string) *BowrainClient {
	return &BowrainClient{
		baseURL:    strings.TrimRight(serverURL, "/"),
		projectID:  projectID,
		workspace:  workspace,
		authToken:  authToken,
		httpClient: &http.Client{},
	}
}

// NewClaimTokenClient creates a client that uses claim token auth for anonymous projects.
func NewClaimTokenClient(serverURL, projectID, claimToken string) *BowrainClient {
	return &BowrainClient{
		baseURL:    strings.TrimRight(serverURL, "/"),
		projectID:  projectID,
		claimToken: claimToken,
		httpClient: &http.Client{},
	}
}

// projectPrefix returns the URL prefix for project-scoped endpoints.
// Workspace project: /api/v1/workspaces/{ws}/projects/{pid}
// Unclaimed project: /api/v1/projects/{pid}
func (c *BowrainClient) projectPrefix() string {
	if c.workspace != "" {
		return fmt.Sprintf("%s/api/v1/workspaces/%s/projects/%s", c.baseURL, c.workspace, c.projectID)
	}
	return fmt.Sprintf("%s/api/v1/projects/%s", c.baseURL, c.projectID)
}

// applyAuth adds authorization header if a token is set.
func (c *BowrainClient) applyAuth(req *http.Request) {
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	} else if c.claimToken != "" {
		req.Header.Set("Authorization", "ClaimToken "+c.claimToken)
	}
}

// SetRefreshToken configures auto-refresh with the given refresh token.
// The onRefresh callback is called after a successful token refresh so the
// caller can persist the new tokens.
func (c *BowrainClient) SetRefreshToken(token string, onRefresh func(newAccess, newRefresh string)) {
	c.refreshToken = token
	c.onTokenRefresh = onRefresh
}

// doRequest executes an HTTP request and automatically retries once with a
// refreshed access token when the server returns 401 Unauthorized. Auto-retry
// is only attempted for requests without a body (GET, HEAD, DELETE) because
// the body of POST/PUT requests is consumed on the first attempt.
func (c *BowrainClient) doRequest(req *http.Request) (*http.Response, error) {
	c.applyAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Auto-refresh on 401 if we have a refresh token.
	canRetry := req.Body == nil || req.Body == http.NoBody
	if resp.StatusCode == http.StatusUnauthorized && c.refreshToken != "" && c.authToken != "" && canRetry {
		resp.Body.Close()

		if refreshErr := c.doRefresh(req.Context()); refreshErr != nil {
			return nil, fmt.Errorf("token refresh failed: %w", refreshErr)
		}

		// Retry the original request with the new token.
		retryReq, err := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), nil)
		if err != nil {
			return nil, err
		}
		for k, v := range req.Header {
			retryReq.Header[k] = v
		}
		c.applyAuth(retryReq)
		return c.httpClient.Do(retryReq)
	}

	return resp, nil
}

// doRefresh calls the /api/v1/auth/refresh endpoint to obtain a new access
// token and a rotated refresh token.
func (c *BowrainClient) doRefresh(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{"refresh_token": c.refreshToken})
	refreshURL := c.baseURL + "/api/v1/auth/refresh"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh failed with HTTP %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}

	c.authToken = tokenResp.AccessToken
	c.refreshToken = tokenResp.RefreshToken
	if c.onTokenRefresh != nil {
		c.onTokenRefresh(tokenResp.AccessToken, tokenResp.RefreshToken)
	}
	return nil
}

// SyncPushRequest is the request body for pushing blocks.
type SyncPushRequest struct {
	Blocks []BlockInput `json:"blocks"`
}

// BlockInput represents a block in the API.
type BlockInput struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	ItemName string `json:"item_name,omitempty"`
}

// SyncPushResponse is the response from a push.
type SyncPushResponse struct {
	Stored    int   `json:"stored"`
	NewCursor int64 `json:"new_cursor"`
}

// ChangeEntry represents a single change log entry from the server.
type ChangeEntry struct {
	Seq         int64     `json:"seq"`
	BlockID     string    `json:"block_id"`
	ChangeType  string    `json:"change_type"`
	Locale      string    `json:"locale,omitempty"`
	ContentHash string    `json:"content_hash,omitempty"`
	LoggedAt    time.Time `json:"logged_at,omitempty"`
}

// SyncPullResponse is the response from a pull.
type SyncPullResponse struct {
	Changes   []ChangeEntry `json:"changes"`
	NewCursor int64         `json:"new_cursor"`
	HasMore   bool          `json:"has_more"`
}

// BlockContent represents a block with its translations from the server.
type BlockContent struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	ItemName string            `json:"item_name"`
	Source   string            `json:"source"`
	Targets  map[string]string `json:"targets"` // locale → plain text
}

// Push sends blocks to the server.
func (c *BowrainClient) Push(ctx context.Context, blocks []BlockInput) (*SyncPushResponse, error) {
	body, err := json.Marshal(SyncPushRequest{Blocks: blocks})
	if err != nil {
		return nil, fmt.Errorf("marshal push request: %w", err)
	}

	u := c.projectPrefix() + "/sync/push"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.doRequest(req)
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
	u, err := url.Parse(c.projectPrefix() + "/sync/pull")
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

	resp, err := c.doRequest(req)
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

// GetBlocks fetches blocks for a specific item (source file) with their translations.
func (c *BowrainClient) GetBlocks(ctx context.Context, itemName string) ([]BlockContent, error) {
	u, err := url.Parse(c.projectPrefix() + "/sync/blocks")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	q := u.Query()
	if itemName != "" {
		q.Set("item_name", itemName)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("get blocks request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get blocks failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var blocks []BlockContent
	if err := json.NewDecoder(resp.Body).Decode(&blocks); err != nil {
		return nil, fmt.Errorf("decode blocks response: %w", err)
	}
	return blocks, nil
}

// CreateAnonymousProject creates a new anonymous project on a Bowrain server.
// No authentication is required. Returns the project ID and claim token.
// If email is non-empty, the server sends a claim email to that address.
// targetLocales may be empty (server treats as dynamic).
func CreateAnonymousProject(serverURL, name, sourceLocale string, targetLocales []string, email string) (projectID, claimToken string, err error) {
	payload := map[string]any{
		"name":          name,
		"source_locale": sourceLocale,
	}
	if len(targetLocales) > 0 {
		payload["target_locales"] = targetLocales
	}
	if email != "" {
		payload["email"] = email
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	u := strings.TrimRight(serverURL, "/") + "/api/v1/projects/anonymous"
	resp, err := http.Post(u, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("create anonymous project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("create anonymous project failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ProjectID  string `json:"project_id"`
		ClaimToken string `json:"claim_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}

	return result.ProjectID, result.ClaimToken, nil
}

// CreateAuthenticatedProject creates a project on the server as an authenticated user.
// The project is created in the user's workspace. Returns the project ID.
func CreateAuthenticatedProject(serverURL, token, name, sourceLocale string, targetLocales []string, workspace string) (projectID, workspaceSlug string, err error) {
	payload := map[string]any{
		"name":          name,
		"source_locale": sourceLocale,
	}
	if len(targetLocales) > 0 {
		payload["target_locales"] = targetLocales
	}
	if workspace != "" {
		payload["workspace"] = workspace
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	u := strings.TrimRight(serverURL, "/") + "/api/v1/projects"
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("create project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("create project failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID            string `json:"id"`
		WorkspaceSlug string `json:"workspace_slug"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}

	return result.ID, result.WorkspaceSlug, nil
}

// WorkspaceInfo contains basic workspace metadata returned by ListWorkspaces.
type WorkspaceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Type string `json:"type"`
}

// ListWorkspaces returns the workspaces accessible to the authenticated user.
func ListWorkspaces(serverURL, token string) ([]WorkspaceInfo, error) {
	u := strings.TrimRight(serverURL, "/") + "/api/v1/workspaces"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list workspaces failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result []WorkspaceInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result, nil
}

// CreateWorkspace creates a new team workspace on the server and returns it.
func CreateWorkspace(serverURL, token, name, slug string) (*WorkspaceInfo, error) {
	payload := map[string]string{
		"name": name,
		"slug": slug,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	u := strings.TrimRight(serverURL, "/") + "/api/v1/workspaces"
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create workspace failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result WorkspaceInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}
