package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/spf13/cobra"
)

// formatPlanLine compresses the up-plan into the one-line header a run prints
// before executing — cost visibility before any tokens burn.
func formatPlanLine(plan UpPlanOutput) string {
	t := plan.Totals
	if t.MissingTarget == 0 {
		return "plan: every unit has a committed target — verifying gates"
	}
	line := fmt.Sprintf("plan: %d unit(s) missing · %d exact-TM · %d AI · ≈%s tokens",
		t.MissingTarget, t.TMExact, t.AIRemaining, compactTokens(t.TokenEstimate))
	if plan.Provider != "" {
		if plan.Subscription {
			line += fmt.Sprintf(" · %s (your subscription)", plan.Provider)
		} else {
			line += " · " + plan.Provider
		}
	}
	return line
}

// compactTokens renders a token estimate at header scale (61k, 1.2M).
func compactTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dk", n/1_000)
	default:
		return strconv.Itoa(n)
	}
}

// NewUpCmd creates `kapi up`: reconcile the project toward its ship gates.
// It is the porcelain home of convergence (issue #1078 C1): the recipe is the
// desired state, and `up` runs the project's default flow over all content
// across every target language, looping until every gated scope ships or is
// parked for a human. It reuses the same engine as the no-argument `kapi run`
// (runDefaultFlowConverge), with until-gate looping ON by default.
func (a *App) NewUpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "up",
		Short:   "Reconcile the project toward its ship gates (run the default flow until converged)",
		GroupID: "work",
		Args:    cobra.NoArgs,
		Long: `Reconcile the project toward its ship gates: treat the recipe as the desired
state and run the project's default flow (defaults.flow) over all content
across every target language, looping until every gated scope is shippable or
parked for a human.

Without defaults.flow, up runs the built-in default flow — TM reuse (recycle)
followed by AI translate — so a recipe needs no flow YAML at all to converge.
Setting defaults.flow replaces the built-in default.

Before each pass, up re-syncs the project block store with the working tree:
edited source files, a store written by another kapi version, or a missing
store trigger a re-extraction (the same shared path behind the desktop's
Re-extract). --no-extract opts out.

Each pass re-derives coverage from the working tree, runs the flow only for
the locales still short of their gate, and stops when everything ships, a pass
makes no progress (the remainder parks — it needs a human), or the pass cap is
reached. After each pass the project's bound checks run over what was
produced: a unit with failing findings (dropped placeholders, glossary
violations) counts as drafted, not translated, so it cannot lift its locale
over the gate until fixed. --no-checks opts out.

When the loop ends, the materialize policy decides whether localized files
are written from the project store: 'defaults.materialize: on-converge' (or
the --materialize flag) writes them for every locale whose gated scopes are
all shippable; the default ('manual') leaves that to 'kapi merge'.

--plan is a dry run: instead of running anything, up reports the pending work
per (collection, locale) — units missing a target, exact TM leverage, the
remaining AI work, and a rough token estimate — with no provider calls and no
writes. Combine with --json for agents.

up never fails the build on target drift: parked, pending target content is
normal toil, reported rather than thrown. Use 'kapi status' to inspect
standing without running anything, and 'kapi check --ship' to enforce the
gates (e.g. before a release tag).

--passes 1 runs a single pass (the behavior of the bare 'kapi run');
--passes N caps the loop at N passes.`,
		Example: `  kapi up                # loop the default flow until every gated scope ships or parks
  kapi up --plan         # dry run: pending work, TM leverage, and a token estimate per locale
  kapi up --passes 1     # a single pass over every locale that needs work
  kapi up --materialize  # also write localized files for the shippable locales
  kapi up -p app.kapi    # reconcile an explicit project recipe`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := ResolveProjectPath(cmd)
			if err != nil {
				return err
			}
			if projectPath == "" {
				return errors.New("kapi up needs a project — pass -p <recipe> or run from inside a kapi project directory")
			}
			return a.ExecuteUp(cmd, projectPath)
		},
	}

	AddProjectFlag(cmd)
	a.addFlowRunFlags(cmd)
	AddUpFlags(cmd)
	return cmd
}

// AddUpFlags registers the `kapi up` flag set. Exported so a plugin that owns
// the up verb in a server-connected install (kapi-bowrain) presents the exact
// same local surface and delegates to ExecuteUp for the local venue.
func AddUpFlags(cmd *cobra.Command) {
	cmd.Flags().Int("passes", 0, "maximum reconciliation passes (0 = loop until converged or parked, capped at 5; 1 = single pass)")
	cmd.Flags().Int("jobs", 0, "how many languages to converge concurrently per pass (0 = the recipe's defaults.jobs, else 4)")
	cmd.Flags().Bool("no-extract", false, "skip the pre-pass source-drift check and block-store re-extraction")
	cmd.Flags().Bool("no-checks", false, "skip the bound checks in the loop (produced units count as translated even when failing guardrails)")
	cmd.Flags().Bool("materialize", false, "after the loop, write localized files from the project store for every shippable locale (forces defaults.materialize: on-converge)")
	cmd.Flags().Bool("plan", false, "dry run: report pending work, TM leverage, and a token estimate per (collection, locale) — no provider calls, no writes")
	cmd.Flags().Bool("json", false, "output the structured result as JSON")
}

