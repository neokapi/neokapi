package server

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// This file closes the review loop's fan-out to translations (RV-E). The gap it
// fills: when a term/concept is marked forbidden or a brand rule is promoted
// AFTER translations already exist, the governance events fire but nothing
// re-checks the existing targets — so a reviewed translation that now violates
// the change silently stays "approved" and its locale never re-enters the review
// queue. The loop does not learn from a mid-review terminology decision.
//
// The subscriber below reacts to the resulting concept/brand-rule events, finds
// the approved targets that now contain a forbidden term, DEMOTES them to draft,
// and RE-QUEUES their locales for review. It publishes nothing back onto the bus
// and deliberately does not start a convergence run — RV-E only pulls the
// affected work back into the review queue; the human (or bulk approve-passing)
// re-drives completion → delivery through the existing review-completion path.

// reviewRecheckGroup is the durable consumer-group name for the RV-E re-check.
// Stable by design (see reviewCompletionGroup / convergeOnPushGroup): the group's
// acknowledged position on the Redis bus is keyed by this name, so a governance
// event dropped during a deploy rollover is redelivered rather than lost — a
// forbidden term marked during a rollover must still pull its translations back.
const reviewRecheckGroup = "review-recheck"

// subscribeReviewRecheck wires RV-E: a durable consumer group that reacts to
//
//   - concept.created / concept.updated / concept.term.status_changed — a term
//     became (or was created) forbidden/preferred through the governed change-set
//     path (the subscriber is strictly downstream of governance: it reacts to the
//     RESULTING event, it does not bypass the change-set gate); and
//   - brand.voice.rule_promoted / rule_auto_promoted — the correction-learning
//     loop promoted a forbidden vocabulary rule.
//
// For each, it re-checks the workspace's existing reviewed/signed-off targets and
// demotes the ones that now violate the change, re-queuing their locales.
//
// Idempotency / anti-loop (the load-bearing argument):
//
//   - The re-check only demotes targets at reviewed/signed-off that a matcher says
//     now contain a forbidden term. A target already below reviewed (draft /
//     translated) is left alone — it is already pending review — so a
//     demoted-then-reviewed target that still conforms is never re-demoted, and
//     re-checking an already-conforming target changes nothing. Replaying the same
//     event therefore demotes nothing new and creates no duplicate task
//     (createReviewTasksForLocales dedups per open locale).
//   - RV-E publishes NOTHING onto the event bus (it stores demoted blocks and
//     creates review tasks, neither of which re-enters this group) and never
//     re-publishes a concept/brand event, so it cannot re-trigger itself. It also
//     never publishes review.completed nor starts a convergence run — that is the
//     review-completion path's job (subscribeReviewCompletion). RV-E's whole
//     effect is to raise the block-derived pending-review count; a person or
//     bulk approve-passing then re-drives the loop to delivery.
func (s *Server) subscribeReviewRecheck() {
	if s.EventBus == nil {
		return
	}
	s.EventBus.SubscribeGroup(reviewRecheckGroup, s.handleReviewRecheckEvent)
}

// handleReviewRecheckEvent type-dispatches a governance event to the right
// re-check. Group handlers see every event; only the concept and brand-rule
// changes below do anything. The work runs synchronously so the consumer group
// acknowledges the event only after the re-check completes (durability): a crash
// mid-re-check redelivers rather than stranding the un-re-checked translations.
func (s *Server) handleReviewRecheckEvent(ev platev.Event) {
	ctx := context.Background()
	switch ev.Type {
	case knowledge.EventConceptCreated, knowledge.EventConceptUpdated, knowledge.EventConceptTermStatusChanged:
		// Concept events carry the workspace id on WorkspaceID and the concept on
		// Data["concept_id"] (publishKnowledgeEvents). A governed term-status flip
		// arrives as concept.term.status_changed; a governed forbidden-concept
		// creation as concept.created; other governed edits as concept.updated —
		// all three are handled the same way (re-check the concept's forbidden terms).
		conceptID := ev.Data["concept_id"]
		if ev.WorkspaceID == "" || conceptID == "" {
			return
		}
		s.recheckConceptViolations(ctx, ev.WorkspaceID, conceptID, ev.Actor)
	case EventBrandVoiceRulePromoted, EventBrandVoiceRuleAutoPromoted:
		// Brand rule events stash the workspace id on ProjectID and the promoted
		// term on Data["term"] (publishBrandRuleEvent); WorkspaceID wins if a future
		// publisher sets it. A promotion that also minted a knowledge-graph concept
		// additionally fires concept.created (handled above); idempotency makes the
		// overlap a no-op on the second pass.
		wsID := ev.WorkspaceID
		if wsID == "" {
			wsID = ev.ProjectID
		}
		term := ev.Data["term"]
		if wsID == "" || strings.TrimSpace(term) == "" {
			return
		}
		s.recheckRuleViolations(ctx, wsID, ev.Data["profile_id"], term, ev.Actor)
	}
}

