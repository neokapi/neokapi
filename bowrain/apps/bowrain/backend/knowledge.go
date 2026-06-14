package backend

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// knowledge.go is the desktop's REST proxy for the brand knowledge-graph
// surface (AD-021): concepts (the terminology API), their relations,
// observations, comments and blast radius, the graph viz, markets, and the
// change-set / experiment lifecycle. Like governance.go these *App methods do
// the authenticated HTTP calls through govRequest and return decoded JSON; the
// desktop is a working copy of the server and never authors the graph offline.
// Paths mirror bowrain/packages/ui/src/api/rest-adapter.ts.
//
// Ordinary concept terminology edits (add/update/delete) keep using the gRPC
// editor methods in terms.go (AddConcept/UpdateConcept/DeleteConcept); the REST
// /concepts surface here adds list/get/create plus the graph + governance
// methods that hang off a concept.

// --- Path helpers ---

func conceptsPath(ws string) string {
	return "/api/v1/" + url.PathEscape(ws) + "/concepts"
}

func conceptPath(ws, conceptID string) string {
	return conceptsPath(ws) + "/" + url.PathEscape(conceptID)
}

func marketsPath(ws string) string {
	return "/api/v1/" + url.PathEscape(ws) + "/markets"
}

func changesetsPath(ws string) string {
	return "/api/v1/" + url.PathEscape(ws) + "/changesets"
}

func changesetPath(ws, changesetID string) string {
	return changesetsPath(ws) + "/" + url.PathEscape(changesetID)
}

// withQuery appends an encoded query string to path when non-empty.
func withQuery(path string, q url.Values) string {
	if qs := q.Encode(); qs != "" {
		return path + "?" + qs
	}
	return path
}

// --- Concept request bodies / query params (mirror types/brand-graph.ts) ---

// Validity is the temporal + tag scope on a relation (core/graph.Validity).
type Validity struct {
	ValidFrom string            `json:"valid_from,omitempty"`
	ValidTo   string            `json:"valid_to,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// ListConceptsArgs holds the GET /concepts query params (ListConceptsParams).
type ListConceptsArgs struct {
	Q         string `json:"q,omitempty"`
	Status    string `json:"status,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Market    string `json:"market,omitempty"`
	Locale    string `json:"locale,omitempty"`
	Source    string `json:"source,omitempty"`
	Stream    string `json:"stream,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Offset    int    `json:"offset,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// AddConceptRelationArgs is the body of POST /concepts/:cid/relations.
type AddConceptRelationArgs struct {
	TargetID     string    `json:"target_id"`
	RelationType string    `json:"relation_type"`
	Note         string    `json:"note,omitempty"`
	Validity     *Validity `json:"validity,omitempty"`
}

// AddObservationArgs is the body of POST /concepts/:cid/observations.
type AddObservationArgs struct {
	Kind   string `json:"kind"`
	Quote  string `json:"quote"`
	Source string `json:"source"`
	URL    string `json:"url,omitempty"`
	Locale string `json:"locale,omitempty"`
	Market string `json:"market,omitempty"`
	Note   string `json:"note,omitempty"`
}

// AddCommentArgs is the body of POST /concepts/:cid/comments.
type AddCommentArgs struct {
	Body        string `json:"body"`
	ParentID    string `json:"parent_id,omitempty"`
	ChangesetID string `json:"changeset_id,omitempty"`
}

// --- Concepts (read) ---

// ListConcepts searches the workspace concept/terminology graph. The shape is
// opaque to the proxy; the frontend has the TermSearchResult type.
func (a *App) ListConcepts(workspaceSlug string, params ListConceptsArgs) (json.RawMessage, error) {
	q := url.Values{}
	if params.Q != "" {
		q.Set("q", params.Q)
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
	if params.Stream != "" {
		q.Set("stream", params.Stream)
	}
	if params.ProjectID != "" {
		q.Set("project_id", params.ProjectID)
	}
	// Pair offset with an explicit limit so an unset (zero) limit doesn't cap
	// the page to zero rows; let the server apply its default when no limit.
	if params.Limit > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	return a.govRaw(http.MethodGet, withQuery(conceptsPath(workspaceSlug), q), nil)
}

// GetConcept returns a single concept by id.
func (a *App) GetConcept(workspaceSlug, conceptID string) (json.RawMessage, error) {
	return a.govRaw(http.MethodGet, conceptPath(workspaceSlug, conceptID), nil)
}

// CreateConcept creates a concept via the REST /concepts surface. The body
// reuses the AddConceptRequest shape (project_id, domain, definition, terms).
func (a *App) CreateConcept(workspaceSlug string, req AddConceptRequest) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, conceptsPath(workspaceSlug), req)
}

// GetConceptStory returns a concept's merged chronological timeline.
func (a *App) GetConceptStory(workspaceSlug, conceptID string) (json.RawMessage, error) {
	return a.govRaw(http.MethodGet, conceptPath(workspaceSlug, conceptID)+"/story", nil)
}

