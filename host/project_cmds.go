package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host/output"
)

// InitOptions configures InitProject.
type InitOptions struct {
	// Name is the project id written into the recipe's name: field. Empty
	// defaults to the root directory's basename.
	Name string
	// SourceLocale is the BCP-47 source language. Empty defaults to "en".
	SourceLocale string
	// TargetLocales, when non-empty (or when Framework is set), opts into the
	// translation scaffold rather than the on-brand content scaffold.
	TargetLocales []string
	// Framework pre-fills the content mapping for a known stack (a preset name)
	// and scaffolds a translation project.
	Framework string
	// MintID asks for a stable project id on a recipe that is already there
	// and carries none. A recipe InitProject writes itself always gets one, so
	// this only governs the adopt path.
	MintID bool
}

// InitResult reports what InitProject did.
type InitResult struct {
	Name       string
	RecipePath string
	StateDir   string
	// ID is the project's stable id, empty when the adopted recipe has none
	// and none was asked for.
	ID string
	// IDMinted is true when this call wrote ID into the recipe.
	IDMinted bool
	// AlreadyInitialized is true when a recipe was already present; InitProject
	// adopts it and leaves it untouched (init is idempotent).
	AlreadyInitialized bool
}

// InitProject scaffolds (or adopts) a kapi project at root: it writes the recipe
// (unless one is already present), ensures the .kapi state layout, and stamps
// the state manifest. It is idempotent — re-running on an initialized project
// adopts the existing recipe and returns AlreadyInitialized. This is the whole
// composition `kapi init` performs, lifted here so Kapi Desktop can create a
// project through the same path rather than reproducing it.
func InitProject(root string, opts InitOptions) (*InitResult, error) {
	name := opts.Name
	if name == "" {
		name = filepath.Base(root)
	}
	sourceLocale := opts.SourceLocale
	if sourceLocale == "" {
		sourceLocale = "en"
	}

	content, err := FrameworkContent(opts.Framework)
	if err != nil {
		return nil, err
	}
	voiceProfile, termsSource := FrameworkBindings(opts.Framework)

	recipeExists, err := RecipeExists(root)
	if err != nil {
		return nil, fmt.Errorf("check for existing project: %w", err)
	}
	recipePath := filepath.Join(root, project.RecipeFileName)
	stateDir := filepath.Join(root, project.StateDirName)

	res := &InitResult{
		Name:               name,
		RecipePath:         recipePath,
		StateDir:           stateDir,
		AlreadyInitialized: recipeExists,
	}

	switch {
	case !recipeExists:
		// Every project kapi scaffolds is born with a stable identity, so
		// nothing it later records is keyed on a label the user is free to
		// edit.
		res.ID, res.IDMinted = project.NewID(), true
		// On-brand content is the default; a target locale or a framework opts
		// into the translation scaffold.
		var recipe []byte
		if len(opts.TargetLocales) > 0 || opts.Framework != "" {
			recipe = ScaffoldRecipe(name, res.ID, sourceLocale, opts.TargetLocales, content, voiceProfile, termsSource)
		} else {
			recipe = ScaffoldContentRecipe(name, res.ID, sourceLocale)
		}
		if err := os.WriteFile(recipePath, recipe, 0o644); err != nil {
			return nil, fmt.Errorf("write recipe: %w", err)
		}
	case opts.MintID:
		id, minted, err := MintProjectID(recipePath)
		if err != nil {
			return nil, err
		}
		res.ID, res.IDMinted = id, minted
	}

	// EnsureLayout/SaveState are safe to run on an existing layout.
	layout := project.Layout{Root: root, RecipePath: recipePath, StateDir: stateDir}
	if err := project.EnsureLayout(layout); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	if !recipeExists {
		if err := project.SaveState(layout, &project.StateManifest{
			Generator: project.StateGenerator{ID: "kapi", Version: version.Version},
			Project: project.StateProjectRef{
				ID:   name,
				Path: "../" + filepath.Base(recipePath),
			},
		}); err != nil {
			return nil, fmt.Errorf("write state manifest: %w", err)
		}
	}

	return res, nil
}

// MintProjectID gives a recipe the stable project id it has none of, and
// reports the id the recipe carries either way along with whether this call
// wrote it.
//
// The write goes through the recipe setter every other pending recipe change
// takes (project.SetField, then project.Save over core/yamledit), so the file
// keeps its comments, its blank lines, its key order and the spelling of every
// value already in it. A recipe that already carries an id is read and left
// alone: SetField refuses to re-key a project, and this path does not ask it to.
func MintProjectID(recipePath string) (string, bool, error) {
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return "", false, fmt.Errorf("load project: %w", err)
	}
	if proj.ID != "" {
		return proj.ID, false, nil
	}
	id := project.NewID()
	raw, err := json.Marshal(id)
	if err != nil {
		return "", false, fmt.Errorf("encode project id: %w", err)
	}
	if _, err := project.SetField(proj, "id", raw); err != nil {
		return "", false, err
	}
	if err := project.Save(recipePath, proj); err != nil {
		return "", false, err
	}
	return id, true, nil
}

