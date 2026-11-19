package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workspace"
)

// contextDeckPageSize is the cards a page holds when the request names no
// limit, and contextDeckMaxPage the most it may ask for.
const (
	contextDeckPageSize = 50
	contextDeckMaxPage  = 200
)

// Codes a context deck decision is refused with.
const (
	deckCodeStale         = "stale"
	deckCodeNotPending    = "not_pending"
	deckCodeNotOffered    = "action_not_offered"
	deckCodeRetireHeld    = "retire_needs_approval"
	deckCodeNoContextKey  = "no_context_key"
	deckModePending       = "pending"
	deckOutcomeAttested   = "attested"
	deckOutcomeKept       = "kept"
	deckOutcomeDropped    = "dropped"
	deckOutcomeProposed   = "proposed"
	deckRetireOriginStart = "context-deck/retire/"
)

func (s *Server) registerContextDeckRoutes(g *echo.Group) {
	g.GET("/:id/context/deck", s.HandleContextDeck)
	g.GET("/:id/context/deck/origins/:op", s.HandleContextDeckOrigin)
	g.GET("/:id/context/deck/examples", s.HandleContextRuleExamples)
	g.POST("/:id/context/deck/decide", s.refuseTool(s.HandleContextDeckDecide))
	s.registerContextSessionRoutes(g)
}

