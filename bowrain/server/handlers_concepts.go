package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	sqltb "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// registerConceptRoutes registers the concept half of the brand knowledge-graph
// REST API (AD-021) on the workspace content group. /:ws/concepts is the
// workspace terminology surface — it replaces the former /:ws/terms routes, so
// every consumer (web, desktop, Pulse, MCP) uses it. Reads gate on workspace
// membership (view_content); ordinary curation (create, ordinary edits,
// observations, comments, markets, relations other than REPLACED_BY) gates on
// manage_terms; governed transitions (banning/promoting a term, deleting a
// concept, REPLACED_BY relations) are refused on the direct path with a 409 and
// a change-set hint, since they must travel through a reviewed change-set.
func (s *Server) registerConceptRoutes(g *echo.Group) {
	// Concept collection + single concept.
	g.GET("/concepts", s.HandleListConcepts)
	g.GET("/concepts/count", s.HandleGetConceptCount)
	g.GET("/concepts/status-counts", s.HandleGetConceptStatusCounts)
	g.GET("/concepts/locale-coverage", s.HandleGetConceptLocaleCoverage)
	g.POST("/concepts", s.HandleCreateConcept)

	// Import/export.
	g.POST("/concepts/import/csv", s.HandleImportConceptsCSV)
	g.POST("/concepts/import/json", s.HandleImportConceptsJSON)
	g.GET("/concepts/export/json", s.HandleExportConceptsJSON)

	g.POST("/concepts/bulk-delete", s.HandleBulkDeleteConcepts)

	g.GET("/concepts/:cid", s.HandleGetConcept)
	g.PUT("/concepts/:cid", s.HandleUpdateConcept)
	g.DELETE("/concepts/:cid", s.HandleDeleteConcept)

	// Concept story — the merged chronological timeline.
	g.GET("/concepts/:cid/story", s.HandleGetConceptStory)

	// Relations.
	g.GET("/concepts/:cid/relations", s.HandleListConceptRelations)
	g.POST("/concepts/:cid/relations", s.HandleAddConceptRelation)
	g.DELETE("/concepts/:cid/relations/:rid", s.HandleDeleteConceptRelation)

	// Where-used / blast radius for a single concept.
	g.GET("/concepts/:cid/blast-radius", s.HandleConceptBlastRadius)

	// Observations — external evidence attached to a concept.
	g.GET("/concepts/:cid/observations", s.HandleListObservations)
	g.POST("/concepts/:cid/observations", s.HandleAddObservation)
	g.DELETE("/concepts/:cid/observations/:oid", s.HandleDeleteObservation)

	// Comments — threaded discussion on a concept.
	g.GET("/concepts/:cid/comments", s.HandleListConceptComments)
	g.POST("/concepts/:cid/comments", s.HandleAddConceptComment)
	g.POST("/concepts/:cid/comments/:id/resolve", s.HandleResolveConceptComment)
	g.DELETE("/concepts/:cid/comments/:id", s.HandleDeleteConceptComment)

	// Markets — workspace-defined scopes for validity tags.
	g.GET("/markets", s.HandleListMarkets)
	g.POST("/markets", s.HandleCreateMarket)
	g.PUT("/markets/:mid", s.HandleUpdateMarket)
	g.DELETE("/markets/:mid", s.HandleDeleteMarket)
}

// ---------------------------------------------------------------------------
// Shared request/response DTOs (concept-graph-specific; concept and term DTOs
// are reused from editor.go: ConceptInfoResponse, TermInfoResponse,
// AddConceptRequest, UpdateConceptRequest, TermSearchResponse).
// ---------------------------------------------------------------------------

// AddConceptRelationRequest creates a relation from the path concept (the
// source) to TargetID. A REPLACED_BY relation is governed and refused on this
// direct path with a change-set hint.
type AddConceptRelationRequest struct {
	TargetID     string          `json:"target_id"`
	RelationType string          `json:"relation_type"`
	Note         string          `json:"note,omitempty"`
	Validity     *graph.Validity `json:"validity,omitempty"`
}

// AddObservationRequest attaches external evidence to a concept.
type AddObservationRequest struct {
	Kind   string `json:"kind"`
	Quote  string `json:"quote"`
	Source string `json:"source"`
	URL    string `json:"url,omitempty"`
	Locale string `json:"locale,omitempty"`
	Market string `json:"market,omitempty"`
	Note   string `json:"note,omitempty"`
}

