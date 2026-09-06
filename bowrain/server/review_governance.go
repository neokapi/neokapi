package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/review"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
)

// reviewSoD is the review gate for one request: the permission for the
// language and the workspace separation-of-duties policy, in one answer.
//
// The answer itself lives in bowrain/review, which the sync worker asks the
// same question of. This is the request's side of it: where the actor, the
// workspace and the language permission come from an echo context, where a
// violation is recorded in the audit trail, and where a refusal becomes the
// reviewFault the routes already render.
type reviewSoD struct {
	gate *review.Gate
}

// quiet stops vet from filing a record per target. An approve-passing pass over
// a corpus the caller wrote would otherwise file one per block, on a bus that
// drops what it cannot keep up with; the handler files one with the count.
func (g *reviewSoD) quiet() *reviewSoD {
	if g != nil {
		g.gate.Quiet()
	}
	return g
}

// violations is how many pairs the policy caught, for the single record a bulk
// pass files.
func (g *reviewSoD) violations() int {
	if g == nil {
		return 0
	}
	return g.gate.Violations()
}

// newReviewSoD opens the gate for a pass: the workspace policy once, and the
// authorship of every (block, locale) pair the pass may touch in one query, so
// a selection of a thousand blocks costs one round trip and not a thousand.
//
// It reports an error only when a store that keeps target authorship failed to
// answer: a discarded error would disable the four-eyes check rather than
// tighten it, so the caller refuses the request instead.
func (s *Server) newReviewSoD(ctx context.Context, c echo.Context, projectID, stream string, blockIDs, locales []string) (*reviewSoD, error) {
	actor, _ := c.Get("user_id").(string)
	wsID, _ := c.Get("workspace_id").(string)

	cfg := review.Config{
		Actor:       actor,
		WorkspaceID: wsID,
		ProjectID:   projectID,
		Stream:      stream,
		BlockIDs:    blockIDs,
		Locales:     locales,
		Permits: func(locale string) bool {
			return allowsLanguage(c, platauth.PermReview, locale)
		},
		Record: func(resource string, mode platauth.SoDMode, targets int) {
			s.recordSoDViolation(c, actor, resource, mode, targets)
		},
	}
	if s.AuthStore != nil {
		cfg.Policy = s.AuthStore
	}
	if ts, ok := s.ContentStore.(platstore.TargetAuthorStore); ok {
		cfg.Authors = ts
	}
	g, err := review.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &reviewSoD{gate: g}, nil
}

// vet asks the gate about one block's target and renders a refusal the way the
// routes answer it: the single route as a 403, a batch route as a record
// against the block, so one protected block never answers for a whole
// selection.
func (g *reviewSoD) vet(blockID, locale string) error {
	if g == nil {
		return reviewFault{http.StatusForbidden, "review permission could not be resolved"}
	}
	err := g.gate.Allow(blockID, locale)
	if refusal, ok := errors.AsType[review.Refusal](err); ok {
		if refusal.Reason == venue.RefusedSeparationOfDuties {
			return reviewFault{http.StatusForbidden, sodRefusal}
		}
		return reviewFault{http.StatusForbidden, "no review permission for " + refusal.Locale}
	}
	return err
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

	// Where this pass decides, resolved on first need and remembered,
	// including a failure: a pass whose project or workspace cannot be read
	// must not re-ask per batch.
	scopeResolved bool
	project       *platstore.Project
	sourceLang    model.LocaleID
	workspaceSlug string

	// The corpus, opened on first need and remembered the same way.
	corpusResolved bool
	memory         memory.Store

	// The governing context per (item, locale), resolved on first need. A bulk
	// approve walks thousands of blocks across a handful of items, and the
	// context is a property of where the unit sits, not of the block.
	governing map[string]string
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
	return &reviewLedger{
		srv: s, ds: ds, projectID: projectID, stream: stream, decider: decider,
		governing: map[string]string{},
	}
}