// contextDeckResponse is a page of the context deck.
type contextDeckResponse struct {
	Mode string `json:"mode"`
	// Bound says the project's context has a project key, which an
	// attestation needs. A project no push has bound offers no keep.
	Bound      bool              `json:"bound"`
	Counts     contextDeckCounts `json:"counts"`
	Cards      []contextDeckCard `json:"cards"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// contextDeckDecideRequest is one decision about one card.
type contextDeckDecideRequest struct {
	Item contextop.ItemRef `json:"item"`
	// Revision is the revision the card showed. A decision about a revision
	// that has moved is refused as stale.
	Revision string `json:"revision"`
	// Action is keep, reject or later.
	Action string `json:"action"`
	Note   string `json:"note,omitempty"`
	// Text is the reviewer's own wording for a kept term-translation card,
	// in place of the proposed term.
	Text string `json:"text,omitempty"`
}

// contextDeckDecideResponse is what a decision did.
type contextDeckDecideResponse struct {
	// Outcome is attested, kept (a suggestion kept and attested), dropped,
	// proposed (a change set waits for review) or later.
	Outcome     string `json:"outcome"`
	ChangesetID string `json:"changeset_id,omitempty"`
}

// contextDeckSource reads what the deck of a project is dealt from.
func (s *Server) contextDeckSource(ctx context.Context, wsID, wsSlug string, project *platstore.Project, userID string) (deckSource, error) {
	src := deckSource{wsID: wsID, wsSlug: wsSlug, project: project}
	cos, ok := s.contextObjects()
	if !ok {
		return src, nil
	}
	raw, err := cos.ContextKey(ctx, project.ID)
	if err != nil {
		return src, err
	}
	src.key = workspace.ProjectKey(raw)
	if src.key == "" {
		return src, nil
	}
	ledger, err := s.ContextLedger(ctx, project.ID, LedgerMember{UserID: userID})
	if err != nil {
		return src, err
	}
	if src.records, err = ledger.Records(ctx, contextop.Filter{}); err != nil {
		return src, err
	}
	return src, nil
}

// requestReviewer is the signed-in member of a request as the deck judges
// them.
func (s *Server) requestReviewer(c echo.Context, wsID string) deckReviewer {
	member := ledgerMemberOf(c)
	ctx := c.Request().Context()
	languages, _ := c.Get("project_languages").([]string)
	return deckReviewer{
		userID:    member.UserID,
		name:      s.displayName(ctx, member.UserID),
		perms:     member.Permissions,
		reach:     member.Reach,
		solo:      s.soleReviewer(ctx, wsID, member.UserID),
		languages: languages,
	}
}

// deckRequest reads the project, the reviewer and the deck source of a deck
// request.
func (s *Server) deckRequest(c echo.Context) (deckSource, deckReviewer, error) {
	if s.ContentStore == nil {
		return deckSource{}, deckReviewer{}, errors.New("content store not configured")
	}
	ctx := c.Request().Context()
	project, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || project == nil {
		return deckSource{}, deckReviewer{}, errNoProject
	}
	wsID, _ := c.Get("workspace_id").(string)
	rv := s.requestReviewer(c, wsID)
	src, err := s.contextDeckSource(ctx, wsID, c.Param("ws"), project, rv.userID)
	return src, rv, err
}

// HandleContextDeck answers GET /:ws/:id/context/deck: the project's context
// items no attestation covers at their current revision, as cards in deck
// order, with their counts by kind. ?mode=pending is the one mode; ?kind=
// narrows to one card kind; ?cursor= and ?limit= page it.
func (s *Server) HandleContextDeck(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if mode := c.QueryParam("mode"); mode != "" && mode != deckModePending {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("mode %q is not supported: the deck deals pending items only", mode)})
	}
	src, rv, err := s.deckRequest(c)
	if errors.Is(err, errNoProject) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	if err != nil {
		return serverErr(c, err)
	}
	cards, err := s.pendingContext(c.Request().Context(), src, rv)
	if err != nil {
		return serverErr(c, err)
	}
	if cards, err = s.withoutHeld(c.Request().Context(), src.project.ID, rv.userID, cards); err != nil {
		return serverErr(c, err)
	}
	out := contextDeckResponse{Mode: deckModePending, Bound: src.key != "", Counts: countDeck(cards)}
	if kind := c.QueryParam("kind"); kind != "" {
		cards = slices.DeleteFunc(cards, func(card contextDeckCard) bool { return card.Kind != kind })
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = contextDeckPageSize
	}
	limit = min(limit, contextDeckMaxPage)
	offset, _ := strconv.Atoi(c.QueryParam("cursor"))
	offset = max(0, min(offset, len(cards)))
	end := min(offset+limit, len(cards))
	out.Cards = cards[offset:end]
	if end < len(cards) {
		out.NextCursor = strconv.Itoa(end)
	}
	if out.Cards == nil {
		out.Cards = []contextDeckCard{}
	}
	return c.JSON(http.StatusOK, out)
}

// contextDeckOriginResponse is a page of the items one bulk operation
// brought in.
type contextDeckOriginResponse struct {
	Origin *contextDeckOrigin `json:"origin"`
	// Total counts the items the project holds from the operation, and
	// Pending those still waiting for review.
	Total      int               `json:"total"`
	Pending    int               `json:"pending"`
	Cards      []contextDeckCard `json:"cards"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// HandleContextDeckOrigin answers GET /:ws/:id/context/deck/origins/:op: the
// bulk operation op and the items of the project it brought in, reviewed or
// not, in deck order. ?cursor= and ?limit= page it.
func (s *Server) HandleContextDeckOrigin(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	src, rv, err := s.deckRequest(c)
	if errors.Is(err, errNoProject) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	if err != nil {
		return serverErr(c, err)
	}
	op := c.Param("op")
	cards, err := s.contextItems(c.Request().Context(), src, rv)
	if err != nil {
		return serverErr(c, err)
	}
	cards = slices.DeleteFunc(cards, func(card contextDeckCard) bool { return card.Origin == nil || card.Origin.ID != op })
	out := contextDeckOriginResponse{Total: len(cards)}
	for _, card := range cards {
		if !card.Reviewed {
			out.Pending++
		}
	}
	if len(cards) > 0 {
		out.Origin = cards[0].Origin
	} else {
		for _, o := range s.deckOrigins(c.Request().Context(), bulkRecords(src), map[string]string{}) {
			if o.ID == op {
				out.Origin = o
			}
		}
	}
	if out.Origin == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "no bulk operation of this project has that id"})
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = contextDeckPageSize
	}
	limit = min(limit, contextDeckMaxPage)
	offset, _ := strconv.Atoi(c.QueryParam("cursor"))
	offset = max(0, min(offset, len(cards)))
	end := min(offset+limit, len(cards))
	out.Cards = slices.Clone(cards[offset:end])
	for i := range out.Cards {
		out.Cards[i].Origin = nil
	}
	if end < len(cards) {
		out.NextCursor = strconv.Itoa(end)
	}
	if out.Cards == nil {
		out.Cards = []contextDeckCard{}
	}
	return c.JSON(http.StatusOK, out)
}

