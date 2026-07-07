package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
)

// This file is the server venue of the convergence loop (strategy
// 2026-07-kapi-up doc 03): it drives the SAME venue-neutral
// core/convergence.Loop the CLI drives, wired with server-backed IO —
// coverage derived from the block store, production via the translation job
// queue. Every emitted convergence.Event is persisted (SSE replay) and fanned
// out live to subscribers. The run entity + REST/SSE live alongside.

// convergenceHub fans out a run's convergence events to live SSE subscribers.
// Each frame carries its persisted sequence so a subscriber that already
// replayed the store can dedupe.
type convergenceHub struct {
	mu      sync.RWMutex
	clients map[string]map[chan convergenceFrame]struct{} // runID → subscribers
}

type convergenceFrame struct {
	seq  int
	data []byte
}

func newConvergenceHub() *convergenceHub {
	return &convergenceHub{clients: make(map[string]map[chan convergenceFrame]struct{})}
}

func (h *convergenceHub) subscribe(runID string) chan convergenceFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan convergenceFrame, 64)
	if h.clients[runID] == nil {
		h.clients[runID] = make(map[chan convergenceFrame]struct{})
	}
	h.clients[runID][ch] = struct{}{}
	return ch
}

func (h *convergenceHub) unsubscribe(runID string, ch chan convergenceFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.clients[runID]; ok {
		delete(m, ch)
		if len(m) == 0 {
			delete(h.clients, runID)
		}
	}
	close(ch)
}

func (h *convergenceHub) broadcast(runID string, frame convergenceFrame) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients[runID] {
		select {
		case ch <- frame:
		default: // drop for a slow subscriber; it recovers via store replay
		}
	}
}

// convergenceOrchestrator owns the in-flight server runs: it drives the loop,
// persists + broadcasts events, and tracks cancellation.
type convergenceOrchestrator struct {
	server *Server
	hub    *convergenceHub

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // runID → cancel
}

func newConvergenceOrchestrator(s *Server) *convergenceOrchestrator {
	return &convergenceOrchestrator{
		server:  s,
		hub:     newConvergenceHub(),
		cancels: map[string]context.CancelFunc{},
	}
}

// convergePollInterval is how often the orchestrator polls the job queue for a
// locale's translation-job completion.
const convergePollInterval = 750 * time.Millisecond

// subscribeConvergeOnPush wires the continuous-convergence policy: a completed
// push starts a convergence run for on-push projects (the default), replacing
// the retired auto-translate-on-push automation. Manual projects converge only
// on demand. Transport (push) stays pure; this is the project's own clock.
func (s *Server) subscribeConvergeOnPush() {
	if s.EventBus == nil || s.convergence == nil {
		return
	}
	// A completed push starts a run for on-push projects.
	s.EventBus.Subscribe(platev.EventPushCompleted, func(ev platev.Event) {
		s.startOnPushRun(ev, "push")
	})
	// Adding a target locale is also new work the on-push policy should absorb:
	// the old auto-translate-new-locale automation was removed, so without this
	// a new locale would translate nothing (F10). handlers_project publishes
	// EventProjectUpdated with new_locales set; a run re-derives coverage and
	// produces the new locale like any other pending one.
	s.EventBus.Subscribe(platev.EventProjectUpdated, func(ev platev.Event) {
		if ev.Data["new_locales"] == "" {
			return // only a locale addition warrants a run
		}
		s.startOnPushRun(ev, "push")
	})
}

// startOnPushRun starts a convergence run for the event's project when its
// policy is on-push. Shared by the push-completed and new-locale triggers.
func (s *Server) startOnPushRun(ev platev.Event, trigger string) {
	if ev.ProjectID == "" || s.ContentStore == nil || s.convergence == nil {
		return
	}
	// Event-bus callback: a run is the project's own clock and must outlive
	// the request that published the event, so no request context applies.
	ctx := context.Background()
	proj, err := s.ContentStore.GetProject(ctx, ev.ProjectID)
	if err != nil {
		return
	}
	if platstore.NormalizeConvergePolicy(proj.ConvergePolicy) != platstore.ConvergePolicyOnPush {
		return // manual: no run
	}
	if _, _, err := s.convergence.StartRun(ctx, ev.ProjectID, trigger, nil); err != nil {
		slog.Warn("convergence: on-push start failed", "project", ev.ProjectID, "error", err)
	}
}

