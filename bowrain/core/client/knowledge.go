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

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/termbase"
)

// This file holds the read-only REST client for the brand knowledge graph
// (Bowrain AD-021). The graph lives on the workspace content group
// (/api/v1/:ws/...), not under a project, so every method here requires a
// workspace-scoped client (NewWorkspaceBowrainClient). It mirrors the GET-with-
// JSON-decode pattern of the rest of the client and lets kapi read the governed
// vocabulary through the bowrain plugin — the concept pull folded into
// `kapi pull`/`kapi sync` and the knowledge-graph MCP tools — so offline
// `kapi verify` gates against the same truth.

// ---------------------------------------------------------------------------
// Response DTOs (mirror the server handlers' JSON shapes)
// ---------------------------------------------------------------------------

// TermInfo is one term within a concept, mirroring the server's
// TermInfoResponse. Status is a lifecycle status string (proposed, approved,
// admitted, preferred, deprecated, forbidden); Locale is a BCP-47 tag.
type TermInfo struct {
	Text         string `json:"text"`
	Locale       string `json:"locale"`
	Status       string `json:"status"`
	PartOfSpeech string `json:"part_of_speech,omitempty"`
	Gender       string `json:"gender,omitempty"`
	Note         string `json:"note,omitempty"`
}

// ConceptInfo is a single concept, mirroring the server's ConceptInfoResponse.
// ProjectID is empty for workspace-scoped concepts; timestamps are RFC3339
// strings as emitted by the server DTO.
type ConceptInfo struct {
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id,omitempty"`
	Domain     string            `json:"domain"`
	Definition string            `json:"definition"`
	Terms      []TermInfo        `json:"terms"`
	Properties map[string]string `json:"properties,omitempty"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
}

// ConceptSearchResult is a page of concept search results, mirroring the
// server's TermSearchResponse.
type ConceptSearchResult struct {
	Concepts   []ConceptInfo `json:"concepts"`
	TotalCount int           `json:"total_count"`
}

// ConceptStoryEntry is one event on a concept's merged timeline. Kind
// discriminates the source (revision, observation, comment, changeset); Data
// carries the kind-specific record verbatim.
type ConceptStoryEntry struct {
	Kind    string          `json:"kind"`
	At      time.Time       `json:"at"`
	Actor   string          `json:"actor,omitempty"`
	Summary string          `json:"summary,omitempty"`
	Ref     string          `json:"ref,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ConceptStory is a concept's merged chronological timeline, oldest first.
type ConceptStory struct {
	ConceptID string              `json:"concept_id"`
	Entries   []ConceptStoryEntry `json:"entries"`
}

// GraphVizNode is one concept node in the graph visualization payload.
type GraphVizNode struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Domain    string `json:"domain,omitempty"`
	Status    string `json:"status,omitempty"`
	Source    string `json:"source,omitempty"`
	TermCount int    `json:"term_count"`
}

// GraphVizEdge is one relation edge in the graph visualization payload.
type GraphVizEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
	Note   string `json:"note,omitempty"`
}

// GraphViz is the force-directed graph payload (nodes + edges), mirroring the
// server's GraphVizResponse.
type GraphViz struct {
	Nodes []GraphVizNode `json:"nodes"`
	Edges []GraphVizEdge `json:"edges"`
}

// ChangeSet is the header of a reviewable draft of edits to the graph and brand
// voice vocabulary, mirroring the server's knowledge.ChangeSet. Status moves
// through draft → in_review → approved → merged, or abandoned.
type ChangeSet struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	MergedAt    *time.Time `json:"merged_at,omitempty"`
	MergedBy    string     `json:"merged_by,omitempty"`
}

