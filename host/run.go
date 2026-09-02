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
				fmt.Fprintf(os.Stderr, "Warning: plugin %q version mismatch: requires %s, installed %s\n",
					issue.Plugin, issue.Required, issue.InstalledVersion)
			}
		}
		return fmt.Errorf("project plugin requirements not met. Install missing plugins or adjust version constraints in %s", projectPath)
	}

	// Create project context to resolve all defaults.
	ctx := project.NewProjectContext(proj, projectPath)
	// Set here, above the built-in-flow return below, because that path leaves
	// this function directly. Downstream reader/writer configuration resolves
	// each file's declared format through this context; without it a
	// project-bound built-in flow falls back to extension detection and
	// silently ignores the recipe's `format:` binding.
	a.ProjectContext = ctx
	defer func() { a.ProjectContext = nil }()

	// Apply project defaults where CLI flags weren't explicitly set.
	a.ResolveSourceLang(ctx.SourceLocale)
	a.ResolveEncoding(ctx.Encoding)
	if !cmd.Flags().Changed("target-lang") && len(ctx.TargetLocales) > 0 {
		a.TargetLang = string(ctx.TargetLocales[0])
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
			return errors.New("no input files found. Specify --input (-i) or add content patterns to the project file")
		}
	}

	// --explain is a plan, never a run: render the resolved bindings and
	// locale passes and return before any tool is built, store or content memory opened,
	// or file written. The built-in path gates this inside RunFlow; without
	// this gate the project-flow path fell through to execution and wrote
	// outputs project-wide (#1295).
	if explain, _ := cmd.Flags().GetBool("explain"); explain {
		outputFlag, _ := cmd.Flags().GetString("output")
		locales := []string{a.TargetLang}
		if !cmd.Flags().Changed("target-lang") {
			if passes := flow.ResolveFlowLocales(spec, flow.BuildToolInfoMap(a.ToolReg), a.SourceLocale(), localeStrings(ctx.TargetLocales)); len(passes) > 0 {
				locales = locales[:0]
				for _, pass := range passes {
					if len(pass) > 1 {
						locales = append(locales, pass[1])
					}
				}
			}
		}
		return explainProjectFlowRun(cmd.OutOrStdout(), flowName, inputPaths, outputFlag, locales)
	}

	// This run resolves the recipe's coordinates; a run at the server venue
	// would not, until they are synced there.
	a.WarnUnsyncedCoordinates(cmd.ErrOrStderr(), proj)

	// Resolve standing voice + terminology bindings so project-flow steps
	// honor them with no flags (defaults.voice / defaults.terms_source), per
	// content collection: the input set splits into one group per distinct
	// binding, and each group runs the flow with its own. A recipe where no
	// collection overrides anything yields one group over every input — the
	// single run, with the single tool chain, that this has always been.
	groups, err := a.groupInputsByBinding(cmd, proj, ctx.ProjectDir, inputPaths)
	if err != nil {
		return err
	}
	// Resolved per (group, locale) inside the pass below: the term rules a
	// binding set carries are the wording approved for one target locale, and a
	// flow whose locales come from the recipe runs several of them.
	groupBindings := a.newLocaleBindings(cmd, proj, projectPath)
	defer func() { a.ProjectBindings = nil }()

	// Build resource context from project file location.
	absProjectPath, _ := filepath.Abs(projectPath)
	rCtx := flow.ResourceContext{
		ProjectDir:   filepath.Dir(absProjectPath),
		SourceLocale: a.SourceLocale(),
		TargetLocale: a.TargetLang,
	}

	// One run per binding group: the group's bindings go on the App before the
	// run, because that is where the tool chain reads them (buildFlowTools →
	// applyBindings). One group — no collection naming a context — is one run
	// over every input, exactly as before.
	runGroups := func() error {
		for _, group := range groups {
			b, berr := groupBindings.at(group.Point, a.TargetLang)
			if berr != nil {
				return berr
			}
			a.ProjectBindings = b
			if err := a.runProjectStepsOver(cmd.Context(), cmd, flowName, spec, &rCtx, group.Inputs); err != nil {
				return err
			}
		}
		return nil
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
		passes := flow.ResolveFlowLocales(spec, flow.BuildToolInfoMap(a.ToolReg), a.SourceLocale(), localeStrings(ctx.TargetLocales))
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
				if err := runGroups(); err != nil {
					return err
				}
			}
			return nil
		}
	}

	return runGroups()
}