// SweepInterruptedRuns reconciles zombie runs on startup: any run left in
// 'running' by a crash or restart has no in-process loop, so it is marked
// failed. Runs before the orchestrator accepts work, so the one-run guard is
// not blocked forever by a dead row (F3).
func (o *convergenceOrchestrator) SweepInterruptedRuns(ctx context.Context) {
	if o.server.ConvergenceRunStore == nil {
		return
	}
	n, err := o.server.ConvergenceRunStore.FailInterruptedRuns(ctx, "interrupted by server restart")
	if err != nil {
		slog.Warn("convergence: startup sweep failed", "error", err)
		return
	}
	if n > 0 {
		slog.Info("convergence: swept interrupted runs on startup", "count", n)
	}
}

// StartRun starts (or returns the already-running) convergence run for a
// project. The returned bool reports whether a NEW run was started (false when
// an existing running run was returned, so the handler can answer 200 vs 201).
func (o *convergenceOrchestrator) StartRun(ctx context.Context, projectID, trigger string, locales []string) (*bstore.ConvergenceRun, bool, error) {
	store := o.server.ConvergenceRunStore
	if store == nil {
		return nil, false, errors.New("convergence runs not configured")
	}
	// Fast path: an already-running run short-circuits before the insert.
	if active, err := store.ActiveRun(ctx, projectID); err != nil {
		return nil, false, err
	} else if active != nil {
		return active, false, nil
	}

	// The DB's partial unique index is the real guard against a concurrent
	// push-event + CLI start both passing the check above; the loser's insert
	// is rejected and it joins the winner's run (F8).
	run := &bstore.ConvergenceRun{ProjectID: projectID, Trigger: trigger, State: bstore.ConvergenceRunRunning}
	if err := store.CreateRunGuarded(ctx, run); err != nil {
		if errors.Is(err, bstore.ErrActiveRunExists) {
			if active, aerr := store.ActiveRun(ctx, projectID); aerr == nil && active != nil {
				return active, false, nil
			}
		}
		return nil, false, err
	}

	// The run outlives the request: drive it on a background context we can
	// cancel from the cancel endpoint.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	o.mu.Lock()
	o.cancels[run.ID] = cancel
	o.mu.Unlock()

	go o.drive(runCtx, run, locales)
	return run, true, nil
}

// Cancel stops a run. It signals the in-process loop when this replica owns it,
// and ALWAYS persists a terminal state on the DB row when the run is still
// running — so a run adopted from another replica or orphaned by a restart is
// reconciled rather than left 'running' (F3). The bool reports whether a live
// loop was signaled.
func (o *convergenceOrchestrator) Cancel(ctx context.Context, run *bstore.ConvergenceRun) (bool, error) {
	o.mu.Lock()
	cancel := o.cancels[run.ID]
	o.mu.Unlock()
	if cancel != nil {
		cancel() // the driving goroutine records the canceled terminal state
		return true, nil
	}
	// No local loop: persist the terminal state directly if still running.
	if run.State == bstore.ConvergenceRunRunning && o.server.ConvergenceRunStore != nil {
		run.State = bstore.ConvergenceRunCanceled
		run.Error = "canceled"
		finished := time.Now().UTC()
		run.FinishedAt = &finished
		if err := o.server.ConvergenceRunStore.UpdateRun(ctx, run); err != nil {
			return false, err
		}
	}
	return false, nil
}

