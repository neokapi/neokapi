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

Each pass re-derives coverage from the working tree, runs the flow only for
the locales still short of their gate, and stops when everything ships, a pass
makes no progress (the remainder parks — it needs a human), or the pass cap is
reached.

up never fails the build on target drift: parked, pending target content is
normal toil, reported rather than thrown. Use 'kapi status' to inspect
standing without running anything, and 'kapi check --ship' to enforce the
gates (e.g. before a release tag).

--passes 1 runs a single pass (the behavior of the bare 'kapi run');
--passes N caps the loop at N passes.`,
		Example: `  kapi up                # loop the default flow until every gated scope ships or parks
  kapi up --passes 1     # a single pass over every locale that needs work
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
	return cmd
}
