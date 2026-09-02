package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
)

// reviewSoD is the separation-of-duties gate for one review request, with the
// authorship the whole request needs resolved up front: one query for every
// (block, locale) pair the pass may touch, so a selection of a thousand blocks
// costs one round trip and not a thousand.
//
// A pair the store attributes to nobody passes. That is the machine-authored
// case the review docs describe: a draft a run produced was written outside any
// request, carries no acting user, and stays approvable by the one person in a
// small workspace.
//
// The policy is read once per request too, for the same reason: judging one
// block at a time would put a workspace lookup between every pair.
type reviewSoD struct {
	srv     *Server
	c       echo.Context
	actor   string
	mode    platauth.SoDMode
	authors map[platstore.TargetRef]string

	// silent holds back the per-target violation record, for a pass that files
	// one for the whole run. violations counts what it held back either way.
	silent     bool
	violations int
}

// quiet stops vet from filing a record per target. An approve-passing pass over
// a corpus the caller wrote would otherwise file one per block, on a bus that
// drops what it cannot keep up with; the handler files one with the count.
func (g *reviewSoD) quiet() *reviewSoD {
	if g != nil {
		g.silent = true
	}
	return g
}

// newReviewSoD reads the workspace policy and, when it is on, the authorship
// for the pass. It reports an error only when a store that keeps target
// authorship failed to answer: a discarded error would disable the four-eyes
// check rather than tighten it, so the caller refuses the request instead. A
// store that keeps no authorship at all knows of no author, and the gate then
// allows every pair.
func (s *Server) newReviewSoD(ctx context.Context, c echo.Context, projectID, stream string, blockIDs, locales []string) (*reviewSoD, error) {
	g := &reviewSoD{srv: s, c: c, mode: platauth.SoDOff}
	g.actor, _ = c.Get("user_id").(string)
	if s.AuthStore == nil || g.actor == "" {
		return g, nil
	}
	wsID, _ := c.Get("workspace_id").(string)
	mode, err := s.AuthStore.GetSoDMode(ctx, wsID)
	if err != nil {
		return g, nil // an unreadable policy is not a reason to refuse a review
	}
	g.mode = mode
	ts, ok := s.ContentStore.(platstore.TargetAuthorStore)
	if mode == platauth.SoDOff || !ok || len(blockIDs) == 0 || len(locales) == 0 {
		return g, nil
	}
	authors, err := ts.LastTargetAuthors(ctx, projectID, stream, blockIDs, locales)
	if err != nil {
		return nil, fmt.Errorf("read target authorship for separation of duties: %w", err)
	}
	g.authors = authors
	return g, nil
}

// vet applies the workspace policy to approving one block's target. It answers
// with a reviewFault the single route renders as a 403 and a batch route
// records against the block, so one protected block never answers for a whole
// selection.
func (g *reviewSoD) vet(blockID, locale string) error {
	if g == nil || g.actor == "" || g.mode == platauth.SoDOff {
		return nil
	}
	author := g.authors[platstore.TargetRef{BlockID: blockID, Locale: locale}]
	if author == "" || author != g.actor {
		return nil // machine-authored, or somebody else's writing
	}
	g.violations++
	if !g.silent {
		g.srv.recordSoDViolation(g.c, g.actor, "approve_block:"+blockID+":"+locale, g.mode, 1)
	}
	if g.mode == platauth.SoDBlock {
		return reviewFault{http.StatusForbidden, sodRefusal}
	}
	return nil // warn: recorded, but allowed
}

// reviewLedger writes review verdicts where they last. Two writes, in order:
// the decision ledger, carrying the decider's identity, the time and the hash
// of the translation each verdict blesses, and then the workspace content
// memory through jobs.PromoteDecisionsToMemory, the same door the push-ingest
// path uses, so the two entry points cannot disagree about what an approval
// admits.
//
// Everything a verdict needs beyond the verdict itself resolves once, when the
// ledger opens: the decider's readable name, and (on the first verdict that is
// a corpus verdict) the project's source language and the workspace memory. A
// pass over a corpus then pays per batch rather than per block.
//
// Best-effort by design: the projected status is already written, so a store
// without the ledger capability, or a failed write, must not fail the review
// the user just made. Every failure is logged, because a ledger that quietly
// misses reviews is worse than none.
type reviewLedger struct {
	srv       *Server
	ds        platstore.DecisionStore
	projectID string
	stream    string
	decider   string

	// The corpus half, resolved on first need and remembered, including a
	// failure: a pass whose workspace has no memory must not re-ask per batch.
	corpusResolved bool
	sourceLang     model.LocaleID
	memory         memory.Store
}

// newReviewLedger opens a ledger over the request's content store, or reports
// nil when the store keeps no decision ledger.
func (s *Server) newReviewLedger(ctx context.Context, c echo.Context, projectID, stream string) *reviewLedger {
	ds, ok := s.ContentStore.(platstore.DecisionStore)
	if !ok {
		return nil
	}
	// The decider, as readably as the auth store can name them. Falls back to
	// the opaque user id, an identity if a terse one.
	decider, _ := c.Get("user_id").(string)
	if s.AuthStore != nil && decider != "" {
		if u, err := s.AuthStore.GetUser(ctx, decider); err == nil && u != nil && u.Email != "" {
			decider = u.Email
		}
	}
	return &reviewLedger{srv: s, ds: ds, projectID: projectID, stream: stream, decider: decider}
}

