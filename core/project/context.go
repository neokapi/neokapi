package project

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/ignore"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// ProjectContext is a resolved runtime derived from a KapiProject.
// It provides project-scoped format detection, resolved defaults, content
// resolution, and reader/writer configuration. Both CLI and desktop app
// create a ProjectContext when operating in project mode.
type ProjectContext struct {
	Project    *KapiProject
	ProjectDir string // absolute path to the directory containing kapi.yaml

	// Resolved defaults
	SourceLocale   model.LocaleID
	TargetLocales  []model.LocaleID
	AllowedSources []string // format sources: ["built-in", "okapi-bridge", ...]
	Encoding       string   // resolved encoding (default: "UTF-8")
	Concurrency    int      // document-level parallelism (0 = auto)
	ParallelBlocks int      // block-level parallelism (0 = flow default)
	LocaleFormat   string   // "bcp-47" (default) or "posix"
	FormatDefaults map[string]FormatDefaults

	// Engine is the engine the recipe prefers (defaults.engine) for a file
	// whose extension several engines claim; "" prefers the built-in formats.
	Engine string
	// EngineOrder ranks the declared plugins after the built-in formats: by
	// plugins.<name>.format_priority, highest first, then by name.
	EngineOrder []string
}

// NewProjectContext creates a ProjectContext from a loaded project and its
// file path. It resolves defaults and computes derived state.
func NewProjectContext(proj *KapiProject, projectPath string) *ProjectContext {
	dir := filepath.Dir(projectPath)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	// Resolve allowed format sources from declared plugins, ranked by
	// format_priority and then name so detection is the same for every
	// reader of the recipe.
	plugins := make([]string, 0, len(proj.Plugins))
	for name := range proj.Plugins {
		plugins = append(plugins, name)
	}
	slices.SortFunc(plugins, func(a, b string) int {
		if c := cmp.Compare(proj.Plugins[b].FormatPriority, proj.Plugins[a].FormatPriority); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	sources := append([]string{registry.SourceBuiltIn}, plugins...)

	// Resolve locale defaults. The recipe keeps whatever style it was written
	// in; everything downstream of here is canonical BCP-47, so a project
	// declaring nb_NO and one declaring nb-NO are the same project to a run.
	targetLocales := normalizeLocales(proj.Defaults.TargetLanguages)

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
		SourceLocale:   locale.Normalize(proj.Defaults.SourceLanguage),
		TargetLocales:  targetLocales,
		AllowedSources: sources,
		Encoding:       encoding,
		Concurrency:    proj.Defaults.Concurrency,
		ParallelBlocks: proj.Defaults.ParallelBlocks,
		LocaleFormat:   localeFormat,
		FormatDefaults: proj.Defaults.Formats,
		Engine:         proj.Defaults.Engine,
		EngineOrder:    plugins,
	}
}

// PathLocale renders a locale the way this project writes it into a file path.
//
// Inside kapi the locale is canonical BCP-47; on disk it is whatever
// `defaults.locale_format` declared. Callers building a target path from a
// project go through here so the projection is made once, in one place, rather
// than by each surface deciding for itself.
func (ctx *ProjectContext) PathLocale(id model.LocaleID) string {
	if ctx == nil {
		return string(id)
	}
	return locale.FormatCode(string(id), ctx.LocaleFormat)
}

// TargetPath expands a content item's target template for one source file and
// locale, rendering the locale in the project's declared style.
func (ctx *ProjectContext) TargetPath(item ContentItem, sourceRel string, id model.LocaleID) string {
	format := ""
	if ctx != nil {
		format = ctx.LocaleFormat
	}
	return ResolveTargetPathIn(item.Path, item.Base, item.Target, sourceRel, string(id), format)
}

// LocaleFromPath canonicalizes a locale read back off disk.
//
// A path is written in the project's declared style, so a locale recovered from
// one arrives in that style and has to come back to BCP-47 before it is
// compared with anything. Without this, a project writing `app/nb_NO.json`
// lists a target language its own recipe does not contain.
func LocaleFromPath(s string) model.LocaleID {
	if id, err := locale.Canonical(s); err == nil {
		return id
	}
	return model.LocaleID(s)
}

// --- Format detection ---

// DetectFormat detects the format for a file path, scoped to the project's
// allowed plugin sources. Returns empty string if no format matches.
func (ctx *ProjectContext) DetectFormat(reg *registry.FormatRegistry, path string) string {
	return ctx.detectFormat(reg, path, nil)
}

// detectFormat is DetectFormat with the content to sniff supplied by content,
// or read from the file at path when content is nil.
func (ctx *ProjectContext) detectFormat(reg *registry.FormatRegistry, path string, content func() (io.ReadSeeker, error)) string {
	if format.Ext(path) == "" {
		return ""
	}
	// Detection is content-aware: when an extension is shared by several
	// formats of the chosen engine (.xliff 1.x/2.x, .xml, …) the file head
	// disambiguates, so a 2.x XLIFF isn't read by the 1.x reader. Falls back
	// to extension-only.
	name, err := reg.Detect(path, ctx.DetectOptions(content))
	if err != nil {
		return ""
	}
	return string(name)
}

// DetectOptions is the detection the recipe declares: its sources, the
// engine it prefers and how it ranks its plugins, the engine it pins per
// format (defaults.formats.<name>.engine) and the priorities it sets among
// an engine's formats (defaults.formats.<name>.priority). content supplies
// the file's bytes when several formats claim its extension; nil reads the
// file at the path.
func (ctx *ProjectContext) DetectOptions(content func() (io.ReadSeeker, error)) registry.DetectOptions {
	opts := registry.DetectOptions{
		AllowedSources: ctx.AllowedSources,
		Engine:         ctx.Engine,
		EngineOrder:    ctx.EngineOrder,
		Content:        content,
	}
	for name, fd := range ctx.FormatDefaults {
		if fd.Priority != 0 {
			if opts.PriorityOverrides == nil {
				opts.PriorityOverrides = make(map[string]int, len(ctx.FormatDefaults))
			}
			opts.PriorityOverrides[name] = fd.Priority
		}
		if fd.Engine != "" {
			if opts.FormatEngines == nil {
				opts.FormatEngines = make(map[string]string, len(ctx.FormatDefaults))
			}
			opts.FormatEngines[name] = fd.Engine
		}
	}
	return opts
}

// --- Content resolution ---

// ResolvedFile represents a file claimed by project content items. Item is the
// item that claims the file's values (KapiProject.ItemForPath), or the one that
// claims its comments when no item claims its values.
type ResolvedFile struct {
	Path       string       // absolute file path
	Relative   string       // path relative to project dir
	Format     string       // detected or explicit format name
	Collection string       // the collection of the item that claimed the file
	Pattern    string       // the pattern of the item that claimed the file
	Item       *ContentItem // the content item that claimed the file
	// CollectionIndex and ItemIndex address the recipe entry this file came
	// from: the collection's position in Collections, and the item's position
	// in its Content (0 for a bare entry).
	//
	// Pattern is the EFFECTIVE pattern, with any collection `base:` folded in
	// by EffectiveItems, so it does not equal what the recipe declares. A
	// caller that needs to join files back to the recipe row a person edits
	// must use these rather than string-matching the two spellings, which is a
	// second implementation of JoinBase and drifts from the first.
	CollectionIndex int
	ItemIndex       int
	// CommentItem is the item that claims the file's comments, which sit at its
	// point (KapiProject.CommentItemForPath): the first item declared for the
	// comments alone, or Item when it declares them and comes first. nil when no
	// item declares the file's comments.
	CommentItem *ContentItem
}

// CommentsOnly reports a file claimed for its comments alone: no item claims its
// values, and its item sets `comments: {only: true}`, or sets `comments: true`
// and no format reads the file. Its comments are checked, and nothing reads its
// values: extraction, drift detection, a convergence run, a flow run, merge,
// coverage, the ship gates and a push each leave them alone.
func (rf ResolvedFile) CommentsOnly() bool {
	return rf.Item != nil && rf.Item.Comments.Declared && (rf.Item.Comments.Only || rf.Format == "")
}

// ResolveContent matches project content patterns against the filesystem and
// returns the resolved file list with detected formats. Ignore rules from
// .kapiignore are applied. Patterns that escape the project root are rejected.
//
// A file resolves once. Items are walked in recipe order, across collections,
// and the claims on each layer of a file follow the rule ItemForPath applies to
// every path-to-item lookup in the hosts (fileClaims). The first item whose
// pattern matches a file and that claims more than its comments claims its
// values and names the file. A later item of that kind never sees the file,
// however specific its pattern, so a recipe lists the item that names a file
// before the glob that would also cover it. An item declared for comments alone
// claims no value wherever it is listed, and claims the comments unless the
// values' item declares them first (ResolvedFile.CommentItem).
//
// It fails rather than resolve a partial content set: a pattern that cannot be
// expanded means the recipe declares content this call cannot account for, and
// every derivation downstream (coverage, plan, checks, review, extract) reads a
// short list as a smaller universe rather than as an error. Callers must not
// discard the error: an empty result and a failed resolution are different
// facts.
//
// A match that cannot be read, such as a dangling symbolic link, is left out of
// the list and handed to OnUnreadableContent, which the CLI reports on stderr.
// ResolveContentReport returns the same matches to its caller.
func (ctx *ProjectContext) ResolveContent(reg *registry.FormatRegistry) ([]ResolvedFile, error) {
	res, err := ctx.ResolveContentReport(reg)
	return res.Files, err
}

// UnreadableMatch is a path a content pattern matched whose file cannot be
// read: a dangling symbolic link, or a link whose target cannot be opened.
type UnreadableMatch struct {
	Relative string // slash-separated, relative to the project directory
	Err      error
}

// OnUnreadableContent is called by content resolution with the matches it left
// out because they cannot be read, when there are any. Nil by default, so the
// framework prints nothing on its own; the host sets it once at startup, as it
// sets OnKeyWarnings, and it is read, not synchronised.
var OnUnreadableContent func(projectDir string, unreadable []UnreadableMatch)

// ContentResolution is what ResolveContentReport resolved: the files, and the
// matches it left out because they cannot be read.
type ContentResolution struct {
	Files      []ResolvedFile
	Unreadable []UnreadableMatch
}

// ResolveContentReport is ResolveContent, also naming the matches it left out
// because they cannot be read.
func (ctx *ProjectContext) ResolveContentReport(reg *registry.FormatRegistry) (ContentResolution, error) {
	if ctx.Project == nil || len(ctx.Project.Collections) == 0 {
		return ContentResolution{}, nil
	}

	ig := ignore.ForProjectDir(ctx.ProjectDir)

	// The items with a pattern, in recipe order across collections.
	// EffectiveItems preserves the recipe's own order, so the index here
	// addresses the item a person edits even though its path has the
	// collection's base folded in.
	type resolvingItem struct {
		item   ContentItem
		ci, ii int
	}
	var items []resolvingItem
	var patterns []string
	for ci, coll := range ctx.Project.Collections {
		for ii, item := range coll.EffectiveItems() {
			// Reject an empty pattern and one that escapes the project root.
			if item.Path == "" || strings.Contains(item.Path, "..") || filepath.IsAbs(item.Path) {
				continue
			}
			// A pattern that will not expand is a fault in the recipe (an
			// unclosed `[`/`{`). Dropping its item would shrink the universe
			// every count downstream is taken over, so resolution fails and
			// names the collection and the pattern.
			if !doublestar.ValidatePattern(item.Path) {
				where := "content"
				if coll.Name != "" {
					where = fmt.Sprintf("content collection %q", coll.Name)
				}
				return ContentResolution{}, fmt.Errorf("%s: pattern %q cannot be expanded, so its content would resolve to nothing. Fix the pattern in the recipe: %w",
					where, item.Path, doublestar.ErrBadPattern)
			}
			items = append(items, resolvingItem{item: item, ci: ci, ii: ii})
			patterns = append(patterns, item.Path)
		}
	}

	// Every pattern resolves against one walk of the project, which never
	// reads an excluded or ignored directory. The ignore rules apply before
	// anything is opened, so a file under an ignored directory is never
	// looked at. A match that cannot be read (a dangling link) is left out
	// and recorded in Unreadable, which the hosts report.
	var res ContentResolution
	unreadable := map[string]bool{}
	expanded, err := ExpandGlobs(ctx.ProjectDir, patterns, GlobOptions{
		Excludes:  ctx.Project.Defaults.Exclude,
		Ignore:    ig,
		FilesOnly: true,
		OnUnreadable: func(rel string, err error) {
			if !unreadable[rel] {
				unreadable[rel] = true
				res.Unreadable = append(res.Unreadable, UnreadableMatch{Relative: rel, Err: err})
			}
		},
	})
	if err != nil {
		return ContentResolution{}, fmt.Errorf("resolve content: %w", err)
	}

	// The files the items match, by slash-relative path, and the order in which
	// an item first matched each.
	matched := map[string]*resolvingFile{}
	var order []string
	for k, ri := range items {
		for _, relSlash := range expanded[k] {
			f := matched[relSlash]
			if f == nil {
				f = &resolvingFile{ctx: ctx, reg: reg, rel: relSlash}
				matched[relSlash] = f
				order = append(order, relSlash)
			}
			if !f.claims.decided() {
				f.claims.offer(ri.item, ri.ci, ri.ii, f.noReader)
			}
		}
	}
	res.Files = make([]ResolvedFile, 0, len(order))
	for _, rel := range order {
		res.Files = append(res.Files, matched[rel].resolved())
	}
	if len(res.Unreadable) > 0 && OnUnreadableContent != nil {
		OnUnreadableContent(ctx.ProjectDir, res.Unreadable)
	}
	return res, nil
}

// ResolvePaths resolves the files among rels, paths relative to the project
// directory, that the recipe declares as content. Each path goes through the
// rule ResolveContent applies to a file it finds: the project's excludes and
// ignore rules, then the claims of the items whose patterns match it
// (fileClaims), and the format of the item that names it or the one detected.
// It looks for no file, so it resolves a path that only a git index or tree
// holds.
//
// content supplies a file's bytes when several formats claim its extension and
// the content decides between them. With nil content the extension and
// priority decide.
func (ctx *ProjectContext) ResolvePaths(reg *registry.FormatRegistry, rels []string, content func(rel string) (io.ReadSeeker, error)) []ResolvedFile {
	if ctx.Project == nil || len(ctx.Project.Collections) == 0 {
		return nil
	}
	ig := ignore.ForProjectDir(ctx.ProjectDir)
	claimed := map[string]bool{}
	var files []ResolvedFile
	for _, rel := range rels {
		relSlash := filepath.ToSlash(rel)
		if !filepath.IsLocal(filepath.FromSlash(rel)) || claimed[relSlash] || ig.MatchPath(relSlash) {
			continue
		}
		f := &resolvingFile{ctx: ctx, reg: reg, rel: relSlash, open: func() (io.ReadSeeker, error) { return nil, os.ErrNotExist }}
		if content != nil {
			f.open = func() (io.ReadSeeker, error) { return content(relSlash) }
		}
		f.claims = ctx.Project.claimsForPath(relSlash, f.noReader)
		if !f.claims.item().ok {
			continue
		}
		claimed[relSlash] = true
		files = append(files, f.resolved())
	}
	return files
}

// resolvingFile is one file while the recipe's items are offered to it: the
// claims so far, and the format detected for it, which is detected at most
// once.
type resolvingFile struct {
	ctx *ProjectContext
	reg *registry.FormatRegistry
	rel string
	// open supplies the content the format is detected from. With nil open the
	// file on disk is read.
	open     func() (io.ReadSeeker, error)
	claims   fileClaims
	detected *string
}

// detect returns the format detected for the file.
func (f *resolvingFile) detect() string {
	if f.detected == nil {
		name := f.ctx.detectFormat(f.reg, filepath.Join(f.ctx.ProjectDir, filepath.FromSlash(f.rel)), f.open)
		f.detected = &name
	}
	return *f.detected
}

// noReader reports that no reader parses the file.
func (f *resolvingFile) noReader() bool {
	return f.detect() == ""
}

// resolved is the file as its claims resolve it. Its format is the one the item
// that names it declares, or the one detected.
func (f *resolvingFile) resolved() ResolvedFile {
	c := f.claims.item()
	fmtName := ""
	if c.item.Format != nil {
		fmtName = c.item.Format.Name
	}
	if fmtName == "" {
		fmtName = f.detect()
	}
	rf := ResolvedFile{
		Path:            filepath.Join(f.ctx.ProjectDir, filepath.FromSlash(f.rel)),
		Relative:        filepath.FromSlash(f.rel),
		Format:          fmtName,
		Collection:      f.ctx.Project.Collections[c.coll].Name,
		Pattern:         c.item.Path,
		Item:            &c.item,
		CollectionIndex: c.coll,
		ItemIndex:       c.index,
	}
	if f.claims.comments.ok {
		comments := f.claims.comments.item
		rf.CommentItem = &comments
	}
	return rf
}

// --- Format configuration ---

// Configurable is implemented by readers and other components that expose
// a DataFormatConfig for applying project-level configuration overrides.
type Configurable interface {
	Config() format.DataFormatConfig
}

// ConfigureReader applies the project's format defaults to any Configurable
// component (typically a DataFormatReader). It is ConfigureReaderFor with no
// content item — the right call only where no item is in scope, such as an
// ad-hoc file the recipe does not claim.
func (ctx *ProjectContext) ConfigureReader(reader Configurable, formatName string) error {
	return ctx.ConfigureReaderFor(reader, formatName, nil)
}

// ConfigureReaderFor applies the format configuration the recipe declares for
// one content item — the project defaults for the format overlaid by the item's
// own `format.config` — onto a reader. A nil item takes the defaults alone.
func (ctx *ProjectContext) ConfigureReaderFor(reader Configurable, formatName string, item *ContentItem) error {
	return ApplyReaderConfig(reader, ctx.FormatConfigFor(formatName, item))
}

// ConfigureWriter applies the project's format defaults to a writer. It is
// ConfigureWriterFor with no content item.
func (ctx *ProjectContext) ConfigureWriter(writer format.DataFormatWriter, formatName string) error {
	return ctx.ConfigureWriterFor(writer, formatName, nil)
}

// ConfigureWriterFor applies to a writer the project encoding plus the format
// configuration the recipe declares for one content item: the shared byte-level
// output options (output.bom, output.newline, output.encoding) for every
// writer, and the format-specific serialization keys for a writer that exposes
// its typed config (format.WriterConfigurable).
func (ctx *ProjectContext) ConfigureWriterFor(writer format.DataFormatWriter, formatName string, item *ContentItem) error {
	if ctx.Encoding != "" {
		writer.SetEncoding(ctx.Encoding)
	}
	return ApplyWriterConfig(writer, ctx.FormatConfigFor(formatName, item))
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

// ValidateFlows checks the project's flows, inline under `flows:` and the files
// in its `flows_dir:` that no inline flow shadows, for tool references that
// require undeclared plugins. Returns nil if all tools are available. A flow
// file that does not load is not checked here: ListDirFlows reports it.
func (ctx *ProjectContext) ValidateFlows(allTools []registry.ToolInfo) []FlowValidationIssue {
	flows := maps.Clone(ctx.Project.Flows)
	if flows == nil {
		flows = map[string]*flow.StepsSpec{}
	}
	for _, def := range ListDirFlows(ctx.Project.FlowsDirIn(ctx.ProjectDir)) {
		if _, inline := flows[def.Name]; !inline && def.Err == nil {
			flows[def.Name] = def.Spec
		}
	}
	if len(flows) == 0 {
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
	for flowName, spec := range flows {
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
