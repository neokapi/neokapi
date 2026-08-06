package host

import (
	"context"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/project"
)

// UpOptions are the knobs of an embedded convergence run (RunUp) — the same
// levers `kapi up` exposes as flags, plus the structured hooks an embedding
// UI listens on.
type UpOptions struct {
	// UntilGate loops the pass until every gated scope ships or a pass stalls
	// (the `kapi up` default); false runs a single pass.
	UntilGate bool
	// MaxPasses caps the loop; <= 0 uses the `kapi up` default cap.
	MaxPasses int
	// NoExtract skips the pre-pass block-store drift check + auto-extract.
	NoExtract bool
	// NoChecks skips the project's bound checks in the loop.
	NoChecks bool
	// Materialize forces the post-loop materialize step regardless of the
	// recipe's defaults.materialize policy.
	Materialize bool
	// Jobs is how many locales one pass runs concurrently; <= 0 uses the
	// `kapi up` default (4).
	Jobs int
	// OnEvent receives the run's convergence.Event stream — the one protocol
	// every surface renders a run from. Events arrive one at a time.
	OnEvent func(convergence.Event)
	// LogWriter receives the run's human-readable log lines (auto-extract
	// notes, per-step output). Discarded when nil.
	LogWriter io.Writer
}

// RunUp is the embedded `kapi up`: it runs the exact convergence engine behind
// the CLI's `up` / no-argument `run` — loop-to-gate over the project's default
// flow, auto-extract on block-store drift before each pass, bound checks in
// the loop, and the recipe's materialize policy — and returns the structured
// result instead of printing it. The desktop's "Bring up to date" binds here
// so the two surfaces share one code path and agree to the unit.
//
// sourceLang overrides the project's source language when non-empty. Like the
// CLI, RunUp never fails on target drift: parked work is reported in the
// result, not returned as an error.
func (a *App) RunUp(ctx context.Context, projectPath, sourceLang string, opts UpOptions) (*ConvergeOutput, error) {
	a.InitRegistries()
	ctx = ctxOrBackground(ctx)
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	if sourceLang != "" {
		a.SourceLang = sourceLang
	}

	logW := opts.LogWriter
	if logW == nil {
		logW = io.Discard
	}
	// The engine speaks cobra (flags, context, output streams); an embedded
	// run drives it through a synthetic command bound to this project. The
	// explicit project flag keeps every downstream resolution (terms,
	// voice profile) on THIS recipe rather than a cwd-relative discovery.
	cmd := NewEnvCommand(ctx, "up")
	AddProjectFlag(cmd)
	if err := cmd.Flags().Set(projectFlagName, projectPath); err != nil {
		return nil, err
	}
	cmd.SetOut(logW)
	cmd.SetErr(logW)

	maxPasses := opts.MaxPasses
	if maxPasses <= 0 {
		maxPasses = ConvergeMaxPassesDefault
	}

	var result ConvergeOutput
	err = a.RunDefaultFlowConverge(cmd, proj, projectPath, ConvergeOptions{
		UntilGate:   opts.UntilGate,
		MaxPasses:   maxPasses,
		noExtract:   opts.NoExtract,
		noChecks:    opts.NoChecks,
		materialize: opts.Materialize,
		jobs:        opts.Jobs,
		onEvent:     opts.OnEvent,
		capture:     &result,
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
