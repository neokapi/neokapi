package host

import (
	"context"
	"sort"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/version"
)

// The evaluation record a check result carries: what the run was evaluated
// against, so a verdict can be explained and reproduced later.
//
// Assembly is best-effort throughout, the way the retrieval provenance it
// reuses is. A workspace that will not open, a recipe that will not load or a
// plugin kapi holds no manifest for costs the record a field. None of it
// reaches the verdict, the score or the gate.

// evaluationProgram names the tool in the record.
const evaluationProgram = "kapi"

// evaluationClock is the clock an evaluation record is stamped from. One run
// reads it once, so every surface of that run reports one instant, and a test
// pins it by replacing the function.
var evaluationClock = time.Now

// checkEvaluation assembles the record for one run: the project it read its
// governance from, the build and plugins that ran it, and what each analyzer
// covered. The plugins are read from the ledger ctx carries, which the
// operation attached when it began (observePlugins).
func (a *App) checkEvaluation(ctx context.Context, cmd Command, e *checkExecution) *check.Evaluation {
	var analyzers []check.AnalyzerExecution
	if e != nil {
		analyzers = e.Analyzers
	}
	return buildEvaluation(a.checkProvenance(ctx, cmd), a.evaluationPlugins(pluginLedgerFrom(ctx)), analyzers)
}

// verifyEvaluation assembles the one record a gate run carries, over every
// gate: the analyzers each gate recorded, under the project and the plugins
// the run as a whole reached.
func (a *App) verifyEvaluation(ctx context.Context, cmd Command, gates []verifyGateResult) *check.Evaluation {
	return buildEvaluation(a.checkProvenance(ctx, cmd), a.evaluationPlugins(pluginLedgerFrom(ctx)), gateAnalyzers(gates))
}

// gateAnalyzers is every analyzer execution the gates recorded, in gate order,
// the list the run's coverage projects.
func gateAnalyzers(gates []verifyGateResult) []check.AnalyzerExecution {
	var analyzers []check.AnalyzerExecution
	for _, g := range gates {
		if g.Execution != nil {
			analyzers = append(analyzers, g.Execution.Analyzers...)
		}
	}
	return analyzers
}

// buildEvaluation is the whole shape of the record, assembled from the facts a
// run gathered. It sits apart from the gathering so a golden can pin the
// document with the clock, the versions and every fact fixed.
func buildEvaluation(prov *check.ContextProvenance, plugins []check.EvaluationPlugin, analyzers []check.AnalyzerExecution) *check.Evaluation {
	return &check.Evaluation{
		At:      evaluationClock().UTC().Format(time.RFC3339),
		Context: prov,
		Tool: check.EvaluationTool{
			Name:    evaluationProgram,
			Version: version.Version,
			Commit:  version.Commit,
		},
		Plugins:   plugins,
		Analyzers: check.AnalyzerCoverageOf(analyzers),
	}
}

// checkProvenance is the project a run read its governance from and the state
// that project's context was in. A run outside any project reads no context,
// and carries none.
//
// A run that threads no command read no project's governance either, whatever
// its working directory sits in: that is how an ungoverned draft check reaches
// here, and naming a project for it would claim a governance that applied to
// nothing.
func (a *App) checkProvenance(ctx context.Context, cmd Command) *check.ContextProvenance {
	if cmd == nil {
		return nil
	}
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return nil
	}
	// A recipe that will not load leaves the identity out rather than guessing
	// one. The workspace revision and the projection's state stand on their own.
	proj, _ := project.LoadWithOptions(path, project.LoadOptions{SkipRequiresCheck: true})
	return a.contextProvenanceAt(ctx, cmd, proj)
}

// evaluationPlugins names the plugins that served the run, with the version
// each one's manifest declares and what it did for this run.
func (a *App) evaluationPlugins(l *pluginLedger) []check.EvaluationPlugin {
	if l == nil || len(l.serves) == 0 {
		return nil
	}
	out := make([]check.EvaluationPlugin, 0, len(l.serves))
	for name, serves := range l.serves {
		p := check.EvaluationPlugin{Name: name, Version: l.versions[name]}
		if p.Version == "" && a.PluginHost != nil {
			if installed := a.PluginHost.Plugin(name); installed != nil {
				p.Version = installed.Version()
			}
		}
		for s := range serves {
			p.Serves = append(p.Serves, s)
		}
		sort.Strings(p.Serves)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// recordFormatPlugin records, in the ledger ctx carries, the plugin that reads
// file under the format the run resolved for it. An empty fmtName means the
// format the file's extension detects. A format a kapi binary carries itself
// records nothing, and so does a context carrying no ledger.
//
// It sits at the reads themselves (readBlocksAs, readBlocksValidated,
// readWithExtents) rather than at their callers, so a gate or a check that
// reaches a plugin through any of them is recorded without naming it.
func (a *App) recordFormatPlugin(ctx context.Context, file, fmtName string) {
	ledger := pluginLedgerFrom(ctx)
	if ledger == nil || a.PluginHost == nil {
		return
	}
	id := fmtName
	if id == "" {
		// The same detection the read does (readBlocksValidated), so the record
		// names the format the file was actually read under.
		detected, err := a.FormatReg.Detect(file, registry.DetectOptions{ExtensionOnly: true})
		if err != nil {
			return
		}
		id = string(detected)
	}
	ids := []string{id}
	// A recipe may name a format through a preset, `yaml:frontmatter`. The
	// route is keyed by the registry name behind it.
	if name, _, err := a.resolveFormatRef(id); err == nil && name != id {
		ids = append(ids, name)
	}
	for _, name := range ids {
		if route := a.PluginHost.FormatRoute(name); route != nil {
			ledger.served(route.Plugin.Name(), route.Plugin.Version(), "format:"+name)
			return
		}
	}
}
