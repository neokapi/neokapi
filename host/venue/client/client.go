package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/storage/compression"
)

// defaultHTTPClient is the http.Client used by package-level helper functions.
// Override in tests to avoid real network calls.
var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

// StreamHeader is an HTTP header name for the active stream. No request in
// this package sends or reads it: every sync and asset route carries the
// stream as the `:ref` path segment (see streamPrefix and assetPrefix).
const StreamHeader = "X-Bowrain-Stream"

// BowrainClient is a REST client for the Bowrain server sync API.
// It supports two auth modes:
//   - ClaimToken: unclaimed project, flat routes /api/v1/projects/:id/sync/*
//   - JWT + workspace: workspace project, routes /api/v1/:ws/:id/sync/:ref/*
//
// It is bound to one project for its whole life: the project id and workspace
// are constructor arguments, not per-call ones.
type BowrainClient struct {
	*Transport

	projectID string
	workspace string // workspace slug; empty for unclaimed (ClaimToken) projects
	stream    string // active stream name; empty or "main" means default

	compressor *compression.Pool // optional zstd compressor for chunk upload
}

// NewWorkspaceBowrainClient creates a client that uses workspace-scoped routes with auth.
func NewWorkspaceBowrainClient(serverURL, workspace, projectID, authToken string) *BowrainClient {
	return &BowrainClient{
		Transport: NewTransport(serverURL, authToken),
		projectID: projectID,
		workspace: workspace,
	}
}

// NewClaimTokenClient creates a client that uses claim token auth for anonymous projects.
func NewClaimTokenClient(serverURL, projectID, claimToken string) *BowrainClient {
	t := NewTransport(serverURL, "")
	t.claimToken = claimToken
	return &BowrainClient{
		Transport: t,
		projectID: projectID,
	}
}

// NewProjectBearerClient creates a client that uses bearer auth (JWT or API token)
// with flat project routes (no workspace). This supports CI scenarios where the
// project has been claimed but the local config doesn't store the workspace slug.
func NewProjectBearerClient(serverURL, projectID, authToken string) *BowrainClient {
	return &BowrainClient{
		Transport: NewTransport(serverURL, authToken),
		projectID: projectID,
	}
}

// projectPrefix returns the URL prefix for project-scoped endpoints.
// Bowrain AD-011: workspace project uses bare slug /:ws/:pid, unclaimed uses /projects/:pid.
func (c *BowrainClient) projectPrefix() string {
	if c.workspace != "" {
		return fmt.Sprintf("%s/api/v1/%s/%s", c.baseURL, c.workspace, c.projectID)
	}
	return fmt.Sprintf("%s/api/v1/projects/%s", c.baseURL, c.projectID)
}

// ref returns the active stream/tag ref, defaulting to "main".
func (c *BowrainClient) ref() string {
	if c.stream != "" {
		return url.PathEscape(c.stream)
	}
	return "main"
}

// streamPrefix returns the URL prefix for sync-scoped endpoints.
// Bowrain AD-011: resource-first ref pattern — /:ws/:pid/sync/:ref
func (c *BowrainClient) streamPrefix() string {
	return c.projectPrefix() + "/sync/" + c.ref()
}

// assetPrefix returns the URL prefix for asset-scoped endpoints.
// Bowrain AD-011: resource-first ref pattern — /:ws/:pid/assets/:ref
func (c *BowrainClient) assetPrefix() string {
	return c.projectPrefix() + "/assets/" + c.ref()
}

// SetStream sets the active stream for all subsequent requests.
// Empty or "main" means the default stream.
func (c *BowrainClient) SetStream(stream string) {
	c.stream = stream
}

// EnableCompression enables zstd compression for chunk uploads.
// Call with nil dict for default compression, or pass a trained dictionary.
func (c *BowrainClient) EnableCompression(dict []byte) {
	c.compressor = compression.NewPool(dict)
}

// Stream returns the current active stream name.
func (c *BowrainClient) Stream() string {
	if c.stream == "" {
		return "main"
	}
	return c.stream
}

// SyncPushRequest is the request body for pushing blocks.
type SyncPushRequest struct {
	Blocks []BlockInput `json:"blocks"`
	Items  []ItemMeta   `json:"items,omitempty"`
}

