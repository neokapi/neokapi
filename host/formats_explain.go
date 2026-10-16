package host

import (
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/output"
)

// Rules FormatExplanation reports beside the registry's when a format is
// named outright rather than detected.
const (
	// RuleFormatFlag: --format named the format, as a reference the engine
	// index resolved when it carried an engine.
	RuleFormatFlag = "format flag"
	// RuleContentItemFormat: a content item of the recipe names the file's
	// format.
	RuleContentItemFormat = "content item format"
)

// formatRefName is the registry name a format reference resolves to. The
// name as written is the format, unless the reference names an engine
// (`html@okapi-bridge`) or the recipe in scope pins one for that format
// (`defaults.formats.html.engine`): then it is that engine's format for the
// same extensions (FormatRegistry.EngineFormat), versioned as the reference
// says. A reference naming no engine keeps its name whatever the engine
// order prefers: a format named outright is read by that format.
func (a *App) formatRefName(ref preset.FormatRef) (string, error) {
	return a.formatRefNameIn(ref, a.ProjectContext)
}

// formatRefNameIn is formatRefName under the recipe pctx, which may be nil.
func (a *App) formatRefNameIn(ref preset.FormatRef, pctx *project.ProjectContext) (string, error) {
	engine := ref.Engine
	if engine == "" && pctx != nil {
		if fd, ok := pctx.FormatDefaults[ref.Name]; ok {
			engine = fd.Engine
		}
	}
	if engine == "" {
		return ref.RegistryName(), nil
	}
	if err := a.FormatReg.CheckEngine(engine); err != nil {
		return "", err
	}
	id, err := a.FormatReg.EngineFormat(registry.FormatID(ref.Name), engine)
	if err != nil {
		return "", err
	}
	if ref.Version != "" {
		return string(id) + "@" + ref.Version, nil
	}
	return string(id), nil
}

// FormatRefName resolves a format reference as --format reads one: the
// registry name of the format it names, with an `@engine` applied.
func (a *App) FormatRefName(fmtRef string) (string, error) {
	return a.formatRefName(preset.ParseFormatRef(fmtRef))
}

// ExplainFormats resolves each path the way a run would and reports the
// decision: the format, its engine, the rule that chose the engine, the
// engine order consulted and every format claiming the extension. --format
// names the format for every path, as it does on a run. The recipe in scope
// supplies its sources, engine preference and pins, and a content item
// naming the file's format is reported as such; without a recipe the
// registry's own order applies. A path no format reads carries an error
// rather than failing the command.
func (a *App) ExplainFormats(cmd Command, paths []string) (output.FormatExplainOutput, error) {
	var pctx *project.ProjectContext
	recipePath, err := ResolveProjectPath(cmd)
	if err != nil {
		return output.FormatExplainOutput{}, err
	}
	if recipePath != "" {
		proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
		if err != nil {
			return output.FormatExplainOutput{}, fmt.Errorf("load project: %w", err)
		}
		pctx = project.NewProjectContext(proj, recipePath)
	}

	out := output.FormatExplainOutput{Files: make([]output.FormatExplanation, 0, len(paths))}
	for _, path := range paths {
		if a.FormatFlag != "" {
			out.Files = append(out.Files, a.explainNamedFormat(pctx, path))
			continue
		}
		out.Files = append(out.Files, a.explainFormat(pctx, path))
	}
	return out, nil
}

// explainNamedFormat reports the format --format names for path: the
// reference resolved under the recipe, or the error the reference raises.
func (a *App) explainNamedFormat(pctx *project.ProjectContext, path string) output.FormatExplanation {
	ex := output.FormatExplanation{Path: path, Rule: RuleFormatFlag}
	name, err := a.formatRefNameIn(preset.ParseFormatRef(a.FormatFlag), pctx)
	if err != nil {
		ex.Error = err.Error()
		return ex
	}
	if a.FormatReg.FormatInfo(registry.FormatID(name)) == nil {
		ex.Error = fmt.Sprintf("unknown format %q", a.FormatFlag)
		return ex
	}
	ex.Format = name
	ex.Engine = registry.EngineName(a.FormatReg.FormatSource(registry.FormatID(name)))
	return ex
}

// explainFormat is ExplainFormats for one path.
func (a *App) explainFormat(pctx *project.ProjectContext, path string) output.FormatExplanation {
	ex := output.FormatExplanation{Path: path}
	opts := registry.DetectOptions{}
	if pctx != nil {
		opts = pctx.DetectOptions(nil)
		if name, ok := a.itemFormat(pctx, path); ok {
			ex.Format = name
			ex.Engine = registry.EngineName(a.FormatReg.FormatSource(registry.FormatID(name)))
			ex.Rule = RuleContentItemFormat
			return ex
		}
	}
	res, err := a.FormatReg.Resolve(path, opts)
	if err != nil {
		ex.Error = err.Error()
		return ex
	}
	ex.Format = string(res.Format)
	ex.Engine = res.Engine
	ex.Rule = res.Rule
	ex.Content = res.Content
	ex.Order = res.Order
	for _, c := range res.Candidates {
		ex.Candidates = append(ex.Candidates, output.FormatCandidate{
			Format: string(c),
			Engine: registry.EngineName(a.FormatReg.FormatSource(c)),
		})
	}
	return ex
}

// itemFormat is the format a content item of the recipe names for path,
// resolved as a reference, when the path lies in the project and an item
// claiming it names one.
func (a *App) itemFormat(pctx *project.ProjectContext, path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(pctx.ProjectDir, abs)
	if err != nil || rel == "." || filepath.IsAbs(rel) || len(rel) >= 2 && rel[:2] == ".." {
		return "", false
	}
	item, _, ok := pctx.Project.ItemForPath(filepath.ToSlash(rel))
	if !ok || item.Format == nil || item.Format.Name == "" {
		return "", false
	}
	name, err := a.formatRefNameIn(preset.ParseFormatRef(item.Format.Name), pctx)
	if err != nil {
		return "", false
	}
	return name, true
}