// drive runs one server-venue run: it builds the server-backed LoopFuncs
// (block-store coverage + job-queue production) and drives the loop.
func (o *convergenceOrchestrator) drive(ctx context.Context, run *bstore.ConvergenceRun, localeFilter []string) {
	o.driveWith(ctx, run, convergence.LoopFuncs{
		Derive:  o.deriveFunc(run.ProjectID, localeFilter),
		Produce: o.produceFunc(run.ProjectID),
	})
}

// driveWith runs the venue-neutral loop for one run to completion with the
// given IO, persisting and broadcasting every event, then records the terminal
// state. The LoopFuncs are a parameter so the orchestrator's run lifecycle
// (event persistence, SSE fan-out, state transitions, park→review) is testable
// against an in-memory model without the full block store + job queue.
func (o *convergenceOrchestrator) driveWith(ctx context.Context, run *bstore.ConvergenceRun, funcs convergence.LoopFuncs) {
	s := o.server
	store := s.ConvergenceRunStore
	defer func() {
		o.mu.Lock()
		delete(o.cancels, run.ID)
		o.mu.Unlock()
	}()

	standing := newStandingTracker()
	emit := convergence.NewEmitter(func(ev convergence.Event) {
		standing.observe(ev)
		payload, _ := json.Marshal(ev)
		seq, err := store.AppendEvent(context.WithoutCancel(ctx), run.ID, payload)
		if err != nil {
			// The persisted log is the source of truth for SSE replay. If the
			// event didn't persist, do NOT broadcast a frame{seq:0} — every
			// subscriber's dedupe would drop it and it would be absent from
			// replay too (F4). Subscribers heal from the store on the next
			// reconcile tick once persistence recovers.
			slog.Warn("convergence: persist event failed", "run", run.ID, "error", err)
			return
		}
		o.hub.broadcast(run.ID, convergenceFrame{seq: seq, data: payload})
		// Persist the run-row rollup at pass boundaries (cheap, keeps ListRuns
		// and the active-run guard current without a write per unit_progress).
		if ev.Type == convergence.EventPassDone || ev.Type == convergence.EventLocaleDone {
			run.Passes = standing.passes
			run.FailingChecks = standing.failingChecks
			run.Standing = standing.snapshot()
			_ = store.UpdateRun(context.WithoutCancel(ctx), run)
		}
	})

	res, loopErr := convergence.Loop(ctx, convergence.LoopOptions{
		UntilGate: true,
		MaxPasses: 5,
		Jobs:      4, // per-pass locale concurrency
	}, funcs, emit)

	// Terminal state.
	finished := time.Now().UTC()
	run.FinishedAt = &finished
	run.Passes = res.Passes
	run.FailingChecks = standing.failingChecks

	// The error field carries the cause the CLI prints for a failed/canceled
	// run (cancellation surfaces as a loop error too, so it is checked first and
	// gets a clean message rather than "context canceled").
	switch {
	case ctx.Err() != nil:
		run.State = bstore.ConvergenceRunCanceled
		run.Error = "canceled"
	case loopErr != nil:
		run.State = bstore.ConvergenceRunFailed
		run.Error = loopErr.Error()
		slog.Warn("convergence: run failed", "run", run.ID, "error", loopErr)
	case len(res.Final.Pending) > 0:
		run.State = bstore.ConvergenceRunParked
	default:
		run.State = bstore.ConvergenceRunConverged
	}

	// Emit the terminal done event (venue policy, after the loop) so every
	// subscriber sees the same closing frame the CLI venue emits. State carries
	// the REAL outcome — converged | parked | canceled | failed — so a caller
	// never mistakes a failed/canceled run for parked work and exits 0 (F2).
	// The store's state strings are exactly the core/convergence.Run* wire
	// values. Emit BEFORE the final row write so the done event settles the
	// per-locale standing (parked locales) into the snapshot the run records.
	emit.Emit(convergence.Event{Type: convergence.EventDone, State: run.State})

	run.Standing = standing.snapshot()
	_ = store.UpdateRun(context.WithoutCancel(ctx), run)

	// On completion (converged OR parked), a workflow-enabled project's content
	// enters the team's review queue — the single-player→multiplayer seam. This
	// replaces the retired create-review-tasks automation, for BOTH outcomes
	// (converged is the common case that previously created tasks after
	// translation completed), carrying the run/items/locales linkage the old
	// rule's push_id/items carried. Failed/canceled runs create nothing.
	if run.State == bstore.ConvergenceRunConverged || run.State == bstore.ConvergenceRunParked {
		o.createCompletionReviewTasks(context.WithoutCancel(ctx), run)
	}
}