// HandleContextDeckDecide answers POST /:ws/:id/context/deck/decide: one
// decision about one card. Keep records an attestation of the revision the
// card showed, and for a suggestion keeps it first. Keep on a candidate term
// makes its proposed term preferred, attests the concept as it then reads,
// and starts proposing its term in each target language. Keep on a
// term-translation card adds the proposed term, or the reviewer's wording
// in text, to the concept. Reject drops a term rule or a note from the ledger, deletes a
// candidate term and records that it is not one, and proposes a change set
// that deprecates the terms of any other concept. Later records nothing. A revision that has moved since the
// card was dealt answers 409 stale with the card as it reads now. A card
// another person's live session holds answers 409 held; a decision about a
// card the caller's own session holds is recorded in that session and
// releases the card.
func (s *Server) HandleContextDeckDecide(c echo.Context) error {
	var req contextDeckDecideRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	req.Note = strings.TrimSpace(req.Note)
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	css, sessions := s.contextSessionStore()
	if !sessions || req.Action == deckActionLater {
		return s.decideContextCard(c, req)
	}
	ctx := c.Request().Context()
	userID := ledgerMemberOf(c).UserID
	own, held, err := s.sessionHolding(ctx, css, projectParam(c), userID, req.Item.Key())
	if err != nil {
		return serverErr(c, err)
	}
	if held {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: "another person's session holds this card", Code: deckCodeHeld})
	}
	if err := s.decideContextCard(c, req); err != nil || own == "" || c.Response().Status != http.StatusOK {
		return err
	}
	now := sessionNow().UTC()
	d := platstore.ContextSessionDecision{Item: req.Item.Key(), Outcome: sessionOutcome(req), At: now}
	if err := css.RecordContextSessionDecision(ctx, own, d, now.Add(contextSessionIdle)); err != nil {
		c.Logger().Errorf("record a decision in context session %s: %v", own, err)
	}
	return nil
}

// decideContextCard is HandleContextDeckDecide's decision itself.
func (s *Server) decideContextCard(c echo.Context, req contextDeckDecideRequest) error {
	if req.Action == deckActionLater {
		return c.JSON(http.StatusOK, contextDeckDecideResponse{Outcome: deckActionLater})
	}
	if req.Action != deckActionKeep && req.Action != deckActionReject {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("action %q is none of keep, reject and later", req.Action)})
	}
	ctx := c.Request().Context()
	src, rv, err := s.deckRequest(c)
	if errors.Is(err, errNoProject) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	if err != nil {
		return serverErr(c, err)
	}
	if src.key == "" {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: ErrNoContextKey.Error(), Code: deckCodeNoContextKey})
	}
	cards, err := s.pendingContext(ctx, src, rv)
	if err != nil {
		return serverErr(c, err)
	}
	card, ok := findCard(cards, req.Item)
	if !ok {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: "the item is not waiting for review", Code: deckCodeNotPending})
	}
	if card.Revision != req.Revision {
		return c.JSON(http.StatusConflict, map[string]any{
			"error": "the item changed since the card was dealt", "code": deckCodeStale, "card": card,
		})
	}
	if !slices.Contains(card.Actions, req.Action) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: req.Action + " is not offered on this card", Code: deckCodeNotOffered})
	}

	if card.Kind == deckKindTermTranslation {
		return s.decideTermTranslation(c, src, rv, cards, card, req)
	}
	promoted := false
	if card.Proposed && card.Item.Kind == contextop.ItemConcept {
		if req.Action == deckActionReject {
			return s.forgetCandidate(c, src, rv, card)
		}
		fresh, err := s.keepCandidate(c, src, rv, card)
		if errors.Is(err, errCandidateGone) {
			return c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error(), Code: deckCodeNotPending})
		}
		if err != nil {
			return serverErr(c, err)
		}
		card, promoted = fresh, true
	}
	if req.Action == deckActionReject && card.Item.Kind == contextop.ItemConcept {
		return s.proposeConceptRetire(c, src, rv, card)
	}

	st := &decideState{
		project: src.project,
		key:     src.key,
		member:  ledgerMemberOf(c),
		reader:  contextop.Actor{Kind: contextop.ActorPerson, Name: rv.name},
		byID:    map[string]contextop.Record{},
	}
	for _, r := range src.records {
		st.byID[r.ID] = r
	}
	plan, outcome, refusal := s.deckPlan(c, src, rv, st, card, req)
	if refusal != nil {
		return refusal
	}
	recorded, err := s.recordPlan(ctx, st, plan)
	if err != nil {
		if errors.Is(err, ErrOutsideReach) {
			return c.JSON(http.StatusForbidden, ErrorResponse{Error: err.Error(), Code: contextOutsideReachCode})
		}
		if errors.Is(err, contextop.ErrRefused) {
			return c.JSON(http.StatusForbidden, ErrorResponse{Error: err.Error()})
		}
		return serverErr(c, err)
	}
	s.emitDecidedContext(c, st, card.Item.ID, recorded)
	if promoted {
		outcome = deckOutcomeKept
		s.proposeAfterKeep(c, src, card.Item.ID)
	}
	return c.JSON(http.StatusOK, contextDeckDecideResponse{Outcome: outcome})
}

