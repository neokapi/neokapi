package cli

import (
	"errors"
	"fmt"
	"os"
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

// builtinComposedFlowNames returns the set of composed (multi-tool) flow names.
// Derived from flow.BuiltInFlows() rather than hardcoded.
func builtinComposedFlowNames() map[string]bool {
	names := make(map[string]bool)
	for _, fi := range builtinComposedFlows() {
		names[fi.Name] = true
	}
	return names
}

// resolveFallbackRunE returns the fallback function configured on the
// command, or — if none was set explicitly — the App-level FallbackRunE
// installed by plugins via RegisterAppInitializer. Read at RunE time so
// plugin initializers (which fire during PersistentPreRun) have already
// run.
func (a *App) resolveFallbackRunE(opts RunCmdOptions) func(cmd *cobra.Command, flowName string, args []string) error {
	if opts.FallbackRunE != nil {
		return opts.FallbackRunE
	}
	return a.FallbackRunE
}

// NewRunCmd creates the "run" command for executing composed flows.
//
//	kapi run translate-qa -i file.xliff --target-lang fr
//	kapi run my-custom-flow -p project.kapi
func (a *App) NewRunCmd(opts RunCmdOptions) *cobra.Command {
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
				proj, perr := a.LoadProjectInteractive(cmd.Context(), projectPath, LoadProjectInteractiveOptions{AssumeYes: a.AssumeYes})
				if perr != nil {
					return fmt.Errorf("load project: %w", perr)
				}
				a.InitRegistries()
				untilGate, _ := cmd.Flags().GetBool("until-gate")
				maxPasses, _ := cmd.Flags().GetInt("max-passes")
				if maxPasses == 0 {
					maxPasses = convergeMaxPassesDefault
				}
				return a.runDefaultFlowConverge(cmd, proj, projectPath, untilGate, maxPasses)
			}

			flowName := args[0]

			fallbackRunE := a.resolveFallbackRunE(opts)

			// If a project file is specified (or auto-discovered), apply its defaults.
			if projectPath != "" {
				return a.runFromProject(cmd, flowName, projectPath, RunCmdOptions{
					FallbackRunE: fallbackRunE,
				})
			}

			flowOpts := FlowCmdOptions{
				FallbackRunE: fallbackRunE,
			}

			// Built-in composed flow — run directly.
			if builtinComposedFlowNames()[flowName] {
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
	a.addFlowRunFlags(cmd)
	cmd.Flags().Bool("until-gate", false, "loop the default flow until every gated scope is shippable (or a pass stalls); parks the rest")
	cmd.Flags().Int("max-passes", 0, "cap on --until-gate passes (default 5)")
	return cmd
}

// runFromProject loads a .kapi project file and runs the named flow.
// Project settings (source/target language, content patterns) are used as
// defaults; CLI flags override everything.
func (a *App) runFromProject(cmd *cobra.Command, flowName, projectPath string, opts RunCmdOptions) error {
	proj, err := a.LoadProjectInteractive(cmd.Context(), projectPath, LoadProjectInteractiveOptions{
		AssumeYes: a.AssumeYes,
	})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}

	// Check that declared plugin requirements are met.
	if status := project.CheckPlugins(proj, a.InstalledPluginList()); !status.Satisfied {
		for _, issue := range status.Issues {
			switch issue.Type {
			case "missing":
				fmt.Fprintf(os.Stderr, "Warning: plugin %q required by project but not installed\n", issue.Plugin)
			case "version_mismatch":
				fmt.Fprintf(os.Stderr, "Warning: plugin %q version mismatch — requires %s, installed %s\n",
					issue.Plugin, issue.Required, issue.InstalledVersion)
			}
		}
		return fmt.Errorf("project plugin requirements not met — install missing plugins or adjust version constraints in %s", projectPath)
	}

	// Create project context to resolve all defaults.
	ctx := project.NewProjectContext(proj, projectPath)

	// Apply project defaults where CLI flags weren't explicitly set.
	if !cmd.Flags().Changed("source-lang") && ctx.SourceLocale != "" {
		a.SourceLang = string(ctx.SourceLocale)
	}
	if !cmd.Flags().Changed("target-lang") && len(ctx.TargetLocales) > 0 {
		a.TargetLang = string(ctx.TargetLocales[0])
	}
	if !cmd.Flags().Changed("encoding") && ctx.Encoding != "" {
		a.Encoding = ctx.Encoding
	}

	// Check if it's a built-in flow first (project can reference built-in flows).
	if builtinComposedFlowNames()[flowName] {
		return a.RunFlow(cmd.Context(), cmd, flowName, FlowCmdOptions{
			FallbackRunE: opts.FallbackRunE,
		})
	}

	// Look up the flow in the project file.
	spec := proj.Flow(flowName)
	if spec == nil {
		// Try fallback (e.g. bowrain project flows).
		if opts.FallbackRunE != nil {
			return opts.FallbackRunE(cmd, flowName, []string{flowName})
		}
		return fmt.Errorf("flow %q not found in project file %s", flowName, projectPath)
	}

	inputPaths, _ := cmd.Flags().GetStringSlice("input")

	// Resolve content patterns if no --input flag was provided.
	if len(inputPaths) == 0 {
		resolved, err := ctx.ResolveContent(a.FormatReg)
		if err != nil {
			return fmt.Errorf("resolve content: %w", err)
		}
		for _, rf := range resolved {
			inputPaths = append(inputPaths, rf.Path)
		}
		if len(inputPaths) == 0 {
			return errors.New("no input files found — specify --input (-i) or add content patterns to the project file")
		}
	}

	// Store project context for downstream reader/writer configuration.
	a.projectContext = ctx
	defer func() { a.projectContext = nil }()

	// Resolve standing brand-voice + glossary bindings so project-flow steps
	// honor them with no flags (defaults.brand_voice / defaults.termbase).
	bindings, err := a.resolveProjectBindings(cmd, proj, projectPath)
	if err != nil {
		return err
	}
	a.projectBindings = bindings
	defer func() { a.projectBindings = nil }()

	// Build resource context from project file location.
	absProjectPath, _ := filepath.Abs(projectPath)
	rCtx := flow.ResourceContext{
		ProjectDir:   filepath.Dir(absProjectPath),
		SourceLocale: a.SourceLang,
		TargetLocale: a.TargetLang,
	}

	return a.runProjectSteps(cmd.Context(), cmd, flowName, spec, &rCtx)
}