// AddCommentRequest posts a comment on a concept (or, when ChangesetID is set,
// on a change-set thread anchored to the concept).
type AddCommentRequest struct {
	Body        string `json:"body"`
	ParentID    string `json:"parent_id,omitempty"`
	ChangesetID string `json:"changeset_id,omitempty"`
}

// ResolveCommentRequest toggles a comment's resolved flag (defaults to true).
type ResolveCommentRequest struct {
	Resolved *bool `json:"resolved,omitempty"`
}

// MarketRequest creates or updates a market.
type MarketRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Locales     []string `json:"locales,omitempty"`
}

// ConceptStoryEntry is one event on a concept's merged timeline. Kind discriminates
// the source (revision, observation, comment, changeset); Data carries the
// kind-specific record.
type ConceptStoryEntry struct {
	Kind    string    `json:"kind"`
	At      time.Time `json:"at"`
	Actor   string    `json:"actor,omitempty"`
	Summary string    `json:"summary,omitempty"`
	Ref     string    `json:"ref,omitempty"`
	Data    any       `json:"data,omitempty"`
}

// ConceptStoryResponse is the merged chronological timeline of a concept.
type ConceptStoryResponse struct {
	ConceptID string              `json:"concept_id"`
	Entries   []ConceptStoryEntry `json:"entries"`
}

// ---------------------------------------------------------------------------
// Concept CRUD
// ---------------------------------------------------------------------------

// conceptSortUpdatedAt is the one value the concept list's sort param accepts:
// most recently updated first.
const conceptSortUpdatedAt = "updated_at"

// HandleListConcepts searches the workspace's concepts, narrowing the page by
// status, domain, market, locale, and source. The locale query param scopes the
// text search to a source locale; stream inheritance is honored when a non-main
// stream is given; sort=updated_at pages the workspace newest-changed first.
//
// ?at= resolves the page at an instant: a term whose own validity window does
// not cover it drops out, and a concept left with no term in force drops with
// it. Without it the answer is the as-declared view — every term the workspace
// holds, window unapplied — which is a real question, but it is not "what
// governs here now", and a reader who asked the second must not be handed the
// first. ?market= narrows the same resolution to one validity tag.
func (s *Server) HandleListConcepts(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	query := c.QueryParam("q")
	locale := model.LocaleID(c.QueryParam("locale"))
	statusFilter := model.TermStatus(c.QueryParam("status"))
	domainFilter := c.QueryParam("domain")
	marketFilter := c.QueryParam("market")
	sourceFilter := terms.TermSource(c.QueryParam("source"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 50
	}
	sortBy := c.QueryParam("sort")
	if sortBy != "" && sortBy != conceptSortUpdatedAt {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("unknown sort %q (the concept list sorts by %q)", sortBy, conceptSortUpdatedAt),
		})
	}

	tb, err := s.wsStores.getTerms(ws)
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}

	ctx := c.Request().Context()
	stream := c.QueryParam("stream")
	var concepts []terms.Concept
	var total int
	switch {
	case stream != "" && stream != "main" && s.ContentStore != nil:
		chain := buildStreamChain(ctx, s.ContentStore, c.QueryParam("project_id"), stream)
		concepts, total, err = tb.SearchForStream(ctx, query, locale, "", stream, chain[1:], offset, limit)
	case sortBy == conceptSortUpdatedAt:
		concepts, total, err = searchConceptsByRecency(ctx, tb, query, locale, offset, limit)
	default:
		concepts, total, err = tb.Search(ctx, query, locale, "", offset, limit)
	}
	if err != nil {
		return serverErr(c, err)
	}
	if sortBy == conceptSortUpdatedAt {
		sort.SliceStable(concepts, func(i, j int) bool {
			return concepts[i].UpdatedAt.After(concepts[j].UpdatedAt)
		})
	}

	// Post-filter the page by the graph-specific facets. These facets are derived
	// from a concept's terms (status/market/source) or its domain — fields the
	// terms text search does not index — so they are applied to the page here.
	// total stays the terms store's DB-wide match count (an upper bound once a facet
	// narrows the page) rather than len(filtered): overwriting it with the
	// post-filtered page count would collapse a workspace of hundreds to a
	// single-digit count whenever a facet is active.
	if statusFilter != "" || domainFilter != "" || marketFilter != "" || sourceFilter != "" {
		filtered := make([]terms.Concept, 0, len(concepts))
		for _, cp := range concepts {
			if conceptMatchesFacets(cp, statusFilter, domainFilter, marketFilter, sourceFilter) {
				filtered = append(filtered, cp)
			}
		}
		concepts = filtered
	}

	// The window is resolved last, over the page the facets left, because it is
	// not a facet: it decides which of a concept's terms are in force at the
	// point asked about rather than which concepts match a search.
	if raw := strings.TrimSpace(c.QueryParam("at")); raw != "" {
		at, atErr := validityScopeAt(raw, marketFilter)
		if atErr != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: atErr.Error()})
		}
		concepts = conceptsInForce(concepts, at)
	}

	infos := make([]ConceptInfoResponse, len(concepts))
	for i, cp := range concepts {
		infos[i] = editorConceptToInfo(cp)
	}
	return c.JSON(http.StatusOK, TermSearchResponse{Concepts: infos, TotalCount: total})
}

