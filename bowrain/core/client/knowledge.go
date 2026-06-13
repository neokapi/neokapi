package client

import (
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

	"github.com/neokapi/neokapi/termbase"
)

// This file holds the read-only REST client for the brand knowledge graph
// (Bowrain AD-021). The graph lives on the workspace content group
// (/api/v1/:ws/...), not under a project, so every method here requires a
// workspace-scoped client (NewWorkspaceBowrainClient). It mirrors the GET-with-
// JSON-decode pattern of the rest of the client and lets kapi read the governed
// vocabulary through the bowrain plugin (`kapi terms pull`, MCP tools), so
// offline `kapi verify` gates against the same truth.

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

// GetConcept returns a single concept by ID (GET /api/v1/:ws/concepts/:cid).
func (c *BowrainClient) GetConcept(ctx context.Context, conceptID string) (*ConceptInfo, error) {
	var out ConceptInfo
	if err := c.getKnowledgeJSON(ctx, "/concepts/"+url.PathEscape(conceptID), nil, &out); err != nil {
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
// Helpers
// ---------------------------------------------------------------------------

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
