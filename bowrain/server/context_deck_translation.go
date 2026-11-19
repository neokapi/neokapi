package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// deckKindTermTranslation is a card proposing the term a kept concept takes
// in one target language.
const deckKindTermTranslation = "term_translation"

// itemTermProposal names a proposed target term: its id is the concept id
// and the target locale, joined by termProposalSep. The ledger records no
// attestation of one: keeping it writes a term of the concept, and the
// concept is what the deck attests.
const (
	itemTermProposal contextop.ItemKind = "term_proposal"
	termProposalSep  string             = "/"
)

// contextDeckTranslation is what a term-translation card proposes.
type contextDeckTranslation struct {
	// Term is the concept's source-language term.
	Term string `json:"term"`
	// Proposed is the term proposed in Locale.
	Proposed string `json:"proposed"`
	Locale   string `json:"locale"`
}

// termProposalItem is the item a concept's proposal for locale is dealt as.
func termProposalItem(conceptID string, locale model.LocaleID) contextop.ItemRef {
	return contextop.ItemRef{Kind: itemTermProposal, ID: conceptID + termProposalSep + string(locale)}
}

// parseTermProposalItem reads the concept id and the locale back from a
// term-proposal item.
func parseTermProposalItem(item contextop.ItemRef) (string, model.LocaleID, bool) {
	if item.Kind != itemTermProposal {
		return "", "", false
	}
	cid, loc, ok := strings.Cut(item.ID, termProposalSep)
	return cid, model.LocaleID(loc), ok && cid != "" && loc != ""
}

// translationCards are the cards of the target terms proposed for a concept,
// one per locale, each placed where the concept card stands: the same
// reach, where and time. Its revision is a digest of the proposed text, so
// a proposal replaced by another is a new card.
func (s *Server) translationCards(ctx context.Context, cp terms.Concept, concept contextDeckCard, source model.LocaleID, names map[string]string) []contextDeckCard {
	var out []contextDeckCard
	for _, tr := range termcandidate.Translations(cp) {
		sum := sha256.Sum256([]byte(string(tr.Locale) + "\x00" + tr.Text))
		out = append(out, contextDeckCard{
			Item:     termProposalItem(cp.ID, tr.Locale),
			Kind:     deckKindTermTranslation,
			Revision: "p" + hex.EncodeToString(sum[:8]),
			Text:     cp.Definition,
			Language: string(tr.Locale),
			Translation: &contextDeckTranslation{
				Term: termcandidate.SourceText(cp, source), Proposed: tr.Text, Locale: string(tr.Locale),
			},
			Where:  concept.Where,
			Reach:  concept.Reach,
			At:     concept.At,
			Author: s.memberAuthor(ctx, termcandidate.ProposerIdentity, names),
		})
	}
	return out
}

// mayReviewLanguage reports whether a reviewer's language scope covers
// locale. An empty scope covers every language.
func (rv deckReviewer) mayReviewLanguage(locale string) bool {
	return len(rv.languages) == 0 || slices.Contains(rv.languages, locale)
}

// followConcepts moves each term-translation card to just after the card of
// its concept, when the deck holds that card, so the reviewer who decides a
// source term decides its translations next. A translation whose concept is
// reviewed already keeps its place among the candidates, by reach.
func followConcepts(cards []contextDeckCard) []contextDeckCard {
	after := map[string][]contextDeckCard{}
	present := map[string]bool{}
	for _, c := range cards {
		if c.Item.Kind == contextop.ItemConcept {
			present[c.Item.ID] = true
		}
	}
	out := make([]contextDeckCard, 0, len(cards))
	for _, c := range cards {
		if cid, _, ok := parseTermProposalItem(c.Item); ok && present[cid] {
			after[cid] = append(after[cid], c)
			continue
		}
		out = append(out, c)
	}
	if len(after) == 0 {
		return cards
	}
	merged := make([]contextDeckCard, 0, len(cards))
	for _, c := range out {
		merged = append(merged, c)
		if c.Item.Kind == contextop.ItemConcept {
			merged = append(merged, after[c.Item.ID]...)
		}
	}
	return merged
}

// errProposalGone is a proposal changed or cleared between the deal and the
// decision.
var errProposalGone = errors.New("the proposed term changed or is gone")