// ItemMeta carries per-item editor metadata generated during push.
//
// The JSON tags are not free: on the sync path this struct is marshaled into
// the commit manifest and read back by the worker as a pb.SyncItemMeta, so a
// tag that does not match the generated one silently drops the field. That is
// what happened to BlockIndex, which travelled as "block_index" against the
// proto's "block_index_json" and never reached a stored item. Keep the tags
// aligned with core/proto/sync/v1.SyncItemMeta.
type ItemMeta struct {
	Name        string `json:"name"`                       // item name (relative file path)
	Format      string `json:"format"`                     // detected format
	BlockIndex  string `json:"block_index_json,omitempty"` // JSON-serialized BlockIndex
	PreviewHTML string `json:"preview_html,omitempty"`     // pre-rendered editor preview HTML

	// Collection names the recipe collection whose glob claims this item's
	// path — the linkage that gives the item a place in the project's declared
	// structure. Empty when no collection claims it: an ad-hoc file syncs
	// ungrouped, governed by the project's default point.
	Collection string `json:"collection,omitempty"`
}

// BlockInput represents a block in the API.
type BlockInput struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	Name       string `json:"name,omitempty"`
	Type       string `json:"type,omitempty"`
	ItemName   string `json:"item_name,omitempty"`
	Collection string `json:"collection,omitempty"`
}

// SyncPushResponse is the response from a push.
type SyncPushResponse struct {
	Stored    int    `json:"stored"`
	NewCursor int64  `json:"new_cursor"`
	PushID    string `json:"push_id,omitempty"`

	// UndeclaredCollections carries forward what the init negotiation reported:
	// recipe-owned collections the server holds that this push no longer
	// declares. Not a server field on the commit response — Push copies it here
	// so one return value carries the whole push's outcome.
	UndeclaredCollections []string `json:"-"`

	// BlocksUploaded and ChunkCount are what this push actually put on the
	// wire after diff negotiation — filled in by Push, not by the server.
	// Distinct from the caller's changed-block count: a push that carried 75k
	// candidate blocks and uploaded none is a bug the changed count hides.
	BlocksUploaded int `json:"-"`
	ChunkCount     int `json:"-"`

	// ServerRef is the freshness ref the init negotiation reported, carried
	// forward by Push like UndeclaredCollections. It is what a project pushing
	// before it has ever pulled learns about the governance it did NOT write:
	// the components this push put in force are its own to record, and this
	// fills in the rest without a second round trip.
	ServerRef *ref.Ref `json:"-"`
}

// ChangeEntry represents a single change log entry from the server.
type ChangeEntry struct {
	Seq         int64     `json:"seq"`
	BlockID     string    `json:"block_id"`
	ChangeType  string    `json:"change_type"`
	Locale      string    `json:"locale,omitempty"`
	ContentHash string    `json:"content_hash,omitempty"`
	LoggedAt    time.Time `json:"logged_at,omitzero"`
}

// SyncPullResponse is the response from a pull.
type SyncPullResponse struct {
	Changes   []ChangeEntry `json:"changes"`
	NewCursor int64         `json:"new_cursor"`
	HasMore   bool          `json:"has_more"`
}

// PushStatusResponse is the response from the push status endpoint.
type PushStatusResponse struct {
	PushID     string `json:"push_id"`
	Status     string `json:"status"` // "in_progress", "completed", "failed"
	Total      int    `json:"total"`
	Completed  int    `json:"completed"`
	Failed     int    `json:"failed"`
	InProgress int    `json:"in_progress"`
}

// BlockContent represents a block with its translations from the server.
type BlockContent struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	ItemName string            `json:"item_name"`
	Source   string            `json:"source"`
	Targets  map[string]string `json:"targets"` // locale → plain text
}

// Pull fetches full blocks, terms, and media from the server since the given cursor.
// Returns a RichPullResponse with structured SyncBlock records. The response is
// automatically decompressed when the server returns zstd-compressed data.
func (c *BowrainClient) Pull(ctx context.Context, cursor int64, locales []string, limit int) (*RichPullResponse, error) {
	u, err := url.Parse(c.streamPrefix() + "/pull")
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
	req.Header.Set("Accept-Encoding", "zstd, gzip")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("pull", resp.StatusCode, respBody)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read pull response: %w", err)
	}

	// Decompress zstd if the server compressed the response.
	if resp.Header.Get("Content-Encoding") == "zstd" {
		decompressor := compression.NewPool(nil)
		body, err = decompressor.Decompress(body)
		if err != nil {
			return nil, fmt.Errorf("decompress pull response: %w", err)
		}
	}

	var result RichPullResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode pull response: %w", err)
	}
	return &result, nil
}