// errCandidateGone is a candidate term deleted between the deal and the
// decision.
var errCandidateGone = errors.New("the candidate term is gone")

// keepCandidate makes a candidate term a term of the project: its proposed
// source-language term becomes preferred. It returns the card as the deck
// deals it after the write, whose revision the keep then attests, so the
// concept does not come back for review as changed.
//
// A move to preferred is a governed transition (terms.IsGovernedTransition),
// which other surfaces route through a change set. Here the keep is that
// review: the scan proposed the term, and a person who may manage terms,
// judged against the scan as the author (contextDeckCard.judge), decides it.
// The attestation the keep records names the person and the basis.
func (s *Server) keepCandidate(c echo.Context, src deckSource, rv deckReviewer, card contextDeckCard) (contextDeckCard, error) {
	if s.wsStores == nil {
		return card, errKnowledgeUnavailable
	}
	ctx := c.Request().Context()
	tb, err := s.wsStores.getTerms(ctx, src.wsSlug)
	if err != nil {
		return card, err
	}
	concept, ok, err := tb.GetConcept(ctx, card.Item.ID)
	if err != nil {
		return card, err
	}
	if !ok {
		return card, errCandidateGone
	}
	if termcandidate.Promote(&concept, src.project.DefaultSourceLanguage) {
		concept.UpdatedAt = time.Now().UTC()
		if err := tb.AddConcept(ctx, concept); err != nil {
			return card, err
		}
		s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
			conceptEvent(knowledge.EventConceptUpdated, src.wsID, concept.ID, rv.userID),
		})
	}
	cards, err := s.contextItems(ctx, src, rv)
	if err != nil {
		return card, err
	}
	fresh, ok := findCard(cards, card.Item)
	if !ok {
		return card, nil
	}
	fresh.Basis = card.Basis
	return fresh, nil
}

// forgetCandidate deletes a candidate term and records on the project that it
// is not a term (termcandidate.ProjectPropRejected), so no later scan files
// it again. The rejection is recorded first: a deletion that fails after it
// leaves a candidate the next decision deletes, never one the next scan
// brings back.
func (s *Server) forgetCandidate(c echo.Context, src deckSource, rv deckReviewer, card contextDeckCard) error {
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	ctx := c.Request().Context()
	tb, err := s.wsStores.getTerms(ctx, src.wsSlug)
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	concept, ok, err := tb.GetConcept(ctx, card.Item.ID)
	if err != nil {
		return serverErr(c, err)
	}
	term := ""
	if ok {
		term = termcandidate.SourceText(concept, src.project.DefaultSourceLanguage)
	}
	if term == "" && len(card.Terms) > 0 {
		term = card.Terms[0].Text
	}
	project, err := s.ContentStore.GetProject(ctx, src.project.ID)
	if err != nil || project == nil {
		return serverErr(c, fmt.Errorf("read the project: %w", err))
	}
	props := maps.Clone(project.Properties)
	if props == nil {
		props = map[string]string{}
	}
	if termcandidate.AddRejected(props, term) {
		project.Properties = props
		if err := s.ContentStore.UpdateProject(ctx, project); err != nil {
			return serverErr(c, err)
		}
	}
	if ok {
		if err := tb.DeleteConcept(ctx, concept.ID); err != nil {
			return serverErr(c, err)
		}
		s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
			conceptEvent(knowledge.EventConceptDeleted, src.wsID, concept.ID, rv.userID),
		})
	}
	return c.JSON(http.StatusOK, contextDeckDecideResponse{Outcome: deckOutcomeDropped})
}

