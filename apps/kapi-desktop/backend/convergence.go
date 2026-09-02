package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// errProjectFilesMissing is the typed, user-facing error GetConvergence /
// GetConvergePlan return when the open tab's recipe no longer exists on disk
// (the directory was moved or deleted — e.g. mid sample reset). The frontend
// renders it as a quiet state instead of raw open-errors; the underlying
// ENOENT is logged once per disappearance, not per poll.
var errProjectFilesMissing = errors.New("project files are missing or moved. Reopen the project")

// checkProjectOnDisk verifies the tab's recipe still exists before a derive.
// Missing → errProjectFilesMissing (logged once until the file reappears).
func (a *App) checkProjectOnDisk(op *openProject) error {
	if op.Path == "" {
		return nil
	}
	if _, err := os.Stat(op.Path); err != nil {
		if op.missingWarned.CompareAndSwap(false, true) {
			a.logger.Printf("project tab %s: recipe unreadable at %s (%v), reporting missing/moved to the UI", op.ID, op.Path, err)
		}
		return errProjectFilesMissing
	}
	op.missingWarned.Store(false)
	return nil
}

// GetConvergence returns the derived convergence report for a project tab: the
// per-(collection, locale) target coverage and ship-gate standing, the source
// authoring readiness, and the review queue. It is the same file-based
// derivation `kapi status`, `kapi status --review`, and `kapi check --ship`
// report, computed in-process so the desktop and the CLI agree to the unit.
//
// A project with no recipe path yet (unsaved) returns an empty report rather
// than an error, so the panel renders a "nothing tracked yet" state.
func (a *App) GetConvergence(tabID string) (*host.ConvergenceReport, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return &host.ConvergenceReport{}, nil
	}
	if err := a.checkProjectOnDisk(op); err != nil {
		return nil, err
	}
	src := string(op.Project.Defaults.SourceLanguage)
	return a.hostEngine().ProjectConvergence(context.Background(), op.Path, src)
}

// ConvergePlan is the desktop's pre-flight picture for "Bring up to date": the
// dry-run work plan `kapi up --plan` computes (per (collection, locale):
// missing targets, exact content-memory leverage, remaining AI work, token estimate) plus
// the block-store drift the run's auto-extract would heal. Both derivations
// are cheap and read-only — stat checks, file reads, no provider calls.
type ConvergePlan struct {
	Plan host.UpPlanOutput `json:"plan"`
	// ChangedFiles / RemovedFiles count the source files whose bytes drifted
	// from (or vanished since) the last extraction's stamps.
	ChangedFiles int `json:"changedFiles"`
	RemovedFiles int `json:"removedFiles"`
	// StoreMissing: the project has never been extracted (or the cache was
	// cleared); VersionStale: the store was written by another kapi version.
	StoreMissing bool `json:"storeMissing"`
	VersionStale bool `json:"versionStale"`
}

// GetConvergePlan returns the pre-flight convergence plan for a project tab:
// the same dry-run derivation as `kapi up --plan` (through the shared host.App
// path), plus the store drift summary the home hero renders. Read-only; safe
// to call on every home load/refresh.
func (a *App) GetConvergePlan(tabID string) (*ConvergePlan, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return &ConvergePlan{}, nil
	}
	if err := a.checkProjectOnDisk(op); err != nil {
		return nil, err
	}
	src := string(op.Project.Defaults.SourceLanguage)
	plan, err := a.hostEngine().UpPlan(context.Background(), op.Path, src)
	if err != nil {
		return nil, err
	}
	out := &ConvergePlan{Plan: *plan}

	// Drift summary via the shared core detection (`kapi up`'s pre-pass check).
	// A resolution failure is propagated, not skipped: DetectStoreDrift over a
	// short (or empty) file list reports "0 changed, 0 removed", which the plan
	// panel renders as "nothing has drifted" — the wrong number, not an error.
	// Strictness costs nothing here: UpPlan above resolves the same recipe
	// through UnitsFromProject and has already hard-failed on it.
	//
	// A project with no store yet has drifted from nothing, and reporting that
	// must not create one: a plan is a dry run, and `kapi up --plan` declines the
	// same open for the same reason.
	db, ok := a.existingProjectStore(op)
	if !ok {
		out.StoreMissing = true
		return out, nil
	}
	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, rerr := pctx.ResolveContent(a.formatReg)
	if rerr != nil {
		return nil, rerr
	}
	drift := db.DetectStoreDrift(context.Background(), resolved)
	out.ChangedFiles = len(drift.Changed)
	out.RemovedFiles = len(drift.Removed)
	out.StoreMissing = drift.StoreMissing
	out.VersionStale = drift.VersionStale
	return out, nil
}

