package client

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

	"github.com/neokapi/neokapi/core/model"
)

// This file adds the "editor" surface of the Bowrain REST API to BowrainClient:
// the workspace/project browsing, block editing, translation-memory,
// terminology, and AI-provider operations that the Bowrain desktop app used to
// reach over a bespoke gRPC EditorService client. Unlike the sync surface
// (which is scoped to a single project via the struct's workspace/projectID
// fields), these methods take the workspace slug and project id as explicit
// arguments: the desktop connects to one server but browses many workspaces and
// projects, so a single BowrainClient (constructed with NewEditorClient) serves
// the whole editor UI.
//
// The REST responses use the canonical content model (core/model.Run) for block
// runs, so no bespoke wire encoding is needed — callers convert model.Run to
// their own presentation type directly.

// NewEditorClient creates a client for the workspace/project-browsing editor
// surface. It holds only a base URL and bearer token; workspace and project are
// passed per call, so it is not bound to a single project like the sync clients.
func NewEditorClient(serverURL, authToken string) *BowrainClient {
	return &BowrainClient{
		baseURL:    strings.TrimRight(serverURL, "/"),
		authToken:  authToken,
		httpClient: &http.Client{Timeout: defaultClientTimeout},
	}
}

// SetAuthToken updates the bearer token used for authentication. It is used by
// the desktop after a token refresh or re-login.
func (c *BowrainClient) SetAuthToken(token string) {
	c.authToken = token
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

// EditorBlock mirrors BlockInfoResponse. Runs travel as canonical model.Run.
type EditorBlock struct {
	ID           string                 `json:"id"`
	SourceRuns   []model.Run            `json:"source_runs,omitempty"`
	TargetRuns   map[string][]model.Run `json:"targets_runs,omitempty"`
	Translatable bool                   `json:"translatable"`
	Properties   map[string]string      `json:"properties"`
}

// EditorTMMatch mirrors TMMatchInfoResponse.
type EditorTMMatch struct {
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

// EditorTMEntry mirrors TMEntryInfoResponse. Note the server serializes the
// locales as source_language/target_language.
type EditorTMEntry struct {
	ID             string `json:"id"`
	Source         string `json:"source"`
	Target         string `json:"target"`
	SourceLanguage string `json:"source_language"`
	TargetLanguage string `json:"target_language"`
	UpdatedAt      string `json:"updated_at"`
}

// EditorTMSearchResult mirrors TMSearchResponse.
type EditorTMSearchResult struct {
	Entries    []EditorTMEntry `json:"entries"`
	TotalCount int             `json:"total_count"`
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

// editorDo issues an editor request against an absolute path under the server
// base URL, optionally sending a JSON body and decoding a JSON response. It
// accepts any 2xx status. A nil out skips decoding (for 204 responses).
func (c *BowrainClient) editorDo(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", strings.TrimPrefix(path, "/"), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s failed (HTTP %d): %s", strings.TrimPrefix(path, "/"), resp.StatusCode, string(respBody))
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode %s response: %w", strings.TrimPrefix(path, "/"), err)
		}
	}
	return nil
}

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
func (c *BowrainClient) ListWorkspaces(ctx context.Context) ([]EditorWorkspace, error) {
	var out []EditorWorkspace
	if err := c.editorDo(ctx, http.MethodGet, "/api/v1/workspaces", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListEditorProjects returns the projects in a workspace.
func (c *BowrainClient) ListEditorProjects(ctx context.Context, ws string) ([]EditorProject, error) {
	var out []EditorProject
	if err := c.editorDo(ctx, http.MethodGet, wsPath(ws, "/projects"), nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetEditorProject returns a single project by id.
func (c *BowrainClient) GetEditorProject(ctx context.Context, ws, projectID string) (*EditorProject, error) {
	var out EditorProject
	path := "/api/v1/" + url.PathEscape(ws) + "/" + url.PathEscape(projectID)
	if err := c.editorDo(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// Blocks
// ---------------------------------------------------------------------------

// GetEditorBlocks returns the blocks of an item in a project.
func (c *BowrainClient) GetEditorBlocks(ctx context.Context, ws, projectID, itemName string) ([]EditorBlock, error) {
	q := url.Values{}
	q.Set("item", itemName)
	path := fmt.Sprintf("/api/v1/%s/%s/blocks/%s", url.PathEscape(ws), url.PathEscape(projectID), editorRef)
	var out []EditorBlock
	if err := c.editorDo(ctx, http.MethodGet, path, q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateBlockTargetRuns replaces a block's target with a canonical Run sequence.
func (c *BowrainClient) UpdateBlockTargetRuns(ctx context.Context, ws, projectID, blockID, targetLocale string, runs []model.Run) error {
	body := struct {
		TargetLocale string      `json:"target_locale"`
		Runs         []model.Run `json:"runs"`
	}{TargetLocale: targetLocale, Runs: runs}
	return c.editorDo(ctx, http.MethodPut, blockPath(ws, projectID, blockID, "/runs"), nil, body, nil)
}

// ---------------------------------------------------------------------------
// Per-block TM & term lookup
// ---------------------------------------------------------------------------

// LookupTMForBlock returns TM matches for a block's source.
func (c *BowrainClient) LookupTMForBlock(ctx context.Context, ws, projectID, blockID, targetLocale string) ([]EditorTMMatch, error) {
	q := url.Values{}
	q.Set("target_locale", targetLocale)
	var out []EditorTMMatch
	if err := c.editorDo(ctx, http.MethodGet, blockPath(ws, projectID, blockID, "/tm-matches"), q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LookupTermsForBlock returns term matches for a block's source.
func (c *BowrainClient) LookupTermsForBlock(ctx context.Context, ws, projectID, blockID, targetLocale string) ([]EditorTermMatch, error) {
	q := url.Values{}
	q.Set("target_locale", targetLocale)
	var out []EditorTermMatch
	if err := c.editorDo(ctx, http.MethodGet, blockPath(ws, projectID, blockID, "/term-matches"), q, nil, &out); err != nil {
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
func (c *BowrainClient) UploadItems(ctx context.Context, ws, projectID string, files map[string][]byte) (*EditorProject, error) {
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

	u := c.baseURL + projectPath(ws, projectID, "/items/"+editorRef)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("request items: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("items upload failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var out EditorProject
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode items response: %w", err)
	}
	return &out, nil
}

// RemoveItem deletes an item from a project and returns the refreshed project.
func (c *BowrainClient) RemoveItem(ctx context.Context, ws, projectID, itemName string) (*EditorProject, error) {
	q := url.Values{}
	q.Set("item", itemName)
	var out EditorProject
	if err := c.editorDo(ctx, http.MethodDelete, projectPath(ws, projectID, "/items/"+editorRef), q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PseudoTranslateItem pseudo-translates all blocks in an item on the server.
func (c *BowrainClient) PseudoTranslateItem(ctx context.Context, ws, projectID, itemName, targetLocale string) (*EditorTranslationStats, error) {
	return c.itemStatsAction(ctx, ws, projectID, itemName, "pseudo-translate", targetLocale)
}

// TMTranslateItem leverages the workspace TM to translate an item on the server.
func (c *BowrainClient) TMTranslateItem(ctx context.Context, ws, projectID, itemName, targetLocale string) (*EditorTranslationStats, error) {
	return c.itemStatsAction(ctx, ws, projectID, itemName, "tm-translate", targetLocale)
}

// itemStatsAction issues a bulk item action that returns translation stats.
func (c *BowrainClient) itemStatsAction(ctx context.Context, ws, projectID, itemName, verb, targetLocale string) (*EditorTranslationStats, error) {
	q := url.Values{}
	q.Set("item", itemName)
	body := struct {
		TargetLocale string `json:"target_locale"`
	}{targetLocale}
	var out EditorTranslationStats
	if err := c.editorDo(ctx, http.MethodPost, actionPath(ws, projectID, verb), q, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TermEnforceItem runs the server-owned terminology-enforcement check over an
// item's blocks and returns the violations found.
func (c *BowrainClient) TermEnforceItem(ctx context.Context, ws, projectID, itemName, targetLocale string) ([]EditorTermEnforceResult, error) {
	q := url.Values{}
	q.Set("item", itemName)
	body := struct {
		TargetLocale string `json:"target_locale"`
	}{targetLocale}
	var out []EditorTermEnforceResult
	if err := c.editorDo(ctx, http.MethodPost, actionPath(ws, projectID, "term-enforce"), q, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Translation memory
// ---------------------------------------------------------------------------

// GetTMEntries searches the workspace translation memory.
func (c *BowrainClient) GetTMEntries(ctx context.Context, ws, query, sourceLocale, targetLocale string, offset, limit int) (*EditorTMSearchResult, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("source_locale", sourceLocale)
	q.Set("target_locale", targetLocale)
	q.Set("offset", strconv.Itoa(offset))
	q.Set("limit", strconv.Itoa(limit))
	var out EditorTMSearchResult
	if err := c.editorDo(ctx, http.MethodGet, wsPath(ws, "/translation-memory"), q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTMCount returns the workspace TM entry count.
func (c *BowrainClient) GetTMCount(ctx context.Context, ws string) (int, error) {
	var out struct {
		Count int `json:"count"`
	}
	if err := c.editorDo(ctx, http.MethodGet, wsPath(ws, "/translation-memory/count"), nil, nil, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

// AddTMEntry adds a TM entry and returns the stored record.
func (c *BowrainClient) AddTMEntry(ctx context.Context, ws, source, target, sourceLocale, targetLocale string) (*EditorTMEntry, error) {
	body := struct {
		Source       string `json:"source"`
		Target       string `json:"target"`
		SourceLocale string `json:"source_locale"`
		TargetLocale string `json:"target_locale"`
	}{source, target, sourceLocale, targetLocale}
	var out EditorTMEntry
	if err := c.editorDo(ctx, http.MethodPost, wsPath(ws, "/translation-memory"), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTMEntry updates an existing TM entry.
func (c *BowrainClient) UpdateTMEntry(ctx context.Context, ws, entryID, source, target, sourceLocale, targetLocale string) error {
	body := struct {
		Source       string `json:"source"`
		Target       string `json:"target"`
		SourceLocale string `json:"source_locale"`
		TargetLocale string `json:"target_locale"`
	}{source, target, sourceLocale, targetLocale}
	return c.editorDo(ctx, http.MethodPut, wsPath(ws, "/translation-memory/"+url.PathEscape(entryID)), nil, body, nil)
}

// DeleteTMEntry deletes a TM entry.
func (c *BowrainClient) DeleteTMEntry(ctx context.Context, ws, entryID string) error {
	return c.editorDo(ctx, http.MethodDelete, wsPath(ws, "/translation-memory/"+url.PathEscape(entryID)), nil, nil, nil)
}

// ---------------------------------------------------------------------------
// Terminology (concepts)
// ---------------------------------------------------------------------------

// GetTerms searches workspace terminology concepts. sourceLocale narrows the
// term-text search to that locale (server "locale" facet).
func (c *BowrainClient) GetTerms(ctx context.Context, ws, query, sourceLocale, targetLocale string, offset, limit int) (*EditorTermSearchResult, error) {
	q := url.Values{}
	q.Set("q", query)
	if sourceLocale != "" {
		q.Set("locale", sourceLocale)
	}
	q.Set("offset", strconv.Itoa(offset))
	q.Set("limit", strconv.Itoa(limit))
	var out EditorTermSearchResult
	if err := c.editorDo(ctx, http.MethodGet, wsPath(ws, "/concepts"), q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTermCount returns the workspace concept count.
func (c *BowrainClient) GetTermCount(ctx context.Context, ws string) (int, error) {
	var out struct {
		Count int `json:"count"`
	}
	if err := c.editorDo(ctx, http.MethodGet, wsPath(ws, "/concepts/count"), nil, nil, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

// EditorAddConcept creates a workspace-scoped terminology concept.
func (c *BowrainClient) EditorAddConcept(ctx context.Context, ws, domain, definition string, terms []EditorTerm) (*EditorConcept, error) {
	body := struct {
		Domain     string       `json:"domain"`
		Definition string       `json:"definition"`
		Terms      []EditorTerm `json:"terms"`
	}{domain, definition, terms}
	var out EditorConcept
	if err := c.editorDo(ctx, http.MethodPost, wsPath(ws, "/concepts"), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EditorUpdateConcept updates a concept's domain, definition, and terms.
func (c *BowrainClient) EditorUpdateConcept(ctx context.Context, ws, conceptID, domain, definition string, terms []EditorTerm) error {
	body := struct {
		Domain     string       `json:"domain"`
		Definition string       `json:"definition"`
		Terms      []EditorTerm `json:"terms"`
	}{domain, definition, terms}
	return c.editorDo(ctx, http.MethodPut, wsPath(ws, "/concepts/"+url.PathEscape(conceptID)), nil, body, nil)
}

// EditorDeleteConcept deletes a concept.
func (c *BowrainClient) EditorDeleteConcept(ctx context.Context, ws, conceptID string) error {
	return c.editorDo(ctx, http.MethodDelete, wsPath(ws, "/concepts/"+url.PathEscape(conceptID)), nil, nil, nil)
}

// ImportTermsCSV imports terminology from CSV and returns the imported count.
func (c *BowrainClient) ImportTermsCSV(ctx context.Context, ws, csvContent, sourceLocale, targetLocale, domain string, hasHeader bool) (int, error) {
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
	if err := c.editorDo(ctx, http.MethodPost, wsPath(ws, "/concepts/import/csv"), nil, body, &out); err != nil {
		return 0, err
	}
	return out.Imported, nil
}

// ImportTermsJSON imports terminology from JSON and returns the imported count.
func (c *BowrainClient) ImportTermsJSON(ctx context.Context, ws, jsonContent string) (int, error) {
	body := struct {
		JSONContent string `json:"json_content"`
	}{jsonContent}
	var out struct {
		Imported int `json:"imported"`
	}
	if err := c.editorDo(ctx, http.MethodPost, wsPath(ws, "/concepts/import/json"), nil, body, &out); err != nil {
		return 0, err
	}
	return out.Imported, nil
}

// ExportTermsJSON exports the workspace terminology as a JSON document.
func (c *BowrainClient) ExportTermsJSON(ctx context.Context, ws, name string) (string, error) {
	q := url.Values{}
	if name != "" {
		q.Set("name", name)
	}
	u := c.baseURL + wsPath(ws, "/concepts/export/json")
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	resp, err := c.doRequest(req)
	if err != nil {
		return "", fmt.Errorf("request concepts/export/json: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("concepts/export/json failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return string(respBody), nil
}

// ---------------------------------------------------------------------------
// AI provider configuration
// ---------------------------------------------------------------------------

// ListProviderConfigs returns the workspace AI provider configs (never keys).
func (c *BowrainClient) ListProviderConfigs(ctx context.Context, ws string) ([]EditorProviderConfig, error) {
	var out []EditorProviderConfig
	if err := c.editorDo(ctx, http.MethodGet, wsPath(ws, "/providers"), nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveProviderConfig creates or updates an AI provider config.
func (c *BowrainClient) SaveProviderConfig(ctx context.Context, ws string, req EditorSaveProviderRequest) (*EditorProviderConfig, error) {
	var out EditorProviderConfig
	if err := c.editorDo(ctx, http.MethodPost, wsPath(ws, "/providers"), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteProviderConfig removes an AI provider config.
func (c *BowrainClient) DeleteProviderConfig(ctx context.Context, ws, id string) error {
	return c.editorDo(ctx, http.MethodDelete, wsPath(ws, "/providers/"+url.PathEscape(id)), nil, nil, nil)
}

// TestProviderConfig verifies an AI provider config can reach its backend.
func (c *BowrainClient) TestProviderConfig(ctx context.Context, ws string, req EditorSaveProviderRequest) error {
	return c.editorDo(ctx, http.MethodPost, wsPath(ws, "/providers/test"), nil, req, nil)
}
