// Package editorclient is the REST/SSE client for the Bowrain editor surface:
// workspace and project browsing, block editing and review, content memory,
// terminology, and AI-provider configuration.
//
// It is the multi-user, governed half of the server API — the surface that
// drives collaborative review, presence and bulk approval — and is therefore
// distinct from the venue sync client, which drives one project's local work.
// The two share only HTTP plumbing, through an embedded client.Transport.
package editorclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/core/model"
)

// EditorClient reaches the editor surface of the Bowrain REST API. Unlike the
// sync client (which is scoped to a single project by its constructor), its
// methods take the workspace slug and project id as explicit arguments: the
// desktop connects to one server but browses many workspaces and projects, so a
// single EditorClient serves the whole editor UI.
//
// The REST responses use the canonical content model (core/model.Run) for block
// runs, so no bespoke wire encoding is needed — callers convert model.Run to
// their own presentation type directly.
type EditorClient struct {
	*client.Transport
}

// New creates a client for the workspace/project-browsing editor surface. It
// holds only a base URL and bearer token; workspace and project are passed per
// call, so it is not bound to a single project like the sync clients.
func New(serverURL, authToken string) *EditorClient {
	return &EditorClient{Transport: client.NewTransport(serverURL, authToken)}
}

// ---------------------------------------------------------------------------
// Editor response/request types (mirror the server/editor.go REST shapes)
// ---------------------------------------------------------------------------

// EditorWorkspace mirrors a workspace from GET /api/v1/workspaces.
type EditorWorkspace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	LogoURL     string `json:"logo_url"`
	Role        string `json:"role"`
}

// EditorProjectItem mirrors ProjectItemResponse.
type EditorProjectItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Format     string `json:"format"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	BlockCount int    `json:"block_count"`
	WordCount  int    `json:"word_count"`
}

// EditorProject mirrors the subset of ProjectInfoResponse the editor consumes.
type EditorProject struct {
	ID                    string              `json:"id"`
	Name                  string              `json:"name"`
	DefaultSourceLanguage string              `json:"default_source_language"`
	TargetLanguages       []string            `json:"target_languages"`
	Items                 []EditorProjectItem `json:"items"`
	CreatedAt             string              `json:"created_at"`
	ModifiedAt            string              `json:"modified_at"`
}

// EditorTranslationStats mirrors TranslationStatsResponse — the result of a
// bulk item action (pseudo-translate, tm-translate).
type EditorTranslationStats struct {
	TotalBlocks      int `json:"total_blocks"`
	TranslatedBlocks int `json:"translated_blocks"`
	WordCount        int `json:"word_count"`
}

// EditorTermEnforceResult mirrors TermEnforceResultResponse — one terminology
// violation reported by the server-owned term-enforce action.
type EditorTermEnforceResult struct {
	BlockID      string   `json:"block_id"`
	SourceTerm   string   `json:"source_term"`
	ConceptID    string   `json:"concept_id"`
	Expected     []string `json:"expected"`
	SourceText   string   `json:"source_text"`
	TargetText   string   `json:"target_text"`
	SourceLocale string   `json:"source_locale"`
	TargetLocale string   `json:"target_locale"`
}

// EditorBlockTarget mirrors BlockTargetInfo: one locale's committed target
// text plus its per-locale review status (the model.Target.Status ladder,
// "" | draft | translated | reviewed | signed-off). The desktop reads review
// state from here — the legacy block-global Properties["translation-status"]
// is write-never on the server and only a read fallback for old blocks.
type EditorBlockTarget struct {
	Text   string `json:"text"`
	Status string `json:"status,omitempty"`
}

// EditorBlock mirrors BlockInfoResponse. Runs travel as canonical model.Run;
// Targets carries each locale's committed text and per-locale review status.
type EditorBlock struct {
	ID           string                       `json:"id"`
	SourceRuns   []model.Run                  `json:"source_runs,omitempty"`
	Targets      map[string]EditorBlockTarget `json:"targets,omitempty"`
	TargetRuns   map[string][]model.Run       `json:"targets_runs,omitempty"`
	Translatable bool                         `json:"translatable"`
	Properties   map[string]string            `json:"properties"`
}

// EditorMemoryMatch mirrors MemoryMatchInfoResponse.
type EditorMemoryMatch struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"`
}