// decideTermTranslation keeps or rejects a proposed target term. Keep makes
// the proposal, or the reviewer's own wording in req.Text, the preferred
// term of the concept in that language. Like a candidate keep, the decision
// is the review: it writes the terms store directly, with no change set. When
// the concept was reviewed before the keep, the keep attests the concept as
// it then reads, in the reviewer's name, so it does not come back for review
// as changed. Reject clears the proposal and records its text as turned
// down, so it is not proposed again.
func (s *Server) decideTermTranslation(c echo.Context, src deckSource, rv deckReviewer, pending []contextDeckCard, card contextDeckCard, req contextDeckDecideRequest) error {
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	cid, loc, _ := parseTermProposalItem(card.Item)
	if !rv.mayReviewLanguage(string(loc)) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: "no access to language: " + string(loc), Code: deckCodeNotOffered})
	}
	ctx := c.Request().Context()
	tb, err := s.wsStores.getTerms(ctx, src.wsSlug)
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	concept, ok, err := tb.GetConcept(ctx, cid)
	if err != nil {
		return serverErr(c, err)
	}
	if !ok || card.Translation == nil || termcandidate.TranslationFor(concept, loc) != card.Translation.Proposed {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: errProposalGone.Error(), Code: deckCodeNotPending})
	}

	if req.Action == deckActionReject {
		termcandidate.RejectTranslation(&concept, loc)
		if err := tb.AddConcept(ctx, concept); err != nil {
			return serverErr(c, err)
		}
		return c.JSON(http.StatusOK, contextDeckDecideResponse{Outcome: deckOutcomeDropped})
	}

	_, conceptPending := findCard(pending, contextop.ItemRef{Kind: contextop.ItemConcept, ID: cid})
	text := strings.TrimSpace(req.Text)
	if text == "" {
		text = card.Translation.Proposed
	}
	termcandidate.KeepTranslation(&concept, loc, text)
	concept.UpdatedAt = time.Now().UTC()
	if err := tb.AddConcept(ctx, concept); err != nil {
		return serverErr(c, err)
	}
	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		conceptEvent(knowledge.EventConceptUpdated, src.wsID, concept.ID, rv.userID),
	})
	if !conceptPending {
		if err := s.attestConceptAfterWrite(c, src, rv, cid, req.Note); err != nil {
			return serverErr(c, err)
		}
	}
	return c.JSON(http.StatusOK, contextDeckDecideResponse{Outcome: deckOutcomeKept})
}

// attestConceptAfterWrite records the reviewer's attestation of a concept
// at the revision it reads at now, when the deck would otherwise deal it
// again and the reviewer may keep it.
func (s *Server) attestConceptAfterWrite(c echo.Context, src deckSource, rv deckReviewer, conceptID, note string) error {
	ctx := c.Request().Context()
	cards, err := s.contextItems(ctx, src, rv)
	if err != nil {
		return err
	}
	fresh, ok := findCard(cards, contextop.ItemRef{Kind: contextop.ItemConcept, ID: conceptID})
	if !ok || fresh.Reviewed || fresh.Basis == "" || !slices.Contains(fresh.Actions, deckActionKeep) {
		return nil
	}
	st := &decideState{
		project: src.project,
		key:     src.key,
		member:  ledgerMemberOf(c),
		reader:  contextop.Actor{Kind: contextop.ActorPerson, Name: rv.name},
		byID:    map[string]contextop.Record{},
	}
	attest := contextop.Record{
		Project: src.key,
		Actor:   st.reader,
		Kind:    contextop.KindAttest,
		Note:    note,
		Attest: &contextop.Attestation{
			Item: fresh.Item, Revision: fresh.Revision, Verdict: contextop.VerdictKeep, Basis: fresh.Basis,
		},
	}
	recorded, err := s.recordPlan(ctx, st, []contextop.Record{attest})
	if err != nil {
		return err
	}
	s.emitDecidedContext(c, st, conceptID, recorded)
	return nil
}