// HandleGetConceptCount returns the workspace concept count.
func (s *Server) HandleGetConceptCount(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	count, err := tb.Count(c.Request().Context())
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]int{"count": count})
}

// ConceptStatusCountsResponse is the workspace vocabulary counted by term
// lifecycle status. Every known status is present, zero included, so a caller
// renders a stable set of bars.
type ConceptStatusCountsResponse struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
}

// LocaleCoverageResponse is per-locale concept coverage over the whole
// workspace, most complete locale first.
type LocaleCoverageResponse struct {
	Total   int                    `json:"total"`
	Locales []sqltb.LocaleCoverage `json:"locales"`
}

// HandleGetConceptStatusCounts returns the workspace's concept total and the
// number of concepts carrying a term at each lifecycle status, from one grouped
// query rather than a list call per status.
func (s *Server) HandleGetConceptStatusCounts(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}

	ctx := c.Request().Context()
	var counts sqltb.ConceptCounts
	if agg, ok := tb.(sqltb.ConceptAggregates); ok {
		counts, err = agg.ConceptCounts(ctx)
	} else {
		var all []terms.Concept
		all, err = tb.Concepts(ctx)
		counts = sqltb.CountsFromConcepts(all)
	}
	if err != nil {
		return serverErr(c, err)
	}

	byStatus := make(map[string]int, len(sqltb.TermStatusOrder))
	for _, status := range sqltb.TermStatusOrder {
		byStatus[string(status)] = counts.ByStatus[status]
	}
	return c.JSON(http.StatusOK, ConceptStatusCountsResponse{Total: counts.Total, ByStatus: byStatus})
}

// HandleGetConceptLocaleCoverage returns, per locale, how many of the
// workspace's concepts define a term in it — computed over every concept, not a
// page of them.
func (s *Server) HandleGetConceptLocaleCoverage(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}

	ctx := c.Request().Context()
	var coverage []sqltb.LocaleCoverage
	if agg, ok := tb.(sqltb.ConceptAggregates); ok {
		coverage, err = agg.ConceptLocaleCoverage(ctx)
	} else {
		var all []terms.Concept
		all, err = tb.Concepts(ctx)
		coverage = sqltb.CoverageFromConcepts(all)
	}
	if err != nil {
		return serverErr(c, err)
	}

	total := 0
	if len(coverage) > 0 {
		total = coverage[0].Total
	} else if total, err = tb.Count(ctx); err != nil {
		return serverErr(c, err)
	}
	if coverage == nil {
		coverage = []sqltb.LocaleCoverage{}
	}
	return c.JSON(http.StatusOK, LocaleCoverageResponse{Total: total, Locales: coverage})
}

// searchConceptsByRecency pages concepts newest-updated first, in the database
// when the store can order there and over the requested page otherwise.
func searchConceptsByRecency(ctx context.Context, tb terms.Store, query string, locale model.LocaleID, offset, limit int) ([]terms.Concept, int, error) {
	if rs, ok := tb.(sqltb.RecentSearcher); ok {
		return rs.SearchRecent(ctx, query, locale, "", offset, limit)
	}
	return tb.Search(ctx, query, locale, "", offset, limit)
}