// EditorTermMatch mirrors BlockTermMatchResponse.
type EditorTermMatch struct {
	SourceTerm  string   `json:"source_term"`
	TargetTerms []string `json:"target_terms"`
	Domain      string   `json:"domain"`
	Status      string   `json:"status"`
	Start       int      `json:"start"`
	End         int      `json:"end"`
}

// EditorMemoryEntry mirrors MemoryEntryInfoResponse. Note the server serializes the
// locales as source_language/target_language.
type EditorMemoryEntry struct {
	ID             string `json:"id"`
	Source         string `json:"source"`
	Target         string `json:"target"`
	SourceLanguage string `json:"source_language"`
	TargetLanguage string `json:"target_language"`
	UpdatedAt      string `json:"updated_at"`
}

// EditorMemorySearchResult mirrors MemorySearchResponse.
type EditorMemorySearchResult struct {
	Entries    []EditorMemoryEntry `json:"entries"`
	TotalCount int                 `json:"total_count"`
}

// EditorTerm mirrors TermInfoResponse.
type EditorTerm struct {
	Text         string `json:"text"`
	Locale       string `json:"locale"`
	Status       string `json:"status"`
	PartOfSpeech string `json:"part_of_speech,omitempty"`
	Gender       string `json:"gender,omitempty"`
	Note         string `json:"note,omitempty"`
}

// EditorConcept mirrors ConceptInfoResponse.
type EditorConcept struct {
	ID         string            `json:"id"`
	Domain     string            `json:"domain"`
	Definition string            `json:"definition"`
	Terms      []EditorTerm      `json:"terms"`
	Properties map[string]string `json:"properties,omitempty"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
}

// EditorTermSearchResult mirrors TermSearchResponse.
type EditorTermSearchResult struct {
	Concepts   []EditorConcept `json:"concepts"`
	TotalCount int             `json:"total_count"`
}

// EditorProviderConfig mirrors ProviderConfigResponse (never carries the key).
type EditorProviderConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url"`
}

// EditorSaveProviderRequest mirrors SaveProviderConfigRequest.
type EditorSaveProviderRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
}

// ---------------------------------------------------------------------------
// HTTP plumbing for the editor surface
// ---------------------------------------------------------------------------

// editorRef is the stream/tag ref the desktop editor operates on. The gRPC
// editor never selected a stream, so the REST calls target the default "main".
const editorRef = "main"

// wsPath builds /api/v1/:ws/<suffix> with the slug path-escaped.
func wsPath(ws, suffix string) string {
	return "/api/v1/" + url.PathEscape(ws) + suffix
}

// blockPath builds /api/v1/:ws/:pid/blocks/main/:bid<suffix>.
func blockPath(ws, projectID, blockID, suffix string) string {
	return fmt.Sprintf("/api/v1/%s/%s/blocks/%s/%s%s",
		url.PathEscape(ws), url.PathEscape(projectID), editorRef, url.PathEscape(blockID), suffix)
}

// ---------------------------------------------------------------------------
// Workspaces & projects
// ---------------------------------------------------------------------------

