package host

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"

	"github.com/neokapi/neokapi/host/flowdef"
)

// RunCmdOptions configures the run command.
type RunCmdOptions struct {
	// FallbackRunE is called when the flow name doesn't match a built-in flow.
	// Used by bowrain CLI for project flows from .bowrain/flows/.
	FallbackRunE func(cmd Command, flowName string, args []string) error
}

// BuiltinComposedFlowNames returns the set of composed (multi-tool) flow names.
// Derived from flowdef.BuiltInFlows() rather than hardcoded.
func BuiltinComposedFlowNames() map[string]bool {
	names := make(map[string]bool)
	for _, fi := range builtinComposedFlows() {
		names[fi.Name] = true
	}
	return names
}

// BuiltinFlowNames returns every built-in catalog flow ID, single-node flows
// included — the resolution set for `kapi run <flow>` and the flow-backed
// porcelain. (BuiltinComposedFlowNames stays the ≥2-tool subset used by
// listings that present "composed" pipelines.)
func BuiltinFlowNames() map[string]bool {
	names := make(map[string]bool)
	for _, def := range flowdef.BuiltInFlows() {
		names[def.ID] = true
	}
	return names
}

// ResolveFallbackRunE returns the fallback function configured on the
// command, or — if none was set explicitly — the App-level FallbackRunE
// installed by plugins via RegisterAppInitializer. Read at RunE time so
// plugin initializers (which fire during PersistentPreRun) have already
// run.
func (a *App) ResolveFallbackRunE(opts RunCmdOptions) func(cmd Command, flowName string, args []string) error {
	if opts.FallbackRunE != nil {
		return opts.FallbackRunE
	}
	return a.FallbackRunE
}

// RunFromProject loads a .kapi project file and runs the named flow.
// Project settings (source/target language, content patterns) are used as
// defaults; CLI flags override everything.
func (a *App) RunFromProject(cmd Command, flowName, projectPath string, opts RunCmdOptions) error {
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
	if BuiltinFlowNames()[flowName] {
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
	if len(inputPaths) > 0 {
		expanded, ferr := resolveFiles(inputPaths)
		if ferr != nil {
			return ferr
		}
		inputPaths = expanded
	}

	// Resolve content patterns if no --input flag was provided. The resolved
	// set is passed explicitly to runProjectStepsOver below (re-reading the
	// flag there would silently drop the content-derived inputs and run over
	// zero files).
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
	a.ProjectContext = ctx
	defer func() { a.ProjectContext = nil }()

	// Resolve standing brand-voice + glossary bindings so project-flow steps
	// honor them with no flags (defaults.brand_voice / defaults.termbase).
	bindings, err := a.resolveProjectBindings(cmd, proj, projectPath)
	if err != nil {
		return err
	}
	a.ProjectBindings = bindings
	defer func() { a.ProjectBindings = nil }()

	// Build resource context from project file location.
	absProjectPath, _ := filepath.Abs(projectPath)
	rCtx := flow.ResourceContext{
		ProjectDir:   filepath.Dir(absProjectPath),
		SourceLocale: a.SourceLang,
		TargetLocale: a.TargetLang,
	}

	// Locale selection: an explicit --target-lang wins (one pass with it);
	// otherwise the flow's locale passes come from flow.ResolveFlowLocales —
	// the SAME applicability-based selection the desktop runner and the
	// shared orchestrator (RunFlowAllLocales) use, so the CLI and the desktop
	// can never disagree about which locales a flow runs for. (Convergence
	// deliberately answers a different question — "which locales still need
	// work" — via localesNeedingPass in converge.go.) A source-only flow
	// (nil passes) keeps the single default-target run.
	if !cmd.Flags().Changed("target-lang") {
		passes := flow.ResolveFlowLocales(spec, flow.BuildToolInfoMap(a.ToolReg), a.SourceLang, localeStrings(ctx.TargetLocales))
		if len(passes) > 0 {
			savedTarget := a.TargetLang
			defer func() { a.TargetLang = savedTarget }()
			for _, pass := range passes {
				// Target locale is the second element of the pass (if
				// present). A one-element multilingual pass (no project
				// targets) leaves it empty so runProjectStepsOver returns
				// its "--target-lang is required" error instead of the
				// run silently doing nothing.
				lang := ""
				if len(pass) > 1 {
					lang = pass[1]
				}
				a.TargetLang = lang
				rCtx.TargetLocale = lang
				if err := a.runProjectStepsOver(cmd.Context(), cmd, flowName, spec, &rCtx, inputPaths); err != nil {
					return err
				}
			}
			return nil
		}
	}

	return a.runProjectStepsOver(cmd.Context(), cmd, flowName, spec, &rCtx, inputPaths)
}