// HandleCreateConcept creates a concept through ordinary curation. Creating a
// term that is already forbidden or preferred is a governed transition and is
// refused with a change-set hint.
func (s *Server) HandleCreateConcept(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)

	var req AddConceptRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	termList := editorTermsFromInfo(req.Terms)
	if governedConceptCreate(termList) {
		return conceptGovernedConflict(c, "a concept whose term is created as forbidden or preferred")
	}

	tb, err := s.wsStores.getTerms(ws)
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	concept := terms.Concept{
		ID:         id.New(),
		ProjectID:  req.ProjectID,
		Domain:     req.Domain,
		Definition: req.Definition,
		Terms:      termList,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	stream := streamParam(c)
	if stream != "" && stream != "main" {
		err = tb.AddConceptWithStream(c.Request().Context(), concept, stream)
	} else {
		err = tb.AddConcept(c.Request().Context(), concept)
	}
	if err != nil {
		return serverErr(c, err)
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		conceptEvent(knowledge.EventConceptCreated, wsID, concept.ID, actor),
	})
	return c.JSON(http.StatusCreated, editorConceptToInfo(concept))
}

// HandleGetConcept returns a single concept by ID.
func (s *Server) HandleGetConcept(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	concept, ok, err := tb.GetConcept(c.Request().Context(), c.Param("cid"))
	if err != nil {
		return serverErr(c, err)
	}
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("concept %q not found", c.Param("cid"))})
	}
	return c.JSON(http.StatusOK, editorConceptToInfo(concept))
}

// HandleUpdateConcept applies ordinary concept edits (definition, notes,
// non-status term metadata, adding admitted/approved/proposed/deprecated terms).
// A governed transition — setting a term forbidden or preferred, un-forbidding a
// term — is refused with a change-set hint; it must travel through a change-set.
func (s *Server) HandleUpdateConcept(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	cid := c.Param("cid")
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)

	var req UpdateConceptRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	tb, err := s.wsStores.getTerms(ws)
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}

	existing, ok, err := tb.GetConcept(c.Request().Context(), cid)
	if err != nil {
		return serverErr(c, err)
	}
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("concept %q not found", cid)})
	}

	newTerms := editorTermsFromInfo(req.Terms)
	if governedConceptUpdate(existing.Terms, newTerms) {
		return conceptGovernedConflict(c, "a term status transition to/from forbidden or preferred")
	}

	existing.Domain = req.Domain
	existing.Definition = req.Definition
	existing.Terms = newTerms
	existing.UpdatedAt = time.Now()

	if err := tb.AddConcept(c.Request().Context(), existing); err != nil {
		return serverErr(c, err)
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		conceptEvent(knowledge.EventConceptUpdated, wsID, cid, actor),
	})
	return c.NoContent(http.StatusNoContent)
}

// HandleDeleteConcept refuses a direct concept deletion: a deletion is governed
// and must travel through a reviewed change-set. It returns a 409 with the hint.
func (s *Server) HandleDeleteConcept(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	return conceptGovernedConflict(c, "deleting a concept")
}

// ---------------------------------------------------------------------------
// Concept story
// ---------------------------------------------------------------------------

