package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/billing"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/observe"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
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
	// tokens is the running total of AI tokens the run's translation jobs
	// reported, accumulated per production pass and read once at the terminal
	// state for the run's consumed-credits bucket (grant sizing). Kept in memory
	// rather than on the run row: it is analytics-only, and a replica that did
	// not drive the run has nothing to report anyway.
	tokens map[string]int // runID → AI tokens used
}

// runStream normalizes a run's stream scope: pre-stream rows carry "".
func runStream(stream string) string {
	if stream == "" {
		return "main"
	}
	return stream
}

func newConvergenceOrchestrator(s *Server) *convergenceOrchestrator {
	return &convergenceOrchestrator{
		server:  s,
		hub:     newConvergenceHub(),
		cancels: map[string]context.CancelFunc{},
		tokens:  map[string]int{},
	}
}

// addRunTokens accumulates one production pass's reported AI token usage onto
// the run. Called once per pass/locale as the pass returns.
func (o *convergenceOrchestrator) addRunTokens(runID string, tokens int) {
	if runID == "" || tokens <= 0 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.tokens[runID] += tokens
}

// takeRunTokens reads and clears a run's accumulated AI token usage — called
// once, at the terminal state, so the map never grows past the in-flight runs.
func (o *convergenceOrchestrator) takeRunTokens(runID string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := o.tokens[runID]
	delete(o.tokens, runID)
	return n
}

// convergePollInterval is how often the orchestrator polls the job queue for a
// locale's translation-job completion.
const convergePollInterval = 750 * time.Millisecond

// convergeOnPushGroup is the durable consumer-group name for the on-push
// convergence triggers. Stable by design: on the Redis bus the group's resume
// position is keyed by this name, so renaming it discards the acknowledged
// position and re-creates the group at the current stream head.
const convergeOnPushGroup = "convergence-onpush"

// subscribeConvergeOnPush wires the continuous-convergence policy: a completed
// push starts a convergence run for on-push projects (the default), replacing
// the retired auto-translate-on-push automation. Manual projects converge only
// on demand. Transport (push) stays pure; this is the project's own clock.
//
// Durability: this trigger is state-advancing — a lost EventPushCompleted means
// convergence-on-push silently never happens — so it joins a consumer group
// (SubscribeGroup → XREADGROUP) instead of tailing with Subscribe. A plain
// Subscribe reads from the CURRENT stream position with no resume, so an event
// published while the server was mid-deploy-rollover simply vanished. The
// group's acknowledged position survives restarts (the resubscribing instance
// drains everything published during its downtime) and exactly one instance in
// the group handles each event.
//
// Idempotency (same argument as durable ingest, #1364): replaying an old push
// event — the startup drain, or a redelivered un-acked entry — starts at worst
// one extra convergence run over already-converged state, which derives
// coverage, finds nothing pending, and terminates. StartRun's active-run guard
// (DB partial unique index) additionally collapses a replay racing a live run
// into joining that run.
func (s *Server) subscribeConvergeOnPush() {
	if s.EventBus == nil || s.convergence == nil {
		return
	}
	// One group subscription dispatching on event type: a group handler
	// receives every event (no bus-side type filter — the same shape as the
	// audit/notifications/automations group consumers). Two SubscribeGroup
	// calls with the same name would be competing consumers splitting the
	// stream between them, not two filtered feeds.
	//
	// The handler always acknowledges. Everything it can fail at is logged and
	// re-derivable — a run that could not start over already-converged state
	// would find nothing pending anyway — and a redelivery is a second attempt
	// at the same run, which StartRun's active-run guard collapses.
	s.EventBus.SubscribeGroup(convergeOnPushGroup, func(ev platev.Event) error {
		switch ev.Type {
		case platev.EventPushCompleted:
			// A completed push starts a run for on-push projects.
			s.startOnPushRun(ev, "push")
		case platev.EventProjectUpdated:
			// Adding a target locale is also new work the on-push policy should
			// absorb: the old auto-translate-new-locale automation was removed,
			// so without this a new locale would translate nothing (F10).
			// handlers_project publishes EventProjectUpdated with new_locales
			// set; a run re-derives coverage and produces the new locale like
			// any other pending one.
			if ev.Data["new_locales"] == "" {
				return nil // only a locale addition warrants a run
			}
			s.startOnPushRun(ev, "push")
		}
		return nil
	})
}

// startOnPushRun starts a convergence run for the event's project when its
// policy is on-push. Shared by the push-completed and new-locale triggers.
func (s *Server) startOnPushRun(ev platev.Event, trigger string) {
	// Receipt + every exit reason logged: a silently dropped push event is
	// indistinguishable from a bus delivery failure without these lines.
	slog.Info("convergence: push event received",
		"project", ev.ProjectID, "source", ev.Source, "trigger", trigger)
	if ev.ProjectID == "" || s.ContentStore == nil || s.convergence == nil {
		slog.Warn("convergence: on-push skipped",
			"reason", "missing project id or orchestrator wiring", "project", ev.ProjectID)
		return
	}
	// Event-bus callback: a run is the project's own clock and must outlive
	// the request that published the event, so no request context applies.
	ctx := context.Background()
	proj, err := s.ContentStore.GetProject(ctx, ev.ProjectID)
	if err != nil {
		slog.Warn("convergence: on-push skipped",
			"reason", "project lookup failed", "project", ev.ProjectID, "error", err)
		return
	}
	if platstore.NormalizeConvergePolicy(proj.ConvergePolicy) != platstore.ConvergePolicyOnPush {
		slog.Info("convergence: on-push skipped",
			"reason", "policy is manual", "project", ev.ProjectID, "policy", proj.ConvergePolicy)
		return // manual: no run
	}
	// The push event names the stream its content landed on; the run it
	// triggers derives and produces on that same stream.
	if _, _, err := s.convergence.StartRun(ctx, ev.ProjectID, ev.Data["stream"], trigger, nil); err != nil {
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
// StartRun starts (or joins) a run scoped to one stream. An empty stream means
// "main" — the pre-stream behavior every existing caller keeps.
func (o *convergenceOrchestrator) StartRun(ctx context.Context, projectID, stream, trigger string, locales []string) (*bstore.ConvergenceRun, bool, error) {
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
	if stream == "" {
		stream = "main"
	}
	run := &bstore.ConvergenceRun{ProjectID: projectID, Stream: stream, Trigger: trigger, State: bstore.ConvergenceRunRunning}
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

	o.trackRunStarted(ctx, run)
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
		Derive:  o.deriveFunc(run.ProjectID, run.Stream, localeFilter),
		Produce: o.produceFunc(run.ProjectID, run.Stream, run.ID),
	})
}