// deriveFunc builds the server venue's Derive: coverage from the block store,
// gated on full block coverage AND the project's bound QA checks — the same
// two conditions the local venue applies (F6). A locale reaches its gate only
// when every translatable block has a target AND none fails the checks; a
// failing block demotes its unit below the gate, so a connected `kapi up`
// parks on failing terminology/length checks exactly like local `up` rather
// than silently claiming converged.
func (o *convergenceOrchestrator) deriveFunc(projectID string, localeFilter []string) func(context.Context) (convergence.PassState, error) {
	s := o.server
	filter := map[string]bool{}
	for _, l := range localeFilter {
		filter[l] = true
	}
	return func(ctx context.Context) (convergence.PassState, error) {
		proj, err := s.ContentStore.GetProject(ctx, projectID)
		if err != nil {
			return convergence.PassState{}, fmt.Errorf("load project: %w", err)
		}
		stats, err := editorGetDashboardStats(ctx, s.ContentStore, proj, "main")
		if err != nil {
			return convergence.PassState{}, fmt.Errorf("derive coverage: %w", err)
		}
		byLocale := map[string]platstore.LocaleTranslationStats{}
		for _, ls := range stats.LocaleStats {
			byLocale[ls.Locale] = ls
		}

		// Blocks are loaded at most once per derive, and only if some locale is
		// fully covered (a check pass is pointless below full coverage — the
		// locale is pending regardless). This bounds the expensive full-block
		// read to runs approaching their gate.
		var blocks []*platstore.StoredBlock
		var blocksErr error
		blocksLoaded := false
		loadBlocks := func() ([]*platstore.StoredBlock, error) {
			if !blocksLoaded {
				blocks, blocksErr = s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
				blocksLoaded = true
			}
			return blocks, blocksErr
		}

		var pending []string
		produced := 0
		failingTotal := 0
		unitTotals := map[string]int{}
		for _, loc := range proj.TargetLanguages {
			l := string(loc)
			if len(filter) > 0 && !filter[l] {
				continue
			}
			ls := byLocale[l]
			total := ls.TotalBlocks
			if total == 0 {
				total = stats.TranslatableBlocks
			}
			unitTotals[l] = total
			if ls.TranslatedBlocks < total {
				// Under-covered: pending on coverage alone, no need to check.
				produced += ls.TranslatedBlocks
				pending = append(pending, l)
				continue
			}
			// Fully covered: run the bound checks and demote failing units below
			// the gate (they read as not-produced for gating, like local up).
			bl, berr := loadBlocks()
			if berr != nil {
				return convergence.PassState{}, fmt.Errorf("load blocks for checks: %w", berr)
			}
			failing := countFailingBlocks(ctx, bl, loc)
			failingTotal += failing
			effective := max(ls.TranslatedBlocks-failing, 0)
			produced += effective
			if effective < total {
				pending = append(pending, l)
			}
		}
		return convergence.PassState{
			Pending:       pending,
			Produced:      produced,
			FailingChecks: failingTotal,
			UnitTotals:    unitTotals,
		}, nil
	}
}

// countFailingBlocks runs the project's QA checks over the locale's translated
// blocks and returns how many carry an error-severity finding — the units the
// gate must demote. At full coverage every translatable block has a target for
// the locale, so the QA tool always reads a real translation.
func countFailingBlocks(ctx context.Context, blocks []*platstore.StoredBlock, locale model.LocaleID) int {
	failing := 0
	for _, sb := range blocks {
		if sb.Block == nil || !sb.Block.Translatable {
			continue
		}
		for _, issue := range runQAOnBlock(ctx, sb.Block, locale) {
			if issue.Severity == "error" {
				failing++
				break
			}
		}
	}
	return failing
}

