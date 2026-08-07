package server

import (
	"context"
	"log/slog"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
)

// This file connects the two loops the platform previously ran disconnected —
// governed review and delivery (DECISIONS R11). Approving the last pending
// review now continues the loop straight through to delivery with no extra user
// action:
//
//	approve last block ──▶ review.completed ──▶ completing convergence run
//	                                              ──▶ run.completed(converged)
//	                                              ──▶ forge delivery (approved content)
//
// The helpers here are shared by the per-block review endpoint
// (HandleReviewBlock) and the bulk approve-passing endpoint
// (HandleApprovePassing).

// targetApproved reports whether a block's target for a locale carries a review
// decision — reviewed or signed-off on the model.TargetStatus ladder. Governed
// delivery ships only these.
func targetApproved(b *model.Block, loc model.LocaleID) bool {
	t := b.Target(loc)
	return t != nil && t.Status.Rank() >= model.TargetStatusReviewed.Rank()
}

// targetPendingReview reports whether a block's target for a locale is awaiting
// human review: it has committed, non-empty content that has not yet reached the
// reviewed rung (i.e. draft or translated). A reviewed/signed-off target — or a
// block with no non-empty target for the locale — is not pending. This is the
// per-block "still needs a reviewer" predicate the review-loop continuation and
// the bulk approve-passing endpoint both key on.
func targetPendingReview(b *model.Block, loc model.LocaleID) bool {
	if b == nil || !b.Translatable {
		return false
	}
	t := b.Target(loc)
	// Presence is the shared run-aware predicate, matching
	// convergence.TargetState: a target whose only run is an inline code is
	// produced content. Read through TargetText() it flattened to "" and the unit
	// never entered the review queue, so no reviewer could ever approve it.
	if t == nil || !model.RunsHaveContent(b.TargetRuns(loc)) {
		return false
	}
	return t.Status.Rank() < model.TargetStatusReviewed.Rank()
}

// localeHasPendingReview reports whether any translatable block still has a
// target awaiting review for the locale. When it goes false for a locale, that
// locale's open review task(s) can close.
func localeHasPendingReview(blocks []*platstore.StoredBlock, loc model.LocaleID) bool {
	for _, sb := range blocks {
		if sb != nil && targetPendingReview(sb.Block, loc) {
			return true
		}
	}
	return false
}

// projectHasPendingReview reports whether the project still has ANY translated-
// but-unreviewed target block for a configured target locale — the block-derived,
// assignment-independent definition of "pending review" that the review-session
// UI and the ship gate already reason about. It is the completion condition for
// the review loop: zero pending-review blocks means the loop can continue to
// delivery. Deriving it from block status (not the open-review-task count) keeps
// completion correct even when task creation is imperfect (e.g. a member-less
// project that only got unassigned/owner-routed tasks), and makes the UI queue
// and the completion trigger agree exactly.
func projectHasPendingReview(blocks []*platstore.StoredBlock, targets []model.LocaleID) bool {
	for _, sb := range blocks {
		if sb == nil {
			continue
		}
		for _, loc := range targets {
			if targetPendingReview(sb.Block, loc) {
				return true
			}
		}
	}
	return false
}

// advanceReviewLoop is the governed-review continuation shared by the per-block
// review endpoint and the bulk approve-passing endpoint. It is called only after
// a real transition of at least one target to reviewed (touched carries the
// locales approved in this call). It (1) closes the review TASK(s) for any
// touched locale that now has no block awaiting review — the human-routing layer
// — and (2) when the project as a whole has zero blocks pending review for any
// configured locale, publishes review.completed, which the durable
// review-completion subscription turns into a completing convergence run →
// delivery. It reports whether review.completed was published.
//
// The completion condition is BLOCK-derived, not task-count-derived
// (projectHasPendingReview): the loop continues exactly when the honest coverage
// truth says nothing is left to review, independent of whether every review task
// was created/assigned. Tasks are the "for you" projection on top. It is a no-op
// (returns false) for a non-governed project — those never enter the review loop,
// so their behavior is unchanged — and whenever any pending-review block remains,
// so an approval that is not the project's last never publishes.
func (s *Server) advanceReviewLoop(ctx context.Context, proj *platstore.Project, stream string, touched []model.LocaleID, actor string) bool {
	if proj == nil || s.ContentStore == nil || s.EventBus == nil {
		return false
	}
	if !workflowReviewEnabled(proj) {
		return false
	}
	if len(touched) == 0 {
		return false
	}

	blocks, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: proj.ID, Stream: stream})
	if err != nil {
		slog.WarnContext(ctx, "review loop: load blocks failed", "project", proj.ID, "error", err)
		return false
	}

	// Human-routing layer (best-effort, not the completion gate): close the open
	// review task(s) for any touched locale whose review work is now done, so a
	// reviewer's "for you" queue clears as they finish rather than needing a
	// separate "complete task" click.
	if s.TaskStore != nil {
		seen := map[model.LocaleID]bool{}
		for _, loc := range touched {
			if seen[loc] {
				continue
			}
			seen[loc] = true
			if !localeHasPendingReview(blocks, loc) {
				s.completeLocaleReviewTasks(ctx, proj, string(loc), actor)
			}
		}
	}

	// Completion gate (block-derived): continue the loop only when NO configured
	// target locale has a block still awaiting review.
	if projectHasPendingReview(blocks, proj.TargetLanguages) {
		return false
	}

	s.EventBus.Publish(platev.Event{
		ID:          id.New(),
		Type:        platev.EventReviewCompleted,
		Source:      "review",
		ProjectID:   proj.ID,
		WorkspaceID: proj.WorkspaceID,
		Actor:       actor,
		Timestamp:   time.Now().UTC(),
		Data:        map[string]string{"stream": stream},
	})
	slog.InfoContext(ctx, "review loop: project review queue emptied; continuing to delivery", "project", proj.ID)
	return true
}