// write files a batch of verdicts and promotes what they bless.
func (l *reviewLedger) write(ctx context.Context, decisions []venue.UnitDecision) {
	if l == nil || len(decisions) == 0 {
		return
	}
	if _, err := l.ds.UpsertUnitDecisions(ctx, l.projectID, l.stream, decisions); err != nil {
		slog.WarnContext(ctx, "review decisions not recorded in ledger",
			"project", l.projectID, "stream", l.stream, "count", len(decisions), "error", err)
		return
	}
	corpus := make([]venue.UnitDecision, 0, len(decisions))
	for _, d := range decisions {
		if d.ReviewState != "" {
			corpus = append(corpus, d) // a plain un-review is no corpus verdict
		}
	}
	if len(corpus) == 0 || !l.resolveCorpus(ctx) {
		return
	}
	jobs.PromoteDecisionsToMemory(ctx, l.srv.ContentStore, l.memory, l.projectID, l.stream, l.sourceLang, corpus)
}

// resolveCorpus finds the project's source language and its workspace content
// memory, once, and reports whether promotion can proceed.
func (l *reviewLedger) resolveCorpus(ctx context.Context) bool {
	if l.corpusResolved {
		return l.memory != nil
	}
	l.corpusResolved = true

	proj, err := l.srv.ContentStore.GetProject(ctx, l.projectID)
	if err != nil || proj == nil || proj.DefaultSourceLanguage == "" {
		slog.WarnContext(ctx, "memory promotion skipped: project unresolved",
			"project", l.projectID, "error", err)
		return false
	}
	slug := ""
	if l.srv.AuthStore != nil && proj.WorkspaceID != "" {
		if ws, werr := l.srv.AuthStore.GetWorkspace(ctx, proj.WorkspaceID); werr == nil && ws != nil {
			slug = ws.Slug
		}
	}
	if slug == "" {
		slog.WarnContext(ctx, "memory promotion skipped: no workspace slug", "project", l.projectID)
		return false
	}
	tm, terr := l.srv.wsStores.getMemory(slug)
	if terr != nil {
		slog.WarnContext(ctx, "memory promotion skipped: workspace memory unavailable",
			"workspace", slug, "error", terr)
		return false
	}
	l.sourceLang = proj.DefaultSourceLanguage
	l.memory = tm
	return true
}

// unitDecisionFor renders one block's rung change as a ledger row.
func unitDecisionFor(sb *venue.StoredBlock, locale string, status model.TargetStatus, approved bool, decider string) venue.UnitDecision {
	reviewState := ""
	switch {
	case approved && status == model.TargetStatusSignedOff:
		reviewState = "signed-off"
	case approved:
		reviewState = "approved"
	case status == model.TargetStatusDraft:
		reviewState = "rejected"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return venue.UnitDecision{
		ItemName:    sb.ItemName,
		Unit:        sb.SourceID,
		Variant:     locale,
		Status:      string(status),
		TargetHash:  state.TargetHash(sb.Block.TargetText(model.LocaleID(locale))),
		ContentHash: state.SourceHash(sb.Block.SourceText()),
		ReviewState: reviewState,
		DecidedBy:   decider,
		DecidedAt:   now,
		Updated:     now,
	}
}

// reviewGateFor names the permission a review call needs. Approving is the
// review permission; withdrawing an approval or rejecting is the translate
// permission that edits the target, so a translator can take back their own
// work without holding review.
func reviewGateFor(approving bool) platauth.Permission {
	if approving {
		return platauth.PermReview
	}
	return platauth.PermTranslate
}

// reviewDecisionName labels a rung change for the audit log: what the decider
// did, in the vocabulary the review surfaces use.
func reviewDecisionName(approved bool, to model.TargetStatus) string {
	switch {
	case approved:
		return "approved"
	case to == model.TargetStatusDraft:
		return "rejected"
	default:
		return "unreviewed"
	}
}

// emitReviewDecisionAudit records one review decision in the audit log: who
// decided, on which block and locale, and the rungs the target moved between.
// The decision ledger carries the same verdict for the content pipeline; this
// is the security trail, and it is written for every rung change a person makes.
func (s *Server) emitReviewDecisionAudit(c echo.Context, projectID, stream, blockID, locale string, from, to model.TargetStatus, approved bool, reason string) {
	data := map[string]string{
		"locale":   locale,
		"stream":   stream,
		"decision": reviewDecisionName(approved, to),
	}
	if reason != "" {
		data["reason"] = reason
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventReviewDecided,
		ProjectID:    projectID,
		ResourceType: "block",
		ResourceID:   blockID,
		Data:         data,
		Before:       map[string]string{"status": string(from)},
		After:        map[string]string{"status": string(to)},
	})
}