// HandleGetConceptStory assembles a concept's merged chronological timeline from
// the knowledge store (revisions, observations, comments) and the change-sets
// whose ops touch the concept. Entries are sorted oldest-first.
func (s *Server) HandleGetConceptStory(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}

	cid := c.Param("cid")
	wsID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	var entries []ConceptStoryEntry

	revisions, err := s.KnowledgeStore.ListRevisions(ctx, wsID, cid)
	if err != nil {
		return serverErr(c, err)
	}
	for _, r := range revisions {
		entries = append(entries, ConceptStoryEntry{
			Kind:    "revision",
			At:      r.CreatedAt,
			Actor:   r.Actor,
			Summary: r.Summary,
			Ref:     strconv.FormatInt(r.Rev, 10),
			Data:    r,
		})
	}

	observations, err := s.KnowledgeStore.ListObservationsByConcept(ctx, wsID, cid)
	if err != nil {
		return serverErr(c, err)
	}
	for _, o := range observations {
		entries = append(entries, ConceptStoryEntry{
			Kind:    "observation",
			At:      o.CreatedAt,
			Actor:   o.CreatedBy,
			Summary: fmt.Sprintf("%s observation: %s", o.Kind, o.Quote),
			Ref:     o.ID,
			Data:    o,
		})
	}

	comments, err := s.KnowledgeStore.ListCommentsByConcept(ctx, wsID, cid)
	if err != nil {
		return serverErr(c, err)
	}
	for _, cm := range comments {
		entries = append(entries, ConceptStoryEntry{
			Kind:    "comment",
			At:      cm.CreatedAt,
			Actor:   cm.Author,
			Summary: cm.Body,
			Ref:     cm.ID,
			Data:    cm,
		})
	}

	// Change-sets whose ops touch this concept.
	changesets, err := s.KnowledgeStore.ListChangeSets(ctx, wsID, "")
	if err != nil {
		return serverErr(c, err)
	}
	for _, cs := range changesets {
		ops, err := s.KnowledgeStore.ListOps(ctx, wsID, cs.ID)
		if err != nil {
			return serverErr(c, err)
		}
		if !changeSetTouchesConcept(ops, cid) {
			continue
		}
		at := cs.UpdatedAt
		if at.IsZero() {
			at = cs.CreatedAt
		}
		entries = append(entries, ConceptStoryEntry{
			Kind:    "changeset",
			At:      at,
			Actor:   cs.CreatedBy,
			Summary: fmt.Sprintf("change-set %q (%s)", changeSetName(cs), cs.Status),
			Ref:     cs.ID,
			Data:    cs,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool { return entries[i].At.Before(entries[j].At) })
	return c.JSON(http.StatusOK, ConceptStoryResponse{ConceptID: cid, Entries: entries})
}

// ---------------------------------------------------------------------------
// Relations
// ---------------------------------------------------------------------------

// HandleListConceptRelations returns the relations touching a concept (either
// direction), optionally scoped by as_of (RFC3339) and market.
func (s *Server) HandleListConceptRelations(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	rels, err := tb.RelationsOf(c.Request().Context(), c.Param("cid"), scopeFromQuery(c))
	if err != nil {
		return serverErr(c, err)
	}
	if rels == nil {
		rels = []terms.ConceptRelation{}
	}
	return c.JSON(http.StatusOK, rels)
}

// HandleAddConceptRelation adds an ordinary relation from the path concept to a
// target. A REPLACED_BY relation is governed and refused with a change-set hint.
func (s *Server) HandleAddConceptRelation(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	cid := c.Param("cid")
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)

	var req AddConceptRelationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.TargetID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "target_id is required"})
	}
	if req.RelationType == graph.LabelReplacedBy {
		return conceptGovernedConflict(c, "a REPLACED_BY relation")
	}

	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	rel := terms.ConceptRelation{
		ID:           id.New(),
		SourceID:     cid,
		TargetID:     req.TargetID,
		RelationType: req.RelationType,
		Note:         req.Note,
		Validity:     req.Validity,
		CreatedAt:    time.Now(),
	}
	if err := tb.AddRelation(c.Request().Context(), rel); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		conceptEvent(knowledge.EventConceptRelationAdded, wsID, cid, actor),
	})
	return c.JSON(http.StatusCreated, rel)
}

