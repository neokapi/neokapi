package kpz

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/safeio"
	"gopkg.in/yaml.v3"
)

// A .kpz carries the project's FULL recipe — the same core/project.KapiProject
// schema a .kapi file uses — so there is one source of truth for intent and no
// parallel model (AD-025 §6). The recipe is manifest metadata, deliberately
// EXCLUDED from the content RootHash: content defines package identity, intent
// is metadata, mirroring how a workspace recipe already rides outside the hash.
//
// On the wire the recipe is stored as a JSON string holding the YAML encoding
// of the KapiProject. KapiProject is a yaml-tagged struct with an
// `Extras map[string]yaml.Node` inline catch-all (and yaml-only fields like
// flows' StepsSpec), so YAML is its faithful, lossless encoding; JSON-stringing
// that YAML keeps the manifest a single JSON document while preserving every
// recipe field, including platform Extras.

// WorkspaceMetaKey is the top-level Extras key under which a .kpz stashes its
// ad-hoc workspace metadata (the merge-time output layout). It lives in the
// recipe's Extras rather than as a KapiProject field because it is a kpz
// container concern, not a framework project concern — the `out` layout has no
// home in the .kapi schema (AD-025 §6).
const WorkspaceMetaKey = "kpz"

// WorkspaceMeta is the kpz-owned recipe extension: the small amount of
// container intent a .kapi project has no field for. Decoded from / encoded to
// the recipe's top-level Extras under WorkspaceMetaKey.
type WorkspaceMeta struct {
	// Out is the output path template `merge` writes to (placeholders
	// {name} {lang} {ext} {dir}); empty means the default per-locale layout.
	Out string `yaml:"out,omitempty" json:"out,omitempty"`
	// Workspace marks the package as an ad-hoc `.kpz` workspace (created by
	// `extract -o work.kpz`), as opposed to a whole-project snapshot created by
	// `pack`. `unpack` uses it to decide whether to rebuild the shadow cache (a
	// workspace) or rehydrate a `.kapi/` state dir (a project snapshot). Both
	// profiles carry a full recipe, so the recipe alone no longer
	// distinguishes them.
	Workspace bool `yaml:"workspace,omitempty" json:"workspace,omitempty"`
}

func init() {
	// Register the kpz workspace-meta extension so a recipe carrying a
	// `kpz:` block validates (and round-trips) through the framework loader.
	project.RegisterExtension(project.Extension{
		Name:  WorkspaceMetaKey,
		Scope: project.ScopeProject,
		Group: "kpz",
		Decoder: project.ExtensionDecoderFunc(func(node yaml.Node) error {
			var m WorkspaceMeta
			return node.Decode(&m)
		}),
	})
}

// marshalRecipe encodes a KapiProject as a JSON string holding its YAML
// representation, suitable for embedding in the manifest. Returns (nil, nil)
// for a nil recipe.
func marshalRecipe(p *project.KapiProject) (json.RawMessage, error) {
	if p == nil {
		return nil, nil
	}
	yml, err := yaml.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("kpz: marshal recipe yaml: %w", err)
	}
	out, err := json.Marshal(string(yml))
	if err != nil {
		return nil, fmt.Errorf("kpz: encode recipe: %w", err)
	}
	return out, nil
}

// unmarshalRecipe decodes the manifest's recipe field (a JSON string holding
// YAML) back into a KapiProject. Returns (nil, nil) when no recipe is present.
func unmarshalRecipe(raw json.RawMessage) (*project.KapiProject, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var yml string
	if err := json.Unmarshal(raw, &yml); err != nil {
		return nil, fmt.Errorf("kpz: decode recipe: %w", err)
	}
	if yml == "" {
		return nil, nil
	}
	var p project.KapiProject
	if err := yaml.Unmarshal([]byte(yml), &p); err != nil {
		return nil, fmt.Errorf("kpz: parse recipe yaml: %w", err)
	}
	return &p, nil
}

// execClassTools are the built-in tool IDs that run code the recipe names
// rather than transforming content: `external-command` spawns a subprocess from
// its own config, and `script` evaluates recipe-supplied JavaScript (and reads
// an arbitrary file via scriptFile). A recipe that rides in a .kpz has crossed a
// trust boundary, so neither may survive into a project that adopts it.
var execClassTools = map[string]bool{
	"external-command": true,
	"script":           true,
}

