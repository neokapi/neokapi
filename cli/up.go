package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

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
			proj, perr := a.LoadProjectInteractive(cmd.Context(), projectPath, LoadProjectInteractiveOptions{AssumeYes: a.AssumeYes})
			if perr != nil {
				return fmt.Errorf("load project: %w", perr)
			}
			a.InitRegistries()

			if boolFlag(cmd, "plan") {
				return a.runUpPlan(cmd, proj, projectPath)
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
			return a.runDefaultFlowConverge(cmd, proj, projectPath, convergeOptions{
				untilGate:   untilGate,
				maxPasses:   maxPasses,
				noExtract:   boolFlag(cmd, "no-extract"),
				noChecks:    boolFlag(cmd, "no-checks"),
				materialize: boolFlag(cmd, "materialize"),
			})
		},
	}

	AddProjectFlag(cmd)
	a.addFlowRunFlags(cmd)
	cmd.Flags().Int("passes", 0, "maximum reconciliation passes (0 = loop until converged or parked, capped at 5; 1 = single pass)")
	cmd.Flags().Bool("no-extract", false, "skip the pre-pass source-drift check and block-store re-extraction")
	cmd.Flags().Bool("no-checks", false, "skip the bound checks in the loop (produced units count as translated even when failing guardrails)")
	cmd.Flags().Bool("materialize", false, "after the loop, write localized files from the project store for every shippable locale (forces defaults.materialize: on-converge)")
	cmd.Flags().Bool("plan", false, "dry run: report pending work, TM leverage, and a token estimate per (collection, locale) — no provider calls, no writes")
	cmd.Flags().Bool("json", false, "output the structured result as JSON")
	return cmd
}