// completeLocaleReviewTasks closes every open/in-progress review task for a
// project scoped to the given locale (the linkage createReviewTasks stamps in
// Data["locale"]), returning how many it completed. Marking review status is not
// itself a task action, so the loop closes the locale's task when the locale's
// review work is done rather than requiring a separate "complete task" click.
func (s *Server) completeLocaleReviewTasks(ctx context.Context, proj *platstore.Project, locale, actor string) int {
	res, err := s.TaskStore.List(ctx, bstore.TaskQuery{
		WorkspaceID: proj.WorkspaceID,
		ProjectID:   proj.ID,
		Type:        string(bstore.TaskReview),
		Statuses:    []string{string(bstore.TaskStatusOpen), string(bstore.TaskStatusInProgress)},
		Limit:       500,
	})
	if err != nil {
		slog.WarnContext(ctx, "review loop: list open review tasks failed", "project", proj.ID, "error", err)
		return 0
	}
	completedBy := actor
	if completedBy == "" {
		completedBy = "system"
	}
	n := 0
	for _, t := range res.Tasks {
		if t.Data["locale"] != locale {
			continue
		}
		if err := s.TaskStore.Complete(ctx, t.ID, completedBy); err != nil {
			slog.WarnContext(ctx, "review loop: complete review task failed", "task", t.ID, "error", err)
			continue
		}
		n++
	}
	return n
}

// reviewCompletionGroup is the durable consumer-group name for the review →
// delivery continuation. Stable by design (see convergeOnPushGroup): the group's
// resume position on the Redis bus is keyed by this name.
const reviewCompletionGroup = "review-completion"

// subscribeReviewCompletion wires the second half of the governed review loop:
// when a project's open review queue empties (the last pending review approved),
// review.completed fires and this starts a *completing* convergence run so the
// now-approved content flows straight to delivery — no extra user action.
//
// Durability: like convergence-on-push and forge delivery (#1376), this is a
// state-advancing trigger — a review.completed dropped during a deploy rollover
// would strand approved content that never ships — so it joins a durable
// consumer group (SubscribeGroup → XREADGROUP): the group's acknowledged
// position survives restarts and exactly one instance in the group handles each
// event.
//
// Idempotency / anti-loop (the load-bearing argument):
//
//   - The completing run derives coverage over content that is now fully
//     approved. Every block already carries a reviewed/signed-off target, so the
//     derive finds 0 pending locales, the loop runs 0 production passes
//     (LoopResult.Passes == 0), converges immediately, and emits
//     run.completed(converged). RV-A's approval-gated materializeDelivery then
//     ships exactly the approved content.
//   - Because the run ran 0 passes, driveWith's completion fan-out
//     (createCompletionReviewTasks, gated on run.Passes > 0) creates NO new
//     review tasks. The open review queue stays empty, so nothing re-publishes
//     review.completed. review → run → review therefore cannot loop.
//   - A replayed review.completed (startup drain / redelivery) at worst starts
//     one extra already-converged run: 0 pending → 0 passes → converges → no
//     tasks. StartRun's active-run guard collapses a replay racing a live run.
func (s *Server) subscribeReviewCompletion() {
	if s.EventBus == nil || s.convergence == nil {
		return
	}
	//
	// The handler always acknowledges: the run it starts is already
	// replay-collapsing, and a redelivery would at worst start another
	// already-converged one.
	s.EventBus.SubscribeGroup(reviewCompletionGroup, func(ev platev.Event) error {
		if ev.Type != platev.EventReviewCompleted {
			return nil // group handlers see every event; only review completions matter here
		}
		s.startReviewCompletionRun(ev)
		return nil
	})
}

// startReviewCompletionRun starts the completing convergence run for a project
// whose review queue just emptied. Unlike the on-push trigger it does NOT gate
// on the project's converge policy: review completion is itself the signal to
// finish and deliver, so even a manual-policy governed project ships its
// approved content. The run outlives the request that approved the last block,
// so a fresh background context is correct here (matching startOnPushRun).
func (s *Server) startReviewCompletionRun(ev platev.Event) {
	if ev.ProjectID == "" || s.convergence == nil {
		return
	}
	slog.Info("review loop: starting completing convergence run", "project", ev.ProjectID)
	ctx := context.Background()
	if _, _, err := s.convergence.StartRun(ctx, ev.ProjectID, ev.Data["stream"], "review", nil); err != nil {
		slog.Warn("review loop: completing run start failed", "project", ev.ProjectID, "error", err)
	}
}
