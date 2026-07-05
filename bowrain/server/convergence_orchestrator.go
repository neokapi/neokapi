package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/id"
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
	s.EventBus.Subscribe(platev.EventPushCompleted, func(ev platev.Event) {
		if ev.ProjectID == "" || s.ContentStore == nil {
			return
		}
		ctx := context.Background()
		proj, err := s.ContentStore.GetProject(ctx, ev.ProjectID)
		if err != nil {
			return
		}
		if platstore.NormalizeConvergePolicy(proj.ConvergePolicy) != platstore.ConvergePolicyOnPush {
			return // manual: no run on push
		}
		if _, _, err := s.convergence.StartRun(ctx, ev.ProjectID, "push", nil); err != nil {
			slog.Warn("convergence: on-push start failed", "project", ev.ProjectID, "error", err)
		}
	})
}

// StartRun starts (or returns the already-running) convergence run for a
// project. The returned bool reports whether a NEW run was started (false when
// an existing running run was returned, so the handler can answer 200 vs 201).
func (o *convergenceOrchestrator) StartRun(ctx context.Context, projectID, trigger string, locales []string) (*bstore.ConvergenceRun, bool, error) {
	store := o.server.ConvergenceRunStore
	if store == nil {
		return nil, false, errors.New("convergence runs not configured")
	}
	// One run per project at a time: return the in-flight run rather than
	// stacking a second.
	if active, err := store.ActiveRun(ctx, projectID); err != nil {
		return nil, false, err
	} else if active != nil {
		return active, false, nil
	}

	run := &bstore.ConvergenceRun{ProjectID: projectID, Trigger: trigger, State: bstore.ConvergenceRunRunning}
	if err := store.CreateRun(ctx, run); err != nil {
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

// Cancel requests cancellation of an in-flight run.
func (o *convergenceOrchestrator) Cancel(runID string) bool {
	o.mu.Lock()
	cancel := o.cancels[runID]
	o.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
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
			slog.Warn("convergence: persist event failed", "run", run.ID, "error", err)
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

	switch {
	case ctx.Err() != nil:
		run.State = bstore.ConvergenceRunCanceled
	case loopErr != nil:
		run.State = bstore.ConvergenceRunFailed
		slog.Warn("convergence: run failed", "run", run.ID, "error", loopErr)
	case len(res.Final.Pending) > 0:
		run.State = bstore.ConvergenceRunParked
	default:
		run.State = bstore.ConvergenceRunConverged
	}

	// Emit the terminal done event (venue policy, after the loop) so every
	// subscriber sees the same closing frame the CLI venue emits. Emit BEFORE
	// the final row write so the done event settles the per-locale standing
	// (parked locales) into the snapshot the run row records.
	doneState := convergence.RunConverged
	if run.State != bstore.ConvergenceRunConverged {
		doneState = convergence.RunParked
	}
	emit.Emit(convergence.Event{Type: convergence.EventDone, State: doneState})

	run.Standing = standing.snapshot()
	_ = store.UpdateRun(context.WithoutCancel(ctx), run)

	// Parked units enter the team's review queue — the single-player→multiplayer
	// seam. This replaces the retired create-review-tasks-on-automation-complete
	// rule: a run that parks creates the same tasks that rule created.
	if run.State == bstore.ConvergenceRunParked {
		o.createParkReviewTasks(context.WithoutCancel(ctx), run)
	}
}

// deriveFunc builds the server venue's Derive: coverage from the block store,
// with 100%-block-coverage as the ship gate (the server carries no per-scope
// gate data yet; when it does, swap this for the gate check).
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
		var pending []string
		produced := 0
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
			produced += ls.TranslatedBlocks
			// Gate: full block coverage. A locale short of its total is pending.
			if ls.TranslatedBlocks < total {
				pending = append(pending, l)
			}
		}
		return convergence.PassState{
			Pending:    pending,
			Produced:   produced,
			UnitTotals: unitTotals,
		}, nil
	}
}

// produceFunc builds the server venue's Produce: enqueue the missing-block
// translation jobs for one locale and wait for the job queue to drain them,
// emitting unit_progress from completion counts.
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
		// Wait for the worker to drain these jobs, streaming progress.
		done := 0
		for {
			if err := ctx.Err(); err != nil {
				return done, 0, done, err
			}
			jobList, err := s.JobStore.ListJobsByPushID(ctx, pushID)
			if err != nil {
				return done, 0, done, fmt.Errorf("poll jobs: %w", err)
			}
			completed, failed, inProgress := 0, 0, 0
			for _, j := range jobList {
				switch j.Status {
				case jobs.StatusCompleted:
					completed++
				case jobs.StatusFailed:
					failed++
				default:
					inProgress++
				}
			}
			done = completed
			emit.Emit(convergence.Event{
				Type: convergence.EventUnitProgress, Pass: pass, Locale: locale,
				Done: completed, ViaAI: completed,
			})
			if inProgress == 0 {
				// viaTM is unknown server-side; attribute produced units to AI.
				return completed, 0, completed, nil
			}
			select {
			case <-ctx.Done():
				return done, 0, done, ctx.Err()
			case <-time.After(convergePollInterval):
			}
		}
	}
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

// createParkReviewTasks creates the per-locale review tasks for a parked run,
// reusing the same task-creation the retired create-review-tasks automation
// used. The synthetic event carries the run's project so createReviewTasks
// resolves members and locales exactly as before.
func (o *convergenceOrchestrator) createParkReviewTasks(ctx context.Context, run *bstore.ConvergenceRun) {
	ev := platev.Event{
		Type:      platev.EventPushAutomationsCompleted,
		Source:    "convergence",
		ProjectID: run.ProjectID,
		Data:      map[string]string{"run_id": run.ID},
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
