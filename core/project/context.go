package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/ignore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// ProjectContext is a resolved runtime derived from a KapiProject.
// It provides project-scoped format detection, resolved defaults, content
// resolution, and reader/writer configuration. Both CLI and desktop app
// create a ProjectContext when operating in project mode.
type ProjectContext struct {
	Project    *KapiProject
	ProjectDir string // absolute path to the directory containing the .kapi file

	// Resolved defaults
	SourceLocale   model.LocaleID
	TargetLocales  []model.LocaleID
	AllowedSources []string // format sources: ["built-in", "okapi-bridge", ...]
	Encoding       string   // resolved encoding (default: "UTF-8")
	Concurrency    int      // document-level parallelism (0 = auto)
	ParallelBlocks int      // block-level parallelism (0 = flow default)
	LocaleFormat   string   // "bcp-47" (default) or "posix"
	FormatDefaults map[string]FormatDefaults
}

// NewProjectContext creates a ProjectContext from a loaded project and its
// file path. It resolves defaults and computes derived state.
func NewProjectContext(proj *KapiProject, projectPath string) *ProjectContext {
	dir := filepath.Dir(projectPath)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	// Resolve allowed format sources from declared plugins.
	sources := []string{registry.SourceBuiltIn}
	for name := range proj.Plugins {
		sources = append(sources, name)
	}

	// Resolve locale defaults.
	targetLocales := proj.Defaults.TargetLanguages

	// Resolve encoding (default UTF-8).
	encoding := proj.Defaults.Encoding
	if encoding == "" {
		encoding = "UTF-8"
	}

	// Resolve locale format (default bcp-47).
	localeFormat := proj.Defaults.LocaleFormat
	if localeFormat == "" {
		localeFormat = "bcp-47"
	}

	return &ProjectContext{
		Project:        proj,
		ProjectDir:     dir,
		SourceLocale:   proj.Defaults.SourceLanguage,
		TargetLocales:  targetLocales,
		AllowedSources: sources,
		Encoding:       encoding,
		Concurrency:    proj.Defaults.Concurrency,
		ParallelBlocks: proj.Defaults.ParallelBlocks,
		LocaleFormat:   localeFormat,
		FormatDefaults: proj.Defaults.Formats,
	}
}

// --- Format detection ---

// DetectFormat detects the format for a file path, scoped to the project's
// allowed plugin sources. Returns empty string if no format matches.
func (ctx *ProjectContext) DetectFormat(reg *registry.FormatRegistry, path string) string {
	if filepath.Ext(path) == "" {
		return ""
	}
	// DetectFile is content-aware: when an extension is shared by several
	// formats (.xliff 1.x/2.x, .xml, …) the file head disambiguates, so a 2.x
	// XLIFF isn't read by the 1.x reader. Falls back to extension-only.
	// Priority overrides from `defaults.formats[name].priority` let a recipe pick
	// the preferred engine when several formats claim an extension (e.g. okf_vtt
	// over okf_regex for .srt).
	name, err := reg.DetectFileWithPriorities(path, ctx.AllowedSources, ctx.formatPriorityOverrides())
	if err != nil {
		return ""
	}
	return string(name)
}

// formatPriorityOverrides returns the per-format detection priorities declared
// in the project's defaults.formats, or nil when none are set.
func (ctx *ProjectContext) formatPriorityOverrides() map[string]int {
	if len(ctx.FormatDefaults) == 0 {
		return nil
	}
	var overrides map[string]int
	for name, fd := range ctx.FormatDefaults {
		if fd.Priority != 0 {
			if overrides == nil {
				overrides = make(map[string]int, len(ctx.FormatDefaults))
			}
			overrides[name] = fd.Priority
		}
	}
	return overrides
}

// --- Content resolution ---

// ResolvedFile represents a file matched by a project content pattern.
type ResolvedFile struct {
	Path       string       // absolute file path
	Relative   string       // path relative to project dir
	Format     string       // detected or explicit format name
	Collection string       // parent collection name
	Pattern    string       // the content pattern that matched
	Item       *ContentItem // the content item definition
}

// ResolveContent matches project content patterns against the filesystem and
// returns the resolved file list with detected formats. Ignore rules from
// .kapiignore are applied. Patterns that escape the project root are rejected.
func (ctx *ProjectContext) ResolveContent(reg *registry.FormatRegistry) ([]ResolvedFile, error) {
	if ctx.Project == nil || len(ctx.Project.Content) == 0 {
		return nil, nil
	}

	ig := ignore.ForProjectDir(ctx.ProjectDir)

	var files []ResolvedFile
	for _, coll := range ctx.Project.Content {
		collName := coll.Name
		for _, item := range coll.EffectiveItems() {
			if item.Path == "" {
				continue
			}
			// Reject patterns that escape the project root.
			if strings.Contains(item.Path, "..") {
				continue
			}
			if filepath.IsAbs(item.Path) {
				continue
			}

			// Expand via ExpandGlob (doublestar) so `**` recurses like the
			// content-listing path does — filepath.Glob has no `**` support
			// and silently matched only one directory level. Project-wide
			// excludes apply here exactly as they do for `kapi ls`.
			rels, err := ExpandGlob(ctx.ProjectDir, item.Path, ctx.Project.Defaults.Exclude...)
			if err != nil {
				continue
			}
			matches := make([]string, 0, len(rels))
			for _, rel := range rels {
				matches = append(matches, filepath.Join(ctx.ProjectDir, rel))
			}

			for _, f := range matches {
				info, err := os.Stat(f)
				if err != nil || info.IsDir() {
					continue
				}

				// Verify the file is within the project root.
				absFile, _ := filepath.Abs(f)
				if !strings.HasPrefix(absFile, ctx.ProjectDir+string(filepath.Separator)) {
					continue
				}

				rel, _ := filepath.Rel(ctx.ProjectDir, f)

				// Apply ignore rules.
				if ig.Match(filepath.ToSlash(rel), false) {
					continue
				}

				// Determine format: explicit > auto-detected.
				fmtName := ""
				if item.Format != nil {
					fmtName = item.Format.Name
				}
				if fmtName == "" {
					fmtName = ctx.DetectFormat(reg, f)
				}

				itemCopy := item
				files = append(files, ResolvedFile{
					Path:       absFile,
					Relative:   rel,
					Format:     fmtName,
					Collection: collName,
					Pattern:    item.Path,
					Item:       &itemCopy,
				})
			}
		}
	}
	return files, nil
}