// recheckConceptViolations re-checks every existing reviewed/signed-off target in
// the workspace against the changed concept's forbidden (and competitor) terms,
// demoting and re-queuing the ones that now contain one. It reuses the concept
// where-used machinery's approach (a single-concept in-memory termbase +
// LookupAll, exactly as knowledge.Engine.ConceptUsage) as the violation oracle.
func (s *Server) recheckConceptViolations(ctx context.Context, wsID, conceptID, actor string) {
	tb, err := s.workspaceTermbaseByID(ctx, wsID)
	if err != nil || tb == nil {
		slog.WarnContext(ctx, "review recheck: workspace termbase unavailable", "workspace", wsID, "error", err)
		return
	}
	concept, ok, err := tb.GetConcept(ctx, conceptID)
	if err != nil || !ok {
		return
	}
	// A concept with no forbidden/competitor term cannot be violated by a target —
	// nothing to re-check (e.g. a plain definition edit that fired concept.updated).
	if !conceptHasForbiddenTerm(concept) {
		return
	}
	// Single-concept termbase so LookupAll matches only this concept's terms — the
	// same isolation ConceptUsage uses behind GET /concepts/:cid/blast-radius.
	cTB := termbase.NewInMemoryTermBase()
	if err := cTB.AddConcept(ctx, concept); err != nil {
		return
	}
	violates := func(text string, loc model.LocaleID) bool {
		return targetHasForbiddenConceptTerm(ctx, cTB, text, loc)
	}
	s.recheckWorkspaceTargets(ctx, wsID, "concept:"+conceptID, violates, actor)
}

// recheckRuleViolations re-checks existing reviewed/signed-off targets against a
// newly promoted forbidden brand rule, scoped to the promoted term so an existing
// target that only tripped an OLDER rule is not swept up. It reuses the canonical
// brand-vocabulary matcher (core/brand.MatchVocabulary — the single source the
// brand-vocab-check tool and the blast radius both call) as the oracle.
func (s *Server) recheckRuleViolations(ctx context.Context, wsID, profileID, term, actor string) {
	if s.BrandStore == nil || profileID == "" {
		return
	}
	profile, err := s.BrandStore.GetProfile(ctx, profileID)
	if err != nil || profile == nil {
		return
	}
	lowerTerm := strings.ToLower(strings.TrimSpace(term))
	if lowerTerm == "" {
		return
	}
	violates := func(text string, _ model.LocaleID) bool {
		for _, hit := range corebrand.MatchVocabulary(profile, text) {
			if strings.ToLower(hit.Term) == lowerTerm {
				return true
			}
		}
		return false
	}
	s.recheckWorkspaceTargets(ctx, wsID, "rule:"+profileID+":"+lowerTerm, violates, actor)
}

// recheckWorkspaceTargets walks the governed projects of one workspace and
// re-checks their targets with `violates`, demoting + re-queuing the failures.
// The scan is bounded to each project's "main" stream (the same bound the
// dashboard ship-state pass and ConceptUsage's default walk apply); non-governed
// projects are skipped because they have no review queue to pull work back into.
func (s *Server) recheckWorkspaceTargets(ctx context.Context, wsID, reason string, violates func(text string, loc model.LocaleID) bool, actor string) {
	if s.ContentStore == nil {
		return
	}
	projects, err := s.ContentStore.ListProjects(ctx)
	if err != nil {
		slog.WarnContext(ctx, "review recheck: list projects failed", "reason", reason, "error", err)
		return
	}
	for _, proj := range projects {
		if proj == nil || proj.WorkspaceID != wsID {
			continue
		}
		if !workflowReviewEnabled(proj) {
			continue
		}
		s.recheckProjectTargets(ctx, proj, reason, violates, actor)
	}
}