// PushStatus checks the status of auto-triggered translation jobs for a push.
func (c *BowrainClient) PushStatus(ctx context.Context, pushID string) (*PushStatusResponse, error) {
	u, err := url.Parse(c.streamPrefix() + "/status")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	q := u.Query()
	q.Set("push_id", pushID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("push status request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("push status", resp.StatusCode, respBody)
	}

	var result PushStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode push status response: %w", err)
	}
	return &result, nil
}

// GetBlocks fetches blocks for a specific item (source file) with full structured
// content including segments, spans, annotations, and metadata.
func (c *BowrainClient) GetBlocks(ctx context.Context, itemName string) ([]SyncBlock, error) {
	u, err := url.Parse(c.streamPrefix() + "/blocks")
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

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get blocks request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("get blocks", resp.StatusCode, respBody)
	}

	var blocks []SyncBlock
	if err := json.NewDecoder(resp.Body).Decode(&blocks); err != nil {
		return nil, fmt.Errorf("decode blocks response: %w", err)
	}
	return blocks, nil
}

// ProjectMetadata contains server-side project metadata fetched during sync.
type ProjectMetadata struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	DefaultSourceLanguage string   `json:"default_source_language"`
	TargetLanguages       []string `json:"target_languages"`
}

// GetProjectMetadata fetches project metadata from the server.
// This reuses the existing GET /projects/:id endpoint.
func (c *BowrainClient) GetProjectMetadata(ctx context.Context) (*ProjectMetadata, error) {
	u := c.projectPrefix()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get project metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("get project metadata", resp.StatusCode, respBody)
	}

	var meta ProjectMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("decode project metadata: %w", err)
	}
	return &meta, nil
}