// runSettleSource is the server run's source-first phase (epic 019): it settles
// the source once at the start of a run — before any target locale is produced —
// stamps each source block's SourceStatus, and reports how many blocks remain
// below the gate. It emits a settle_source stage event so a surface can show the
// run "settling your source" and records the blocked-on-source count on the run
// row so the UI can render "N segments need source review" without a second
// round-trip. A settlement error is non-fatal: the run degrades to the previous
// (gate-off) behavior rather than failing on a source-check hiccup.
func (o *convergenceOrchestrator) runSettleSource(ctx context.Context, run *bstore.ConvergenceRun, emit *convergence.Emitter) settleResult {
	// Announce the phase BEFORE the work: first-run settlement over a large
	// corpus reads and rewrites every source block, which can take tens of
	// minutes — and a run whose current_stage stays empty for that long is
	// indistinguishable from a dead one. The gate is resolved up front only to
	// keep an opted-out project event-free, as before.
	if cs := o.server.ContentStore; cs != nil {
		if proj, perr := cs.GetProject(ctx, run.ProjectID); perr == nil && sourceGateFor(proj) != model.SourceGateNone {
			emit.Emit(convergence.Event{
				Type:    convergence.EventLog,
				Stage:   convergence.StageSettleSource,
				Message: "Settling source (first run over a large corpus reads and re-stamps every block, so this can take a while)…",
			})
		}
	}
	settleStart := time.Now()
	res, err := o.settleSource(ctx, run.ProjectID)
	if d := time.Since(settleStart); d > 30*time.Second {
		slog.Info("convergence: source settlement finished", "run", run.ID, "duration", d.Round(time.Second),
			"total", res.Total, "settled", res.Settled, "blocked", res.BlockedOnSource)
	}
	if err != nil {
		slog.Warn("convergence: source settlement failed; proceeding without gate", "run", run.ID, "error", err)
		return settleResult{Gate: model.SourceGateNone}
	}
	if res.Gate == model.SourceGateNone {
		return res // opt-out: no settle event, no hold
	}
	run.BlockedOnSource = res.BlockedOnSource
	emit.Emit(convergence.Event{
		Type:            convergence.EventLog,
		Stage:           convergence.StageSettleSource,
		SettledSource:   res.Settled,
		BlockedOnSource: res.BlockedOnSource,
		Message: fmt.Sprintf("Settled source: %d block(s) checked, %d below the %q gate.",
			res.Total, res.BlockedOnSource, res.Gate),
	})
	return res
}

