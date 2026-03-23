// Package agentictesting provides the standalone agentic testing server.
package agentictesting

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// BowrainClient wraps bowrain's REST API for read-only workspace data access.
type BowrainClient struct {
	BaseURL string       // e.g. https://dev.bowrain.cloud
	Token   string       // bwt_* API token
	HTTP    *http.Client // defaults to http.DefaultClient
}

func (c *BowrainClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *BowrainClient) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("bowrain API %s: %d %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

// Workspace is a bowrain workspace summary.
type Workspace struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Plan string `json:"plan,omitempty"`
}

// ListWorkspaces returns all workspaces visible to the API token.
func (c *BowrainClient) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	data, err := c.get(ctx, "/api/v1/workspaces", nil)
	if err != nil {
		return nil, err
	}
	var workspaces []Workspace
	if err := json.Unmarshal(data, &workspaces); err != nil {
		return nil, fmt.Errorf("decode workspaces: %w", err)
	}
	return workspaces, nil
}

// Project is a bowrain project summary.
type Project struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	DefaultSourceLanguage string   `json:"default_source_language"`
	TargetLanguages       []string `json:"target_languages"`
	TargetLanguageMode    string   `json:"target_language_mode,omitempty"`
	WorkspaceID           string   `json:"workspace_id,omitempty"`
	Archived              bool     `json:"archived,omitempty"`
	CreatedAt             string   `json:"created_at,omitempty"`
	UpdatedAt             string   `json:"updated_at,omitempty"`
}

// ListProjects returns projects in a workspace.
func (c *BowrainClient) ListProjects(ctx context.Context, wsSlug string) ([]Project, error) {
	data, err := c.get(ctx, fmt.Sprintf("/api/v1/workspaces/%s/projects", wsSlug), nil)
	if err != nil {
		return nil, err
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("decode projects: %w", err)
	}
	return projects, nil
}

// Member is a bowrain workspace member.
type Member struct {
	UserID      string `json:"user_id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Role        string `json:"role"`
	JoinedAt    string `json:"joined_at,omitempty"`
}

// ListMembers returns members of a workspace.
func (c *BowrainClient) ListMembers(ctx context.Context, wsSlug string) ([]Member, error) {
	data, err := c.get(ctx, fmt.Sprintf("/api/v1/workspaces/%s/members", wsSlug), nil)
	if err != nil {
		return nil, err
	}
	var members []Member
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, fmt.Errorf("decode members: %w", err)
	}
	return members, nil
}

// AuditEntry is a bowrain audit log entry.
type AuditEntry struct {
	ID        int64  `json:"id"`
	ProjectID string `json:"project_id,omitempty"`
	EventType string `json:"event_type"`
	Actor     string `json:"actor"`
	Source    string `json:"source,omitempty"`
	Data      string `json:"data,omitempty"`
	CreatedAt string `json:"created_at"`
}

// ListAuditLog returns recent audit log entries for a workspace.
func (c *BowrainClient) ListAuditLog(ctx context.Context, wsSlug string, limit int) ([]AuditEntry, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	data, err := c.get(ctx, fmt.Sprintf("/api/v1/workspaces/%s/audit-log", wsSlug), q)
	if err != nil {
		return nil, err
	}
	var entries []AuditEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("decode audit log: %w", err)
	}
	return entries, nil
}

// Block is a bowrain translation block.
type Block struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target,omitempty"`
	Status string `json:"status,omitempty"`
}

// BlockListOptions controls block listing.
type BlockListOptions struct {
	Locale string
	Status string
	Limit  int
}

// ListBlocks returns translation blocks for a project.
func (c *BowrainClient) ListBlocks(ctx context.Context, wsSlug, projectID string, opts BlockListOptions) ([]Block, error) {
	q := url.Values{}
	if opts.Locale != "" {
		q.Set("locale", opts.Locale)
	}
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	data, err := c.get(ctx, fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/sync/blocks", wsSlug, projectID), q)
	if err != nil {
		return nil, err
	}
	var blocks []Block
	if err := json.Unmarshal(data, &blocks); err != nil {
		return nil, fmt.Errorf("decode blocks: %w", err)
	}
	return blocks, nil
}
