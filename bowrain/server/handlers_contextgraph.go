package server

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/clip"
	"github.com/neokapi/neokapi/core/contextgraph"
	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// The read surface over the workspace context graph.
//
// The graph is a projection rebuilt on every push (contextgraph.go); this is
// where it is read. "Which projects use this concept" is two hops out of one
// workspace-scoped concept node, and it is a different question from the
// change-set blast radius even though both answer with projects: the blast
// radius asks what a DRAFT would do to stored content and has to read the text
// to know, while this asks what the workspace already recorded and reads only
// the relationships.
//
// The distinction is the reason the pane moved here. A scan over every block of
// every project answers correctly and costs the whole workspace per question;
// the graph answers the same question off edges that were written once, on the
// push that produced them.

// registerContextGraphRoutes registers the workspace context graph's read
// surface on the workspace content group. Reads gate on workspace membership
// (view_content), like every other question about what the workspace holds.
func (s *Server) registerContextGraphRoutes(g *echo.Group) {
	g.GET("/context/concepts/:cid/projects", s.HandleConceptProjects)
}

// conceptUsesSampleLimit is how many recorded uses one answer carries. The
// graph holds every use; the response carries a page of them, because the pane
// renders a list and the rollup above it already states the totals.
const conceptUsesSampleLimit = 25

// conceptUsesMaxSamples caps what a caller may ask for. Each sampled use costs
// one indexed block lookup to attach its text, so the cap is what keeps a
// generous ?limit from turning a bounded read back into a walk.
const conceptUsesMaxSamples = 100

// defaultConceptScanBudget bounds the content walk this endpoint falls back to
// when the graph holds no record of the concept. The fallback exists so a
// workspace that has not pushed since the writer landed still gets an answer;
// the budget exists so that answer arrives, marked as a floor, rather than not
// at all.
const defaultConceptScanBudget = 8 * time.Second

// scanBudget is the bound the fallback walk runs under.
func (s *Server) scanBudget() time.Duration {
	if s.conceptScanBudget > 0 {
		return s.conceptScanBudget
	}
	return defaultConceptScanBudget
}

// ConceptTermUseResponse is one of a concept's terms as one project uses it.
type ConceptTermUseResponse struct {
	Term        string `json:"term"`
	TermLocale  string `json:"term_locale,omitempty"`
	Status      string `json:"status,omitempty"`
	Discouraged bool   `json:"discouraged,omitempty"`
	Blocks      int    `json:"blocks"`
	Occurrences int    `json:"occurrences"`
}

// ConceptProjectUseResponse is one project's share of a concept's use, and how
// the concept stands there.
type ConceptProjectUseResponse struct {
	ProjectID   string   `json:"project_id"`
	ProjectName string   `json:"project_name,omitempty"`
	Blocks      int      `json:"blocks"`
	Occurrences int      `json:"occurrences"`
	Collections []string `json:"collections,omitempty"`
	// Status is the standing the project's own usage gives the concept, and
	// Discouraged whether that standing is one a writer must act on.
	Status      string `json:"status,omitempty"`
	Discouraged bool   `json:"discouraged,omitempty"`
	// Replacement is what to say instead, when the project reached for a
	// discouraged spelling: the concept's preferred term, resolved at the same
	// instant, so a replacement whose own window has not opened is not offered.
	Replacement string                   `json:"replacement,omitempty"`
	Terms       []ConceptTermUseResponse `json:"terms,omitempty"`
}

// ConceptUseResponse is one recorded use: where it sits, which term it used,
// and the wording it sits in where the content store still holds it.
type ConceptUseResponse struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name,omitempty"`
	Stream      string `json:"stream,omitempty"`
	Collection  string `json:"collection,omitempty"`
	Document    string `json:"document,omitempty"`
	BlockID     string `json:"block_id,omitempty"`
	Locale      string `json:"locale,omitempty"`
	Term        string `json:"term,omitempty"`
	Status      string `json:"status,omitempty"`
	Discouraged bool   `json:"discouraged,omitempty"`
	Occurrences int    `json:"occurrences"`
	Text        string `json:"text,omitempty"`
}