// execClassFormats are format names that spawn a subprocess to read content.
// `exec` runs its configured command line, so a package may not name it.
var execClassFormats = map[string]bool{"exec": true}

// sideEffectingExtras are the recipe extension keys that bind a project to
// something outside itself — a server, a hook, a scheduled automation. They are
// stripped at EVERY Extras scope rather than only the top level, because the
// scope an extension is registered at is a platform's choice
// (core/project.RegisterExtension) and can change without this file being
// touched. Sweeping by name everywhere means the denylist cannot quietly stop
// applying because a key moved from `server:` to `defaults.server:`.
var sideEffectingExtras = map[string]bool{
	"server":      true,
	"hooks":       true,
	"automations": true,
}

// SanitizeRecipe returns a copy of the recipe with everything side-effecting
// removed, plus the list of removals in human-readable form (empty when the
// recipe was already inert). A nil recipe returns (nil, nil).
//
// A .kpz carries the full project recipe so a package is a runnable project in
// a file (AD-025 §6) — which is exactly why the recipe cannot be trusted on the
// way back in. The format's original contract asked the PACKER to sanitize
// before packing; a hostile packer simply won't, so the guarantee has to hold
// on ingest. Sanitizing on both sides costs nothing and means the answer no
// longer depends on who wrote the archive.
//
// What is removed:
//
//   - Side-effecting Extras ([sideEffectingExtras]) at every scope — top-level,
//     `defaults:`, per-collection and per-item — so they re-activate only with
//     explicit re-auth / re-arming.
//   - Exec-class flow steps ([execClassTools]) anywhere in a flow — including
//     nested `parallel` branches and `source_transforms` — and the per-tool
//     config that would arm them (`defaults.tools`, `defaults.locales.*.tools`).
//   - Exec-class format bindings ([execClassFormats]) on any content item.
//   - Every path-valued field naming somewhere outside the project
//     ([sanitizeRecipePath]): the terms, terms-source, memory-source and state
//     bindings, the redaction rules file, the brand-voice profile file, and each
//     content entry's `base` and `target`. A project's OWN recipe may point
//     wherever its owner wants — that stays supported, and is asserted
//     separately — but a packaged recipe is answering for a machine it has
//     never seen, so it may only name places inside the project it lands in.
//   - A kpz `out:` layout that is not a local path, since a package describes
//     where its output goes relative to wherever it is merged, never an
//     absolute or climbing destination. An explicit `merge -o` is the supported
//     way to send output somewhere else.
//
// Secrets never live in a recipe (they are in the OS keychain), so there is
// nothing else to scrub.
//
// Note that `requires:` survives, deliberately. It is the package's declaration
// of which plugins its flows need, and dropping it would remove a statement of
// fact without removing any capability: a recipe cannot install anything by
// itself, and a plugin left undeclared simply fails later with a less useful
// error. What acts on `requires:` is the prompt in LoadProjectInteractive, and
// what bounds it is that an install resolves through the signed plugin registry
// — a package can ask for a plugin neokapi publishes, not for code of its own.
// That is the same line AD-038 draws for `--yes`: "install the plugin this
// recipe asks for" and "run the commands this recipe names" are different
// classes of decision, and only the second is withheld here.
func SanitizeRecipe(p *project.KapiProject) (*project.KapiProject, []string) {
	if p == nil {
		return nil, nil
	}
	var removed []string
	note := func(format string, args ...any) {
		removed = append(removed, fmt.Sprintf(format, args...))
	}

	// A shallow struct copy aliases every map and slice, so mutating the clone
	// would corrupt the caller's recipe. Each field below is rebuilt, never
	// edited in place.
	clone := *p

	clone.Extras = sanitizeExtras(p.Extras, "", note)

	if len(p.Flows) > 0 {
		flows := make(map[string]*flow.StepsSpec, len(p.Flows))
		for name, spec := range p.Flows {
			if spec == nil {
				flows[name] = nil
				continue
			}
			s := *spec
			s.Steps = sanitizeSteps(spec.Steps, name, note)
			s.SourceTransforms = sanitizeSteps(spec.SourceTransforms, name, note)
			flows[name] = &s
		}
		clone.Flows = flows
	}

	clone.Defaults.Extras = sanitizeExtras(p.Defaults.Extras, "defaults.", note)
	clone.Defaults.TermsSource = sanitizeRecipePath(p.Defaults.TermsSource, "defaults.terms_source", note)
	clone.Defaults.MemorySource = sanitizeRecipePath(p.Defaults.MemorySource, "defaults.memory_source", note)
	clone.Defaults.Redaction = sanitizeRedaction(p.Defaults.Redaction, "defaults.redaction.rules", note)
	if bv := p.Defaults.Voice; bv != nil {
		// A pointer field is shared with the caller's recipe, so it is rebuilt
		// rather than edited, the same rule the maps above follow.
		c := *bv
		c.ProfileFile = sanitizeRecipePath(bv.ProfileFile, "defaults.voice.profile_file", note)
		clone.Defaults.Voice = &c
	}

	clone.Defaults.Tools = sanitizeToolConfig(p.Defaults.Tools, "defaults.tools", note)
	if len(p.Defaults.Locales) > 0 {
		locales := make(map[string]project.LocaleDefaults, len(p.Defaults.Locales))
		for loc, ld := range p.Defaults.Locales {
			ld.Tools = sanitizeToolConfig(ld.Tools, "defaults.locales."+loc+".tools", note)
			locales[loc] = ld
		}
		clone.Defaults.Locales = locales
	}

	if len(p.Collections) > 0 {
		collections := make([]project.Collection, len(p.Collections))
		for i, coll := range p.Collections {
			collections[i] = sanitizeContent(coll, i, note)
		}
		clone.Collections = collections
	}

	if meta := RecipeWorkspaceMeta(&clone); meta.Out != "" && !IsLocalOutTemplate(meta.Out) {
		note("the out layout %q (it names a destination outside the merge directory)", meta.Out)
		meta.Out = ""
		_ = SetRecipeWorkspaceMeta(&clone, meta)
	}

	return &clone, removed
}