// HandleDeleteConceptRelation removes a relation by ID (an ordinary edit).
func (s *Server) HandleDeleteConceptRelation(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	cid := c.Param("cid")
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)

	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	if err := tb.DeleteRelation(c.Request().Context(), c.Param("rid")); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		conceptEvent(knowledge.EventConceptRelationRemoved, wsID, cid, actor),
	})
	return c.NoContent(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Blast radius (where used)
// ---------------------------------------------------------------------------

// HandleConceptBlastRadius reports where a concept's terms occur across the
// workspace's stored content (engine.ConceptUsage) — the "consequences" a
// steward sees before proposing a change.
//
// By default the walk evaluates each block in its source locale (the canonical
// compliant text). Passing ?locales=fr,de evaluates the translations instead, so
// the mark-term UI can preview the RV-E re-check honestly as "re-checks N
// translations across M locales" before a term is marked forbidden.
func (s *Server) HandleConceptBlastRadius(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	engine, err := s.knowledgeEngineFor(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	opts := knowledge.EvalOptions{}
	if raw := c.QueryParam("locales"); raw != "" {
		for part := range strings.SplitSeq(raw, ",") {
			if p := strings.TrimSpace(part); p != "" {
				opts.Locales = append(opts.Locales, model.LocaleID(p))
			}
		}
	}
	wsID, _ := c.Get("workspace_id").(string)
	usage, err := engine.ConceptUsage(c.Request().Context(), wsID, c.Param("cid"), opts)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, usage)
}

// ---------------------------------------------------------------------------
// Observations
// ---------------------------------------------------------------------------

// HandleListObservations returns a concept's observations, newest first.
func (s *Server) HandleListObservations(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	obs, err := s.KnowledgeStore.ListObservationsByConcept(c.Request().Context(), wsID, c.Param("cid"))
	if err != nil {
		return serverErr(c, err)
	}
	if obs == nil {
		obs = []*knowledge.Observation{}
	}
	return c.JSON(http.StatusOK, obs)
}

// HandleAddObservation attaches external evidence to a concept.
func (s *Server) HandleAddObservation(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}

	cid := c.Param("cid")
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)

	var req AddObservationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	kind := knowledge.ObservationKind(req.Kind)
	if !kind.IsValid() {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("unknown observation kind %q", req.Kind)})
	}
	if strings.TrimSpace(req.Quote) == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "quote is required"})
	}

	obs := &knowledge.Observation{
		WorkspaceID: wsID,
		ConceptID:   cid,
		Kind:        kind,
		Quote:       req.Quote,
		Source:      req.Source,
		URL:         req.URL,
		Locale:      model.LocaleID(req.Locale),
		Market:      req.Market,
		Note:        req.Note,
		CreatedBy:   actor,
	}
	if err := s.KnowledgeStore.AddObservation(c.Request().Context(), obs); err != nil {
		return serverErr(c, err)
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		conceptEvent(knowledge.EventObservationAdded, wsID, cid, actor),
	})
	return c.JSON(http.StatusCreated, obs)
}

// HandleDeleteObservation removes an observation by ID.
func (s *Server) HandleDeleteObservation(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	if err := s.KnowledgeStore.DeleteObservation(c.Request().Context(), wsID, c.Param("oid")); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

// HandleListConceptComments returns a concept's comments in thread order.
func (s *Server) HandleListConceptComments(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	comments, err := s.KnowledgeStore.ListCommentsByConcept(c.Request().Context(), wsID, c.Param("cid"))
	if err != nil {
		return serverErr(c, err)
	}
	if comments == nil {
		comments = []*knowledge.Comment{}
	}
	return c.JSON(http.StatusOK, comments)
}

// HandleAddConceptComment posts a comment on a concept thread.
func (s *Server) HandleAddConceptComment(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}

	cid := c.Param("cid")
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)

	var req AddCommentRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if strings.TrimSpace(req.Body) == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "body is required"})
	}

	comment := &knowledge.Comment{
		WorkspaceID: wsID,
		ConceptID:   cid,
		ParentID:    req.ParentID,
		ChangesetID: req.ChangesetID,
		Body:        req.Body,
		Author:      actor,
	}
	if err := s.KnowledgeStore.AddComment(c.Request().Context(), comment); err != nil {
		return serverErr(c, err)
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		conceptEvent(knowledge.EventConceptCommentAdded, wsID, cid, actor),
	})
	return c.JSON(http.StatusCreated, comment)
}

// HandleResolveConceptComment toggles a comment's resolved flag (default true).
func (s *Server) HandleResolveConceptComment(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	resolved := true
	var req ResolveCommentRequest
	if err := c.Bind(&req); err == nil && req.Resolved != nil {
		resolved = *req.Resolved
	}
	wsID, _ := c.Get("workspace_id").(string)
	if err := s.KnowledgeStore.ResolveComment(c.Request().Context(), wsID, c.Param("id"), resolved); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleDeleteConceptComment removes a comment by ID.
func (s *Server) HandleDeleteConceptComment(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	if err := s.KnowledgeStore.DeleteComment(c.Request().Context(), wsID, c.Param("id")); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Markets
// ---------------------------------------------------------------------------

// HandleListMarkets returns the workspace's markets, ordered by name.
func (s *Server) HandleListMarkets(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	markets, err := s.KnowledgeStore.ListMarkets(c.Request().Context(), wsID)
	if err != nil {
		return serverErr(c, err)
	}
	if markets == nil {
		markets = []*knowledge.Market{}
	}
	return c.JSON(http.StatusOK, markets)
}

// HandleCreateMarket creates a market.
func (s *Server) HandleCreateMarket(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	var req MarketRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if strings.TrimSpace(req.Name) == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}

	market := &knowledge.Market{
		WorkspaceID: wsID,
		Name:        req.Name,
		Description: req.Description,
		Locales:     localeIDs(req.Locales),
	}
	if err := s.KnowledgeStore.CreateMarket(c.Request().Context(), market); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusCreated, market)
}