// BringUpToDate reconciles the project toward its ship gates through the same
// dispatch as the CLI's `kapi up` and the MCP `up` tool (host.App.RunUpDispatch
// → host.App.ResolveUpVenue): a project whose recipe binds a convergence venue
// runs on the server — pushed, converged on the org's keys against the shared
// content memory and terminology, streamed back, pulled — and every other
// project runs the loop here, loop-to-gate over the project's default flow,
// auto-extract on block-store drift before each pass, bound checks in the loop,
// and the recipe's materialize policy.
//
// It returns once the run is launched; per-pass progress streams through the
// run-event channel as typed "converge_event" events and the final structured
// result rides the "complete" event, so the runner renders passes and locale
// rows rather than raw flow logs, at either venue. Whatever the loop can't carry
// to the ship gate parks for review (never an error).
func (a *App) BringUpToDate(tabID string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil {
		return errors.New("project has no recipe loaded")
	}
	if op.Path == "" {
		return errors.New("project has no file path; save it before bringing it up to date")
	}
	// No defaults.flow is fine: the shared up engine synthesizes the built-in
	// default flow (#1078 G6 — content memory reuse then AI translate). The label only
	// tags the run's events for the UI.
	flowName := op.Project.Defaults.Flow
	if flowName == "" {
		flowName = "up"
	}
	if len(op.Project.Defaults.TargetLanguages) == 0 {
		return errors.New("no target languages configured (defaults.target_languages)")
	}

	if a.runState == nil {
		a.runState = newRunner()
	}
	a.runState.mu.Lock()
	if a.runState.running {
		a.runState.mu.Unlock()
		return errors.New("a flow is already running. Cancel it first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.runState.state = RunStateRunning
	a.runState.cancel = cancel
	a.runState.running = true
	a.runState.events = nil
	a.runState.mu.Unlock()

	src := string(op.Project.Defaults.SourceLanguage)
	go a.executeConvergeRun(ctx, tabID, op.Path, flowName, src)
	return nil
}

// executeConvergeRun drives the shared up dispatch on a run-scoped host.App and
// translates its progress into run events.
//
// The App is its own so the per-run state is: TargetLang, the tool slots each
// locale worker fills, the progress tap. A concurrent GetConvergence on the
// engine therefore cannot race it. What it does NOT own is the project store —
// it borrows the engine's, so the run, the tab's status panel and the review
// loop all write through one pool and the in-process write gate can order them.
// Two pools on `.kapi/work/store.db` would leave the gate with nothing to gate, and a
// review decision recorded while a pass was learning wording would go back to
// losing on SQLite's busy backoff. It borrows the engine's plugin host too, so
// the venue route it resolves is the one the tab surfaces see.
//
// Borrowed, therefore never Shut down: the stores belong to the engine.
//
// OnEvent carries a server run's events as well as a local one's: the venue
// plumbing writes the same typed events to its stdout as NDJSON, and the
// dispatch decodes them line by line into this callback. The run view renders
// one stream and never asks which venue produced it.
func (a *App) executeConvergeRun(ctx context.Context, tabID, projectPath, flowName, sourceLang string) {
	defer func() {
		a.runState.mu.Lock()
		a.runState.running = false
		a.runState.mu.Unlock()
	}()

	start := time.Now()
	a.emitRunEvent(RunEvent{Type: "state", FlowID: flowName, Message: "running"})

	// Every model call this run makes is attributed to the run in the session's
	// AI activity log. A run's calls carry no unit address: the loop translates
	// in batches, so the exchange itself is where the content is.
	ctx = withAIScope(ctx, AIActivityScope{Surface: "convergence", Action: flowName})

	// Share the desktop's AI defaulting + credential resolution with the run's
	// registries, exactly like the CLI's Init does — so the built-in default
	// flow's translate step resolves the configured ai.provider/ai.model and
	// its saved key with no per-flow pinning.
	capp := a.borrowEngine(&host.App{Credentials: a.credentials, Config: a.aiConfig})
	capp.InitRegistries()
	// The same preprocessor the desktop's own registry carries — one definition,
	// so a converge run resolves provider/model/credentials identically to a
	// plain flow run and to the CLI.
	capp.ToolReg.SetConfigPreprocessor(a.aiConfigPreprocessor())
	out, err := capp.RunUpDispatch(ctx, projectPath, sourceLang, host.UpOptions{
		UntilGate: true,
		// OnEvent is the stream the live run view renders from: one typed event
		// per pass/locale transition. Log events degrade to the existing
		// progress lines (auto-extract notes) so the generic job feed and
		// reconnect path keep working; every other event rides as a typed
		// "converge_event" the frontend folds into per-locale rows.
		OnEvent: func(ev convergence.Event) {
			if ev.Type == convergence.EventLog {
				if msg := strings.TrimSpace(ev.Message); msg != "" {
					a.emitRunEvent(RunEvent{Type: "progress", FlowID: flowName, Message: msg})
				}
				return
			}
			e := ev
			a.emitRunEvent(RunEvent{Type: "converge_event", FlowID: flowName, ConvergeEvent: &e})
		},
		LogWriter: &runEventWriter{app: a, flowID: flowName},
	})
	if err != nil {
		// A cancelled run is a terminal "canceled", not an error: the message
		// keeps the "context canceled" marker the frontend's job feed maps to
		// its cancelled state (mirroring a plain flow-run cancel).
		if errors.Is(err, context.Canceled) {
			a.emitRunEvent(RunEvent{Type: "error", FlowID: flowName, Message: "run canceled (context canceled)"})
			a.runState.mu.Lock()
			if a.runState.state == RunStateRunning {
				a.runState.state = RunStateCanceled
			}
			a.runState.mu.Unlock()
			// A cancelled run still wrote whatever it finished before the stop,
			// so the derived state must be recomputed exactly as on the success
			// path — see the note below.
			a.recordRunFailure(classifyRunError(err))
			a.emitEvent("project:extracted", map[string]any{"tabID": tabID})
			a.emitEvent("outputs-changed", map[string]any{})
			return
		}
		a.emitRunEvent(RunEvent{
			Type:    "error",
			FlowID:  flowName,
			Message: err.Error(),
			Error:   classifyRunError(err),
		})
		a.runState.mu.Lock()
		if a.runState.state == RunStateRunning {
			a.runState.state = RunStateError
		}
		a.runState.mu.Unlock()
		// A partly-failed run has already materialized the passes that DID
		// succeed (the engine writes files as it goes), so the on-disk state has
		// moved while the UI's derived state has not. Without these two events
		// the home surfaces keep showing the pre-run numbers — which, for a run
		// that failed on its very first locale, reads as "up to date". Re-derive
		// on failure exactly as on success; recordRunFailure then makes the
		// failure itself visible rather than leaving the UI to infer it.
		a.recordRunFailure(classifyRunError(err))
		a.emitEvent("project:extracted", map[string]any{"tabID": tabID})
		a.emitEvent("outputs-changed", map[string]any{})
		return
	}

	a.runState.mu.Lock()
	if a.runState.state == RunStateRunning {
		a.runState.state = RunStateComplete
	}
	a.runState.mu.Unlock()
	a.recordRunFailure(nil)

	parked := len(out.ParkedScopes)
	msg := fmt.Sprintf("Up to date in %d pass(es) in %s", out.Passes, time.Since(start).Round(time.Millisecond))
	if !out.Converged {
		msg = fmt.Sprintf("Finished %d pass(es) in %s: %d scope(s) parked for review",
			out.Passes, time.Since(start).Round(time.Millisecond), parked)
	}
	a.emitRunEvent(RunEvent{
		Type:           "complete",
		FlowID:         flowName,
		DurationMs:     time.Since(start).Milliseconds(),
		Message:        msg,
		ConvergeResult: out,
	})
	// The run may have re-extracted and materialized files — nudge the home
	// surfaces to re-derive coverage and refresh their file tables.
	a.emitEvent("project:extracted", map[string]any{"tabID": tabID})
	a.emitEvent("outputs-changed", map[string]any{})
}

// runEventWriter forwards the up engine's human-readable log lines (the
// auto-extract notes) into the run-event stream as trace lines.
type runEventWriter struct {
	app    *App
	flowID string
	buf    bytes.Buffer
}

func (w *runEventWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Partial line — keep it buffered for the next write.
			w.buf.WriteString(line)
			break
		}
		if trimmed := string(bytes.TrimSpace([]byte(line))); trimmed != "" {
			w.app.emitRunEvent(RunEvent{Type: "progress", FlowID: w.flowID, Message: trimmed})
		}
	}
	return len(p), nil
}

// ApproveReviewItem promotes one review-queue unit to `reviewed`: it records an
// `approved` decision in the project state store, bound to the content hash of
// the translation it blesses, through the shared host.ApplyReviewDecision path.
// After it returns, GetConvergence shows the unit reviewed and it leaves the
// queue. The unit is addressed by (locale, file, key) as listed in the review
// queue.
func (a *App) ApproveReviewItem(tabID, locale, file, key string) error {
	return a.applyReviewDecision(tabID, locale, file, key, host.ReviewDecisionApproved, "")
}
