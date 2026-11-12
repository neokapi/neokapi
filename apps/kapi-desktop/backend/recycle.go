package backend

import (
	"context"
	"time"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/host"
)

// RecycledNote is what the last recycle pass in a project tab filled.
type RecycledNote struct {
	// Blocks counts the targets the pass filled from the content memory.
	Blocks int       `json:"blocks"`
	At     time.Time `json:"at"`
}

// recycleExactMatches fills every target the content memory answers exactly,
// through a recycle-only convergence pass (host.RecycleOnlyFlowSpec). It
// returns how many targets the pass filled.
//
// An exact match is the project's own approved wording for that source, so
// the app applies it without asking, the way it extracts without asking. A
// near match, and everything the memory does not answer, waits for a run the
// person starts. The pass writes target files under the recipe's materialize
// policy, as `kapi up` does.
//
// It does nothing, and reports 0, when the plan counts no exact match, when a
// run already holds the project, for a folder without a saved recipe, and in
// a project that converges on its server, where the server's content memory
// is the one that answers. The caller holds storeSyncMu.
func (a *App) recycleExactMatches(op *openProject) (int, error) {
	if op.Project == nil || op.Path == "" || op.unsaved.Load() || len(op.Project.Defaults.TargetLanguages) == 0 {
		return 0, nil
	}
	ctx := context.Background()
	src := string(op.Project.Defaults.SourceLanguage)
	engine := a.hostEngine()
	plan, err := engine.UpPlan(ctx, op.Path, src)
	if err != nil || plan.Totals.MemoryExact == 0 {
		return 0, err
	}
	dec, err := engine.ResolveUpVenue(op.Path, host.UpVenueOptions{})
	if err != nil || dec.Venue == host.UpVenueServer {
		return 0, err
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

	filled := map[string]int{}
	capp := a.borrowEngine(&host.App{Credentials: a.credentials, Config: a.aiConfig})
	capp.InitRegistries()
	_, err = capp.RunUp(ctx, op.Path, src, host.UpOptions{
		MaxPasses:   1,
		NoExtract:   true,
		RecycleOnly: true,
		OnEvent: func(ev convergence.Event) {
			if ev.Type == convergence.EventLocaleDone {
				filled[ev.Locale] += ev.ViaMemory
			}
		},
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range filled {
		n += c
	}
	if n > 0 {
		op.recordRecycled(RecycledNote{Blocks: n, At: time.Now()})
	}
	return n, nil
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