// recheckProjectTargets re-checks one project's reviewed/signed-off targets,
// demotes the failures to draft, persists them, and re-queues the affected
// locales for review. A conforming target is never touched (no needless churn),
// and a project with no failure produces no store write, no task, and no event.
func (s *Server) recheckProjectTargets(ctx context.Context, proj *platstore.Project, reason string, violates func(text string, loc model.LocaleID) bool, actor string) {
	blocks, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: proj.ID, Stream: "main"})
	if err != nil {
		slog.WarnContext(ctx, "review recheck: load blocks failed", "project", proj.ID, "error", err)
		return
	}

	changed := map[string]*platstore.StoredBlock{} // deduped by block id — one store write per block
	affected := map[model.LocaleID]bool{}
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil || !sb.Block.Translatable {
			continue
		}
		for _, loc := range proj.TargetLanguages {
			t := sb.Block.Target(loc)
			if t == nil {
				continue
			}
			// RV-E pulls back only APPROVED work. A target below reviewed is already
			// pending review, so re-checking it would change nothing — leaving it
			// alone is what makes replays idempotent and avoids demoting a target
			// that was already re-queued.
			if t.Status.Rank() < model.TargetStatusReviewed.Rank() {
				continue
			}
			text := sb.Block.TargetText(loc)
			if strings.TrimSpace(text) == "" {
				continue
			}
			if !violates(text, loc) {
				continue // still conforms — untouched
			}
			// Demote to draft, mirroring HandleReviewBlock's rejection mapping: the
			// translation is now wrong (it carries a forbidden term), so it re-enters
			// the work queue, not merely re-review.
			t.Status = model.TargetStatusDraft
			changed[sb.Block.ID] = sb
			affected[loc] = true
		}
	}
	if len(changed) == 0 {
		return
	}

	toStore := make([]*model.Block, 0, len(changed))
	for _, sb := range changed {
		toStore = append(toStore, sb.Block)
	}
	if err := s.ContentStore.StoreBlocks(ctx, proj.ID, "main", toStore); err != nil {
		slog.WarnContext(ctx, "review recheck: store demoted blocks failed", "project", proj.ID, "error", err)
		return
	}
	s.invalidateDashboardCache(proj.WorkspaceID, proj.ID)
	for _, sb := range changed {
		s.publishEditorBlockChange(proj.ID, sb.Block.ID, sb.ItemName, "main", "updated", actor, "")
	}

	locales := sortedLocales(affected)
	slog.InfoContext(ctx, "review recheck: demoted approved targets that now violate a governed change; re-queued for review",
		"project", proj.ID, "reason", reason, "blocks", len(toStore), "locales", locales)

	// Re-queue the affected locales into the review queue, reusing the review-core
	// task creation (owner-fallback assignment + per-locale dedup). The block-derived
	// pending-review count already reflects the demotions above; the tasks are the
	// human-routing "for you" projection on top.
	ev := platev.Event{ProjectID: proj.ID, Data: map[string]string{"mode": "review"}}
	s.createReviewTasksForLocales(ctx, proj, locales, event.AutomationAction{Config: map[string]string{"mode": "review"}}, ev, "")
}

// workspaceTermbaseByID resolves the workspace termbase from a workspace ID. The
// event bus carries the workspace id, but the per-workspace TM/termbase stores are
// keyed by slug, so this bridges id → slug via the auth store.
func (s *Server) workspaceTermbaseByID(ctx context.Context, wsID string) (termbase.TBStore, error) {
	if s.AuthStore == nil || s.wsStores == nil {
		return nil, errKnowledgeUnavailable
	}
	ws, err := s.AuthStore.GetWorkspace(ctx, wsID)
	if err != nil {
		return nil, err
	}
	slug := ws.Slug
	if slug == "" {
		slug = wsID
	}
	return s.wsStores.getTB(slug)
}

// conceptHasForbiddenTerm reports whether a concept carries any forbidden or
// competitor term — the only kind a target can violate by containing it.
func conceptHasForbiddenTerm(c termbase.Concept) bool {
	for _, term := range c.Terms {
		if term.Status == model.TermForbidden || term.CompetitorTerm {
			return true
		}
	}
	return false
}

// targetHasForbiddenConceptTerm reports whether text (a target translation in
// locale loc) contains a forbidden or competitor term of the single concept in
// cTB. It uses the same termbase LookupAll the concept blast radius uses, keyed on
// the target locale so a per-locale terminology decision is matched in the
// language it governs.
func targetHasForbiddenConceptTerm(ctx context.Context, cTB *termbase.InMemoryTermBase, text string, loc model.LocaleID) bool {
	matches, err := cTB.LookupAll(ctx, text, termbase.LookupOptions{SourceLocale: loc})
	if err != nil {
		return false
	}
	for _, m := range matches {
		if m.Term.Status == model.TermForbidden || m.Term.CompetitorTerm {
			return true
		}
	}
	return false
}

// sortedLocales returns the keys of the affected-locale set in a stable order.
func sortedLocales(set map[model.LocaleID]bool) []model.LocaleID {
	out := make([]model.LocaleID, 0, len(set))
	for loc := range set {
		out = append(out, loc)
	}
	slices.SortFunc(out, func(a, b model.LocaleID) int { return strings.Compare(string(a), string(b)) })
	return out
}