// CreateAnonymousProject creates a new anonymous project on a Bowrain server.
// No authentication is required. Returns the project ID and claim token.
// If email is non-empty, the server sends a claim email to that address.
// targetLocales may be empty (server treats as dynamic).
func CreateAnonymousProject(ctx context.Context, serverURL, name, sourceLocale string, targetLocales []string, email string) (projectID, claimToken string, err error) {
	payload := map[string]any{
		"name":                    name,
		"default_source_language": sourceLocale,
	}
	if len(targetLocales) > 0 {
		payload["target_languages"] = targetLocales
	}
	if email != "" {
		payload["email"] = email
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	u := strings.TrimRight(serverURL, "/") + "/api/v1/projects/anonymous"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("create anonymous project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", NewStatusError("create anonymous project", resp.StatusCode, respBody)
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
func CreateAuthenticatedProject(ctx context.Context, serverURL, token, name, sourceLocale string, targetLocales []string, workspace string) (projectID, workspaceSlug string, err error) {
	// AD-011: authenticated projects are created under the workspace-scoped
	// collection (POST /api/v1/:ws/projects). There is no flat /api/v1/projects
	// create route, so resolve the caller's workspace when one isn't supplied
	// (single workspace → use it; otherwise the first — callers may pass an
	// explicit slug to target a specific one).
	if workspace == "" {
		wss, werr := ListWorkspaces(ctx, serverURL, token)
		if werr != nil {
			return "", "", fmt.Errorf("resolve workspace: %w", werr)
		}
		if len(wss) == 0 {
			return "", "", errors.New("no workspace available for this account — create one first (kapi workspace create)")
		}
		workspace = wss[0].Slug
	}

	payload := map[string]any{
		"name":                    name,
		"default_source_language": sourceLocale,
	}
	if len(targetLocales) > 0 {
		payload["target_languages"] = targetLocales
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	u := strings.TrimRight(serverURL, "/") + "/api/v1/" + url.PathEscape(workspace) + "/projects"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("create project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", NewStatusError("create project", resp.StatusCode, respBody)
	}

	var result struct {
		ID            string `json:"id"`
		WorkspaceSlug string `json:"workspace_slug"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}

	// The workspace-scoped create route may not echo the slug (it's in the URL);
	// fall back to the workspace we resolved/targeted.
	slug := result.WorkspaceSlug
	if slug == "" {
		slug = workspace
	}
	return result.ID, slug, nil
}

// WorkspaceInfo contains basic workspace metadata returned by ListWorkspaces.
type WorkspaceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Type string `json:"type"`
}

// ListWorkspaces returns the workspaces accessible to the authenticated user.
func ListWorkspaces(ctx context.Context, serverURL, token string) ([]WorkspaceInfo, error) {
	u := strings.TrimRight(serverURL, "/") + "/api/v1/workspaces"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("list workspaces", resp.StatusCode, respBody)
	}

	var result []WorkspaceInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result, nil
}

// ProjectInfo contains project metadata returned by GetProject.
type ProjectInfo struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	DefaultSourceLanguage string   `json:"default_source_language"`
	TargetLanguages       []string `json:"target_languages"`
	WorkspaceID           string   `json:"workspace_id"`
}

// GetProject retrieves a project by ID.
func GetProject(ctx context.Context, serverURL, token, projectID string) (*ProjectInfo, error) {
	u := fmt.Sprintf("%s/api/v1/projects/%s", strings.TrimRight(serverURL, "/"), projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("get project", resp.StatusCode, respBody)
	}

	var result ProjectInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// DeleteProject deletes a project by ID.
func DeleteProject(ctx context.Context, serverURL, token, projectID string) error {
	u := fmt.Sprintf("%s/api/v1/projects/%s", strings.TrimRight(serverURL, "/"), projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return NewStatusError("delete project", resp.StatusCode, respBody)
	}
	return nil
}

// ClaimProjectResponse is returned after claiming an anonymous project.
type ClaimProjectResponse struct {
	ProjectID     string `json:"project_id"`
	WorkspaceSlug string `json:"workspace_slug"`
}

// ClaimProject moves an anonymous project into the user's workspace.
func ClaimProject(ctx context.Context, serverURL, token, claimToken string) (*ClaimProjectResponse, error) {
	payload := map[string]string{"claim_token": claimToken}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	u := strings.TrimRight(serverURL, "/") + "/api/v1/projects/claim"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claim project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("claim project", resp.StatusCode, respBody)
	}

	var result ClaimProjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// JoinWorkspace accepts a workspace invite code and joins the workspace.
func JoinWorkspace(ctx context.Context, serverURL, token, inviteCode string) error {
	u := fmt.Sprintf("%s/api/v1/join/%s", strings.TrimRight(serverURL, "/"), inviteCode)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("join workspace: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return NewStatusError("join workspace", resp.StatusCode, respBody)
	}
	return nil
}

// CreateWorkspace creates a new team workspace on the server and returns it.
func CreateWorkspace(ctx context.Context, serverURL, token, name, slug string) (*WorkspaceInfo, error) {
	payload := map[string]string{
		"name": name,
		"slug": slug,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	u := strings.TrimRight(serverURL, "/") + "/api/v1/workspaces"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("create workspace", resp.StatusCode, respBody)
	}

	var result WorkspaceInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// ---------------------------------------------------------------------------
// Stream management
// ---------------------------------------------------------------------------

// StreamInfo represents a stream in the API.
type StreamInfo struct {
	Name        string   `json:"name"`
	Parent      string   `json:"parent"`
	BaseCursor  int64    `json:"base_cursor"`
	Archived    bool     `json:"archived"`
	Visibility  string   `json:"visibility"`
	Description string   `json:"description"`
	CreatedAt   string   `json:"created_at"`
	CreatedBy   string   `json:"created_by"`
	SharedWith  []string `json:"shared_with,omitempty"`
}

// CreateStreamRequest is the request body for creating a stream.
type CreateStreamRequest struct {
	Name        string `json:"name"`
	Parent      string `json:"parent,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	Description string `json:"description,omitempty"`
}

// MergeStreamResponse is the response from merging a stream.
type MergeStreamResponse struct {
	MergedBlocks   int `json:"merged_blocks"`
	AddedBlocks    int `json:"added_blocks"`
	ModifiedBlocks int `json:"modified_blocks"`
	RemovedBlocks  int `json:"removed_blocks"`
}

// BlockChangeInfo represents a single block change in a diff.
type BlockChangeInfo struct {
	BlockID    string `json:"block_id"`
	ChangeType string `json:"change_type"`
	OldHash    string `json:"old_hash,omitempty"`
	NewHash    string `json:"new_hash,omitempty"`
}

// DiffStreamResponse is the response from diffing a stream.
type DiffStreamResponse struct {
	StreamName string            `json:"stream_name"`
	ParentName string            `json:"parent_name"`
	Changes    []BlockChangeInfo `json:"changes"`
}

// ListStreams returns all streams for the current project.
func (c *BowrainClient) ListStreams(ctx context.Context, includeArchived bool) ([]StreamInfo, error) {
	u := c.projectPrefix() + "/streams"
	if includeArchived {
		u += "?include_archived=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("list streams", resp.StatusCode, respBody)
	}

	var streams []StreamInfo
	if err := json.NewDecoder(resp.Body).Decode(&streams); err != nil {
		return nil, fmt.Errorf("decode streams: %w", err)
	}
	return streams, nil
}

// CreateStream creates a new stream.
func (c *BowrainClient) CreateStream(ctx context.Context, csReq CreateStreamRequest) (*StreamInfo, error) {
	body, err := json.Marshal(csReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	u := c.projectPrefix() + "/streams"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("create stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("create stream", resp.StatusCode, respBody)
	}

	var stream StreamInfo
	if err := json.NewDecoder(resp.Body).Decode(&stream); err != nil {
		return nil, fmt.Errorf("decode stream: %w", err)
	}
	return &stream, nil
}

// MergeStream merges a stream into its parent.
func (c *BowrainClient) MergeStream(ctx context.Context, streamName string, dryRun bool) (*MergeStreamResponse, error) {
	payload := map[string]bool{"dry_run": dryRun}
	body, _ := json.Marshal(payload)

	u := c.projectPrefix() + "/streams/" + url.PathEscape(streamName) + "/merge"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("merge stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("merge stream", resp.StatusCode, respBody)
	}

	var mergeResult MergeStreamResponse
	if err := json.NewDecoder(resp.Body).Decode(&mergeResult); err != nil {
		return nil, fmt.Errorf("decode merge result: %w", err)
	}
	return &mergeResult, nil
}

// DiffStream gets the diff between a stream and its parent.
func (c *BowrainClient) DiffStream(ctx context.Context, streamName string) (*DiffStreamResponse, error) {
	u := c.projectPrefix() + "/streams/" + url.PathEscape(streamName) + "/diff"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("diff stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("diff stream", resp.StatusCode, respBody)
	}

	var diffResult DiffStreamResponse
	if err := json.NewDecoder(resp.Body).Decode(&diffResult); err != nil {
		return nil, fmt.Errorf("decode diff: %w", err)
	}
	return &diffResult, nil
}

// ArchiveStream archives a stream.
func (c *BowrainClient) ArchiveStream(ctx context.Context, streamName string) error {
	u := c.projectPrefix() + "/streams/" + url.PathEscape(streamName) + "/archive"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("archive stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return NewStatusError("archive stream", resp.StatusCode, respBody)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Asset sync (Bowrain AD-007)
// ---------------------------------------------------------------------------

// AssetInput represents an asset to push to the server.
type AssetInput struct {
	BlobKey        string            `json:"blob_key"`
	ItemName       string            `json:"item_name"`
	SourceID       string            `json:"source_id"`
	MimeType       string            `json:"mime_type"`
	Filename       string            `json:"filename"`
	SizeBytes      int64             `json:"size_bytes"`
	AltText        string            `json:"alt_text,omitempty"`
	Properties     map[string]string `json:"properties,omitempty"`
	ProcessingHint string            `json:"processing_hint,omitempty"`
}

// AssetUploadURLResponse is the response from the upload-url endpoint.
type AssetUploadURLResponse struct {
	UploadURL string `json:"upload_url,omitempty"`
	Exists    bool   `json:"exists"`
}

// AssetResponse is the API response for an asset.
type AssetResponse struct {
	ID               string            `json:"id"`
	ProjectID        string            `json:"project_id"`
	ItemName         string            `json:"item_name"`
	SourceID         string            `json:"source_id"`
	BlobKey          string            `json:"blob_key"`
	MimeType         string            `json:"mime_type"`
	Filename         string            `json:"filename"`
	SizeBytes        int64             `json:"size_bytes"`
	AltText          string            `json:"alt_text"`
	Properties       map[string]string `json:"properties,omitempty"`
	ProcessingStatus string            `json:"processing_status"`
	DownloadURL      string            `json:"download_url,omitempty"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
}

// AssetListResponse wraps a list of assets.
type AssetListResponse struct {
	Assets []AssetResponse `json:"assets"`
}

// GetAssetUploadURL requests a pre-signed upload URL for a blob.
func (c *BowrainClient) GetAssetUploadURL(ctx context.Context, blobKey, contentType string, size int64) (*AssetUploadURLResponse, error) {
	payload := map[string]any{
		"blob_key":     blobKey,
		"content_type": contentType,
		"size":         size,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal upload-url request: %w", err)
	}

	u := c.assetPrefix() + "/upload-url"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload-url request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("upload-url", resp.StatusCode, respBody)
	}

	var result AssetUploadURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode upload-url response: %w", err)
	}
	return &result, nil
}

// PushAsset registers asset metadata on the server (after blob upload).
func (c *BowrainClient) PushAsset(ctx context.Context, asset AssetInput) (*AssetResponse, error) {
	body, err := json.Marshal(asset)
	if err != nil {
		return nil, fmt.Errorf("marshal asset: %w", err)
	}

	u := c.assetPrefix()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("push asset request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("push asset", resp.StatusCode, respBody)
	}

	var result AssetResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode asset response: %w", err)
	}
	return &result, nil
}

// ListAssets fetches assets for a project, optionally filtered by item name.
func (c *BowrainClient) ListAssets(ctx context.Context, itemName string) ([]AssetResponse, error) {
	u, err := url.Parse(c.assetPrefix())
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if itemName != "" {
		q := u.Query()
		q.Set("item_name", itemName)
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list assets request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("list assets", resp.StatusCode, respBody)
	}

	var result AssetListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode assets response: %w", err)
	}
	return result.Assets, nil
}

// AssetVariantResponse is the API response for a locale variant.
type AssetVariantResponse struct {
	AssetID     string `json:"asset_id"`
	Locale      string `json:"locale"`
	BlobKey     string `json:"blob_key"`
	Status      string `json:"status"`
	MimeType    string `json:"mime_type"`
	SizeBytes   int64  `json:"size_bytes"`
	DownloadURL string `json:"download_url,omitempty"`
}

// AssetVariantListResponse wraps a list of variants.
type AssetVariantListResponse struct {
	Variants []AssetVariantResponse `json:"variants"`
}

// ListAssetVariants fetches locale variants for an asset.
func (c *BowrainClient) ListAssetVariants(ctx context.Context, assetID string) ([]AssetVariantResponse, error) {
	u := c.assetPrefix() + "/" + assetID + "/variants"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list variants request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("list variants", resp.StatusCode, respBody)
	}

	var result AssetVariantListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode variants response: %w", err)
	}
	return result.Variants, nil
}

// DownloadBlob downloads binary content from a URL (SAS URL or server proxy).
// ownAPIPath reports whether a URL addresses this client's own server, and the
// request path when it does. A root-relative URL is ours by construction; an
// absolute one is ours only if it starts at our base.
func (c *BowrainClient) ownAPIPath(downloadURL string) (string, bool) {
	if strings.HasPrefix(downloadURL, "/") {
		return downloadURL, true
	}
	if base := c.BaseURL(); base != "" && strings.HasPrefix(downloadURL, base+"/") {
		return strings.TrimPrefix(downloadURL, base), true
	}
	return "", false
}

func (c *BowrainClient) DownloadBlob(ctx context.Context, downloadURL string) ([]byte, error) {
	// A URL on our own API gets our credential; a URL anywhere else does not.
	//
	// The two are genuinely different things. A pre-signed object-storage URL
	// IS the grant, and attaching an Authorization header to it tells the
	// object store something it has no business seeing — some backends reject
	// the request outright. Our own streaming route, which a store that cannot
	// pre-sign falls back to, is an ordinary authorized API call.
	if path, ours := c.ownAPIPath(downloadURL); ours {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL()+path, nil)
		if err != nil {
			return nil, fmt.Errorf("create download request: %w", err)
		}
		resp, err := c.Do(req)
		if err != nil {
			return nil, fmt.Errorf("download blob: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return nil, NewStatusError("download", resp.StatusCode, body)
		}
		return io.ReadAll(resp.Body)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download blob: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, NewStatusError("download", resp.StatusCode, body)
	}
	return io.ReadAll(resp.Body)
}