// writeScaffoldID writes the project's stable id under `version:`, with the one
// line of explanation a reader of a fresh recipe needs for a value they did not
// type. An empty id writes nothing, which is what a caller building a recipe
// for comparison rather than for a project asks for.
func writeScaffoldID(b *strings.Builder, id string) {
	if id == "" {
		return
	}
	b.WriteString("# This project's stable identity. It survives a rename, a move and a clone,\n")
	b.WriteString("# and everything kapi records about the project is keyed on it. Keep it.\n")
	b.WriteString("id: ")
	b.WriteString(id)
	b.WriteByte('\n')
}

// scaffoldContent is one content mapping written into a scaffolded recipe.
type scaffoldContent struct {
	Path   string
	Format string
	Target string
}

// PrintPresetList emits the full preset catalog (framework scaffolds plus
// per-format parsing presets) — the `kapi init --list-presets` surface that
// absorbed `kapi presets list`. Preset details remain available through the
// hidden `kapi presets show` alias for one release.
func PrintPresetList(cmd Command) error {
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)
	entries := CollectAllPresets(reg)
	return output.Print(cmd, output.PresetsListOutput{Presets: entries, Total: len(entries)})
}

// FrameworkContent resolves a framework preset's catalog mappings into scaffold
// content entries. Returns nil for an empty framework. The neokapi-i18n stack
// scaffolds the clean nested layout — source KBF catalogs under i18n/src/,
// per-locale targets under i18n/{lang}/ — the same mapping the preset carries.
func FrameworkContent(framework string) ([]scaffoldContent, error) {
	if framework == "" {
		return nil, nil
	}
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)

	fp := reg.GetFrameworkPreset(framework)
	if fp == nil {
		var names []string
		for _, p := range reg.ListFrameworkPresets() {
			names = append(names, p.Name)
		}
		return nil, fmt.Errorf("unknown framework %q; available: %s", framework, strings.Join(names, ", "))
	}

	var content []scaffoldContent
	for _, m := range fp.Mappings {
		// Recipe targets use the {lang} placeholder.
		content = append(content, scaffoldContent{
			Path:   m.Local,
			Format: m.Format,
			Target: strings.ReplaceAll(m.TargetPath, "{locale}", "{lang}"),
		})
	}
	return content, nil
}

// FrameworkBindings returns the standing project-context bindings a framework
// preset declares — a voice profile file and a terms store source — for the
// scaffolder to write under defaults:. Both are empty when the framework
// declares none (or is unknown/empty).
func FrameworkBindings(framework string) (voiceProfile, termsSource string) {
	if framework == "" {
		return "", ""
	}
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)
	fp := reg.GetFrameworkPreset(framework)
	if fp == nil {
		return "", ""
	}
	return fp.VoiceProfile, fp.TermsSource
}

// ─── helpers ────────────────────────────────────────────────────

func ResolveDir(flag string) (string, error) {
	if flag == "" {
		return os.Getwd()
	}
	abs, err := filepath.Abs(flag)
	if err != nil {
		return "", fmt.Errorf("resolve --dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", abs, err)
	}
	return abs, nil
}