// produceFunc builds the server venue's Produce: enqueue the missing-block
// translation jobs for one locale, wait for the job queue to drain them, and
// stream progress. Progress (Done) is counted in BLOCKS — the translated-block
// count for the locale — to match the block-count Units denominator the loop's
// locale_start carries. Job counts (one job per item) would render e.g. 3/500
// for a 3-item, 500-block locale (F9).
func (o *convergenceOrchestrator) produceFunc(projectID string) func(context.Context, string, int, *convergence.Emitter) (int, int, int, error) {
	s := o.server
	return func(ctx context.Context, locale string, pass int, emit *convergence.Emitter) (int, int, int, error) {
		if s.JobStore == nil || s.JobQueue == nil || s.ContentStore == nil {
			return 0, 0, 0, nil // no job infra: nothing produced, the loop parks
		}
		proj, err := s.ContentStore.GetProject(ctx, projectID)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("load project: %w", err)
		}
		items, err := s.ContentStore.ListItems(ctx, projectID, "main")
		if err != nil {
			return 0, 0, 0, fmt.Errorf("list items: %w", err)
		}
		var itemNames []string
		for _, it := range items {
			itemNames = append(itemNames, it.Name)
		}
		if len(itemNames) == 0 {
			return 0, 0, 0, nil
		}
		pushID := id.New()
		jobIDs := s.createTranslationJobs(ctx, proj, itemNames, []string{locale}, pushID, o.workspaceSlug(ctx, proj), "")
		if len(jobIDs) == 0 {
			return 0, 0, 0, nil
		}
		// Job completion is the "this pass is done producing" signal; the
		// translated-BLOCK count is the reported progress.
		translated := 0
		for {
			if err := ctx.Err(); err != nil {
				return translated, 0, translated, err
			}
			jobList, err := s.JobStore.ListJobsByPushID(ctx, pushID)
			if err != nil {
				return translated, 0, translated, fmt.Errorf("poll jobs: %w", err)
			}
			inProgress := 0
			for _, j := range jobList {
				if j.Status != jobs.StatusCompleted && j.Status != jobs.StatusFailed {
					inProgress++
				}
			}
			translated = o.localeTranslatedBlocks(ctx, proj, locale)
			emit.Emit(convergence.Event{
				Type: convergence.EventUnitProgress, Pass: pass, Locale: locale,
				Done: translated, ViaAI: translated,
			})
			if inProgress == 0 {
				// viaTM is unknown server-side; attribute produced units to AI.
				return translated, 0, translated, nil
			}
			select {
			case <-ctx.Done():
				return translated, 0, translated, ctx.Err()
			case <-time.After(convergePollInterval):
			}
		}
	}
}

// localeTranslatedBlocks returns the current translated-block count for a
// locale from the lightweight dashboard stats (0 on error).
func (o *convergenceOrchestrator) localeTranslatedBlocks(ctx context.Context, proj *platstore.Project, locale string) int {
	stats, err := editorGetDashboardStats(ctx, o.server.ContentStore, proj, "main")
	if err != nil {
		return 0
	}
	for _, ls := range stats.LocaleStats {
		if ls.Locale == locale {
			return ls.TranslatedBlocks
		}
	}
	return 0
}

// workspaceSlug best-effort resolves a project's workspace slug for job quota
// attribution ("_anon" when unknown, matching the auto-translate path).
func (o *convergenceOrchestrator) workspaceSlug(ctx context.Context, proj *platstore.Project) string {
	if proj.WorkspaceID != "" && o.server.AuthStore != nil {
		if ws, err := o.server.AuthStore.GetWorkspace(ctx, proj.WorkspaceID); err == nil && ws.Slug != "" {
			return ws.Slug
		}
	}
	return "_anon"
}