// ListWorkspaces returns all workspaces the authenticated user belongs to.
func (c *EditorClient) ListWorkspaces(ctx context.Context) ([]EditorWorkspace, error) {
	var out []EditorWorkspace
	if err := c.DoJSON(ctx, http.MethodGet, "/api/v1/workspaces", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListEditorProjects returns the projects in a workspace.
func (c *EditorClient) ListEditorProjects(ctx context.Context, ws string) ([]EditorProject, error) {
	var out []EditorProject
	if err := c.DoJSON(ctx, http.MethodGet, wsPath(ws, "/projects"), nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetEditorProject returns a single project by id.
func (c *EditorClient) GetEditorProject(ctx context.Context, ws, projectID string) (*EditorProject, error) {
	var out EditorProject
	path := "/api/v1/" + url.PathEscape(ws) + "/" + url.PathEscape(projectID)
	if err := c.DoJSON(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// Blocks
// ---------------------------------------------------------------------------

// GetEditorBlocks returns the blocks of an item in a project.
func (c *EditorClient) GetEditorBlocks(ctx context.Context, ws, projectID, itemName string) ([]EditorBlock, error) {
	q := url.Values{}
	q.Set("item", itemName)
	path := fmt.Sprintf("/api/v1/%s/%s/blocks/%s", url.PathEscape(ws), url.PathEscape(projectID), editorRef)
	var out []EditorBlock
	if err := c.DoJSON(ctx, http.MethodGet, path, q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// EditorBlockQuery narrows QueryEditorBlocks. Every field is optional: the
// zero value pages the project's blocks unfiltered. Status is one of
// "not-started", "draft", "translated", "reviewed" and needs Locale, which
// also scopes the target side of Text.
type EditorBlockQuery struct {
	ItemName     string
	Locale       string
	Status       string
	Text         string
	Translatable *bool
	Limit        int
	Offset       int
}

func (q EditorBlockQuery) values() url.Values {
	v := url.Values{}
	if q.ItemName != "" {
		v.Set("item", q.ItemName)
	}
	if q.Locale != "" {
		v.Set("locale", q.Locale)
	}
	if q.Status != "" {
		v.Set("status", q.Status)
	}
	if q.Text != "" {
		v.Set("q", q.Text)
	}
	if q.Translatable != nil {
		v.Set("translatable", strconv.FormatBool(*q.Translatable))
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Offset > 0 {
		v.Set("offset", strconv.Itoa(q.Offset))
	}
	return v
}

// QueryEditorBlocks returns one filtered page of a project's blocks.
func (c *EditorClient) QueryEditorBlocks(ctx context.Context, ws, projectID string, q EditorBlockQuery) ([]EditorBlock, error) {
	path := fmt.Sprintf("/api/v1/%s/%s/blocks/%s", url.PathEscape(ws), url.PathEscape(projectID), editorRef)
	var out []EditorBlock
	if err := c.DoJSON(ctx, http.MethodGet, path, q.values(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetEditorBlock returns one block, in the same shape QueryEditorBlocks
// returns its elements (GET /api/v1/:ws/:proj/blocks/:stream/:bid).
func (c *EditorClient) GetEditorBlock(ctx context.Context, ws, projectID, blockID string) (*EditorBlock, error) {
	path := fmt.Sprintf("/api/v1/%s/%s/blocks/%s/%s",
		url.PathEscape(ws), url.PathEscape(projectID), editorRef, url.PathEscape(blockID))
	var out EditorBlock
	if err := c.DoJSON(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EditorBlockStatusCounts is the per-locale status histogram.
type EditorBlockStatusCounts struct {
	NotStarted int `json:"not-started"`
	Draft      int `json:"draft"`
	Translated int `json:"translated"`
	Reviewed   int `json:"reviewed"`
}

// EditorBlockCounts mirrors BlockCountsResponse: the totals and histogram for
// a block query, answered without shipping a single block.
type EditorBlockCounts struct {
	Total        int                     `json:"total"`
	Translatable int                     `json:"translatable"`
	Locale       string                  `json:"locale,omitempty"`
	Status       EditorBlockStatusCounts `json:"status"`
}

// GetEditorBlockCounts returns the totals and status histogram for a block
// query (GET /api/v1/:ws/:proj/blocks/:stream/counts). The query's Status is
// ignored — the histogram is what the call reports.
func (c *EditorClient) GetEditorBlockCounts(ctx context.Context, ws, projectID string, q EditorBlockQuery) (*EditorBlockCounts, error) {
	q.Status = ""
	path := fmt.Sprintf("/api/v1/%s/%s/blocks/%s/counts", url.PathEscape(ws), url.PathEscape(projectID), editorRef)
	var out EditorBlockCounts
	if err := c.DoJSON(ctx, http.MethodGet, path, q.values(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EditorItem mirrors ItemInfoResponse: one item's metadata and block tallies.
type EditorItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Format       string `json:"format"`
	Type         string `json:"type"`
	CollectionID string `json:"collection_id,omitempty"`
	BlockCount   int    `json:"block_count"`
	Translatable int    `json:"translatable"`
}

// GetEditorItem returns one item's metadata, without the project's whole item
// list (GET /api/v1/:ws/:proj/items/:stream/one).
func (c *EditorClient) GetEditorItem(ctx context.Context, ws, projectID, itemName string) (*EditorItem, error) {
	q := url.Values{}
	q.Set("item", itemName)
	path := fmt.Sprintf("/api/v1/%s/%s/items/%s/one", url.PathEscape(ws), url.PathEscape(projectID), editorRef)
	var out EditorItem
	if err := c.DoJSON(ctx, http.MethodGet, path, q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EditorBulkReviewRequest applies one review decision to a selection of
// blocks. Status picks the demotion rung when Approve is false: "translated"
// (the default) or "draft", a rejection that re-opens the work.
type EditorBulkReviewRequest struct {
	BlockIDs     []string `json:"block_ids"`
	TargetLocale string   `json:"target_locale"`
	Approve      bool     `json:"approve"`
	Status       string   `json:"status,omitempty"`
	Comment      string   `json:"comment,omitempty"`
	ItemName     string   `json:"item_name,omitempty"`
}

// EditorBlockResult is one block's outcome inside a batch.
type EditorBlockResult struct {
	BlockID string `json:"block_id"`
	OK      bool   `json:"ok"`
	Status  string `json:"status,omitempty"`
	Error   string `json:"error,omitempty"`
}

// EditorBulkReviewResponse mirrors BulkReviewResponse.
type EditorBulkReviewResponse struct {
	Results         []EditorBlockResult `json:"results"`
	Succeeded       int                 `json:"succeeded"`
	Failed          int                 `json:"failed"`
	ReviewCompleted bool                `json:"review_completed"`
}

// BulkReviewBlocks applies one review decision across a selection of blocks in
// a single request (POST /api/v1/:ws/:proj/blocks/:stream/bulk-review). A
// block that refuses is reported in its own result; the call itself succeeds.
func (c *EditorClient) BulkReviewBlocks(ctx context.Context, ws, projectID string, req EditorBulkReviewRequest) (*EditorBulkReviewResponse, error) {
	path := fmt.Sprintf("/api/v1/%s/%s/blocks/%s/bulk-review", url.PathEscape(ws), url.PathEscape(projectID), editorRef)
	var out EditorBulkReviewResponse
	if err := c.DoJSON(ctx, http.MethodPost, path, nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EditorBulkApplyMemoryRequest applies the best content-memory match to a
// selection of blocks. A nil Threshold takes the server default of 1 — an
// exact match.
type EditorBulkApplyMemoryRequest struct {
	BlockIDs     []string `json:"block_ids"`
	TargetLocale string   `json:"target_locale"`
	Threshold    *float64 `json:"threshold,omitempty"`
}

// EditorAppliedMemory names a block that took a match, and what it took.
type EditorAppliedMemory struct {
	BlockID string  `json:"block_id"`
	Text    string  `json:"text"`
	Score   float64 `json:"score"`
}

// EditorSkippedMemory names a block that took nothing, and why.
type EditorSkippedMemory struct {
	BlockID string `json:"block_id"`
	Reason  string `json:"reason"`
}

// EditorBulkApplyMemoryResponse mirrors BulkApplyMemoryResponse.
type EditorBulkApplyMemoryResponse struct {
	Applied []EditorAppliedMemory `json:"applied"`
	Skipped []EditorSkippedMemory `json:"skipped"`
}

// BulkApplyMemory writes the best content-memory match above the threshold
// into each selected block's target, in one request (POST
// /api/v1/:ws/:proj/blocks/:stream/bulk-apply-memory).
func (c *EditorClient) BulkApplyMemory(ctx context.Context, ws, projectID string, req EditorBulkApplyMemoryRequest) (*EditorBulkApplyMemoryResponse, error) {
	path := fmt.Sprintf("/api/v1/%s/%s/blocks/%s/bulk-apply-memory", url.PathEscape(ws), url.PathEscape(projectID), editorRef)
	var out EditorBulkApplyMemoryResponse
	if err := c.DoJSON(ctx, http.MethodPost, path, nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PendingReviewEntry is one entry of the server-side translation review
// queue: a (block, locale) pair awaiting a decision, with its block payload.
type PendingReviewEntry struct {
	BlockID  string       `json:"block_id"`
	ItemName string       `json:"item_name"`
	Locale   string       `json:"locale"`
	Block    *EditorBlock `json:"block,omitempty"`
}

// PendingReviewPage is one page of the queue plus the queue's total size.
type PendingReviewPage struct {
	Entries []PendingReviewEntry `json:"entries"`
	Total   int                  `json:"total"`
	Limit   int                  `json:"limit"`
	Offset  int                  `json:"offset"`
}

// GetPendingReview pages the translation review queue (GET
// /api/v1/:ws/:proj/pending-review/:stream).
func (c *EditorClient) GetPendingReview(ctx context.Context, ws, projectID string, locales []string, limit, offset int) (*PendingReviewPage, error) {
	q := url.Values{}
	if len(locales) > 0 {
		q.Set("locales", strings.Join(locales, ","))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	path := fmt.Sprintf("/api/v1/%s/%s/pending-review/%s", url.PathEscape(ws), url.PathEscape(projectID), editorRef)
	var out PendingReviewPage
	if err := c.DoJSON(ctx, http.MethodGet, path, q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateBlockTargetRuns replaces a block's target with a canonical Run sequence.
func (c *EditorClient) UpdateBlockTargetRuns(ctx context.Context, ws, projectID, blockID, targetLocale string, runs []model.Run) error {
	body := struct {
		TargetLocale string      `json:"target_locale"`
		Runs         []model.Run `json:"runs"`
	}{TargetLocale: targetLocale, Runs: runs}
	return c.DoJSON(ctx, http.MethodPut, blockPath(ws, projectID, blockID, "/runs"), nil, body, nil)
}

// ---------------------------------------------------------------------------
// Per-block content memory & term lookup
// ---------------------------------------------------------------------------

// LookupMemoryForBlock returns content-memory matches for a block's source.
func (c *EditorClient) LookupMemoryForBlock(ctx context.Context, ws, projectID, blockID, targetLocale string) ([]EditorMemoryMatch, error) {
	q := url.Values{}
	q.Set("target_locale", targetLocale)
	var out []EditorMemoryMatch
	if err := c.DoJSON(ctx, http.MethodGet, blockPath(ws, projectID, blockID, "/tm-matches"), q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LookupTermsForBlock returns term matches for a block's source.
func (c *EditorClient) LookupTermsForBlock(ctx context.Context, ws, projectID, blockID, targetLocale string) ([]EditorTermMatch, error) {
	q := url.Values{}
	q.Set("target_locale", targetLocale)
	var out []EditorTermMatch
	if err := c.DoJSON(ctx, http.MethodGet, blockPath(ws, projectID, blockID, "/term-matches"), q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Item management & bulk actions (Bowrain AD-011: /:ws/:id/items|actions/:ref)
// ---------------------------------------------------------------------------

// projectPath builds /api/v1/:ws/:id<suffix> with slug and id path-escaped.
func projectPath(ws, projectID, suffix string) string {
	return fmt.Sprintf("/api/v1/%s/%s%s", url.PathEscape(ws), url.PathEscape(projectID), suffix)
}

// actionPath builds /api/v1/:ws/:id/actions/main/<verb>.
func actionPath(ws, projectID, verb string) string {
	return projectPath(ws, projectID, "/actions/"+editorRef+"/"+verb)
}

// UploadItems uploads one or more files into a project as items (server-side
// parse + block extraction) and returns the refreshed project. The desktop
// routes AddItems through this so local file ingest reaches the server, which
// owns the authoritative content store.
func (c *EditorClient) UploadItems(ctx context.Context, ws, projectID string, files map[string][]byte) (*EditorProject, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, data := range files {
		fw, err := mw.CreateFormFile("files", name)
		if err != nil {
			return nil, fmt.Errorf("create form file %q: %w", name, err)
		}
		if _, err := fw.Write(data); err != nil {
			return nil, fmt.Errorf("write form file %q: %w", name, err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close multipart: %w", err)
	}

	u := c.BaseURL() + projectPath(ws, projectID, "/items/"+editorRef)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request items: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, client.NewStatusError("items upload", resp.StatusCode, respBody)
	}
	var out EditorProject
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode items response: %w", err)
	}
	return &out, nil
}

// RemoveItem deletes an item from a project and returns the refreshed project.
func (c *EditorClient) RemoveItem(ctx context.Context, ws, projectID, itemName string) (*EditorProject, error) {
	q := url.Values{}
	q.Set("item", itemName)
	var out EditorProject
	if err := c.DoJSON(ctx, http.MethodDelete, projectPath(ws, projectID, "/items/"+editorRef), q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PseudoTranslateItem pseudo-translates all blocks in an item on the server.
func (c *EditorClient) PseudoTranslateItem(ctx context.Context, ws, projectID, itemName, targetLocale string) (*EditorTranslationStats, error) {
	return c.itemStatsAction(ctx, ws, projectID, itemName, "pseudo-translate", targetLocale)
}

// MemoryTranslateItem leverages the workspace content memory to translate an item on the server.
func (c *EditorClient) MemoryTranslateItem(ctx context.Context, ws, projectID, itemName, targetLocale string) (*EditorTranslationStats, error) {
	return c.itemStatsAction(ctx, ws, projectID, itemName, "tm-translate", targetLocale)
}

// itemStatsAction issues a bulk item action that returns translation stats.
func (c *EditorClient) itemStatsAction(ctx context.Context, ws, projectID, itemName, verb, targetLocale string) (*EditorTranslationStats, error) {
	q := url.Values{}
	q.Set("item", itemName)
	body := struct {
		TargetLocale string `json:"target_locale"`
	}{targetLocale}
	var out EditorTranslationStats
	if err := c.DoJSON(ctx, http.MethodPost, actionPath(ws, projectID, verb), q, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TermEnforceItem runs the server-owned terminology-enforcement check over an
// item's blocks and returns the violations found.
func (c *EditorClient) TermEnforceItem(ctx context.Context, ws, projectID, itemName, targetLocale string) ([]EditorTermEnforceResult, error) {
	q := url.Values{}
	q.Set("item", itemName)
	body := struct {
		TargetLocale string `json:"target_locale"`
	}{targetLocale}
	var out []EditorTermEnforceResult
	if err := c.DoJSON(ctx, http.MethodPost, actionPath(ws, projectID, "term-enforce"), q, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Content memory
// ---------------------------------------------------------------------------

// GetMemoryEntries searches the workspace content memory.
func (c *EditorClient) GetMemoryEntries(ctx context.Context, ws, query, sourceLocale, targetLocale string, offset, limit int) (*EditorMemorySearchResult, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("source_locale", sourceLocale)
	q.Set("target_locale", targetLocale)
	q.Set("offset", strconv.Itoa(offset))
	q.Set("limit", strconv.Itoa(limit))
	var out EditorMemorySearchResult
	if err := c.DoJSON(ctx, http.MethodGet, wsPath(ws, "/translation-memory"), q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetMemoryCount returns the workspace content-memory entry count.
func (c *EditorClient) GetMemoryCount(ctx context.Context, ws string) (int, error) {
	var out struct {
		Count int `json:"count"`
	}
	if err := c.DoJSON(ctx, http.MethodGet, wsPath(ws, "/translation-memory/count"), nil, nil, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

// AddMemoryEntry adds a content-memory entry and returns the stored record.
func (c *EditorClient) AddMemoryEntry(ctx context.Context, ws, source, target, sourceLocale, targetLocale string) (*EditorMemoryEntry, error) {
	body := struct {
		Source       string `json:"source"`
		Target       string `json:"target"`
		SourceLocale string `json:"source_locale"`
		TargetLocale string `json:"target_locale"`
	}{source, target, sourceLocale, targetLocale}
	var out EditorMemoryEntry
	if err := c.DoJSON(ctx, http.MethodPost, wsPath(ws, "/translation-memory"), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateMemoryEntry updates an existing content-memory entry.
func (c *EditorClient) UpdateMemoryEntry(ctx context.Context, ws, entryID, source, target, sourceLocale, targetLocale string) error {
	body := struct {
		Source       string `json:"source"`
		Target       string `json:"target"`
		SourceLocale string `json:"source_locale"`
		TargetLocale string `json:"target_locale"`
	}{source, target, sourceLocale, targetLocale}
	return c.DoJSON(ctx, http.MethodPut, wsPath(ws, "/translation-memory/"+url.PathEscape(entryID)), nil, body, nil)
}

// DeleteMemoryEntry deletes a content-memory entry.
func (c *EditorClient) DeleteMemoryEntry(ctx context.Context, ws, entryID string) error {
	return c.DoJSON(ctx, http.MethodDelete, wsPath(ws, "/translation-memory/"+url.PathEscape(entryID)), nil, nil, nil)
}

// ---------------------------------------------------------------------------
// Terminology (concepts)
// ---------------------------------------------------------------------------

// GetTerms searches workspace terminology concepts. sourceLocale narrows the
// term-text search to that locale (server "locale" facet).
func (c *EditorClient) GetTerms(ctx context.Context, ws, query, sourceLocale, targetLocale string, offset, limit int) (*EditorTermSearchResult, error) {
	q := url.Values{}
	q.Set("q", query)
	if sourceLocale != "" {
		q.Set("locale", sourceLocale)
	}
	q.Set("offset", strconv.Itoa(offset))
	q.Set("limit", strconv.Itoa(limit))
	var out EditorTermSearchResult
	if err := c.DoJSON(ctx, http.MethodGet, wsPath(ws, "/concepts"), q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTermCount returns the workspace concept count.
func (c *EditorClient) GetTermCount(ctx context.Context, ws string) (int, error) {
	var out struct {
		Count int `json:"count"`
	}
	if err := c.DoJSON(ctx, http.MethodGet, wsPath(ws, "/concepts/count"), nil, nil, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

// EditorAddConcept creates a workspace-scoped terminology concept.
func (c *EditorClient) EditorAddConcept(ctx context.Context, ws, domain, definition string, terms []EditorTerm) (*EditorConcept, error) {
	body := struct {
		Domain     string       `json:"domain"`
		Definition string       `json:"definition"`
		Terms      []EditorTerm `json:"terms"`
	}{domain, definition, terms}
	var out EditorConcept
	if err := c.DoJSON(ctx, http.MethodPost, wsPath(ws, "/concepts"), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EditorUpdateConcept updates a concept's domain, definition, and terms.
func (c *EditorClient) EditorUpdateConcept(ctx context.Context, ws, conceptID, domain, definition string, terms []EditorTerm) error {
	body := struct {
		Domain     string       `json:"domain"`
		Definition string       `json:"definition"`
		Terms      []EditorTerm `json:"terms"`
	}{domain, definition, terms}
	return c.DoJSON(ctx, http.MethodPut, wsPath(ws, "/concepts/"+url.PathEscape(conceptID)), nil, body, nil)
}

// EditorDeleteConcept deletes a concept.
func (c *EditorClient) EditorDeleteConcept(ctx context.Context, ws, conceptID string) error {
	return c.DoJSON(ctx, http.MethodDelete, wsPath(ws, "/concepts/"+url.PathEscape(conceptID)), nil, nil, nil)
}

// ImportTermsCSV imports terminology from CSV and returns the imported count.
func (c *EditorClient) ImportTermsCSV(ctx context.Context, ws, csvContent, sourceLocale, targetLocale, domain string, hasHeader bool) (int, error) {
	body := struct {
		CSVContent   string `json:"csv_content"`
		SourceLocale string `json:"source_locale"`
		TargetLocale string `json:"target_locale"`
		Domain       string `json:"domain"`
		HasHeader    bool   `json:"has_header"`
	}{csvContent, sourceLocale, targetLocale, domain, hasHeader}
	var out struct {
		Imported int `json:"imported"`
	}
	if err := c.DoJSON(ctx, http.MethodPost, wsPath(ws, "/concepts/import/csv"), nil, body, &out); err != nil {
		return 0, err
	}
	return out.Imported, nil
}

// ImportTermsJSON imports terminology from JSON and returns the imported count.
func (c *EditorClient) ImportTermsJSON(ctx context.Context, ws, jsonContent string) (int, error) {
	body := struct {
		JSONContent string `json:"json_content"`
	}{jsonContent}
	var out struct {
		Imported int `json:"imported"`
	}
	if err := c.DoJSON(ctx, http.MethodPost, wsPath(ws, "/concepts/import/json"), nil, body, &out); err != nil {
		return 0, err
	}
	return out.Imported, nil
}

// ExportTermsJSON exports the workspace terminology as a JSON document.
func (c *EditorClient) ExportTermsJSON(ctx context.Context, ws, name string) (string, error) {
	q := url.Values{}
	if name != "" {
		q.Set("name", name)
	}
	u := c.BaseURL() + wsPath(ws, "/concepts/export/json")
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("request concepts/export/json: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", client.NewStatusError("concepts/export/json", resp.StatusCode, respBody)
	}
	return string(respBody), nil
}

// ---------------------------------------------------------------------------
// AI provider configuration
// ---------------------------------------------------------------------------

// ListProviderConfigs returns the workspace AI provider configs (never keys).
func (c *EditorClient) ListProviderConfigs(ctx context.Context, ws string) ([]EditorProviderConfig, error) {
	var out []EditorProviderConfig
	if err := c.DoJSON(ctx, http.MethodGet, wsPath(ws, "/providers"), nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveProviderConfig creates or updates an AI provider config.
func (c *EditorClient) SaveProviderConfig(ctx context.Context, ws string, req EditorSaveProviderRequest) (*EditorProviderConfig, error) {
	var out EditorProviderConfig
	if err := c.DoJSON(ctx, http.MethodPost, wsPath(ws, "/providers"), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteProviderConfig removes an AI provider config.
func (c *EditorClient) DeleteProviderConfig(ctx context.Context, ws, id string) error {
	return c.DoJSON(ctx, http.MethodDelete, wsPath(ws, "/providers/"+url.PathEscape(id)), nil, nil, nil)
}

// TestProviderConfig verifies an AI provider config can reach its backend.
func (c *EditorClient) TestProviderConfig(ctx context.Context, ws string, req EditorSaveProviderRequest) error {
	return c.DoJSON(ctx, http.MethodPost, wsPath(ws, "/providers/test"), nil, req, nil)
}