// governingFingerprint is the context in force where a unit sits, for the
// locale being decided: the voice guidance and term rules the platform would
// translate that unit under, folded by the one function every producer stamps
// with.
//
// It is resolved through jobs.TranslateBinding, the binding the editor
// translate and the worker's queued jobs already read their governing context
// from, so an approval records the same context a translation of the same unit
// would be produced under. Best-effort throughout: a workspace with no voice
// and no terminology yields the empty fingerprint, which reads as an ad-hoc
// decision rather than failing the review the person just made.
//
// Cached per (item, locale) for the life of the ledger.
func (l *reviewLedger) governingFingerprint(ctx context.Context, itemName, locale string) string {
	if l == nil || itemName == "" || locale == "" || !l.resolveScope(ctx) {
		return ""
	}
	key := itemName + "\x00" + locale
	if fingerprint, ok := l.governing[key]; ok {
		return fingerprint
	}
	voiceCtx := l.srv.editorVoiceContext()
	b := jobs.TranslateBinding{
		Store:            l.srv.ContentStore,
		Voice:            voiceCtx.Voice,
		WorkspaceDefault: voiceCtx.WorkspaceDefault,
		Terms:            editorTerms(ctx, voiceCtx, l.workspaceSlug),
		Project:          l.project,
		WorkspaceID:      l.project.WorkspaceID,
		ProjectID:        l.projectID,
		Stream:           l.stream,
		ItemName:         itemName,
		TargetLocale:     model.LocaleID(locale),
	}
	fingerprint := b.GoverningFingerprint(ctx)
	l.governing[key] = fingerprint
	return fingerprint
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

// resolveScope finds the project this pass decides in and the slug of the
// workspace holding its stores, once, and reports whether both answered.
//
// It is the half the governing context needs. Promotion needs the content
// memory on top, which resolveCorpus opens.
func (l *reviewLedger) resolveScope(ctx context.Context) bool {
	if l.scopeResolved {
		return l.project != nil
	}
	l.scopeResolved = true

	proj, err := l.srv.ContentStore.GetProject(ctx, l.projectID)
	if err != nil || proj == nil || proj.DefaultSourceLanguage == "" {
		slog.WarnContext(ctx, "review ledger: project unresolved",
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
		slog.WarnContext(ctx, "review ledger: no workspace slug", "project", l.projectID)
		return false
	}
	l.project = proj
	l.sourceLang = proj.DefaultSourceLanguage
	l.workspaceSlug = slug
	return true
}

// resolveCorpus opens the workspace content memory, once, and reports whether
// promotion can proceed.
func (l *reviewLedger) resolveCorpus(ctx context.Context) bool {
	if l.corpusResolved {
		return l.memory != nil
	}
	l.corpusResolved = true

	if !l.resolveScope(ctx) {
		return false
	}
	tm, terr := l.srv.wsStores.getMemory(l.workspaceSlug)
	if terr != nil {
		slog.WarnContext(ctx, "memory promotion skipped: workspace memory unavailable",
			"workspace", l.workspaceSlug, "error", terr)
		return false
	}
	l.memory = tm
	return true
}

// unitDecisionFor renders one block's rung change as a ledger row.
//
// governing is the fingerprint of the context the decision was made under (see
// reviewLedger.governingFingerprint): the claim a verdict supports is that
// this answer stands under this context, so the context travels with it, on
// the wire and into the content memory the approval promotes to.
func unitDecisionFor(sb *venue.StoredBlock, locale string, status model.TargetStatus, approved bool, decider, governing string) venue.UnitDecision {
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
		ItemName:             sb.ItemName,
		Unit:                 sb.SourceID,
		Variant:              locale,
		Status:               string(status),
		TargetHash:           state.TargetHash(sb.Block.TargetText(model.LocaleID(locale))),
		ContentHash:          state.SourceHash(sb.Block.SourceText()),
		ReviewState:          reviewState,
		GoverningFingerprint: governing,
		DecidedBy:            decider,
		DecidedAt:            now,
		Updated:              now,
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

// emitReviewDecisionAudit records one review decision in the audit log: who
// decided, on which block and locale, and the rungs the target moved between.
// The decision ledger carries the same verdict for the content pipeline; this
// is the security trail, and it is written for every rung change a person makes.
func (s *Server) emitReviewDecisionAudit(c echo.Context, projectID, stream, blockID, locale string, from, to model.TargetStatus, approved bool, reason string) {
	data := map[string]string{
		"locale":   locale,
		"stream":   stream,
		"decision": review.DecisionName(approved, to),
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

// mode is the workspace policy this pass was judged under, for the single
// record a bulk pass files.
func (g *reviewSoD) mode() platauth.SoDMode {
	if g == nil {
		return platauth.SoDOff
	}
	return g.gate.Mode()
}