// ChangeSetOp is one ordered operation within a change-set, mirroring the
// server's knowledge.ChangeSetOp. Payload is the op-specific JSON kept verbatim.
type ChangeSetOp struct {
	WorkspaceID string          `json:"workspace_id"`
	ChangesetID string          `json:"changeset_id"`
	Seq         int64           `json:"seq"`
	Op          string          `json:"op"`
	Payload     json.RawMessage `json:"payload"`
	BaseRev     int64           `json:"base_rev"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
}

// ChangeSetReview records a reviewer's verdict (approve, reject) on a
// change-set, mirroring the server's knowledge.ChangeSetReview.
type ChangeSetReview struct {
	WorkspaceID string    `json:"workspace_id"`
	ChangesetID string    `json:"changeset_id"`
	Reviewer    string    `json:"reviewer"`
	Verdict     string    `json:"verdict"`
	Comment     string    `json:"comment,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Pilot binds a change-set to a project's content stream as a shadow of the
// draft, mirroring the server's knowledge.Pilot.
type Pilot struct {
	WorkspaceID string    `json:"workspace_id"`
	ChangesetID string    `json:"changeset_id"`
	ProjectID   string    `json:"project_id"`
	Stream      string    `json:"stream"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// ChangeSetDetail is a change-set with its ops, reviews, and pilots, plus
// whether it carries a governed op. It mirrors the server's
// ChangeSetDetailResponse, whose embedded change-set is flattened onto the top
// level of the JSON — hence the embedded ChangeSet here.
type ChangeSetDetail struct {
	ChangeSet
	Governed bool              `json:"governed"`
	Ops      []ChangeSetOp     `json:"ops"`
	Reviews  []ChangeSetReview `json:"reviews"`
	Pilots   []Pilot           `json:"pilots"`
}

// ChangeSetImpact is the blast radius of a change-set over stored content,
// mirroring the server's knowledge.ChangeSetImpact: how many (block, locale)
// rows the draft would newly flag or resolve, per project → collection →
// (stream, locale), with a source word count as a re-translation effort proxy
// and a capped sample of affected blocks.
type ChangeSetImpact struct {
	TotalBlocks    int             `json:"total_blocks"`
	AffectedBlocks int             `json:"affected_blocks"`
	NewViolations  int             `json:"new_violations"`
	Resolved       int             `json:"resolved"`
	Words          int             `json:"words"`
	Projects       []ProjectImpact `json:"projects"`
	Samples        []BlockSample   `json:"samples"`
}

// ProjectImpact is the per-project slice of a ChangeSetImpact.
type ProjectImpact struct {
	ProjectID      string             `json:"project_id"`
	ProjectName    string             `json:"project_name"`
	AffectedBlocks int                `json:"affected_blocks"`
	NewViolations  int                `json:"new_violations"`
	Resolved       int                `json:"resolved"`
	Words          int                `json:"words"`
	Collections    []CollectionImpact `json:"collections"`
}

// CollectionImpact is the per-collection slice of a ProjectImpact.
type CollectionImpact struct {
	CollectionID   string         `json:"collection_id"`
	CollectionName string         `json:"collection_name"`
	AffectedBlocks int            `json:"affected_blocks"`
	NewViolations  int            `json:"new_violations"`
	Resolved       int            `json:"resolved"`
	Words          int            `json:"words"`
	Locales        []LocaleImpact `json:"locales"`
}

// LocaleImpact is the per-(stream, locale) leaf of a CollectionImpact.
type LocaleImpact struct {
	Stream         string `json:"stream"`
	Locale         string `json:"locale"`
	AffectedBlocks int    `json:"affected_blocks"`
	NewViolations  int    `json:"new_violations"`
	Resolved       int    `json:"resolved"`
	Words          int    `json:"words"`
}

// BlockSample is one inspectable affected block in an impact report.
type BlockSample struct {
	ProjectID      string `json:"project_id"`
	Stream         string `json:"stream"`
	CollectionID   string `json:"collection_id"`
	CollectionName string `json:"collection_name"`
	Locale         string `json:"locale"`
	ItemName       string `json:"item_name"`
	BlockID        string `json:"block_id"`
	Text           string `json:"text"`
	NewViolations  int    `json:"new_violations,omitempty"`
	Resolved       int    `json:"resolved,omitempty"`
	Occurrences    int    `json:"occurrences,omitempty"`
}

// ListConceptsParams narrows a concept search. All fields are optional; zero
// values are omitted from the query.
type ListConceptsParams struct {
	Query  string // free-text query (q)
	Status string // term lifecycle status facet
	Domain string // subject-field facet
	Market string // market validity-tag facet
	Locale string // source locale to scope the text search to
	Source string // term source facet (terminology, brand_vocabulary)
	Offset int    // page offset
	Limit  int    // page size (server defaults to 50)
}

// GraphParams scopes a graph visualization request. All fields are optional.
type GraphParams struct {
	Focus  string // restrict to the neighborhood of this concept
	Depth  int    // hops from Focus (server defaults to 2)
	Domain string // narrow nodes to this domain
	Status string // narrow nodes to this term status
	AsOf   string // RFC3339 instant scoping relation validity
	Market string // market validity-tag scoping relations
}

// ---------------------------------------------------------------------------
// Methods
// ---------------------------------------------------------------------------

// ListConcepts searches the workspace's concepts, narrowing the page by the
// optional facets in params (GET /api/v1/:ws/concepts).
func (c *BowrainClient) ListConcepts(ctx context.Context, params ListConceptsParams) (*ConceptSearchResult, error) {
	q := url.Values{}
	if params.Query != "" {
		q.Set("q", params.Query)
	}
	if params.Status != "" {
		q.Set("status", params.Status)
	}
	if params.Domain != "" {
		q.Set("domain", params.Domain)
	}
	if params.Market != "" {
		q.Set("market", params.Market)
	}
	if params.Locale != "" {
		q.Set("locale", params.Locale)
	}
	if params.Source != "" {
		q.Set("source", params.Source)
	}
	if params.Offset > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	var out ConceptSearchResult
	if err := c.getKnowledgeJSON(ctx, "/concepts", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetConceptStory returns a concept's merged chronological timeline
// (GET /api/v1/:ws/concepts/:cid/story).
func (c *BowrainClient) GetConceptStory(ctx context.Context, conceptID string) (*ConceptStory, error) {
	var out ConceptStory
	if err := c.getKnowledgeJSON(ctx, "/concepts/"+url.PathEscape(conceptID)+"/story", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListConceptRelations returns the relations touching a concept in either
// direction (GET /api/v1/:ws/concepts/:cid/relations), optionally scoped by
// asOf (RFC3339) and market. It returns the framework's termbase.ConceptRelation
// directly so a `terms pull` caller can feed each value straight into
// termbase.AddRelation.
func (c *BowrainClient) ListConceptRelations(ctx context.Context, conceptID, asOf, market string) ([]termbase.ConceptRelation, error) {
	q := url.Values{}
	if asOf != "" {
		q.Set("as_of", asOf)
	}
	if market != "" {
		q.Set("market", market)
	}
	var out []termbase.ConceptRelation
	if err := c.getKnowledgeJSON(ctx, "/concepts/"+url.PathEscape(conceptID)+"/relations", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetGraph returns the concept graph as nodes and edges for a navigator canvas
// (GET /api/v1/:ws/graph), scoped by the optional facets in params.
func (c *BowrainClient) GetGraph(ctx context.Context, params GraphParams) (*GraphViz, error) {
	q := url.Values{}
	if params.Focus != "" {
		q.Set("focus", params.Focus)
	}
	if params.Depth > 0 {
		q.Set("depth", strconv.Itoa(params.Depth))
	}
	if params.Domain != "" {
		q.Set("domain", params.Domain)
	}
	if params.Status != "" {
		q.Set("status", params.Status)
	}
	if params.AsOf != "" {
		q.Set("as_of", params.AsOf)
	}
	if params.Market != "" {
		q.Set("market", params.Market)
	}
	var out GraphViz
	if err := c.getKnowledgeJSON(ctx, "/graph", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListChangesets lists the workspace's change-sets, newest first, optionally
// filtered to a single status (GET /api/v1/:ws/changesets).
func (c *BowrainClient) ListChangesets(ctx context.Context, status string) ([]ChangeSet, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	var out []ChangeSet
	if err := c.getKnowledgeJSON(ctx, "/changesets", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetChangeset returns a change-set with its ops, reviews, and pilots
// (GET /api/v1/:ws/changesets/:id).
func (c *BowrainClient) GetChangeset(ctx context.Context, id string) (*ChangeSetDetail, error) {
	var out ChangeSetDetail
	if err := c.getKnowledgeJSON(ctx, "/changesets/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetChangesetBlastRadius previews a change-set's blast radius over stored
// content (GET /api/v1/:ws/changesets/:id/blast-radius). Nothing is persisted.
func (c *BowrainClient) GetChangesetBlastRadius(ctx context.Context, id string) (*ChangeSetImpact, error) {
	var out ChangeSetImpact
	if err := c.getKnowledgeJSON(ctx, "/changesets/"+url.PathEscape(id)+"/blast-radius", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// Write methods (concept sync push)
//
// These mirror the server's write surface (handlers_concepts.go,
// handlers_changesets.go). Ordinary concept edits go up directly through the
// concept endpoints; governed edits (a term banned/promoted, a forbidden status
// removed, a REPLACED_BY relation, a concept delete) are refused on the direct
// path with a 409 and must travel through a change-set, which CreateChangeset /
// AppendChangesetOp / SubmitChangeset draft and submit for review.
// ---------------------------------------------------------------------------

// CreateConceptParams creates a concept through ordinary curation (POST
// /api/v1/:ws/concepts). Creating a term already forbidden or preferred is a
// governed transition the server refuses with a 409.
type CreateConceptParams struct {
	ProjectID  string     `json:"project_id,omitempty"`
	Domain     string     `json:"domain"`
	Definition string     `json:"definition"`
	Terms      []TermInfo `json:"terms"`
}

// UpdateConceptParams applies an ordinary concept edit (PUT
// /api/v1/:ws/concepts/:cid): domain, definition, and the full terms list.
// The PUT must not entail a governed status transition or the server refuses it
// with a 409 — a concept-sync push keeps the governed terms at their baseline
// status here and routes the real transition through a change-set.
type UpdateConceptParams struct {
	Domain     string     `json:"domain"`
	Definition string     `json:"definition"`
	Terms      []TermInfo `json:"terms"`
}

// AddRelationParams adds an ordinary typed relation from the path concept to a
// target (POST /api/v1/:ws/concepts/:cid/relations). A REPLACED_BY relation is
// governed and refused with a 409.
type AddRelationParams struct {
	TargetID     string          `json:"target_id"`
	RelationType string          `json:"relation_type"`
	Note         string          `json:"note,omitempty"`
	Validity     *graph.Validity `json:"validity,omitempty"`
}

// ChangeSetOpInput appends one ordered op to a draft change-set (POST
// /api/v1/:ws/changesets/:id/ops). Op is the op-type wire string (e.g.
// "term.status", "relation.add", "concept.delete"); Payload is the op-specific
// JSON the server validates with knowledge.ValidateOp.
type ChangeSetOpInput struct {
	Op      string          `json:"op"`
	Payload json.RawMessage `json:"payload"`
	BaseRev int64           `json:"base_rev,omitempty"`
}

// createChangeSetBody is the POST /changesets body.
type createChangeSetBody struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateConcept creates a concept through ordinary curation and returns the
// stored concept (POST /api/v1/:ws/concepts).
func (c *BowrainClient) CreateConcept(ctx context.Context, params CreateConceptParams) (*ConceptInfo, error) {
	var out ConceptInfo
	if err := c.knowledgeWrite(ctx, http.MethodPost, "/concepts", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateConcept applies an ordinary concept edit (PUT /api/v1/:ws/concepts/:cid).
// The server replies 204 No Content, so nothing is decoded.
func (c *BowrainClient) UpdateConcept(ctx context.Context, conceptID string, params UpdateConceptParams) error {
	return c.knowledgeWrite(ctx, http.MethodPut, "/concepts/"+url.PathEscape(conceptID), params, nil)
}

// DeleteConcept requests a direct concept deletion (DELETE
// /api/v1/:ws/concepts/:cid). A concept delete is governed, so the server
// refuses this direct path with a 409 and a change-set hint; a concept-sync
// push therefore deletes through a change-set rather than calling this. The
// method is provided for completeness and mirrors the route.
func (c *BowrainClient) DeleteConcept(ctx context.Context, conceptID string) error {
	return c.knowledgeWrite(ctx, http.MethodDelete, "/concepts/"+url.PathEscape(conceptID), nil, nil)
}

// AddRelation adds an ordinary typed relation from sourceID to params.TargetID
// and returns the stored relation (POST /api/v1/:ws/concepts/:cid/relations).
func (c *BowrainClient) AddRelation(ctx context.Context, sourceID string, params AddRelationParams) (*termbase.ConceptRelation, error) {
	var out termbase.ConceptRelation
	if err := c.knowledgeWrite(ctx, http.MethodPost, "/concepts/"+url.PathEscape(sourceID)+"/relations", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveRelation removes a relation by ID from a concept (DELETE
// /api/v1/:ws/concepts/:cid/relations/:rid). The server replies 204.
func (c *BowrainClient) RemoveRelation(ctx context.Context, sourceID, relationID string) error {
	return c.knowledgeWrite(ctx, http.MethodDelete,
		"/concepts/"+url.PathEscape(sourceID)+"/relations/"+url.PathEscape(relationID), nil, nil)
}

// CreateChangeset opens a new draft change-set and returns its header (POST
// /api/v1/:ws/changesets).
func (c *BowrainClient) CreateChangeset(ctx context.Context, name, description string) (*ChangeSet, error) {
	var out ChangeSet
	body := createChangeSetBody{Name: name, Description: description}
	if err := c.knowledgeWrite(ctx, http.MethodPost, "/changesets", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AppendChangesetOp appends one validated op to a draft change-set and returns
// the stored op (POST /api/v1/:ws/changesets/:id/ops).
func (c *BowrainClient) AppendChangesetOp(ctx context.Context, changesetID string, op ChangeSetOpInput) (*ChangeSetOp, error) {
	var out ChangeSetOp
	if err := c.knowledgeWrite(ctx, http.MethodPost, "/changesets/"+url.PathEscape(changesetID)+"/ops", op, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SubmitChangeset moves a draft change-set into review and returns its refreshed
// header (POST /api/v1/:ws/changesets/:id/submit).
func (c *BowrainClient) SubmitChangeset(ctx context.Context, changesetID string) (*ChangeSet, error) {
	var out ChangeSet
	if err := c.knowledgeWrite(ctx, http.MethodPost, "/changesets/"+url.PathEscape(changesetID)+"/submit", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// knowledgeWrite issues a workspace-scoped write (POST/PUT/DELETE) against the
// knowledge-graph surface, marshaling body (when non-nil) as the JSON request
// body and decoding the JSON response into out (when non-nil). It accepts the
// 2xx success codes the write handlers return (200/201 with a body, 204 with
// none) and surfaces any other status as an error carrying the server body —
// notably the 409 a governed edit draws on the direct path. It requires a
// workspace-scoped client.
func (c *BowrainClient) knowledgeWrite(ctx context.Context, method, path string, body, out any) error {
	if c.workspace == "" {
		return errors.New("knowledge graph requires a workspace-scoped client (use NewWorkspaceBowrainClient)")
	}
	u, err := url.Parse(c.wsPrefix() + path)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal %s request: %w", strings.TrimPrefix(path, "/"), err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
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

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		if out != nil {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return fmt.Errorf("decode %s response: %w", strings.TrimPrefix(path, "/"), err)
			}
		}
		return nil
	case http.StatusNoContent:
		return nil
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s failed (HTTP %d): %s", strings.TrimPrefix(path, "/"), resp.StatusCode, string(respBody))
	}
}

// wsPrefix returns the URL prefix for workspace content endpoints
// (/api/v1/:ws). Unlike projectPrefix it carries no project segment — the
// knowledge graph is workspace-scoped.
func (c *BowrainClient) wsPrefix() string {
	return fmt.Sprintf("%s/api/v1/%s", c.baseURL, url.PathEscape(c.workspace))
}

// getKnowledgeJSON issues a workspace-scoped GET against the knowledge-graph
// surface and decodes the JSON body into out. It requires a workspace-scoped
// client (the graph lives on /api/v1/:ws/..., not under a project).
func (c *BowrainClient) getKnowledgeJSON(ctx context.Context, path string, query url.Values, out any) error {
	if c.workspace == "" {
		return errors.New("knowledge graph requires a workspace-scoped client (use NewWorkspaceBowrainClient)")
	}
	u, err := url.Parse(c.wsPrefix() + path)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", strings.TrimPrefix(path, "/"), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s failed (HTTP %d): %s", strings.TrimPrefix(path, "/"), resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s response: %w", strings.TrimPrefix(path, "/"), err)
	}
	return nil
}
