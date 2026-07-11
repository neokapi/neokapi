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
		Short: "Run a composed flow (a named multi-tool pipeline)",
		Long: `Run a composed flow that chains multiple tools together.

Flows are multi-tool pipelines. For single-tool operations, use the
tool directly (e.g. "kapi translate" instead of "kapi run translate").
To reconcile the whole project toward its ship gates, use 'kapi up' —
run is the escape hatch for one named pipeline, one pass.

Built-in flows:
  translate-qa    Translate + quality check using AI/LLM

Custom flows can be defined in .kapi project files or .bowrain/flows/ as YAML files.

Use -p to run a flow from a .kapi project file:
  kapi run translate -p myproject.kapi`,
		Example: `  kapi run translate-qa -i app.xliff --target-lang fr
  kapi run translate-qa -i messages.json --target-lang de
  kapi run pseudo -p myproject.kapi         # a project-defined flow`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := ResolveProjectPath(cmd)
			if err != nil {
				return err
			}

			// run takes a flow name; convergence lives in `kapi up` (looped to
			// the gates, venue-aware). No bare-run fallback.
			if len(args) == 0 {
				return errors.New("kapi run needs a flow name (see 'kapi flows') — to reconcile the project toward its gates, use 'kapi up' ('kapi up --passes 1' for a single pass)")
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
	AddProgressFlag(cmd)
	return cmd
}