// sanitizeSteps drops exec-class steps from a flow, recursing into `parallel`
// branches so a nested step cannot survive by hiding one level down.
func sanitizeSteps(steps []flow.FlowStep, flowName string, note func(string, ...any)) []flow.FlowStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]flow.FlowStep, 0, len(steps))
	for _, step := range steps {
		if execClassTools[step.Tool] {
			note("the %q step in flow %q", step.Tool, flowName)
			continue
		}
		step.Parallel = sanitizeSteps(step.Parallel, flowName, note)
		out = append(out, step)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// sanitizeToolConfig drops per-tool config presets for exec-class tools. The
// config alone runs nothing, but it is the argv a re-added step would use, so
// it travels no further than the step does.
func sanitizeToolConfig(in map[string]map[string]any, where string, note func(string, ...any)) map[string]map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]map[string]any, len(in))
	for id, cfg := range in {
		if execClassTools[id] {
			note("the %q config in %s", id, where)
			continue
		}
		out[id] = cfg
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// sanitizeExtras drops the side-effecting keys from one Extras map. prefix
// names the scope in the report ("", "defaults.", "collections[0].") so a recipient
// can tell which block lost what. A nil or empty map stays nil.
func sanitizeExtras(in map[string]yaml.Node, prefix string, note func(string, ...any)) map[string]yaml.Node {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]yaml.Node, len(in))
	for k, v := range in {
		if sideEffectingExtras[k] {
			note("the %s%s block", prefix, k)
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// sanitizeRecipePath clears a package-carried path that names somewhere outside
// the project, and reports it. Clearing rather than refusing keeps ingest
// non-fatal — the rest of the recipe is still useful, and every one of these
// fields has a defined meaning when unset (no bound terms, the default state
// file, the default per-locale target layout).
//
// The value may be a template, so the placeholders are substituted for a
// neutral segment first: they stand for a locale, a base name, an extension, or
// a project-relative source path, none of which can introduce a climb.
func sanitizeRecipePath(value, where string, note func(string, ...any)) string {
	if value == "" || safeio.IsLocalPath(pathTemplateProbe.Replace(value)) {
		return value
	}
	note("the %s path %q (it names a location outside the project)", where, value)
	return ""
}

// sanitizeRedaction rebuilds a redaction spec with a contained rules path. The
// spec is a pointer shared with the caller's recipe, so it is copied.
func sanitizeRedaction(in *project.RedactionSpec, where string, note func(string, ...any)) *project.RedactionSpec {
	if in == nil {
		return nil
	}
	c := *in
	c.Rules = sanitizeRecipePath(in.Rules, where, note)
	return &c
}

// sanitizeContent drops exec-class format bindings from a content collection
// and its items, and contains the paths that decide where merge reads and
// writes. The content globs themselves are left alone: they are resolved
// against an os.DirFS rooted at the project, which cannot express a climb.
func sanitizeContent(coll project.Collection, index int, note func(string, ...any)) project.Collection {
	where := fmt.Sprintf("collections[%d]", index)
	if coll.Format != nil && execClassFormats[coll.Format.Name] {
		note("the %q format on content %q", coll.Format.Name, coll.Path)
		coll.Format = nil
	}
	coll.Base = sanitizeRecipePath(coll.Base, where+".base", note)
	coll.Target = sanitizeRecipePath(coll.Target, where+".target", note)
	coll.Extras = sanitizeExtras(coll.Extras, where+".", note)

	if len(coll.Content) > 0 {
		items := make([]project.ContentItem, len(coll.Content))
		for i, item := range coll.Content {
			iw := fmt.Sprintf("%s.content[%d]", where, i)
			if item.Format != nil && execClassFormats[item.Format.Name] {
				note("the %q format on content %q", item.Format.Name, item.Path)
				item.Format = nil
			}
			item.Base = sanitizeRecipePath(item.Base, iw+".base", note)
			item.Target = sanitizeRecipePath(item.Target, iw+".target", note)
			item.Redaction = sanitizeRedaction(item.Redaction, iw+".redaction.rules", note)
			item.Extras = sanitizeExtras(item.Extras, iw+".", note)
			items[i] = item
		}
		coll.Content = items
	}
	return coll
}

// pathTemplateProbe replaces every placeholder a recipe path template may carry
// with a neutral segment, so a template can be validated as a path without
// being expanded. One replacer covers both the kpz `out:` layout and a content
// entry's `target` / `base` (core/project.ExpandTemplate), because the question
// asked of them is identical and two lists would drift.
//
// Substituting a single segment is conservative: each placeholder stands for a
// locale, a base name, an extension, or a source path already relative to the
// project root, so none of them can introduce a climb that the literal text
// does not already contain. The legacy bare `*` (shorthand for {name}) is
// included so a template using it is measured the same way.
var pathTemplateProbe = strings.NewReplacer(
	"{lang}", "x",
	"{name}", "x",
	"{basename}", "x",
	"{ext}", "x",
	"{dir}", "x",
	"{path}", "x",
	"{relpath}", "x",
	"{filename}", "x",
	"*", "x",
)

// IsLocalOutTemplate reports whether a kpz `out:` layout names a destination
// inside the directory the package is merged in. An empty template (the default
// per-locale layout) is local by definition.
func IsLocalOutTemplate(out string) bool {
	if out == "" {
		return true
	}
	return safeio.IsLocalPath(pathTemplateProbe.Replace(out))
}

// RecipeWorkspaceMeta reads the kpz workspace metadata from a recipe's Extras,
// returning the zero value when absent.
func RecipeWorkspaceMeta(p *project.KapiProject) WorkspaceMeta {
	if p == nil {
		return WorkspaceMeta{}
	}
	var m WorkspaceMeta
	if _, err := p.GetExtra(WorkspaceMetaKey, &m); err != nil {
		return WorkspaceMeta{}
	}
	return m
}

// SetRecipeWorkspaceMeta writes the kpz workspace metadata onto a recipe's
// Extras (creating the recipe is the caller's job). A zero meta clears the key.
func SetRecipeWorkspaceMeta(p *project.KapiProject, m WorkspaceMeta) error {
	if p == nil {
		return nil
	}
	if m == (WorkspaceMeta{}) {
		p.DeleteExtra(WorkspaceMetaKey)
		return nil
	}
	return p.SetExtra(WorkspaceMetaKey, m)
}