// driveWith runs the venue-neutral loop for one run to completion with the
// given IO, persisting and broadcasting every event, then records the terminal
// state. The LoopFuncs are a parameter so the orchestrator's run lifecycle
// (event persistence, SSE fan-out, state transitions, park→review) is testable
// against an in-memory model without the full block store + job queue.
func (o *convergenceOrchestrator) driveWith(ctx context.Context, run *bstore.ConvergenceRun, funcs convergence.LoopFuncs) {
	// A convergence run has no HTTP request ID, but the run ID is the handle the
	// user sees (it is in the run URL). Seed it as the correlation ID so every
	// *Context log line during this run — and any Sentry event — carries
	// request_id=<run.ID>, unifying the background-run and per-request ID spaces.
	ctx = observe.WithRequestID(ctx, run.ID)

	s := o.server
	store := s.ConvergenceRunStore
	defer func() {
		o.mu.Lock()
		delete(o.cancels, run.ID)
		o.mu.Unlock()
	}()

	standing := convergence.NewStanding()
	emit := convergence.NewEmitter(func(ev convergence.Event) {
		standing.Observe(ev)
		// Track the run's live loop position (theme D): the stage the event was
		// emitted from, the locale being produced, and a heartbeat. last_activity
		// advances on every event so a frozen value while jobs are still awaited
		// reads as stalled, not merely slow.
		if ev.Stage != "" {
			run.CurrentStage = ev.Stage
		}
		if ev.Locale != "" {
			run.CurrentLocale = ev.Locale
		}
		now := time.Now().UTC()
		run.LastActivity = &now
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
		// The loop-position columns ride the same boundary write.
		if ev.Type == convergence.EventPassDone || ev.Type == convergence.EventLocaleDone {
			run.Passes = standing.Passes()
			run.FailingChecks = standing.FailingChecks()
			run.Standing = standing.Locales()
			_ = store.UpdateRun(context.WithoutCancel(ctx), run)
		}
	})

	// Source-first phase (epic 019): settle the source ONCE before any target
	// locale is produced, stamp each block's SourceStatus, and record how many
	// blocks are held below the gate. The gate itself is enforced per block in
	// produceFunc (a partially-ready item translates only its ready blocks) and
	// terminates the run with source_not_ready when a locale has nothing
	// producible. Settlement is a no-op when the block store is absent (the
	// in-memory driveWith tests) or the gate is `none`.
	o.runSettleSource(ctx, run, emit)

	res, loopErr := convergence.Loop(ctx, convergence.LoopOptions{
		UntilGate: true,
		MaxPasses: 5,
		Jobs:      4, // per-pass locale concurrency
	}, funcs, emit)

	// Terminal state.
	finished := time.Now().UTC()
	run.FinishedAt = &finished
	run.Passes = res.Passes
	run.FailingChecks = standing.FailingChecks()

	// The error field carries the cause the CLI prints for a failed/canceled
	// run (cancellation surfaces as a loop error too, so it is checked first and
	// gets a clean message rather than "context canceled").
	//
	// A typed stall (e.g. a credit refusal) is NOT a hard failure: the loop
	// returns it as an error because Produce could not advance, but the correct
	// outcome is a PARKED run with a machine-readable stall_reason and a human
	// message — work so far is saved, and the UI/analytics can offer the next
	// action (buy credits / add a key). So the stall sentinel is matched before
	// the generic failed branch (theme C).
	run.StallReason = convergence.StallNone
	var stall *stallError
	switch {
	case ctx.Err() != nil:
		run.State = bstore.ConvergenceRunCanceled
		run.Error = "canceled"
	case loopErr != nil && errors.As(loopErr, &stall):
		run.State = bstore.ConvergenceRunParked
		run.StallReason = stall.reason
		run.Error = stall.message
		slog.InfoContext(ctx, "convergence: run parked on stall", "run", run.ID, "stall_reason", stall.reason)
	case loopErr != nil:
		run.State = bstore.ConvergenceRunFailed
		run.Error = loopErr.Error()
		slog.WarnContext(ctx, "convergence: run failed", "run", run.ID, "error", loopErr)
	case len(res.Final.Pending) > 0:
		run.State = bstore.ConvergenceRunParked
		run.StallReason = parkedStallReason(res.Final)
	default:
		run.State = bstore.ConvergenceRunConverged
	}

	// Record the terminal outcome for the core product loop. run.State is a
	// bounded wire value (converged|parked|failed|canceled), safe as a label.
	observe.ConvergenceRunsTotal.WithLabelValues(run.State).Inc()

	// A terminal run has stopped moving; freeze the loop position so the row does
	// not read as "producing ai_translate" after it finished.
	run.CurrentStage = ""
	run.CurrentLocale = ""

	// Emit the terminal done event (venue policy, after the loop) so every
	// subscriber sees the same closing frame the CLI venue emits. State carries
	// the REAL outcome — converged | parked | canceled | failed — so a caller
	// never mistakes a failed/canceled run for parked work and exits 0 (F2); the
	// stall_reason rides the same frame so the UI can label WHY a parked run
	// stopped without a second round-trip. The store's state strings are exactly
	// the core/convergence.Run* wire values. Emit BEFORE the final row write so
	// the done event settles the per-locale standing (parked locales) into the
	// snapshot the run records.
	emit.Emit(convergence.Event{
		Type: convergence.EventDone, State: run.State, StallReason: run.StallReason,
		BlockedOnSource: run.BlockedOnSource,
	})

	run.Standing = standing.Locales()
	// The last write the run makes, and the one that moves it out of "running".
	// Losing it leaves the row running forever, and the active-run guard then
	// refuses every new run for that project — a silent stop, not a slow one.
	// Nothing here can retry it, but it must not go unrecorded.
	if err := store.UpdateRun(context.WithoutCancel(ctx), run); err != nil {
		slog.ErrorContext(ctx, "convergence: failed to write the final run row; the run may stay marked running",
			"run", run.ID, "project", run.ProjectID, "state", run.State, "error", err)
	}

	// Hold-on-source is a first-class outcome (epic 019): when the run parked
	// because the source is below the gate, create the source-review task(s) for
	// the un-ready source — reusing the existing create_source_review automation
	// — so the user has a clear next action ("settle your source first") instead
	// of a silent hold. Only for the source-not-ready reason; other parks route
	// to the translation review queue below.
	if run.State == bstore.ConvergenceRunParked && run.StallReason == convergence.StallSourceNotReady {
		o.createSourceReviewTasks(context.WithoutCancel(ctx), run)
	}

	// On completion (converged OR parked), the project's content enters the
	// team's review queue — the single-player→multiplayer seam. Governed review
	// is the default: only a project that explicitly set workflow_enabled=false
	// skips the fan-out (the delegate checks). The fan-out covers BOTH outcomes
	// — converged is the common one — and carries the run/items/locales linkage.
	// Failed/canceled runs create nothing.
	// A source-not-ready hold produced no translations, so it routes to SOURCE
	// review (below), not the translation review queue — skip the completion
	// tasks in that case to avoid a "review translations" task for work that was
	// never done. A no-target-locales hold likewise produced nothing to review:
	// the next action is configuration (add a target language), not review.
	//
	// run.Passes > 0 is the anti-loop gate (RV-B): a run that ran zero production
	// passes derived 0 pending on its first pass — it translated nothing new, so
	// there is no fresh draft work to review. That is exactly the shape of the
	// completing run a review.completed triggers: every block is already
	// approved, the derive finds nothing pending, the loop runs 0 passes and
	// converges. Fanning review tasks back out for already-reviewed content would
	// re-open the queue and let review → run → review loop forever. Gating on
	// "the run actually produced work" breaks that loop while still fanning tasks
	// out for every genuine translate/park run (which always ran ≥1 pass).
	if (run.State == bstore.ConvergenceRunConverged || run.State == bstore.ConvergenceRunParked) &&
		run.StallReason != convergence.StallSourceNotReady &&
		run.StallReason != convergence.StallNoTargetLocales &&
		run.Passes > 0 {
		o.createCompletionReviewTasks(context.WithoutCancel(ctx), run)
	}

	// Announce the terminal state on the bus: the forge delivery tier, the
	// activity recorder, and any future subscriber react to finished runs
	// without polling the run store. Published for every terminal state —
	// subscribers filter; a canceled or failed run must be as observable as a
	// converged one.
	//
	// The frame carries the workspace and the run's outcome, not just its id.
	// The activity recorder files every entry under the workspace SLUG, so an
	// event that named only the project landed in the feed under no workspace
	// at all — the same omission the knowledge events had (publishKnowledgeEvents)
	// — and the standing counts are what let the feed line say what the run did
	// rather than merely that it ended.
	if o.server.EventBus != nil {
		wsID := o.projectWorkspaceID(context.WithoutCancel(ctx), run.ProjectID)
		locales, produced := runOutcomeCounts(run)
		o.server.EventBus.Publish(platev.Event{
			ID:        id.New(),
			Type:      platev.EventConvergenceRunCompleted,
			Source:    "convergence",
			ProjectID: run.ProjectID,
			Timestamp: time.Now().UTC(),
			Data: map[string]string{
				"run_id":         run.ID,
				"state":          run.State,
				"passes":         strconv.Itoa(run.Passes),
				"stall_reason":   run.StallReason,
				"workspace_id":   wsID,
				"workspace_slug": o.workspaceSlugFor(context.WithoutCancel(ctx), wsID),
				"locales":        strings.Join(locales, ","),
				"locales_count":  strconv.Itoa(len(locales)),
				"produced_count": strconv.Itoa(produced),
			},
		})
	}

	o.trackRunCompleted(context.WithoutCancel(ctx), run, standing)
}