// ConceptProjectsResponse is the workspace's cross-project answer for one
// concept.
type ConceptProjectsResponse struct {
	ConceptID string `json:"concept_id"`
	// Source names where the answer came from: "graph" when the traversal
	// answered it, "scan" when the graph held no record of the concept and the
	// bounded content walk answered instead. A reader comparing two answers has
	// to know which one they are reading.
	Source string `json:"source"`
	// At is the instant the answer was resolved at.
	At          time.Time                   `json:"at"`
	Projects    []ConceptProjectUseResponse `json:"projects"`
	Uses        []ConceptUseResponse        `json:"uses"`
	Blocks      int                         `json:"blocks"`
	Occurrences int                         `json:"occurrences"`
	// UsesTotal is how many uses the answer holds before Uses was paged.
	UsesTotal int `json:"uses_total"`
	// Partial reports that the answer is a floor: a project absent from
	// Projects means "not reached", not "does not use it".
	Partial       bool     `json:"partial,omitempty"`
	PartialReason string   `json:"partial_reason,omitempty"`
	Notes         []string `json:"notes,omitempty"`
}

// HandleConceptProjects answers which projects use a concept, how much, and
// where — from the workspace context graph.
//
// ?project= narrows the answer to one project without changing which query
// answers it: the dimensions are fields on the nodes, so pinning one is a
// filter on the same traversal. ?at= is the instant the answer is resolved at
// and ?market= the validity tag, so a term whose window has closed or whose
// market this is not drops out. The instant defaults to now, because "which
// projects use this" is asked in the present tense.
func (s *Server) HandleConceptProjects(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	conceptID := c.Param("cid")
	wsID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	at, err := conceptAnswerScope(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	limit := conceptUsesSampleLimit
	if raw := c.QueryParam("limit"); raw != "" {
		if n, convErr := strconv.Atoi(raw); convErr == nil && n > 0 {
			limit = min(n, conceptUsesMaxSamples)
		}
	}

	names, err := s.workspaceProjectNames(ctx, wsID)
	if err != nil {
		return serverErr(c, err)
	}

	filter := contextgraph.Scope{Workspace: wsID, Project: c.QueryParam("project")}
	if materialized, mErr := s.conceptIsInGraph(ctx, filter, conceptID); mErr != nil {
		return serverErr(c, mErr)
	} else if !materialized {
		return s.conceptProjectsByScan(c, wsID, conceptID, at, limit, names)
	}

	rollup, err := contextgraph.UsesByProject(ctx, s.GraphStore, filter, conceptID, at)
	if err != nil {
		return serverErr(c, err)
	}

	res := ConceptProjectsResponse{
		ConceptID: conceptID,
		Source:    "graph",
		At:        at.At,
		Projects:  []ConceptProjectUseResponse{},
		Uses:      []ConceptUseResponse{},
	}
	// The graph says how the concept STANDS in a project; the terms store says
	// what to reach for instead. A terms store that cannot be read costs the
	// second half and says so, rather than costing the answer.
	concept, _, conceptErr := s.conceptFor(ctx, c.Param("ws"), conceptID)
	if conceptErr != nil {
		res.Notes = append(res.Notes,
			"the workspace vocabulary could not be read, so this answer says how the concept stands without saying what to use instead")
	}

	var uses []contextgraph.ConceptUse
	for _, p := range rollup {
		// A project the workspace no longer holds leaves its subgraph behind
		// until something clears it; naming it here would report a project the
		// reader cannot open.
		name, known := names[p.Scope.Project]
		if !known {
			continue
		}
		res.Projects = append(res.Projects, ConceptProjectUseResponse{
			ProjectID:   p.Scope.Project,
			ProjectName: name,
			Blocks:      p.Blocks,
			Occurrences: p.Occurrences,
			Collections: p.Collections,
			Status:      p.Status,
			Discouraged: p.Discouraged,
			Replacement: replacementFor(concept, p, at),
			Terms:       conceptTermUses(p.Terms),
		})
		res.Blocks += p.Blocks
		res.Occurrences += p.Occurrences
		uses = append(uses, p.Uses...)
	}
	res.UsesTotal = len(uses)
	if len(uses) > limit {
		uses = uses[:limit]
	}
	res.Uses = s.conceptUseRows(ctx, uses, names)
	if len(res.Projects) == 0 {
		res.Notes = append(res.Notes,
			"the workspace graph holds this concept, and no project's content uses it")
	}
	return c.JSON(http.StatusOK, res)
}

// conceptIsInGraph reports whether the workspace graph has ever recorded this
// concept.
//
// It is the difference between "no project uses it" and "nowhere to look". An
// empty traversal reads the same either way, and a pane that cannot tell them
// apart states the first when the truth is the second.
func (s *Server) conceptIsInGraph(ctx context.Context, filter contextgraph.Scope, conceptID string) (bool, error) {
	if s.GraphStore == nil {
		return false, nil
	}
	_, err := s.GraphStore.GetNode(ctx, contextgraph.ConceptNodeID(filter, conceptID))
	if errors.Is(err, coreg.ErrNodeNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// conceptProjectsByScan answers from the bounded content walk, for a workspace
// whose graph has not recorded the concept.
//
// The walk is the same one the blast-radius preview runs, held to a budget: an
// answer that says it is a floor beats no answer, and both beat an answer that
// is short without saying so.
func (s *Server) conceptProjectsByScan(
	c echo.Context,
	wsID, conceptID string,
	at coreg.Scope,
	limit int,
	names map[string]string,
) error {
	engine, err := s.knowledgeEngineFor(c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	// The narrowing has to survive the fallback, or a reader who pinned a project
	// gets the workspace back the moment the answer changes source.
	usage, err := engine.ConceptUsage(c.Request().Context(), wsID, conceptID, knowledge.EvalOptions{
		ProjectID:  c.QueryParam("project"),
		Budget:     s.scanBudget(),
		MaxSamples: limit,
	})
	if err != nil {
		return serverErr(c, err)
	}

	res := ConceptProjectsResponse{
		ConceptID:     conceptID,
		Source:        "scan",
		At:            at.At,
		Blocks:        usage.Blocks,
		Occurrences:   usage.Occurrences,
		Partial:       usage.Partial,
		PartialReason: usage.PartialReason,
		Projects:      []ConceptProjectUseResponse{},
		Uses:          []ConceptUseResponse{},
		Notes: []string{
			"no push has recorded this concept in the workspace graph yet, so this answer comes from a bounded scan of stored content",
		},
	}
	for _, p := range usage.Projects {
		collections := make([]string, 0, len(p.Collections))
		for _, col := range p.Collections {
			if col.CollectionName != "" {
				collections = append(collections, col.CollectionName)
			}
		}
		sort.Strings(collections)
		res.Projects = append(res.Projects, ConceptProjectUseResponse{
			ProjectID:   p.ProjectID,
			ProjectName: p.ProjectName,
			Blocks:      p.Blocks,
			Occurrences: p.Occurrences,
			Collections: collections,
		})
	}
	for _, sample := range usage.Samples {
		res.Uses = append(res.Uses, ConceptUseResponse{
			ProjectID:   sample.ProjectID,
			ProjectName: names[sample.ProjectID],
			Stream:      sample.Stream,
			Collection:  sample.CollectionName,
			Document:    sample.ItemName,
			BlockID:     sample.BlockID,
			Locale:      string(sample.Locale),
			Occurrences: sample.Occurrences,
			Text:        sample.Text,
		})
	}
	res.UsesTotal = len(res.Uses)
	return c.JSON(http.StatusOK, res)
}

// conceptUseRows attaches each recorded use's wording, read one indexed block
// at a time.
//
// The graph records the relationship, not the passage, so the text is fetched
// for the page being returned and no further — which is what separates this
// from the walk it replaced: bounded by what is shown, not by what exists.
func (s *Server) conceptUseRows(
	ctx context.Context,
	uses []contextgraph.ConceptUse,
	names map[string]string,
) []ConceptUseResponse {
	out := make([]ConceptUseResponse, 0, len(uses))
	for _, u := range uses {
		out = append(out, ConceptUseResponse{
			ProjectID:   u.Scope.Project,
			ProjectName: names[u.Scope.Project],
			Stream:      u.Scope.Stream,
			Collection:  u.Collection,
			Document:    u.Document,
			BlockID:     u.BlockID,
			Locale:      u.Locale,
			Term:        u.Term,
			Status:      u.Status,
			Discouraged: u.Discouraged,
			Occurrences: u.Occurrences,
			Text:        s.blockWording(ctx, u),
		})
	}
	return out
}

// blockWording reads the text one use sits in, by content key. A block the
// content store no longer holds contributes no text rather than failing the
// answer: the graph is a projection, and a projection may name a row that has
// since moved.
func (s *Server) blockWording(ctx context.Context, u contextgraph.ConceptUse) string {
	if s.ContentStore == nil || u.ContentKey == "" || u.Scope.Project == "" {
		return ""
	}
	blocks, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID:   u.Scope.Project,
		Stream:      u.Scope.Stream,
		ContentHash: u.ContentKey,
		Limit:       1,
	})
	if err != nil || len(blocks) == 0 || blocks[0] == nil {
		return ""
	}
	text := blocks[0].SourceText()
	if u.Locale != "" {
		if targeted := blocks[0].Text(model.LocaleID(u.Locale)); targeted != "" {
			text = targeted
		}
	}
	return clip.Runes(text, conceptUseTextLimit)
}

// conceptUseTextLimit is how much of a block's wording one row carries. Long
// enough to recognise the passage, short enough that a page of them is a page.
const conceptUseTextLimit = 200

// workspaceProjectNames maps the workspace's project ids to their display
// names. The graph keys on the id — which a rename does not touch — so the name
// is joined here, once per answer.
func (s *Server) workspaceProjectNames(ctx context.Context, wsID string) (map[string]string, error) {
	names := map[string]string{}
	if s.ContentStore == nil {
		return names, nil
	}
	projects, err := s.ContentStore.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p == nil || (wsID != "" && p.WorkspaceID != wsID) {
			continue
		}
		names[p.ID] = p.Name
	}
	return names, nil
}

// conceptAnswerScope reads the point this answer is resolved at. The instant
// defaults to now: a reader asking which projects use a concept is asking about
// today unless they say otherwise.
func conceptAnswerScope(c echo.Context) (coreg.Scope, error) {
	at, err := validityScopeAt(c.QueryParam("at"), c.QueryParam("market"))
	if err != nil {
		return coreg.Scope{}, err
	}
	if at.At.IsZero() {
		at.At = time.Now().UTC()
	}
	return at, nil
}

// validityScopeAt reads an instant and a market into the evaluation point a
// validity window is matched against. An absent instant leaves the zero point,
// which is the as-declared view: every window unapplied.
func validityScopeAt(rawAt, market string) (coreg.Scope, error) {
	var out coreg.Scope
	if raw := strings.TrimSpace(rawAt); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return coreg.Scope{}, errors.New("at must be an RFC 3339 instant")
		}
		out.At = parsed.UTC()
	}
	if m := strings.TrimSpace(market); m != "" {
		out.Tags = map[string]string{validityMarketTag: m}
	}
	return out, nil
}

