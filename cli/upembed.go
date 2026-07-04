package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
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
	// OnPass receives a structured snapshot after each pass.
	OnPass func(ConvergePassEvent)
	// OnPhase receives a coarse progress signal before each long-running stage
	// of a pass (content resolution, auto-extract, coverage derivation,
	// per-locale translation) — what an embedding UI shows while the first pass
	// is still deriving, instead of an indeterminate spinner.
	OnPhase func(ConvergePhaseEvent)
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
	if ctx == nil {
		ctx = context.Background()
	}
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
	// explicit project flag keeps every downstream resolution (termbase,
	// brand profile) on THIS recipe rather than a cwd-relative discovery.
	cmd := &cobra.Command{Use: "up"}
	AddProjectFlag(cmd)
	if err := cmd.Flags().Set(ProjectFlagName, projectPath); err != nil {
		return nil, err
	}
	cmd.SetContext(ctx)
	cmd.SetOut(logW)
	cmd.SetErr(logW)

	maxPasses := opts.MaxPasses
	if maxPasses <= 0 {
		maxPasses = convergeMaxPassesDefault
	}

	var result ConvergeOutput
	err = a.runDefaultFlowConverge(cmd, proj, projectPath, convergeOptions{
		untilGate:   opts.UntilGate,
		maxPasses:   maxPasses,
		noExtract:   opts.NoExtract,
		noChecks:    opts.NoChecks,
		materialize: opts.Materialize,
		onPass:      opts.OnPass,
		onPhase:     opts.OnPhase,
		capture:     &result,
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