// deriveFunc builds the server venue's Derive: coverage from the block store,
// gated on full block coverage AND the project's bound checks — the same
// two conditions the local venue applies (F6). A locale reaches its gate only
// when every translatable block has a target AND none fails the checks; a
// failing block demotes its unit below the gate, so a connected `kapi up`
// parks on failing terminology/length checks exactly like local `up` rather
// than silently claiming converged.
func (o *convergenceOrchestrator) deriveFunc(projectID, stream string, localeFilter []string) func(context.Context) (convergence.PassState, error) {
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
		// No configured target languages is a configuration hold, never "up to
		// date": with zero locales the loop below would derive zero pending and
		// the run would read converged over any amount of untranslated source.
		// The typed stall parks the run with a machine-readable reason + the
		// same message the local venue refuses with, so the UI can say "add a
		// target language" instead of a false green (mirrors host/converge.go).
		if len(proj.TargetLanguages) == 0 {
			return convergence.PassState{}, errStallNoTargetLocales
		}
		stats, err := editorGetDashboardStats(ctx, s.ContentStore, proj, runStream(stream))
		if err != nil {
			return convergence.PassState{}, fmt.Errorf("derive coverage: %w", err)
		}
		byLocale := map[string]platstore.LocaleTranslationStats{}
		for _, ls := range stats.LocaleStats {
			byLocale[ls.Locale] = ls
		}

		// The locales this derive is answering for.
		var locales []model.LocaleID
		for _, loc := range proj.TargetLanguages {
			if len(filter) == 0 || filter[string(loc)] {
				locales = append(locales, loc)
			}
		}

		// Coverage first, and only for the locales the dashboard stats cannot
		// answer for. The stats are ITEM-scoped (GetBlockStats scopes its block
		// query to the stream's item rows), so blocks ingested without item
		// bookkeeping — the historical connector-fetch path stored blocks
		// item-less — are invisible to them and the locale would false-read as
		// at-gate ("Up to date") over real pending work. The block store is the
		// truth for pending-work derivation: a translatable block lacking a
		// target for a CONFIGURED locale is pending, item rows or not.
		var needCoverage []model.LocaleID
		for _, loc := range locales {
			ls := byLocale[string(loc)]
			if ls.TotalBlocks == 0 && stats.TranslatableBlocks == 0 {
				needCoverage = append(needCoverage, loc)
			}
		}
		coverage, err := blockCoverage(ctx, s.ContentStore, projectID, runStream(stream), needCoverage)
		if err != nil {
			return convergence.PassState{}, fmt.Errorf("load blocks for coverage: %w", err)
		}

		// Resolve each locale's totals, then check only the fully covered ones —
		// a check pass is pointless below full coverage, the locale is pending
		// regardless.
		type localeState struct {
			total      int
			translated int
		}
		states := make(map[string]localeState, len(locales))
		var toCheck []model.LocaleID
		for _, loc := range locales {
			l := string(loc)
			ls := byLocale[l]
			st := localeState{total: ls.TotalBlocks, translated: ls.TranslatedBlocks}
			if st.total == 0 {
				st.total = stats.TranslatableBlocks
			}
			if cov, ok := coverage[loc]; ok {
				st.total, st.translated = cov.total, cov.translated
			}
			states[l] = st
			if st.translated >= st.total {
				toCheck = append(toCheck, loc)
			}
		}
		failingByLocale, err := countFailingBlocks(ctx, s.ContentStore, projectID, runStream(stream), toCheck)
		if err != nil {
			return convergence.PassState{}, fmt.Errorf("load blocks for checks: %w", err)
		}

		var pending []string
		produced := 0
		failingTotal := 0
		unitTotals := map[string]int{}
		for _, loc := range locales {
			l := string(loc)
			st := states[l]
			unitTotals[l] = st.total
			if st.translated < st.total {
				// Under-covered: pending on coverage alone, no need to check.
				produced += st.translated
				pending = append(pending, l)
				continue
			}
			// Fully covered: the bound checks demote failing units below the gate
			// (they read as not-produced for gating, like local up).
			failing := failingByLocale[loc]
			failingTotal += failing
			effective := max(st.translated-failing, 0)
			produced += effective
			if effective < st.total {
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

// coverageCounts is one locale's block-derived coverage: how many blocks are
// translatable (the unit total) and how many of those carry a non-empty target.
type coverageCounts struct {
	total      int
	translated int
}

// blockCoverage derives coverage directly from the stored blocks for every
// locale asked about. It is the item-bookkeeping-free fallback the derive uses
// when the dashboard stats see zero units — the pending-work question ("does a
// translatable block lack a target for a configured locale?") must never depend
// on item rows existing.
//
// One walk answers for every locale, a batch at a time: the counts are the only
// thing that survives a batch. Asking per locale would read the corpus once per
// language, and reading it whole is what OOM-killed the server.
func blockCoverage(
	ctx context.Context,
	cs platstore.ContentStore,
	projectID, stream string,
	locales []model.LocaleID,
) (map[model.LocaleID]coverageCounts, error) {
	out := make(map[model.LocaleID]coverageCounts, len(locales))
	if len(locales) == 0 {
		return out, nil
	}
	for _, loc := range locales {
		out[loc] = coverageCounts{}
	}
	err := platstore.EachBlockBatch(ctx, cs,
		platstore.BlockQuery{ProjectID: projectID, Stream: stream},
		platstore.DefaultBlockBatch,
		func(batch []*venue.StoredBlock) error {
			for _, sb := range batch {
				if sb == nil || sb.Block == nil || !sb.Block.Translatable {
					continue
				}
				for _, loc := range locales {
					c := out[loc]
					c.total++
					if convergence.TargetState(sb.Block, string(loc)) != "" {
						c.translated++
					}
					out[loc] = c
				}
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// countFailingBlocks runs the project's checks over each locale's translated
// blocks and returns how many carry an error-severity finding — the units the
// gate must demote. At full coverage every translatable block has a target for
// the locale, so the check tool always reads a real translation.
//
// Batched, and every locale answered in the one walk: a count per locale is all
// that outlives a batch.
func countFailingBlocks(
	ctx context.Context,
	cs platstore.ContentStore,
	projectID, stream string,
	locales []model.LocaleID,
) (map[model.LocaleID]int, error) {
	out := make(map[model.LocaleID]int, len(locales))
	if len(locales) == 0 {
		return out, nil
	}
	for _, loc := range locales {
		out[loc] = 0
	}
	err := platstore.EachBlockBatch(ctx, cs,
		platstore.BlockQuery{ProjectID: projectID, Stream: stream},
		platstore.DefaultBlockBatch,
		func(batch []*venue.StoredBlock) error {
			for _, sb := range batch {
				if sb == nil || sb.Block == nil || !sb.Block.Translatable {
					continue
				}
				for _, loc := range locales {
					if blockFailsChecks(ctx, sb.Block, loc) {
						out[loc]++
					}
				}
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// produceFunc builds the server venue's Produce: enqueue the missing-block
// translation jobs for one locale, wait for the job queue to drain them, and
// stream progress. Progress (Done) is counted in BLOCKS — the translated-block
// count for the locale — to match the block-count Units denominator the loop's
// locale_start carries. Job counts (one job per item) would render e.g. 3/500
// for a 3-item, 500-block locale (F9).
// runID scopes the pass's reported AI token usage to the run, so the terminal
// event can report what the run ACTUALLY spent (grant sizing); it may be empty
// (tests driving a bare produce), which simply reports no usage.
func (o *convergenceOrchestrator) produceFunc(projectID, stream, runID string) func(context.Context, string, int, *convergence.Emitter) (convergence.PassProduction, error) {
	s := o.server
	return func(ctx context.Context, locale string, pass int, emit *convergence.Emitter) (convergence.PassProduction, error) {
		// The last poll's token sum is the pass's total (a job's tokens_used is
		// written when it finishes), banked on every exit path so a canceled pass
		// still reports what it burned before the cancel.
		passTokens := 0
		defer func() { o.addRunTokens(runID, passTokens) }()

		if s.JobStore == nil || s.JobQueue == nil || s.ContentStore == nil {
			return convergence.PassProduction{}, nil // no job infra: nothing produced, the loop parks
		}
		proj, err := s.ContentStore.GetProject(ctx, projectID)
		if err != nil {
			return convergence.PassProduction{}, fmt.Errorf("load project: %w", err)
		}
		items, err := s.ContentStore.ListItems(ctx, projectID, runStream(stream))
		if err != nil {
			return convergence.PassProduction{}, fmt.Errorf("list items: %w", err)
		}
		var itemNames []string
		for _, it := range items {
			itemNames = append(itemNames, it.Name)
		}
		if len(itemNames) == 0 {
			return convergence.PassProduction{}, nil
		}

		// Source-first gate (epic 019): before spawning translation jobs, drop
		// any item whose source blocks are ALL below the gate — those hold on
		// source rather than translating an unsettled source. A partially-ready
		// item stays (the worker translates only its ready blocks). When the
		// whole locale has nothing producible AND some source is held, terminate
		// with source_not_ready so the run parks on source review instead of
		// spinning — no AI spend, no discarded work.
		producible, blockedItems, gErr := o.gateItemsBySource(ctx, projectID, itemNames)
		if gErr != nil {
			return convergence.PassProduction{}, gErr
		}
		if len(producible) == 0 {
			if blockedItems > 0 {
				return convergence.PassProduction{}, errStallSourceNotReady
			}
			return convergence.PassProduction{}, nil // nothing to do (already covered), not a hold
		}
		itemNames = producible

		pushID := id.New()
		// A credit refusal returns errStallNeedsCredits (not a silent empty
		// list): propagate it so driveWith labels the run's stall_reason
		// (needs_credits) instead of parking with no reason (theme C). Work so
		// far is untouched — the refusal spawns nothing, it discards nothing.
		// The orchestrator's derivation (stats, pending work, gates) is
		// main-scoped throughout, so its produce step stays on main too;
		// stream-scoped convergence arrives with the orchestrator, not here.
		jobIDs, err := s.createTranslationJobs(ctx, proj, runStream(stream), itemNames, []string{locale}, pushID, o.workspaceSlug(ctx, proj), "")
		if err != nil {
			return convergence.PassProduction{}, err
		}
		if len(jobIDs) == 0 {
			return convergence.PassProduction{}, nil
		}
		// Job completion is the "this pass is done producing" signal; the
		// translated-BLOCK count is the reported progress. Each poll refreshes
		// the run's last_activity from the newest job updated_at so a "slow but
		// alive" run is distinguishable from a stalled one (theme D). content memory-first
		// convergence (theme A2): each translation job records how many blocks it
		// filled from the project content memory (recycled) vs. sent to the AI translator; we
		// sum those splits across the locale's jobs so the run reports a truthful
		// "content memory N · AI M" instead of attributing everything to AI.
		translated := 0
		for {
			if err := ctx.Err(); err != nil {
				return convergence.PassProduction{Done: translated, ViaAI: translated}, err
			}
			jobList, err := s.JobStore.ListJobsByPushID(ctx, pushID)
			if err != nil {
				return convergence.PassProduction{Done: translated, ViaAI: translated}, fmt.Errorf("poll jobs: %w", err)
			}
			inProgress := 0
			viaMemory := 0
			tokens := 0
			for _, j := range jobList {
				viaMemory += j.ViaMemory
				tokens += j.TokensUsed
				if j.Status != jobs.StatusCompleted && j.Status != jobs.StatusFailed {
					inProgress++
				}
			}
			passTokens = tokens
			translated = o.localeTranslatedBlocks(ctx, proj, stream, locale)
			// Split the reported Done across content memory/AI. The recorded viaMemory is
			// authoritative; the remainder is attributed to AI so the counters
			// stay consistent with Done even while a pass is still in flight.
			doneMemory, doneAI := reconcileSplit(translated, viaMemory)
			emit.Emit(convergence.Event{
				Type: convergence.EventUnitProgress, Stage: convergence.StageAITranslate,
				Pass: pass, Locale: locale, Done: translated, ViaMemory: doneMemory, ViaAI: doneAI,
			})
			if inProgress == 0 {
				return convergence.PassProduction{Done: translated, ViaMemory: doneMemory, ViaAI: doneAI}, nil
			}
			select {
			case <-ctx.Done():
				return convergence.PassProduction{Done: translated, ViaMemory: doneMemory, ViaAI: doneAI}, ctx.Err()
			case <-time.After(convergePollInterval):
			}
		}
	}
}

// reconcileSplit distributes a total translated-block count across content memory and AI
// using the jobs' recorded split. The recorded viaMemory is trusted (capped at the
// total so a lagging dashboard stat can't push it negative); the AI share is
// whatever remains, so ViaMemory + ViaAI always equals Done. This keeps the "content memory N ·
// AI M" report internally consistent even while a pass is still in flight and
// some jobs have not yet recorded their split.
func reconcileSplit(total, viaMemory int) (tm, ai int) {
	if total <= 0 {
		return 0, 0
	}
	if viaMemory > total {
		viaMemory = total
	}
	if viaMemory < 0 {
		viaMemory = 0
	}
	return viaMemory, total - viaMemory
}

// localeTranslatedBlocks returns the current translated-block count for a
// locale from the lightweight dashboard stats (0 on error).
func (o *convergenceOrchestrator) localeTranslatedBlocks(ctx context.Context, proj *platstore.Project, stream, locale string) int {
	stats, err := editorGetDashboardStats(ctx, o.server.ContentStore, proj, runStream(stream))
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
// logic (on by default; a project opting out via workflow_enabled=false
// creates none; deduped per locale). The synthetic
// event carries the linkage the old rule carried — run_id plus the run's items
// — so each task points back at the work that produced it (F11).
func (o *convergenceOrchestrator) createCompletionReviewTasks(ctx context.Context, run *bstore.ConvergenceRun) {
	items := ""
	if o.server.ContentStore != nil {
		if list, err := o.server.ContentStore.ListItems(ctx, run.ProjectID, runStream(run.Stream)); err == nil {
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

// createSourceReviewTasks is the hold-on-source action (epic 019): when a run
// parks because the source is below the gate, it fans out the "Review source
// content before translation" task by reusing the existing create_source_review
// automation (Bowrain AD-014). The synthetic event carries the run/items linkage
// and the blocked-on-source count so the task — and the source review queue it
// lands in — points back at the run that held. It also publishes
// EventSourceReviewCompleted's counterpart trigger so any subscriber wired to
// source review reacts. A settled source (or a lowered gate) lets the next run
// translate.
func (o *convergenceOrchestrator) createSourceReviewTasks(ctx context.Context, run *bstore.ConvergenceRun) {
	items := ""
	if o.server.ContentStore != nil {
		if list, err := o.server.ContentStore.ListItems(ctx, run.ProjectID, runStream(run.Stream)); err == nil {
			names := make([]string, 0, len(list))
			for _, it := range list {
				names = append(names, it.Name)
			}
			items = strings.Join(names, ",")
		}
	}
	ev := platev.Event{
		Type:      platev.EventPushCompleted,
		Source:    "convergence",
		ProjectID: run.ProjectID,
		Data: map[string]string{
			"run_id":            run.ID,
			"items":             items,
			"blocked_on_source": strconv.Itoa(run.BlockedOnSource),
			"stall_reason":      run.StallReason,
		},
	}
	o.server.createSourceReviewTask(ctx, event.AutomationAction{Config: map[string]string{}}, ev, "")
}

// stallError is a Produce failure that is really a labeled, resumable stall
// (out of credits, no AI key, rate-limited) rather than a hard error. The loop
// surfaces it as an error because production could not advance; driveWith
// unwraps it to a PARKED run carrying the machine-readable reason and a human
// message, keeping the work so far (strategy 2026-07-dogfood doc 06, theme C).
type stallError struct {
	reason  convergence.StallReason
	message string
}

func (e *stallError) Error() string { return e.message }

// StallNeedsCredits is the platform-only stall reason: the credit pre-check
// refused to produce (a zero-credit workspace). Work so far is saved; add
// credits and resume. Credits are a platform concept, so this reason lives with
// the server rather than in the framework's open StallReason vocabulary. The
// wire value is fixed — it is carried on run events and persisted on run rows.
const StallNeedsCredits convergence.StallReason = "needs_credits"

// errStallNeedsCredits is the typed refusal a zero-credit platform-key
// production returns instead of a silent empty job list. Its message is the
// same two-ways-forward copy the enqueue handler surfaces.
var errStallNeedsCredits = &stallError{
	reason:  StallNeedsCredits,
	message: "out of platform credits. Buy a credit pack, wait for the weekly reset, or configure your own AI provider key",
}

// errStallNoTargetLocales is the typed hold a derive returns when the project
// has no configured target languages: there is no locale to converge toward, so
// the run PARKS on configuration ("no target languages configured") instead of
// false-reading "Up to date" over untranslated source. Same message the local
// venue (`kapi up`) refuses with.
var errStallNoTargetLocales = &stallError{
	reason:  convergence.StallNoTargetLocales,
	message: "no target languages configured. Add target languages to the project, then run again",
}

// errStallSourceNotReady is the typed hold a locale returns when every block it
// would translate is below the source-first gate: rather than fan an unsettled,
// non-compliant, un-term-checked source out to N locales (only to redo it when the
// source changes), the run HOLDS on source and routes to source review. This is
// a first-class outcome — no AI is spent, no work is discarded — distinct from
// an out-of-credits stall (strategy 2026-07-dogfood doc 07 / roadmap epic 019).
var errStallSourceNotReady = &stallError{
	reason:  convergence.StallSourceNotReady,
	message: "source not ready. Settle your source first (terminology, brand, source checks), or set defaults.source_gate: none to translate anyway",
}

// parkedStallReason labels an ordinary parked outcome (no typed stall error):
// checks_failing when the bound checks demoted units below the gate, otherwise
// no_progress (genuine pending work the flow cannot advance unaided). This is
// the "real pending review" case, distinct from an out-of-credits block.
//
// It deliberately does NOT return source_not_ready: a partial run — one that
// translated the ready items and left only the source-held remainder pending —
// still produced real translations that must enter the translation review
// queue. Labeling it source_not_ready would route it to source review and skip
// the completion tasks (driveWith), stranding the translated work. A PURE hold
// (no ready items) never reaches here: produceFunc returns the typed
// errStallSourceNotReady, which driveWith matches before this path. The
// blocked-on-source count still rides on the run row for the UI either way.
func parkedStallReason(final convergence.PassState) convergence.StallReason {
	if final.FailingChecks > 0 {
		return convergence.StallChecksFailing
	}
	return convergence.StallNoProgress
}

// trackRunStarted emits the convergence_run_started analytics event (epic 018
// spine, theme D). Distinct-id is the workspace (falling back to the project),
// matching the service layer's system-event convention.
func (o *convergenceOrchestrator) trackRunStarted(ctx context.Context, run *bstore.ConvergenceRun) {
	if o.server.PostHogClient == nil {
		return
	}
	wsID := o.projectWorkspaceID(ctx, run.ProjectID)
	props := analytics.Props(wsID, run.ProjectID)
	props["trigger"] = run.Trigger
	// Cold-start cohort: is this the workspace's first convergence run ever?
	// It is the cohort the one-time new-workspace grant has to cover, so every
	// run event carries it (grant sizing).
	props[analytics.PropFirstRun] = o.server.isFirstConvergenceRun(ctx, wsID, run.ProjectID, run.CreatedAt, run.ID)
	o.server.trackEvent(convergenceDistinctID(wsID, run.ProjectID), analytics.EventConvergenceRunStarted, props)
}

// trackRunCompleted emits the convergence_run_completed analytics event at a
// terminal state, carrying the outcome, the machine-readable stall_reason, and
// coarse content memory/AI attribution + duration so the fleet-wide "where do runs stall"
// question is answerable (theme D / D3). Never carries content.
//
// It also carries the run's bucketed MAGNITUDE — what it actually spent, and
// how much of its work TM covered for free — plus the first-run cohort marker,
// so "how big must the new-workspace grant be to cover a first project?" is
// answerable from realized spend rather than from a yes/no coverage flag.
func (o *convergenceOrchestrator) trackRunCompleted(ctx context.Context, run *bstore.ConvergenceRun, standing *convergence.Standing) {
	tokens := o.takeRunTokens(run.ID) // read once, terminal state: also frees the entry
	if o.server.PostHogClient == nil {
		return
	}
	wsID := o.projectWorkspaceID(ctx, run.ProjectID)
	firstRun := o.server.isFirstConvergenceRun(ctx, wsID, run.ProjectID, run.CreatedAt, run.ID)
	props := runCompletedProps(run, standing, wsID, tokens, firstRun)
	o.server.trackEvent(convergenceDistinctID(wsID, run.ProjectID), analytics.EventConvergenceRunCompleted, props)
}

// runCompletedProps builds the convergence_run_completed payload. Split out of
// the capture so the taxonomy — every value bucketed, nothing carrying content
// — is directly testable. tokens is the AI token usage the run's translation
// jobs reported; firstRun is the cold-start cohort marker.
func runCompletedProps(run *bstore.ConvergenceRun, standing *convergence.Standing, workspaceID string, tokens int, firstRun bool) map[string]any {
	viaMemory, viaAI := 0, 0
	for _, ls := range standing.Locales() {
		viaMemory += ls.ViaMemory
		viaAI += ls.ViaAI
	}
	duration := time.Duration(0)
	if run.FinishedAt != nil && !run.CreatedAt.IsZero() {
		duration = run.FinishedAt.Sub(run.CreatedAt)
	}
	props := analytics.Props(workspaceID, run.ProjectID)
	props["outcome"] = run.State
	props["stall_reason"] = run.StallReason
	props["passes"] = run.Passes
	props["via_tm"] = analytics.CountBucket(viaMemory)
	props["via_ai"] = analytics.CountBucket(viaAI)
	props["duration_bucket"] = analytics.DurationBucket(duration)
	// Source-first: bucket how many blocks the run held on source, so the
	// fleet-wide "how often does a run hold on unsettled source" question is
	// answerable alongside the credit/checks stalls (epic 019). Never content.
	props["blocked_on_source"] = analytics.CountBucket(run.BlockedOnSource)
	// Grant sizing: what the run ACTUALLY cost (the tokens its translation jobs
	// reported, priced at the platform's 1 token ≈ 1 credit rate) and how much of
	// its produced work TM covered for free. Realized spend is censored by the
	// balance — a run that ran out parks with needs_credits — so it is read
	// together with the uncensored demand on convergence_estimate_computed.
	props[analytics.PropConsumedCreditsBucket] = analytics.CreditBucket(billing.TokensToCredits(tokens))
	props[analytics.PropTMLeveragePctBucket] = analytics.SharePercentBucket(viaMemory, viaMemory+viaAI)
	props[analytics.PropFirstRun] = firstRun
	return props
}

// isFirstConvergenceRun reports whether no convergence run precedes the given
// one anywhere in the workspace — the cold-start cohort marker (`first_run`).
// Scope is the WORKSPACE, not the project: the question the one-time grant has
// to answer is "does a brand-new workspace's first project fit inside it", and
// a second project's first run is no longer cold-start. It degrades to project
// scope when the workspace is unknown (anonymous/self-hosted projects), and to
// false when the run store is unavailable — analytics never fail an operation.
//
// runID is excluded from the probe (the row usually already exists), so the
// answer is identical whether it is asked at start or at the terminal state.
func (s *Server) isFirstConvergenceRun(ctx context.Context, workspaceID, projectID string, at time.Time, runID string) bool {
	if s.ConvergenceRunStore == nil {
		return false
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	scope := []string{projectID}
	if workspaceID != "" {
		if ids := s.workspaceProjectIDs(ctx, workspaceID); len(ids) > 0 {
			scope = ids
		}
	}
	had, err := s.ConvergenceRunStore.HasRunBefore(ctx, scope, at, runID)
	if err != nil {
		slog.Debug("convergence: first-run probe failed", "project", projectID, "error", err)
		return false
	}
	return !had
}

// workspaceProjectIDs lists the workspace's project ids (nil when unknown) —
// the scope of the workspace-wide first-run probe. It reads the project list
// the loop rollup already reads; projects are few per workspace and the list is
// a single store call.
func (s *Server) workspaceProjectIDs(ctx context.Context, workspaceID string) []string {
	if s.ContentStore == nil || workspaceID == "" {
		return nil
	}
	projects, err := s.ContentStore.ListProjects(ctx)
	if err != nil {
		return nil
	}
	var ids []string
	for _, p := range projects {
		if p != nil && p.WorkspaceID == workspaceID {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// projectWorkspaceID best-effort resolves a project's workspace for analytics
// scope ("" when unknown, which the Props helper omits).
func (o *convergenceOrchestrator) projectWorkspaceID(ctx context.Context, projectID string) string {
	if o.server.ContentStore == nil {
		return ""
	}
	if proj, err := o.server.ContentStore.GetProject(ctx, projectID); err == nil && proj != nil {
		return proj.WorkspaceID
	}
	return ""
}

// workspaceSlugFor resolves a workspace id to the slug the activity feed and
// the notification preferences are keyed on. Empty when unresolvable — which
// only costs the entry its workspace filter, never the entry itself, because
// the recorder falls back to the workspace id.
func (o *convergenceOrchestrator) workspaceSlugFor(ctx context.Context, workspaceID string) string {
	if workspaceID == "" || o.server.AuthStore == nil {
		return ""
	}
	ws, err := o.server.AuthStore.GetWorkspace(ctx, workspaceID)
	if err != nil || ws == nil {
		return ""
	}
	return ws.Slug
}

// runOutcomeCounts folds a terminal run's per-locale standing into what the
// activity feed reports: the locales the run worked, in run order, and how many
// translations it produced across them.
func runOutcomeCounts(run *bstore.ConvergenceRun) (locales []string, produced int) {
	for _, ls := range run.Standing {
		if ls.Locale != "" {
			locales = append(locales, ls.Locale)
		}
		produced += ls.Produced
	}
	return locales, produced
}

// convergenceDistinctID picks the PostHog distinct-id for a run's system events
// (no user in scope): the workspace, falling back to the project — the same
// convention service-layer system events use.
func convergenceDistinctID(workspaceID, projectID string) string {
	if workspaceID != "" {
		return workspaceID
	}
	return projectID
}
