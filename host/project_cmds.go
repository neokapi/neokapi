package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

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
// preset declares — a brand voice profile file and a termbase source — for the
// scaffolder to write under defaults:. Both are empty when the framework
// declares none (or is unknown/empty).
func FrameworkBindings(framework string) (brandVoiceProfile, termbaseSource string) {
	if framework == "" {
		return "", ""
	}
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)
	fp := reg.GetFrameworkPreset(framework)
	if fp == nil {
		return "", ""
	}
	return fp.BrandVoiceProfile, fp.TermbaseSource
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

func ScaffoldRecipe(name, sourceLocale string, targetLocales []string, content []scaffoldContent, brandVoiceProfile, termbaseSource string) []byte {
	var b strings.Builder
	b.WriteString("version: v1\n")
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
	// Standing project-context bindings the stack declares — a brand voice
	// profile and a committed native termbase source — so project-scoped brand
	// and terminology checks need no flags.
	if brandVoiceProfile != "" {
		b.WriteString("  brand_voice:\n")
		fmt.Fprintf(&b, "    profile_file: %s\n", brandVoiceProfile)
	}
	if termbaseSource != "" {
		fmt.Fprintf(&b, "  termbase_source: %s\n", termbaseSource)
	}

	if len(content) > 0 {
		// Bare-entry content form: each entry maps a source glob to a target
		// glob via the {lang} placeholder. The format is the short (scalar) form.
		b.WriteString("\ncontent:\n")
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
# Define content and flows. Each bare content entry maps a source glob to a
# target; kapi tools read the source content and edit, check, or translate it.
# The {lang} placeholder in a target fans output out per language. Runtime block
# state lives in .kapi/cache/blocks.db.
#
# content:
#   - path: "src/locales/en/*.json"
#     format: json
#     target: "src/locales/{lang}/*.json"
#
# flows:
#   # Monolingual: check source content against brand voice and terminology.
#   brand-check:
#     steps:
#       - tool: brand-vocab-check
#       - tool: brand-voice-check
#   # Multilingual: translate the source into each target language.
#   translate:
#     steps:
#       - tool: translate
#
# Tip: 'kapi init --framework <stack>' pre-fills content for a known stack.
content: []
flows: {}
`)
	return []byte(b.String())
}

// ScaffoldContentRecipe builds the default content recipe: a project whose job
// is keeping its source content on brand and on-terminology, with no target
// languages. It binds a brand voice profile (a built-in starter pack) and the
// project termbase under defaults: so project-scoped checks need no flags, and
// ships a `check` flow that scores content with the deterministic
// brand-vocabulary check. Passing --target-locale or --framework scaffolds a
// translation project (ScaffoldRecipe) instead.
func ScaffoldContentRecipe(name, sourceLocale string) []byte {
	var b strings.Builder
	b.WriteString("version: v1\n")
	b.WriteString("name: ")
	b.WriteString(name)
	b.WriteString("\ndefaults:\n")
	b.WriteString("  source_language: ")
	b.WriteString(sourceLocale)
	b.WriteByte('\n')
	// brand_voice and termbase are framework bindings under defaults: — standing
	// project context for brand and terminology checks. No target_languages: this
	// project governs its source content, it does not translate it.
	b.WriteString("  brand_voice:\n")
	b.WriteString("    pack: professional-b2b\n")
	b.WriteString("  termbase: .kapi/termbase.db\n")
	b.WriteString(`
# Content project: no target_languages. Point content at the source files to
# keep on brand, then run 'kapi run check' to score them. Block state lives in
# .kapi/cache/blocks.db.
#
# content:
#   - path: "src/**/*.md"
#     format: markdown
#
# Swap the starter pack for your own profile: 'kapi brand new -o brand.yaml',
# fill it in, 'kapi brand import brand.yaml', then set
# defaults.brand_voice.profile instead of pack.
content: []

# The check flow scores content against brand vocabulary (deterministic,
# offline) and gates on the result. Add the AI-driven 'brand-voice-check' step
# for tone and voice scoring (needs an AI provider).
flows:
  check:
    steps:
      - tool: brand-vocab-check
      # - tool: brand-voice-check
`)
	return []byte(b.String())
}