// RecipeExists reports whether dir already holds a kapi.yaml recipe. Used to
// detect an already-initialized project so `kapi init` is idempotent.
func RecipeExists(dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, project.RecipeFileName))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func ScaffoldRecipe(name, id, sourceLocale string, targetLocales []string, content []scaffoldContent, voiceProfile, termsSource string) []byte {
	var b strings.Builder
	b.WriteString("version: v1\n")
	writeScaffoldID(&b, id)
	b.WriteString("name: ")
	b.WriteString(name)
	// Source/target locales live under `defaults:` — the schema the loader
	// reads (KapiProject.Defaults). Top-level sourceLocale/targetLocales keys
	// are not part of the recipe schema and would be ignored.
	b.WriteString("\ndefaults:\n")
	b.WriteString("  source_language: ")
	b.WriteString(sourceLocale)
	b.WriteByte('\n')
	if len(targetLocales) > 0 {
		b.WriteString("  target_languages:\n")
		for _, t := range targetLocales {
			b.WriteString("    - ")
			b.WriteString(t)
			b.WriteByte('\n')
		}
	}
	// Standing project-context bindings the stack declares — a voice profile
	// and a committed native terms source — so project-scoped voice and
	// terminology checks need no flags.
	if voiceProfile != "" {
		b.WriteString("  voice:\n")
		fmt.Fprintf(&b, "    profile_file: %s\n", voiceProfile)
	}
	if termsSource != "" {
		fmt.Fprintf(&b, "  terms_source: %s\n", termsSource)
	}

	if len(content) > 0 {
		// Bare-entry collection form: each entry maps a source glob to a target
		// glob via the {lang} placeholder. The format is the short (scalar) form.
		b.WriteString("\ncollections:\n")
		for _, c := range content {
			fmt.Fprintf(&b, "  - path: %q\n", c.Path)
			fmt.Fprintf(&b, "    format: %s\n", c.Format)
			if c.Target != "" {
				fmt.Fprintf(&b, "    target: %q\n", c.Target)
			}
		}
		b.WriteString("flows: {}\n")
		return []byte(b.String())
	}

	b.WriteString(`
# Define collections and flows. Each bare collection entry maps a source glob to
# a target; kapi tools read the source content and edit, check, or translate it.
# The {lang} placeholder in a target fans output out per language. Runtime block
# state lives in the project store, .kapi/work/store.db.
#
# collections:
#   - path: "src/locales/en/*.json"
#     format: json
#     target: "src/locales/{lang}/*.json"
#
# A named collection binds the point in the context space its content sits at,
# and its base: is the directory it lives in. Every path and target below is
# written relative to that base and joined onto it:
#
# A voice: binding names a profile this project's store holds. Terms, voice
# profiles, content memory and recorded decisions live in your workspace, and
# 'kapi context import' reads a checkout's context files into it.
#
# profiles:
#   acme:
#     channels: [docs]
#     voice: acme-docs
#
# collections:
#   - name: acme-docs
#     channel: acme/docs
#     base: web
#     content:
#       - path: "docs/**/*.md"          # web/docs/**/*.md
#         format: markdown
#         target: "i18n/{lang}/{path}.md"   # web/i18n/{lang}/{path}.md
#
# flows:
#   # Monolingual: check source content against the voice profile and terminology.
#   voice:
#     steps:
#       - tool: voice-vocab-check
#       - tool: voice-check
#   # Multilingual: translate the source into each target language.
#   translate:
#     steps:
#       - tool: translate
#
# Tip: 'kapi init --framework <stack>' pre-fills collections for a known stack.
collections: []
flows: {}
`)
	return []byte(b.String())
}

// ScaffoldContentRecipe builds the default content recipe: a project whose job
// is keeping its source content in voice and on-terminology, with no target
// languages. It binds a voice profile (a built-in starter pack) under
// defaults: so the project-scoped voice check needs no flags, and
// ships a `check` flow that scores content with the deterministic
// voice-vocabulary check. Passing --target-locale or --framework scaffolds a
// translation project (ScaffoldRecipe) instead.
func ScaffoldContentRecipe(name, id, sourceLocale string) []byte {
	var b strings.Builder
	b.WriteString("version: v1\n")
	writeScaffoldID(&b, id)
	b.WriteString("name: ")
	b.WriteString(name)
	b.WriteString("\ndefaults:\n")
	b.WriteString("  source_language: ")
	b.WriteString(sourceLocale)
	b.WriteByte('\n')
	// voice is a framework binding under defaults: — standing project context
	// for the voice check. Terminology needs no binding: the vocabulary lives
	// in the project's own store, which every term-aware command reads with no
	// flag and no recipe entry. No target_languages: this project governs its
	// source content, it does not translate it.
	b.WriteString("  voice:\n")
	b.WriteString("    pack: professional-b2b\n")
	b.WriteString(`
# Content project: no target_languages. Point collections at the source files
# to keep in voice, then run 'kapi check' to score them. Block state lives
# in the project store, .kapi/work/store.db.
#
# collections:
#   - path: "src/**/*.md"
#     format: markdown
#
# Swap the starter pack for your own profile: 'kapi voice new -o voice.yaml',
# fill it in, 'kapi voice import voice.yaml', then set
# defaults.voice.profile instead of pack. The profile lives in your
# workspace from there on, and 'kapi voice edit' opens it again.
collections: []

# The check flow scores content against the voice vocabulary (deterministic,
# offline). 'kapi check' reports the score and gates on it; 'kapi run check'
# runs the same flow over every tracked file. Add the AI-driven 'voice-check'
# step for tone and voice scoring (needs an AI provider).
flows:
  check:
    steps:
      - tool: voice-vocab-check
      # - tool: voice-check
`)
	return []byte(b.String())
}
