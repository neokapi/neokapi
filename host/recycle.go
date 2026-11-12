package host

import (
	"context"
	"io"

	"github.com/neokapi/neokapi/core/convergence"
)

// RecycleOptions are the knobs of RecycleExactMatches.
type RecycleOptions struct {
	// NoExtract skips the pass's block-store drift check and auto-extract,
	// for a caller that has just brought the store current itself.
	NoExtract bool
	// LogWriter receives the pass's log lines. Discarded when nil.
	LogWriter io.Writer
}

// RecycleResult is what a recycle-only pass filled.
type RecycleResult struct {
	// Blocks counts the targets filled from exact content-memory matches,
	// over every locale.
	Blocks int `json:"blocks"`
	// ByLocale splits Blocks by target locale.
	ByLocale map[string]int `json:"by_locale,omitempty"`
}

// RecycleExactMatches fills every target the project's content memory
// answers exactly, through one recycle-only pass (RecycleOnlyFlowSpec), and
// calls no provider. Near matches and the work no match answers are left for
// a full convergence run. Target files are written under the recipe's
// materialize policy, as `kapi up` writes them.
//
// It runs nothing and returns a zero result when the plan counts no exact
// match, and for a project that converges on its server, whose content
// memory is the server's. Any surface may call it; the caller decides when.
func (a *App) RecycleExactMatches(ctx context.Context, projectPath, sourceLang string, opts RecycleOptions) (RecycleResult, error) {
	a.InitRegistries()
	ctx = ctxOrBackground(ctx)
	plan, err := a.UpPlan(ctx, projectPath, sourceLang)
	if err != nil || plan.Totals.MemoryExact == 0 {
		return RecycleResult{}, err
	}
	dec, err := a.ResolveUpVenue(projectPath, UpVenueOptions{})
	if err != nil || dec.Venue == UpVenueServer {
		return RecycleResult{}, err
	}

	res := RecycleResult{ByLocale: map[string]int{}}
	_, err = a.RunUp(ctx, projectPath, sourceLang, UpOptions{
		MaxPasses:   1,
		NoExtract:   opts.NoExtract,
		RecycleOnly: true,
		LogWriter:   opts.LogWriter,
		OnEvent: func(ev convergence.Event) {
			if ev.Type == convergence.EventLocaleDone && ev.ViaMemory > 0 {
				res.ByLocale[ev.Locale] += ev.ViaMemory
				res.Blocks += ev.ViaMemory
			}
		},
	})
	if err != nil {
		return RecycleResult{}, err
	}
	if res.Blocks == 0 {
		res.ByLocale = nil
	}
	return res, nil
}
