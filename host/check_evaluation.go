package host

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/version"
)

// The evaluation record a check result carries: what the run was evaluated
// against, so a verdict read from a CI log, an agent transcript or a bug
// report can be explained and reproduced.
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
// covered.
func (a *App) checkEvaluation(cmd Command, e *checkExecution) *check.Evaluation {
	var analyzers []check.AnalyzerExecution
	if e != nil {
		analyzers = e.Analyzers
	}
	return buildEvaluation(a.checkProvenance(cmd), a.evaluationPlugins(e), analyzers)
}

// buildEvaluation is the whole shape of the record, assembled from the facts a
// run gathered. Splitting it from the gathering is what lets a golden pin the
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
func (a *App) checkProvenance(cmd Command) *check.ContextProvenance {
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return nil
	}
	// A recipe that will not load leaves the identity out rather than guessing
	// one. The workspace revision and the projection's state stand on their own.
	proj, _ := project.LoadWithOptions(path, project.LoadOptions{SkipRequiresCheck: true})
	return a.contextProvenance(cmd, proj)
}

// evaluationPlugins names the plugins that served the run, with the version
// each one's manifest declares and what it did for this run.
func (a *App) evaluationPlugins(e *checkExecution) []check.EvaluationPlugin {
	if e == nil || len(e.plugins) == 0 {
		return nil
	}
	out := make([]check.EvaluationPlugin, 0, len(e.plugins))
	for name, serves := range e.plugins {
		p := check.EvaluationPlugin{Name: name, Version: e.pluginVersions[name]}
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

// recordFormatPlugin records the plugin that reads file under the format the
// run resolved for it. An empty fmtName means the format the file's extension
// detects. A format a kapi binary carries itself records nothing.
func (a *App) recordFormatPlugin(e *checkExecution, file, fmtName string) {
	if e == nil || a.PluginHost == nil {
		return
	}
	id := fmtName
	if id == "" {
		detected, err := a.FormatReg.Detect(file, registry.DetectOptions{ExtensionOnly: true})
		if err != nil {
			return
		}
		id = string(detected)
	} else if name, _, err := a.resolveFormatRef(id); err == nil {
		id = name
	}
	if route := a.PluginHost.FormatRoute(id); route != nil {
		e.served(route.Plugin.Name(), route.Plugin.Version(), "format:"+id)
	}
}

// recordCommentPlugin records the plugin that locates the comments of a file
// whose comments are all a check reads in it. The lookup matches the one
// commentProviderFor dispatches on, so the record names the plugin that ran.
func (a *App) recordCommentPlugin(e *checkExecution, file string) {
	if e == nil || a.PluginHost == nil {
		return
	}
	ext := strings.ToLower(filepath.Ext(file))
	for _, r := range a.PluginHost.CommentRoutes() {
		if slices.ContainsFunc(r.Language.Extensions, func(x string) bool { return strings.ToLower(x) == ext }) {
			e.served(r.Plugin.Name(), r.Plugin.Version(), "comments:"+r.Language.Language)
			return
		}
	}
}