// ExecuteUp is the local-venue `kapi up` execution behind the command: load
// the project, honor --plan, ensure an AI provider, then run the convergence
// loop with the live UX (plan-first header, per-locale TTY progress, NDJSON
// under --json). Exported so the kapi-bowrain plugin's `up` — which owns the
// verb in a connected install and adds the server venue — can delegate the
// local venue here and stay byte-identical with the built-in behavior.
func (a *App) ExecuteUp(cmd *cobra.Command, projectPath string) error {
	proj, perr := a.LoadProjectInteractive(cmd.Context(), projectPath, LoadProjectInteractiveOptions{AssumeYes: a.AssumeYes})
	if perr != nil {
		return fmt.Errorf("load project: %w", perr)
	}
	a.InitRegistries()

	if boolFlag(cmd, "plan") {
		return a.runUpPlan(cmd, proj, projectPath)
	}

	// First-run onboarding: a converge run needs an AI provider; when
	// none is configured anywhere and this is a terminal, walk through
	// the compact provider wizard inline, then continue. Non-TTY runs
	// keep the existing keys-only error path.
	if err := a.EnsureAIProviderInteractive(cmd); err != nil {
		return err
	}

	passes, _ := cmd.Flags().GetInt("passes")
	if passes < 0 {
		return fmt.Errorf("--passes must be >= 0 (0 = loop until converged), got %d", passes)
	}
	// --passes 1 is a single pass (no loop); 0 loops to the default cap;
	// N > 1 loops with N as the cap.
	untilGate := passes != 1
	maxPasses := passes
	if maxPasses == 0 {
		maxPasses = convergeMaxPassesDefault
	}
	jobs, _ := cmd.Flags().GetInt("jobs")
	if jobs <= 0 {
		jobs = proj.Defaults.Jobs
	}

	// The run is live by default: --json streams the convergence
	// events as NDJSON (one event per line, a final result record) for
	// agents and CI; otherwise a renderer paints per-locale progress
	// on stderr — in place on a TTY, line-per-event on plain streams.
	// --quiet keeps today's summary-only behavior.
	jsonOut := boolFlag(cmd, "json")
	var onEvent func(convergence.Event)
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		onEvent = func(ev convergence.Event) { _ = enc.Encode(ev) }
	} else if !a.Quiet {
		// Plan-first: one line of scope before any tokens burn (the
		// full dry-run table stays under --plan).
		if !cmd.Flags().Changed("source-lang") && proj.Defaults.SourceLanguage != "" {
			a.SourceLang = string(proj.Defaults.SourceLanguage)
		}
		if plan, perr := a.computeProjectPlan(cmd.Context(), proj, projectPath); perr == nil {
			fmt.Fprintln(cmd.ErrOrStderr(), formatPlanLine(plan))
		}
		renderer := newConvergeRenderer(cmd.ErrOrStderr(), isatty.IsTerminal(os.Stderr.Fd()))
		onEvent = renderer.OnEvent
	}

	var result ConvergeOutput
	if err := a.runDefaultFlowConverge(cmd, proj, projectPath, convergeOptions{
		untilGate:   untilGate,
		maxPasses:   maxPasses,
		noExtract:   boolFlag(cmd, "no-extract"),
		noChecks:    boolFlag(cmd, "no-checks"),
		materialize: boolFlag(cmd, "materialize"),
		jobs:        jobs,
		onEvent:     onEvent,
		capture:     &result,
	}); err != nil {
		return err
	}
	return PrintUpResult(cmd, result)
}

// PrintUpResult renders a convergence result the way `kapi up` does: the
// text summary, or — under --json — the NDJSON stream's closing record (the
// structured result, flat, discriminated like every other line). Exported for
// the plugin's up verb, whose remote venue produces the same ConvergeOutput.
func PrintUpResult(cmd *cobra.Command, result ConvergeOutput) error {
	if boolFlag(cmd, "json") {
		enc := json.NewEncoder(cmd.OutOrStdout())
		return enc.Encode(struct {
			Type string `json:"type"`
			ConvergeOutput
		}{Type: "result", ConvergeOutput: result})
	}
	return result.FormatText(cmd.OutOrStdout())
}
