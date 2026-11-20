package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/convergence"
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
	// A folder opened without a recipe has none to lose; it is the folder
	// that has to be there.
	probe := op.Path
	if op.unsaved.Load() {
		probe = filepath.Dir(op.Path)
	}
	if _, err := os.Stat(probe); err != nil {
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
	if op.Project == nil || !op.onDisk() {
		return &host.ConvergenceReport{}, nil
	}
	if err := a.checkProjectOnDisk(op); err != nil {
		return nil, err
	}
	src := string(op.Project.Defaults.SourceLanguage)
	return a.newProjectEngine(op).ProjectConvergence(context.Background(), op.Path, src)
}

// ConvergePlan is the desktop's pre-flight picture for "Bring up to date": the
// dry-run work plan `kapi up --plan` computes (per (collection, locale):
// missing targets, exact content-memory leverage, remaining AI work, token
// estimate, and the block-store drift the run's extraction would heal), with
// the drift counted for the home hero. The plan is read-only: stat checks,
// file reads, no provider calls.
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
	// Recycled is what the last recycle pass in this tab filled from exact
	// content-memory matches, or nil when none has filled anything.
	Recycled *RecycledNote `json:"recycled,omitempty"`
}

// GetConvergePlan returns the pre-flight convergence plan for a project tab:
// the same dry-run derivation as `kapi up --plan` (through the shared host.App
// path), with the drift summary the home hero renders. Read-only; safe to call
// on every home load/refresh.
func (a *App) GetConvergePlan(tabID string) (*ConvergePlan, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || !op.onDisk() {
		return &ConvergePlan{}, nil
	}
	if err := a.checkProjectOnDisk(op); err != nil {
		return nil, err
	}
	src := string(op.Project.Defaults.SourceLanguage)
	plan, err := a.newProjectEngine(op).UpPlan(context.Background(), op.Path, src)
	if err != nil {
		return nil, err
	}
	// The store drift is the plan's own (host.UpPlanOutput.Drift), the figure
	// `kapi up --plan` prints, counted here for the home hero.
	return &ConvergePlan{
		Plan:         *plan,
		ChangedFiles: len(plan.Drift.Changed),
		RemovedFiles: len(plan.Drift.Removed),
		StoreMissing: plan.Drift.StoreMissing,
		VersionStale: plan.Drift.VersionStale,
		Recycled:     op.lastRecycled(),
	}, nil
}

// BringUpToDate starts convergence at the venue selected by the host.
// The run publishes progress and its result through flow:event.
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
	flowName := op.Project.Defaults.Flow
	if flowName == "" {
		flowName = "up"
	}
	if err := a.ensureRecipe(op); err != nil {
		return err
	}

	ctx, err := a.runState.start(tabID, "", nil)
	if err != nil {
		return err
	}

	src := string(op.Project.Defaults.SourceLanguage)
	go a.executeConvergeRun(ctx, op, op.Path, flowName, src)
	return nil
}

func (a *App) executeConvergeRun(ctx context.Context, op *openProject, projectPath, flowName, sourceLang string) {
	defer a.runState.release()

	start := time.Now()
	a.emitRunEvent(RunEvent{Type: "state", FlowID: flowName, Message: "running"})

	// Every model call this run makes is attributed to the run in the session's
	// AI activity log. A run's calls carry no unit address: the loop translates
	// in batches, so the exchange itself is where the content is.
	ctx = withAIScope(ctx, AIActivityScope{Surface: "convergence", Action: flowName})

	capp := a.newProjectEngine(op)
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
	event := RunEvent{Type: "complete", FlowID: flowName, DurationMs: time.Since(start).Milliseconds(), ConvergeResult: out}
	if err == nil {
		elapsed := time.Since(start).Round(time.Millisecond)
		switch {
		case out.Monolingual || out.NoTargets:
			event.Message = "Reconciled the source."
		case out.Converged:
			event.Message = fmt.Sprintf("Up to date in %d pass(es) in %s", out.Passes, elapsed)
		default:
			event.Message = fmt.Sprintf("Finished %d pass(es) in %s: %d scope(s) waiting for review", out.Passes, elapsed, len(out.ParkedScopes))
		}
	}
	a.finishRun(ctx, op, event, err)
	a.emitEvent("project:extracted", map[string]any{"tabID": op.ID})
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