// createCompletionReviewTasks creates the per-locale review tasks a completed
// run should fan out, reusing the retired create-review-tasks automation's
// logic (workflow_enabled projects only; deduped per locale). The synthetic
// event carries the linkage the old rule carried — run_id plus the run's items
// — so each task points back at the work that produced it (F11).
func (o *convergenceOrchestrator) createCompletionReviewTasks(ctx context.Context, run *bstore.ConvergenceRun) {
	items := ""
	if o.server.ContentStore != nil {
		if list, err := o.server.ContentStore.ListItems(ctx, run.ProjectID, "main"); err == nil {
			names := make([]string, 0, len(list))
			for _, it := range list {
				names = append(names, it.Name)
			}
			items = strings.Join(names, ",")
		}
	}
	ev := platev.Event{
		Type:      platev.EventPushAutomationsCompleted,
		Source:    "convergence",
		ProjectID: run.ProjectID,
		Data:      map[string]string{"run_id": run.ID, "items": items},
	}
	o.server.createReviewTasks(ctx, event.AutomationAction{Config: map[string]string{"mode": "review"}}, ev, "")
}

// standingTracker rolls the event stream into the per-locale run standing +
// pass/failing-check counters the run row records.
type standingTracker struct {
	mu            sync.Mutex
	passes        int
	failingChecks int
	order         []string
	locales       map[string]bstore.ConvergenceLocaleStanding
}

func newStandingTracker() *standingTracker {
	return &standingTracker{locales: map[string]bstore.ConvergenceLocaleStanding{}}
}

func (t *standingTracker) observe(ev convergence.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch ev.Type {
	case convergence.EventPassStart:
		if ev.Pass > t.passes {
			t.passes = ev.Pass
		}
	case convergence.EventLocaleStart:
		if _, ok := t.locales[ev.Locale]; !ok {
			t.order = append(t.order, ev.Locale)
		}
		cur := t.locales[ev.Locale]
		cur.Locale = ev.Locale
		cur.Units = ev.Units
		cur.State = convergence.LocalePending
		t.locales[ev.Locale] = cur
	case convergence.EventUnitProgress, convergence.EventLocaleDone:
		if _, ok := t.locales[ev.Locale]; !ok {
			t.order = append(t.order, ev.Locale)
		}
		cur := t.locales[ev.Locale]
		cur.Locale = ev.Locale
		if ev.Units > 0 {
			cur.Units = ev.Units
		}
		cur.Produced = ev.Done
		cur.ViaTM = ev.ViaTM
		cur.ViaAI = ev.ViaAI
		if ev.Type == convergence.EventLocaleDone {
			cur.State = convergence.LocaleShippable
			if ev.Done < cur.Units {
				cur.State = convergence.LocalePending
			}
		}
		t.locales[ev.Locale] = cur
	case convergence.EventPassDone:
		t.passes = ev.Pass
		t.failingChecks = ev.FailingChecks
		// Locales still pending after the pass are the park candidates.
		pendingSet := map[string]bool{}
		for _, l := range ev.Pending {
			pendingSet[l] = true
		}
		for l, cur := range t.locales {
			if pendingSet[l] {
				cur.State = convergence.LocalePending
			} else if cur.State == convergence.LocalePending {
				cur.State = convergence.LocaleShippable
			}
			t.locales[l] = cur
		}
	case convergence.EventDone:
		if ev.State == convergence.RunParked {
			for l, cur := range t.locales {
				if cur.State != convergence.LocaleShippable {
					cur.State = convergence.LocaleParked
					t.locales[l] = cur
				}
			}
		}
	}
}

func (t *standingTracker) snapshot() []bstore.ConvergenceLocaleStanding {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]bstore.ConvergenceLocaleStanding, 0, len(t.order))
	for _, l := range t.order {
		out = append(out, t.locales[l])
	}
	return out
}