// enqueueTermProposals starts a job that proposes target terms for the
// project's concepts (all of them when concepts is empty). It reports false
// with no error when the platform has no job queue or the workspace has no
// credits left, which leaves the concepts waiting for a later run.
func (s *Server) enqueueTermProposals(ctx context.Context, wsID, wsSlug string, project *platstore.Project, concepts []string) (string, bool, error) {
	if s.ContextScanStore == nil || s.ContextScanQueue == nil || len(project.TargetLanguages) == 0 {
		return "", false, nil
	}
	if s.insufficientPlatformCredits(ctx, wsID, "platform") {
		return "", false, nil
	}
	raw, err := json.Marshal(jobs.ContextScanRequest{
		Project: project.ID, TermProposals: &jobs.TermProposalRequest{Concepts: concepts},
	})
	if err != nil {
		return "", false, err
	}
	job := &jobs.ContextScanJob{
		ID: id.New(), WorkspaceID: wsID, WorkspaceSlug: wsSlug,
		Status: jobs.ContextScanStatusQueued, Request: raw,
	}
	if err := s.ContextScanStore.CreateContextScanJob(ctx, job); err != nil {
		return "", false, err
	}
	if err := s.ContextScanQueue.Enqueue(ctx, job.ID); err != nil {
		_ = s.ContextScanStore.DeleteContextScanJob(ctx, job.ID)
		return "", false, fmt.Errorf("enqueue failed: %w", err)
	}
	return job.ID, true, nil
}

// proposeAfterKeep starts proposing target terms for a concept a reviewer
// just kept. A failure is logged and costs only the proposal: the keep
// stands, and the project-wide run picks the concept up later.
func (s *Server) proposeAfterKeep(c echo.Context, src deckSource, conceptID string) {
	ctx := c.Request().Context()
	if _, _, err := s.enqueueTermProposals(ctx, src.wsID, src.wsSlug, src.project, []string{conceptID}); err != nil {
		slog.WarnContext(ctx, "term proposal after keep was not started", "project", src.project.ID, "concept", conceptID, "error", err)
	}
}

// termProposalsResponse answers a request to propose target terms.
type termProposalsResponse struct {
	// Waiting counts the concept and language pairs with no usable term.
	Waiting int `json:"waiting"`
	// JobID is the job proposing them, empty when none was started.
	JobID  string `json:"job_id,omitempty"`
	Status string `json:"status,omitempty"`
}

// HandleProposeTermTranslations answers POST /:ws/:id/context/term-proposals:
// it starts a job that proposes, with AI, the term each concept of the
// project takes in each target language that has none, and files each
// proposal on its concept for the context deck to deal. It answers 202 with
// the job, or 200 with no job when nothing waits. The job's progress reads
// at GET /:ws/context-scans/:job_id.
func (s *Server) HandleProposeTermTranslations(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.ContentStore == nil || s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "content or terms store not configured"})
	}
	if s.ContextScanStore == nil || s.ContextScanQueue == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "context scan system not configured"})
	}
	ctx := c.Request().Context()
	wsID, _ := c.Get("workspace_id").(string)
	project, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || project == nil || (wsID != "" && project.WorkspaceID != wsID) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: errNoProject.Error()})
	}
	tb, err := s.wsStores.getTerms(ctx, c.Param("ws"))
	if err != nil || tb == nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return serverErr(c, err)
	}
	out := termProposalsResponse{}
	source := project.DefaultSourceLanguage
	for _, cp := range concepts {
		if p := termcandidate.ProjectOf(cp); p != "" && p != project.ID {
			continue
		}
		for _, loc := range project.TargetLanguages {
			if termcandidate.NeedsTranslation(cp, source, loc) {
				out.Waiting++
			}
		}
	}
	if out.Waiting == 0 {
		return c.JSON(http.StatusOK, out)
	}
	if s.insufficientPlatformCredits(ctx, wsID, "platform") {
		return c.JSON(http.StatusPaymentRequired, errInsufficientCredits)
	}
	jobID, started, err := s.enqueueTermProposals(ctx, wsID, c.Param("ws"), project, nil)
	if err != nil {
		return serverErr(c, err)
	}
	if !started {
		return c.JSON(http.StatusOK, out)
	}
	out.JobID, out.Status = jobID, string(jobs.ContextScanStatusQueued)
	return c.JSON(http.StatusAccepted, out)
}
