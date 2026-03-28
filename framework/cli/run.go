package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// RunCmdOptions configures the run command.
type RunCmdOptions struct {
	// FallbackRunE is called when the flow name doesn't match a built-in flow.
	// Used by bowrain CLI for project flows from .bowrain/flows/.
	FallbackRunE func(cmd *cobra.Command, flowName string, args []string) error
}

// builtinComposedFlowNames lists flows that are genuinely composed (multi-tool).
var builtinComposedFlowNames = map[string]bool{
	"ai-translate-qa": true,
}

// NewRunCmd creates the "run" command for executing composed flows.
//
//	kapi run ai-translate-qa -i file.xliff --target-lang fr
//	kapi run my-custom-flow -i file.json --target-lang de
func (a *App) NewRunCmd(opts RunCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [flow-name] [flags]",
		Short: "Run a composed flow (multi-tool pipeline)",
		Long: `Run a composed flow that chains multiple tools together.

Flows are multi-tool pipelines. For single-tool operations, use the
tool directly (e.g. "kapi ai-translate" instead of "kapi run ai-translate").

Built-in flows:
  ai-translate-qa    Translate + quality check using AI/LLM

Custom flows can be defined in .bowrain/flows/ as YAML files.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowName := args[0]
			flowOpts := FlowCmdOptions{
				FallbackRunE: opts.FallbackRunE,
			}

			// Built-in composed flow — run directly.
			if builtinComposedFlowNames[flowName] {
				return a.RunFlow(context.Background(), cmd, flowName, flowOpts)
			}

			// Try fallback (e.g. project flows from .bowrain/flows/).
			if opts.FallbackRunE != nil {
				return opts.FallbackRunE(cmd, flowName, args)
			}

			return fmt.Errorf("unknown flow: %q\nUse \"flows\" to list available flows, or run a tool directly (e.g. \"kapi %s\")", flowName, flowName)
		},
	}

	a.addFlowRunFlags(cmd)
	return cmd
}