// HandleUpdateMarket updates a market's name, description, and locales.
func (s *Server) HandleUpdateMarket(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	mid := c.Param("mid")
	ctx := c.Request().Context()

	existing, err := s.KnowledgeStore.GetMarket(ctx, wsID, mid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	var req MarketRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if strings.TrimSpace(req.Name) != "" {
		existing.Name = req.Name
	}
	existing.Description = req.Description
	if req.Locales != nil {
		existing.Locales = localeIDs(req.Locales)
	}
	if err := s.KnowledgeStore.UpdateMarket(ctx, existing); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, existing)
}

// HandleDeleteMarket removes a market by ID.
func (s *Server) HandleDeleteMarket(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	if err := s.KnowledgeStore.DeleteMarket(c.Request().Context(), wsID, c.Param("mid")); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Import / export
// ---------------------------------------------------------------------------

// HandleImportConceptsCSV imports concepts from CSV.
func (s *Server) HandleImportConceptsCSV(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	var req ImportCSVRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	count, err := terms.ImportCSV(c.Request().Context(), tb, strings.NewReader(req.CSVContent), terms.CSVImportOptions{
		HasHeader:    req.HasHeader,
		SourceLocale: model.LocaleID(req.SourceLocale),
		TargetLocale: model.LocaleID(req.TargetLocale),
		Domain:       req.Domain,
	})
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]int{"imported": count})
}

// HandleImportConceptsJSON imports concepts from JSON.
func (s *Server) HandleImportConceptsJSON(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	var req ImportJSONRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	count, err := terms.ImportJSON(c.Request().Context(), tb, strings.NewReader(req.JSONContent))
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]int{"imported": count})
}