// deckPlan is the ledger records a keep or a reject of card writes, or the
// response that refuses it.
func (s *Server) deckPlan(c echo.Context, src deckSource, rv deckReviewer, st *decideState, card contextDeckCard, req contextDeckDecideRequest) ([]contextop.Record, string, error) {
	if req.Action == deckActionKeep {
		attest := contextop.Record{
			Project: src.key,
			Actor:   st.reader,
			Kind:    contextop.KindAttest,
			Note:    req.Note,
			Attest: &contextop.Attestation{
				Item: card.Item, Revision: card.Revision, Verdict: contextop.VerdictKeep, Basis: card.Basis,
			},
		}
		if card.Item.Kind != contextop.ItemOp {
			return []contextop.Record{attest}, deckOutcomeAttested, nil
		}
		attest.Target = card.Item.ID
		if card.Status == string(contextop.StatusSuggested) {
			target := st.byID[card.Item.ID]
			keep := st.record(contextop.KindKeep, target)
			keep.Scope = target.Scope
			return []contextop.Record{keep, attest}, deckOutcomeKept, nil
		}
		return []contextop.Record{attest}, deckOutcomeAttested, nil
	}

	// A suggestion is set aside as Decide sets it aside. A rule in force is
	// retired under the workspace's change-set policy: under block a second
	// person has to approve it, which the ledger cannot stage yet.
	if card.Status == string(contextop.StatusEstablished) {
		mode := platauth.SoDOff
		if s.AuthStore != nil {
			if m, err := s.AuthStore.GetSoDMode(c.Request().Context(), src.wsID); err == nil {
				mode = m
			}
		}
		switch {
		case mode == platauth.SoDBlock && !rv.solo:
			return nil, "", c.JSON(http.StatusConflict, ErrorResponse{
				Error: "retiring a rule in force needs a second person's approval under this workspace's policy",
				Code:  deckCodeRetireHeld,
			})
		case mode == platauth.SoDWarn:
			s.recordSoDViolation(c, rv.userID, "context:"+card.Item.ID, mode, 1)
		}
	}
	drop := st.record(contextop.KindDrop, st.byID[card.Item.ID])
	drop.Note = req.Note
	return []contextop.Record{drop}, deckOutcomeDropped, nil
}

