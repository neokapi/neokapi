package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// NewRunCmd creates the "run" command for executing composed flows.
//
//	kapi run translate-qa -i file.xliff --target-lang fr
//	kapi run my-custom-flow -p project.kapi
func NewRunCmd(a *App, opts RunCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [flow-name] [flags]",
		Short: "Run a composed flow, or converge the project's default flow",
		Long: `Run a composed flow that chains multiple tools together.

Flows are multi-tool pipelines. For single-tool operations, use the
tool directly (e.g. "kapi translate" instead of "kapi run translate").

With no flow name, kapi runs the project's default flow (defaults.flow) over
all content across every target language — bringing the project up to date in
one pass. Add --until-gate to loop that pass until every gated scope is
shippable (or a pass stalls), parking whatever still needs a human. Convergence
never fails the build: parked, drifted target content is normal toil, reported
rather than thrown.

The no-argument run's porcelain home is 'kapi up', which loops to the gates by
default; 'kapi run' keeps custom-flow semantics.

Built-in flows:
  translate-qa    Translate + quality check using AI/LLM

Custom flows can be defined in .kapi project files or .bowrain/flows/ as YAML files.

Use -p to run a flow from a .kapi project file:
  kapi run translate -p myproject.kapi`,
		Example: `  kapi run                                  # converge the project's default flow
  kapi run --until-gate                     # loop until every gated scope ships
  kapi run translate-qa -i app.xliff --target-lang fr
  kapi run translate-qa -i messages.json --target-lang de`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := ResolveProjectPath(cmd)
			if err != nil {
				return err
			}

			// No flow argument → convergence: run the project's default flow
			// (defaults.flow) over all content × target languages. Requires a
			// project; one pass by default, looped to the ship gate with --until-gate.
			if len(args) == 0 {
				if projectPath == "" {
					return errors.New("kapi run needs a flow name, or a project with a default flow (defaults.flow); none found")
				}
				// One-release pointer (#1078): the no-argument run has a porcelain
				// home in `kapi up`; run keeps custom-flow semantics.
				fmt.Fprintln(cmd.ErrOrStderr(), "note: `kapi up` is the new home of the no-argument run; `kapi run` keeps custom-flow semantics.")
				proj, perr := a.LoadProjectInteractive(cmd.Context(), projectPath, LoadProjectInteractiveOptions{AssumeYes: a.AssumeYes})
				if perr != nil {
					return fmt.Errorf("load project: %w", perr)
				}
				a.InitRegistries()
				untilGate, _ := cmd.Flags().GetBool("until-gate")
				maxPasses, _ := cmd.Flags().GetInt("max-passes")
				if maxPasses == 0 {
					maxPasses = ConvergeMaxPassesDefault
				}
				return a.RunDefaultFlowConverge(cmd, proj, projectPath, ConvergeOptions{
					UntilGate: untilGate,
					MaxPasses: maxPasses,
				})
			}

			flowName := args[0]

			fallbackRunE := a.ResolveFallbackRunE(opts)

			// If a project file is specified (or auto-discovered), apply its defaults.
			if projectPath != "" {
				return a.RunFromProject(cmd, flowName, projectPath, RunCmdOptions{
					FallbackRunE: fallbackRunE,
				})
			}

			flowOpts := FlowCmdOptions{
				FallbackRunE: fallbackRunE,
			}

			// Built-in composed flow — run directly.
			if BuiltinComposedFlowNames()[flowName] {
				return a.RunFlow(cmd.Context(), cmd, flowName, flowOpts)
			}

			// Try fallback (e.g. project flows from .bowrain/flows/).
			if fallbackRunE != nil {
				return fallbackRunE(cmd, flowName, args)
			}

			return fmt.Errorf("unknown flow: %q\nUse \"flows\" to list available flows, or run a tool directly (e.g. \"kapi %s\")", flowName, flowName)
		},
	}

	AddProjectFlag(cmd)
	a.AddFlowRunFlags(cmd)
	cmd.Flags().Bool("until-gate", false, "loop the default flow until every gated scope is shippable (or a pass stalls); parks the rest")
	cmd.Flags().Int("max-passes", 0, "cap on --until-gate passes (default 5)")
	return cmd
}
