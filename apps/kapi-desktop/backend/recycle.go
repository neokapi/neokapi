package backend

import (
	"context"
	"time"

	"github.com/neokapi/neokapi/host"
)

// RecycledNote is what the last recycle pass in a project tab filled.
type RecycledNote struct {
	// Blocks counts the targets the pass filled from the content memory.
	Blocks int       `json:"blocks"`
	At     time.Time `json:"at"`
}

// recycleExactMatches runs host.RecycleExactMatches for a project tab and
// returns how many targets it filled. The host owns what the pass does and
// when it has nothing to do; the app decides when to ask (on opening a tab and
// after each extraction) and keeps the count for the Overview.
//
// It does nothing, and reports 0, for a folder without a saved recipe or with
// no target languages, and while a run holds the project, which it claims for
// the pass so a run cannot start beside it. The caller holds storeSyncMu.
func (a *App) recycleExactMatches(op *openProject) (int, error) {
	if op.Project == nil || op.Path == "" || op.unsaved.Load() || len(op.Project.Defaults.TargetLanguages) == 0 {
		return 0, nil
	}
	if a.runState == nil {
		a.runState = newRunner()
	}
	a.runState.mu.Lock()
	if a.runState.running {
		a.runState.mu.Unlock()
		return 0, nil
	}
	a.runState.running = true
	a.runState.mu.Unlock()
	defer func() {
		a.runState.mu.Lock()
		a.runState.running = false
		a.runState.mu.Unlock()
	}()

	// A run-scoped App over the engine's stores, as a convergence run uses:
	// the pass sets per-run state (the source language, the target locale)
	// that the tab's own reads must not see.
	capp := a.borrowEngine(&host.App{})
	res, err := capp.RecycleExactMatches(context.Background(), op.Path,
		string(op.Project.Defaults.SourceLanguage), host.RecycleOptions{NoExtract: true})
	if err != nil {
		return 0, err
	}
	if res.Blocks > 0 {
		op.recordRecycled(RecycledNote{Blocks: res.Blocks, At: time.Now()})
	}
	return res.Blocks, nil
}

// recordRecycled keeps what the last recycle pass filled, for the Overview.
func (op *openProject) recordRecycled(note RecycledNote) {
	op.mapMu.Lock()
	defer op.mapMu.Unlock()
	op.recycled = &note
}

// lastRecycled returns what the last recycle pass in this tab filled, or nil.
func (op *openProject) lastRecycled() *RecycledNote {
	op.mapMu.Lock()
	defer op.mapMu.Unlock()
	if op.recycled == nil {
		return nil
	}
	n := *op.recycled
	return &n
}