// proposeConceptRetire proposes a change set that deprecates every term of
// the card's concept still in use, or deletes a concept whose terms are all
// deprecated or forbidden already, and submits it for review. One open
// proposal is kept per concept, so rejecting the card twice proposes once.
func (s *Server) proposeConceptRetire(c echo.Context, src deckSource, rv deckReviewer, card contextDeckCard) error {
	if s.KnowledgeStore == nil || s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	ctx := c.Request().Context()
	tb, err := s.wsStores.getTerms(ctx, src.wsSlug)
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	concept, ok, err := tb.GetConcept(ctx, card.Item.ID)
	if err != nil {
		return serverErr(c, err)
	}
	if !ok {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: "the concept is gone", Code: deckCodeNotPending})
	}
	label := concept.ID
	if len(card.Terms) > 0 {
		label = card.Terms[0].Text
	}
	cs := &knowledge.ChangeSet{
		WorkspaceID: src.wsID,
		Name:        "Retire " + label,
		Status:      knowledge.ChangeSetDraft,
		Origin:      deckRetireOriginStart + concept.ID,
		CreatedBy:   rv.userID,
	}
	type retireOp struct {
		op   knowledge.OpType
		body any
	}
	var payloads []retireOp
	for _, t := range concept.Terms {
		if t.Status == model.TermDeprecated || t.Status == model.TermForbidden {
			continue
		}
		payloads = append(payloads, retireOp{knowledge.OpTermStatus, knowledge.TermStatusPayload{
			ConceptID: concept.ID, Locale: t.Locale, Text: t.Text, From: t.Status, To: model.TermDeprecated,
		}})
	}
	if len(payloads) == 0 {
		payloads = append(payloads, retireOp{knowledge.OpConceptDelete, knowledge.ConceptDeletePayload{ConceptID: concept.ID}})
	}
	if err := s.KnowledgeStore.CreateChangeSet(ctx, cs); err != nil {
		return serverErr(c, err)
	}
	ops := make([]*knowledge.ChangeSetOp, 0, len(payloads))
	for _, p := range payloads {
		body, err := json.Marshal(p.body)
		if err != nil {
			return serverErr(c, err)
		}
		op := &knowledge.ChangeSetOp{WorkspaceID: src.wsID, ChangesetID: cs.ID, Op: p.op, Payload: body, CreatedBy: rv.userID}
		if err := knowledge.ValidateOp(*op); err != nil {
			return serverErr(c, err)
		}
		if err := s.KnowledgeStore.AppendOp(ctx, op); err != nil {
			return serverErr(c, err)
		}
		ops = append(ops, op)
	}
	res, err := knowledge.SubmitChangeSet(ctx, s.KnowledgeStore, cs, ops)
	if errors.Is(err, knowledge.ErrSubmitRefused) {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	}
	if err != nil {
		return serverErr(c, err)
	}
	if !res.Unchanged {
		events := []knowledge.MergeEvent{changesetEvent(knowledge.EventChangeSetCreated, src.wsID, cs.ID, rv.userID)}
		for _, prior := range res.Superseded {
			events = append(events, changesetEvent(knowledge.EventChangeSetSuperseded, src.wsID, prior, rv.userID))
		}
		events = append(events, changesetEvent(knowledge.EventChangeSetSubmitted, src.wsID, cs.ID, rv.userID))
		s.publishKnowledgeEvents(c, events)
		governed, _ := knowledge.ChangeSetIsGoverned(changeSetOpValues(ops))
		s.summonChangeSetReviewers(c, cs, len(ops), governed)
	}
	return c.JSON(http.StatusOK, contextDeckDecideResponse{Outcome: deckOutcomeProposed, ChangesetID: res.ID})
}

// contextInboxEntry is one project with context waiting for the caller's
// review.
type contextInboxEntry struct {
	Workspace     string         `json:"workspace"`
	WorkspaceName string         `json:"workspace_name"`
	Project       string         `json:"project"`
	ProjectName   string         `json:"project_name"`
	Waiting       int            `json:"waiting"`
	ByKind        map[string]int `json:"by_kind"`
}

// contextInbox counts the project's pending context for a member who may
// keep at least one kind of it, and reports false when there is nothing to
// count for them.
func (s *Server) contextInbox(ctx context.Context, ws *platauth.Workspace, p *platstore.Project, userID string, perms platauth.Permission, reach platauth.CoordinateReach) (contextInboxEntry, bool, error) {
	if perms&(platauth.PermManageTerms|platauth.PermManageVoice) == 0 {
		return contextInboxEntry{}, false, nil
	}
	src, err := s.contextDeckSource(ctx, ws.ID, ws.Slug, p, userID)
	if err != nil || src.key == "" {
		return contextInboxEntry{}, false, err
	}
	rv := deckReviewer{userID: userID, name: s.displayName(ctx, userID), perms: perms, reach: reach, solo: s.soleReviewer(ctx, ws.ID, userID)}
	cards, err := s.pendingContext(ctx, src, rv)
	if err != nil {
		return contextInboxEntry{}, false, err
	}
	cards = slices.DeleteFunc(cards, func(c contextDeckCard) bool { return !slices.Contains(c.Actions, deckActionKeep) })
	if cards, err = s.withoutHeld(ctx, p.ID, userID, cards); err != nil {
		return contextInboxEntry{}, false, err
	}
	if len(cards) == 0 {
		return contextInboxEntry{}, false, nil
	}
	counts := countDeck(cards)
	return contextInboxEntry{
		Workspace: ws.Slug, WorkspaceName: ws.Name, Project: p.ID, ProjectName: p.Name,
		Waiting: counts.Total, ByKind: counts.ByKind,
	}, true, nil
}
