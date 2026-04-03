package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
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
//	kapi run my-custom-flow -p project.kapi
func (a *App) NewRunCmd(opts RunCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [flow-name] [flags]",
		Short: "Run a composed flow (multi-tool pipeline)",
		Long: `Run a composed flow that chains multiple tools together.

Flows are multi-tool pipelines. For single-tool operations, use the
tool directly (e.g. "kapi ai-translate" instead of "kapi run ai-translate").

Built-in flows:
  ai-translate-qa    Translate + quality check using AI/LLM

Custom flows can be defined in .kapi project files or .bowrain/flows/ as YAML files.

Use -p to run a flow from a .kapi project file:
  kapi run translate -p myproject.kapi`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowName := args[0]
			projectPath, _ := cmd.Flags().GetString("project")

			// If a project file is specified, apply its defaults.
			if projectPath != "" {
				return a.runFromProject(cmd, flowName, projectPath, opts)
			}

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

	cmd.Flags().StringP("project", "p", "", "path to a .kapi project file")
	a.addFlowRunFlags(cmd)
	return cmd
}

// runFromProject loads a .kapi project file and runs the named flow.
// Project settings (source/target language, content patterns) are used as
// defaults; CLI flags override everything.
func (a *App) runFromProject(cmd *cobra.Command, flowName, projectPath string, opts RunCmdOptions) error {
	proj, err := project.Load(projectPath)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}

	// Apply project defaults where CLI flags weren't explicitly set.
	if !cmd.Flags().Changed("source-lang") && proj.SourceLanguage != "" {
		a.SourceLang = proj.SourceLanguage
	}
	if !cmd.Flags().Changed("target-lang") && len(proj.TargetLanguages) > 0 {
		a.TargetLang = proj.TargetLanguages[0]
	}

	// Check if it's a built-in flow first (project can reference built-in flows).
	if builtinComposedFlowNames[flowName] {
		return a.RunFlow(context.Background(), cmd, flowName, FlowCmdOptions{
			FallbackRunE: opts.FallbackRunE,
		})
	}

	// Look up the flow in the project file.
	spec := proj.GetFlow(flowName)
	if spec == nil {
		// Try fallback (e.g. bowrain project flows).
		if opts.FallbackRunE != nil {
			return opts.FallbackRunE(cmd, flowName, []string{flowName})
		}
		return fmt.Errorf("flow %q not found in project file %s", flowName, projectPath)
	}

	inputPaths, _ := cmd.Flags().GetStringSlice("input")
	if len(inputPaths) == 0 {
		return fmt.Errorf("--input (-i) is required (content pattern resolution not yet implemented)")
	}

	// Build resource context from project file location.
	absProjectPath, _ := filepath.Abs(projectPath)
	rCtx := flow.ResourceContext{
		ProjectDir:   filepath.Dir(absProjectPath),
		SourceLocale: a.SourceLang,
		TargetLocale: a.TargetLang,
	}

	return a.runProjectSteps(context.Background(), cmd, flowName, spec, &rCtx)
}