// validityMarketTag is the validity-tag key a market scopes a term by — the
// same key the concept list facets on.
const validityMarketTag = "market"

func conceptTermUses(in []contextgraph.TermUse) []ConceptTermUseResponse {
	out := make([]ConceptTermUseResponse, 0, len(in))
	for _, t := range in {
		out = append(out, ConceptTermUseResponse{
			Term:        t.Term,
			TermLocale:  t.TermLocale,
			Status:      t.Status,
			Discouraged: t.Discouraged,
			Blocks:      t.Blocks,
			Occurrences: t.Occurrences,
		})
	}
	return out
}

// conceptFor reads the concept the answer is about from the workspace
// terminology. The graph says how the concept STANDS in a project; the terms
// store is what says what to reach for instead, so the two are read together
// rather than one being asked to hold the other's job.
func (s *Server) conceptFor(ctx context.Context, wsSlug, conceptID string) (terms.Concept, bool, error) {
	if s.wsStores == nil {
		return terms.Concept{}, false, nil
	}
	tb, err := s.wsStores.getTerms(wsSlug)
	if err != nil {
		return terms.Concept{}, false, err
	}
	return tb.GetConcept(ctx, conceptID)
}

// replacementFor answers "then what should it say" for a project that reached
// for a discouraged spelling: the concept's preferred term in the language the
// discouraged one was recorded in, resolved at the same instant.
func replacementFor(c terms.Concept, p contextgraph.ProjectUse, at coreg.Scope) string {
	if !p.Discouraged {
		return ""
	}
	locale := model.LocaleID("")
	for _, t := range p.Terms {
		if t.Discouraged {
			locale = model.LocaleID(t.TermLocale)
			break
		}
	}
	return preferredTermAt(c, locale, at)
}

// preferredTermAt is the concept's preferred spelling in force at an instant.
//
// It resolves the window as well as the status, because a preferred term whose
// own window has not opened is not yet the answer to "say this instead", and
// offering it would tell a writer to make a change that is itself out of force.
func preferredTermAt(c terms.Concept, locale model.LocaleID, at coreg.Scope) string {
	fallback := ""
	for _, t := range c.Terms {
		if locale != "" && t.Locale != locale {
			continue
		}
		if !at.At.IsZero() && !t.Validity.Matches(at) {
			continue
		}
		if t.Status == model.TermPreferred {
			return t.Text
		}
		if fallback == "" && !t.Status.Discouraged() {
			fallback = t.Text
		}
	}
	return fallback
}
