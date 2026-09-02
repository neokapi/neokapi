package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// NewRunCmd creates the "run" command for executing composed flows.
//
//	kapi run translate-qa -i file.xliff --target-lang fr
//	kapi run my-custom-flow -p kapi.yaml
func NewRunCmd(a *App, opts RunCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [flow-name] [flags]",
		Short: "Run a composed flow (a named multi-tool pipeline)",
		Long: `Run a composed flow that chains multiple tools together.

Flows are multi-tool pipelines. For single-tool operations, use the
tool directly (e.g. "kapi translate" instead of "kapi run translate").
To bring the whole project up to date, use 'kapi up': the kapi loop
lives there; run is the escape hatch for one named pipeline, one pass.

Built-in flows:
  translate-qa    Translate + quality check using AI/LLM

Custom flows can be defined in the kapi.yaml recipe or .kapi/flows/ as YAML files.

Use -p to run a flow from a kapi.yaml recipe:
  kapi run translate -p kapi.yaml`,
		Example: `  kapi run translate-qa -i app.xliff --target-lang fr
  kapi run translate-qa -i messages.json --target-lang de
  kapi run pseudo -p kapi.yaml         # a project-defined flow`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := ResolveProjectPath(cmd)
			if err != nil {
				return err
			}

			// run takes a flow name; the loop lives in `kapi up` (looped to
			// the gates, venue-aware). No bare-run fallback.
			if len(args) == 0 {
				return errors.New("kapi run needs a flow name (see 'kapi flows'). To bring the project up to date, use 'kapi up' ('kapi up --passes 1' for a single pass)")
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

			// Built-in catalog flow — run directly.
			if BuiltinFlowNames()[flowName] {
				return a.RunFlow(cmd.Context(), cmd, flowName, flowOpts)
			}

			// Try fallback (e.g. project flows from .bowrain/flows/).
			if fallbackRunE != nil {
				return fallbackRunE(cmd, flowName, args)
			}

			return fmt.Errorf("unknown flow: %q\nUse \"flows\" to list available flows, or execute a tool directly (\"kapi exec %s\")", flowName, flowName)
		},
	}

	AddProjectFlag(cmd)
	a.AddFlowRunFlags(cmd)
	AddProgressFlag(cmd)
	return cmd
}