// --- Relations ---

// ListConceptRelations returns the typed edges of a concept. asOf (RFC3339) and
// market scope the read; empty values omit the corresponding query param.
func (a *App) ListConceptRelations(workspaceSlug, conceptID, asOf, market string) (json.RawMessage, error) {
	q := url.Values{}
	if asOf != "" {
		q.Set("as_of", asOf)
	}
	if market != "" {
		q.Set("market", market)
	}
	return a.govRaw(http.MethodGet, withQuery(conceptPath(workspaceSlug, conceptID)+"/relations", q), nil)
}

// AddConceptRelation adds a typed edge from a concept.
func (a *App) AddConceptRelation(workspaceSlug, conceptID string, req AddConceptRelationArgs) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, conceptPath(workspaceSlug, conceptID)+"/relations", req)
}

// DeleteConceptRelation removes a typed edge from a concept.
func (a *App) DeleteConceptRelation(workspaceSlug, conceptID, relationID string) error {
	path := conceptPath(workspaceSlug, conceptID) + "/relations/" + url.PathEscape(relationID)
	return a.govRequest(http.MethodDelete, path, nil, nil)
}

// GetConceptBlastRadius returns the where-used footprint of a concept.
func (a *App) GetConceptBlastRadius(workspaceSlug, conceptID string) (json.RawMessage, error) {
	return a.govRaw(http.MethodGet, conceptPath(workspaceSlug, conceptID)+"/blast-radius", nil)
}

// --- Observations ---

// ListObservations returns the external evidence attached to a concept.
func (a *App) ListObservations(workspaceSlug, conceptID string) (json.RawMessage, error) {
	return a.govRaw(http.MethodGet, conceptPath(workspaceSlug, conceptID)+"/observations", nil)
}

// AddObservation attaches a piece of external evidence to a concept.
func (a *App) AddObservation(workspaceSlug, conceptID string, req AddObservationArgs) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, conceptPath(workspaceSlug, conceptID)+"/observations", req)
}

// DeleteObservation removes an observation from a concept.
func (a *App) DeleteObservation(workspaceSlug, conceptID, observationID string) error {
	path := conceptPath(workspaceSlug, conceptID) + "/observations/" + url.PathEscape(observationID)
	return a.govRequest(http.MethodDelete, path, nil, nil)
}

// --- Comments ---

// ListConceptComments returns the discussion thread on a concept.
func (a *App) ListConceptComments(workspaceSlug, conceptID string) (json.RawMessage, error) {
	return a.govRaw(http.MethodGet, conceptPath(workspaceSlug, conceptID)+"/comments", nil)
}

// AddConceptComment posts a comment to a concept thread.
func (a *App) AddConceptComment(workspaceSlug, conceptID string, req AddCommentArgs) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, conceptPath(workspaceSlug, conceptID)+"/comments", req)
}

// ResolveConceptComment marks a comment resolved (or re-opens it).
func (a *App) ResolveConceptComment(workspaceSlug, conceptID, commentID string, resolved bool) error {
	path := conceptPath(workspaceSlug, conceptID) + "/comments/" + url.PathEscape(commentID) + "/resolve"
	return a.govRequest(http.MethodPost, path, map[string]bool{"resolved": resolved}, nil)
}

// DeleteConceptComment removes a comment from a concept thread.
func (a *App) DeleteConceptComment(workspaceSlug, conceptID, commentID string) error {
	path := conceptPath(workspaceSlug, conceptID) + "/comments/" + url.PathEscape(commentID)
	return a.govRequest(http.MethodDelete, path, nil, nil)
}

// --- Markets ---

// MarketArgs is the body of POST/PUT /markets (MarketRequest).
type MarketArgs struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Locales     []string `json:"locales,omitempty"`
}

// ListMarkets returns the workspace-defined market scopes.
func (a *App) ListMarkets(workspaceSlug string) (json.RawMessage, error) {
	return a.govRaw(http.MethodGet, marketsPath(workspaceSlug), nil)
}

// CreateMarket defines a new market scope.
func (a *App) CreateMarket(workspaceSlug string, req MarketArgs) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, marketsPath(workspaceSlug), req)
}

// UpdateMarket updates an existing market scope.
func (a *App) UpdateMarket(workspaceSlug, marketID string, req MarketArgs) (json.RawMessage, error) {
	path := marketsPath(workspaceSlug) + "/" + url.PathEscape(marketID)
	return a.govRaw(http.MethodPut, path, req)
}

// DeleteMarket removes a market scope.
func (a *App) DeleteMarket(workspaceSlug, marketID string) error {
	path := marketsPath(workspaceSlug) + "/" + url.PathEscape(marketID)
	return a.govRequest(http.MethodDelete, path, nil, nil)
}

// --- Change-sets / experiments ---