// --- Format configuration ---

// Configurable is implemented by readers and other components that expose
// a DataFormatConfig for applying project-level configuration overrides.
type Configurable interface {
	Config() format.DataFormatConfig
}

// ConfigureReader applies project format defaults (config overrides) to any
// Configurable component (typically a DataFormatReader). If no project
// defaults exist for the format, or the component has no config, this is a no-op.
func (ctx *ProjectContext) ConfigureReader(reader Configurable, formatName string) error {
	if ctx.FormatDefaults == nil {
		return nil
	}
	fd, ok := ctx.FormatDefaults[formatName]
	if !ok {
		return nil
	}
	cfg := reader.Config()
	if cfg == nil {
		return nil
	}
	// Apply config overrides.
	if len(fd.Config) > 0 {
		if err := cfg.ApplyMap(fd.Config); err != nil {
			return err
		}
	}
	return nil
}

// ConfigureWriter applies project defaults to a format writer.
// Sets encoding from project defaults. Format-specific writer configuration
// is not currently supported since writers don't expose a Config() interface.
func (ctx *ProjectContext) ConfigureWriter(writer format.DataFormatWriter) {
	if ctx.Encoding != "" {
		writer.SetEncoding(ctx.Encoding)
	}
}

// --- Plugin scoping ---

// AllowedTools returns the names of tools available for this project.
// Built-in tools are always available. Plugin-provided tools are only
// available if the project declares the plugin that provides them.
// Pass all registered tool infos; the method filters by Source.
func (ctx *ProjectContext) AllowedTools(allTools []registry.ToolInfo) []registry.ToolInfo {
	allowed := make(map[string]bool, len(ctx.AllowedSources))
	for _, s := range ctx.AllowedSources {
		allowed[s] = true
	}

	var result []registry.ToolInfo
	for _, t := range allTools {
		source := t.Source
		if source == "" {
			source = registry.SourceBuiltIn
		}
		if allowed[source] {
			result = append(result, t)
		}
	}
	return result
}

// --- Flow validation ---

// FlowValidationIssue describes a problem with a tool reference in a project flow.
type FlowValidationIssue struct {
	FlowName string `json:"flow_name"`
	StepTool string `json:"step_tool"`
	Type     string `json:"type"`             // "unknown" or "undeclared_plugin"
	Source   string `json:"source,omitempty"` // plugin name (for undeclared_plugin)
	Message  string `json:"message"`
}

// ValidateFlows checks all flows in the project for tool references that
// require undeclared plugins. Returns nil if all tools are available.
func (ctx *ProjectContext) ValidateFlows(allTools []registry.ToolInfo) []FlowValidationIssue {
	if ctx.Project.Flows == nil {
		return nil
	}

	// Build tool→source lookup.
	toolSource := make(map[string]string, len(allTools))
	for _, t := range allTools {
		source := t.Source
		if source == "" {
			source = registry.SourceBuiltIn
		}
		toolSource[string(t.Name)] = source
	}

	allowed := make(map[string]bool, len(ctx.AllowedSources))
	for _, s := range ctx.AllowedSources {
		allowed[s] = true
	}

	var issues []FlowValidationIssue
	for flowName, spec := range ctx.Project.Flows {
		for _, step := range spec.Steps {
			validateStep(step, flowName, toolSource, allowed, &issues)
		}
	}
	if len(issues) == 0 {
		return nil
	}
	return issues
}

func validateStep(step flow.FlowStep, flowName string, toolSource map[string]string, allowed map[string]bool, issues *[]FlowValidationIssue) {
	if step.Tool != "" {
		source, known := toolSource[step.Tool]
		if !known {
			// Tool doesn't exist in any registry.
			*issues = append(*issues, FlowValidationIssue{
				FlowName: flowName,
				StepTool: step.Tool,
				Type:     "unknown",
				Message:  fmt.Sprintf("tool %q is not installed or does not exist", step.Tool),
			})
		} else if !allowed[source] {
			// Tool exists but its plugin is not declared by the project.
			*issues = append(*issues, FlowValidationIssue{
				FlowName: flowName,
				StepTool: step.Tool,
				Type:     "undeclared_plugin",
				Source:   source,
				Message:  fmt.Sprintf("tool %q requires plugin %q which is not declared in the project", step.Tool, source),
			})
		}
	}
	for _, p := range step.Parallel {
		validateStep(p, flowName, toolSource, allowed, issues)
	}
}