// HandleExportConceptsJSON exports the workspace concepts as JSON.
func (s *Server) HandleExportConceptsJSON(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	tb, err := s.wsStores.getTerms(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	var buf strings.Builder
	if err := terms.ExportJSON(c.Request().Context(), tb, &buf, c.QueryParam("name")); err != nil {
		return serverErr(c, err)
	}
	return c.JSONBlob(http.StatusOK, []byte(buf.String()))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// conceptGovernedConflict refuses a governed edit on the direct path with a 409
// and a hint to route it through a change-set.
func conceptGovernedConflict(c echo.Context, detail string) error {
	return c.JSON(http.StatusConflict, map[string]any{
		"error":  "governed change requires a change-set",
		"detail": detail,
		"hint":   "open a change-set (POST /:ws/changesets), add the governed op, and submit it for review",
	})
}

// governedConceptCreate reports whether creating a concept with these terms is a
// governed transition — any term created already forbidden or preferred.
func governedConceptCreate(terms []terms.Term) bool {
	for _, t := range terms {
		if t.Status == model.TermForbidden || t.Status == model.TermPreferred {
			return true
		}
	}
	return false
}

// governedConceptUpdate reports whether replacing oldTerms with newTerms entails
// a governed status transition: any added/changed term moving to/from forbidden
// or preferred, or a forbidden term removed (un-forbidding it).
func governedConceptUpdate(oldTerms, newTerms []terms.Term) bool {
	oldByKey := make(map[string]terms.Term, len(oldTerms))
	for _, t := range oldTerms {
		oldByKey[termIdentity(t)] = t
	}
	newByKey := make(map[string]terms.Term, len(newTerms))
	for _, t := range newTerms {
		newByKey[termIdentity(t)] = t
		from := model.TermStatus("")
		if prev, ok := oldByKey[termIdentity(t)]; ok {
			from = prev.Status
		}
		if terms.IsGovernedTransition(from, t.Status) {
			return true
		}
	}
	for k, prev := range oldByKey {
		if _, ok := newByKey[k]; !ok && prev.Status == model.TermForbidden {
			return true
		}
	}
	return false
}

// termIdentity keys a term by locale + lowered text, matching the change-set op
// identity for terms.
func termIdentity(t terms.Term) string {
	return string(t.Locale) + "|" + strings.ToLower(t.Text)
}

// conceptMatchesFacets reports whether a concept passes the optional list facets
// (status, domain, market, source). Empty facets always pass.
func conceptMatchesFacets(cp terms.Concept, status model.TermStatus, domain, market string, source terms.TermSource) bool {
	if domain != "" && cp.Domain != domain {
		return false
	}
	if source != "" && conceptSource(cp) != source {
		return false
	}
	if status != "" && !conceptHasStatus(cp, status) {
		return false
	}
	if market != "" && !conceptHasMarket(cp, market) {
		return false
	}
	return true
}

// conceptsInForce keeps the terms whose own window covers the point, and drops
// the concepts left with none.
//
// It is the workspace's spelling of what a project resolves locally: a term is
// bound at a coordinate, not at a workspace, so a list that never applied the
// window answered "is this word discouraged" when the question asked was "is it
// discouraged HERE" — and those have different answers whenever a term carries
// one.
func conceptsInForce(in []terms.Concept, at graph.Scope) []terms.Concept {
	out := make([]terms.Concept, 0, len(in))
	for _, cp := range in {
		kept := make([]terms.Term, 0, len(cp.Terms))
		for _, t := range cp.Terms {
			if t.Validity.Matches(at) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			continue
		}
		cp.Terms = kept
		out = append(out, cp)
	}
	return out
}

// conceptHasStatus reports whether any of a concept's terms carries the status.
func conceptHasStatus(cp terms.Concept, status model.TermStatus) bool {
	for _, t := range cp.Terms {
		if t.Status == status {
			return true
		}
	}
	return false
}

// conceptHasMarket reports whether any of a concept's terms is validity-scoped to
// the named market.
func conceptHasMarket(cp terms.Concept, market string) bool {
	for _, t := range cp.Terms {
		if t.Validity != nil && t.Validity.Tags[validityMarketTag] == market {
			return true
		}
	}
	return false
}

// conceptSource returns a concept's source, defaulting an unset source to
// terminology (matching the terms store's own default).
func conceptSource(cp terms.Concept) terms.TermSource {
	if cp.Source == "" {
		return terms.TermSourceTerminology
	}
	return cp.Source
}

// scopeFromQuery builds a validity scope from as_of (RFC3339) and market query
// params, or returns nil when neither is set.
func scopeFromQuery(c echo.Context) *graph.Scope {
	asOf := c.QueryParam("as_of")
	market := c.QueryParam("market")
	if asOf == "" && market == "" {
		return nil
	}
	sc := &graph.Scope{At: time.Now().UTC()}
	if asOf != "" {
		if t, err := time.Parse(time.RFC3339, asOf); err == nil {
			sc.At = t
		}
	}
	if market != "" {
		sc.Tags = map[string]string{"market": market}
	}
	return sc
}

// changeSetTouchesConcept reports whether any op in a change-set references the
// concept (by concept_id for concept/term ops, or by relation endpoint for
// relation.add ops).
func changeSetTouchesConcept(ops []*knowledge.ChangeSetOp, cid string) bool {
	var probe struct {
		ConceptID string `json:"concept_id"`
		Relation  struct {
			SourceID string `json:"source_id"`
			TargetID string `json:"target_id"`
		} `json:"relation"`
	}
	for _, op := range ops {
		if op == nil || len(op.Payload) == 0 {
			continue
		}
		probe.ConceptID = ""
		probe.Relation.SourceID = ""
		probe.Relation.TargetID = ""
		if json.Unmarshal(op.Payload, &probe) != nil {
			continue
		}
		if probe.ConceptID == cid || probe.Relation.SourceID == cid || probe.Relation.TargetID == cid {
			return true
		}
	}
	return false
}

// changeSetName returns a change-set's display name, falling back to its ID.
func changeSetName(cs *knowledge.ChangeSet) string {
	if cs.Name != "" {
		return cs.Name
	}
	return cs.ID
}

// localeIDs converts a slice of locale strings to model.LocaleID.
func localeIDs(locales []string) []model.LocaleID {
	out := make([]model.LocaleID, len(locales))
	for i, l := range locales {
		out[i] = model.LocaleID(l)
	}
	return out
}