// CreateChangesetArgs is the body of POST /changesets (CreateChangeSetRequest).
type CreateChangesetArgs struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// UpdateChangesetArgs is the body of PATCH /changesets/:id (UpdateChangeSetRequest).
type UpdateChangesetArgs struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// AddChangesetOpArgs is the body of POST /changesets/:id/ops
// (AddChangeSetOpRequest). Payload is the op-specific union, forwarded as-is.
type AddChangesetOpArgs struct {
	Op      string `json:"op"`
	Payload any    `json:"payload"`
	BaseRev int    `json:"base_rev,omitempty"`
}

// ReviewArgs is the body of POST /changesets/:id/approve|reject (ReviewRequest).
type ReviewArgs struct {
	Comment string `json:"comment,omitempty"`
}

// StartPilotArgs is the body of POST /changesets/:id/pilots (StartPilotRequest).
type StartPilotArgs struct {
	ProjectID string `json:"project_id"`
	Stream    string `json:"stream"`
}

// ListChangesets returns the workspace change-sets, optionally filtered by status.
func (a *App) ListChangesets(workspaceSlug, status string) (json.RawMessage, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	return a.govRaw(http.MethodGet, withQuery(changesetsPath(workspaceSlug), q), nil)
}

// GetChangeset returns a single change-set with its ops, reviews, and pilots.
func (a *App) GetChangeset(workspaceSlug, changesetID string) (json.RawMessage, error) {
	return a.govRaw(http.MethodGet, changesetPath(workspaceSlug, changesetID), nil)
}

// CreateChangeset opens a new draft change-set.
func (a *App) CreateChangeset(workspaceSlug string, req CreateChangesetArgs) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, changesetsPath(workspaceSlug), req)
}

// PatchChangeset edits a change-set's name/description.
func (a *App) PatchChangeset(workspaceSlug, changesetID string, req UpdateChangesetArgs) (json.RawMessage, error) {
	return a.govRaw(http.MethodPatch, changesetPath(workspaceSlug, changesetID), req)
}

// AppendChangesetOp appends an ordered op to a draft change-set.
func (a *App) AppendChangesetOp(workspaceSlug, changesetID string, req AddChangesetOpArgs) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, changesetPath(workspaceSlug, changesetID)+"/ops", req)
}

// RemoveChangesetOp removes the op with the given seq from a draft change-set.
func (a *App) RemoveChangesetOp(workspaceSlug, changesetID string, seq int) error {
	path := changesetPath(workspaceSlug, changesetID) + "/ops/" + strconv.Itoa(seq)
	return a.govRequest(http.MethodDelete, path, nil, nil)
}

// SubmitChangeset moves a draft change-set into review.
func (a *App) SubmitChangeset(workspaceSlug, changesetID string) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, changesetPath(workspaceSlug, changesetID)+"/submit", nil)
}

// ApproveChangeset records an approving review verdict.
func (a *App) ApproveChangeset(workspaceSlug, changesetID string, req ReviewArgs) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, changesetPath(workspaceSlug, changesetID)+"/approve", req)
}

// RejectChangeset records a rejecting review verdict.
func (a *App) RejectChangeset(workspaceSlug, changesetID string, req ReviewArgs) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, changesetPath(workspaceSlug, changesetID)+"/reject", req)
}

// MergeChangeset merges an approved change-set into the live graph.
func (a *App) MergeChangeset(workspaceSlug, changesetID string) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, changesetPath(workspaceSlug, changesetID)+"/merge", nil)
}

// AbandonChangeset abandons a change-set.
func (a *App) AbandonChangeset(workspaceSlug, changesetID string) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, changesetPath(workspaceSlug, changesetID)+"/abandon", nil)
}

// GetChangesetBlastRadius returns the impact of a change-set over stored content.
func (a *App) GetChangesetBlastRadius(workspaceSlug, changesetID string) (json.RawMessage, error) {
	return a.govRaw(http.MethodGet, changesetPath(workspaceSlug, changesetID)+"/blast-radius", nil)
}

// AddPilot binds a change-set to a project's content stream.
func (a *App) AddPilot(workspaceSlug, changesetID string, req StartPilotArgs) (json.RawMessage, error) {
	return a.govRaw(http.MethodPost, changesetPath(workspaceSlug, changesetID)+"/pilots", req)
}

// RemovePilot unbinds a change-set from a project's content stream.
func (a *App) RemovePilot(workspaceSlug, changesetID, projectID, stream string) error {
	path := changesetPath(workspaceSlug, changesetID) + "/pilots/" +
		url.PathEscape(projectID) + "/" + url.PathEscape(stream)
	return a.govRequest(http.MethodDelete, path, nil, nil)
}

// govRaw is a small helper around govRequest for the many knowledge endpoints
// that return an opaque JSON value the frontend decodes into a typed shape.
func (a *App) govRaw(method, path string, body any) (json.RawMessage, error) {
	var out json.RawMessage
	if err := a.govRequest(method, path, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
