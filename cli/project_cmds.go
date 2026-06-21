package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
)

// scaffoldContent is one content mapping written into a scaffolded recipe.
type scaffoldContent struct {
	Path   string
	Format string
	Target string
}

// NewInitCmd returns `kapi init` — scaffold a new kapi project in
// the current directory (or `--dir <path>`). Creates `{name}.kapi`
// + `.kapi/` adjacent to it. Does nothing destructive; aborts if
// either target already exists.
func (a *App) NewInitCmd() *cobra.Command {
	var (
		dir          string
		name         string
		sourceLocale string
		targetLocale []string
		framework    string
	)
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Scaffold a new kapi project in the current directory",
		GroupID: "content",
		Long: `Create a new kapi project with a {name}.kapi recipe and an
adjacent .kapi/ state directory.

By default the project id is the current directory's basename and
source/target locales are en / (none). Override with --name,
--source-locale, --target-locale (repeatable).

--framework <name> pre-fills the content mapping for a known stack's i18n
catalogs (see 'kapi presets list --framework'): react-i18next, react-intl,
nextjs, vue-i18n, flutter, angular.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveDir(dir)
			if err != nil {
				return err
			}
			if name == "" {
				name = filepath.Base(root)
			}
			if sourceLocale == "" {
				sourceLocale = "en"
			}

			content, err := frameworkContent(framework)
			if err != nil {
				return err
			}

			// `kapi init` is idempotent: re-running it (or running it on a
			// project that already has a recipe) is not an error. This lets
			// plugin contributions (e.g. connecting an existing kapi project to
			// a server) run on top of `kapi init` without a separate command —
			// `kapi init --server …` on an existing project just connects it.
			//
			// When the directory already hosts a project we adopt that recipe
			// (so contributions run against it). The one hard error is an
			// explicit --name that names a *different* project than the one
			// already here — that would scaffold a second recipe alongside it.
			recipeExists := false
			if existing, err := existingRecipeName(root); err == nil && existing != "" {
				existingName := strings.TrimSuffix(existing, project.RecipeExt)
				if cmd.Flags().Changed("name") && name != existingName {
					return fmt.Errorf("directory already contains a kapi project (%s); run from a clean directory or use --name %s", existing, existingName)
				}
				name = existingName
				recipeExists = true
			}

			recipePath := filepath.Join(root, name+project.RecipeExt)
			stateDir := filepath.Join(root, project.StateDirName)

			if recipeExists {
				fmt.Fprintf(cmd.OutOrStdout(), "kapi project already initialized: %s\n", recipePath)
			} else {
				recipe := scaffoldRecipe(name, sourceLocale, targetLocale, content)
				if err := os.WriteFile(recipePath, recipe, 0o644); err != nil {
					return fmt.Errorf("write recipe: %w", err)
				}
			}

			// EnsureLayout/SaveState are safe to run on an existing layout.
			layout := project.Layout{Root: root, RecipePath: recipePath, StateDir: stateDir}
			if err := project.EnsureLayout(layout); err != nil {
				return fmt.Errorf("create state dir: %w", err)
			}
			if !recipeExists {
				if err := project.SaveState(layout, &project.StateManifest{
					Generator: project.StateGenerator{ID: "kapi", Version: version.Version},
					Project: project.StateProjectRef{
						ID:   name,
						Path: "../" + filepath.Base(recipePath),
					},
				}); err != nil {
					return fmt.Errorf("write state manifest: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized kapi project %q\n", name)
				fmt.Fprintf(cmd.OutOrStdout(), "  recipe: %s\n", recipePath)
				fmt.Fprintf(cmd.OutOrStdout(), "  state:  %s\n", stateDir)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Directory to scaffold in (default: current directory)")
	cmd.Flags().StringVar(&name, "name", "", "Project id/name (default: directory basename)")
	cmd.Flags().StringVar(&sourceLocale, "source-locale", "en", "Source locale (BCP-47)")
	cmd.Flags().StringSliceVar(&targetLocale, "target-locale", nil, "Target locale (repeatable)")
	cmd.Flags().StringVar(&framework, "framework", "", "Pre-fill content mapping for a known stack (see 'kapi presets list --framework')")
	return cmd
}

// frameworkContent resolves a framework preset's catalog mappings into scaffold
// content entries. Returns nil for an empty framework. The kapi-react stack is
// rejected with guidance because it manages i18n via its own bundler plugin and
// `kapi-react extract|compile`, not a .kapi content mapping.
func frameworkContent(framework string) ([]scaffoldContent, error) {
	if framework == "" {
		return nil, nil
	}
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)

	if framework == preset.KapiReactPresetName {
		return nil, fmt.Errorf("the %q stack manages i18n via its bundler plugin and `kapi-react extract|compile`, not a .kapi content mapping — "+
			"install @neokapi/kapi-react and follow the kapi-react quickstart (see the kapi-i18n skill). "+
			"`kapi init --framework` is for catalog-based stacks", framework)
	}

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

// ─── helpers ────────────────────────────────────────────────────

func resolveDir(flag string) (string, error) {
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

// existingRecipeName returns the base name of the first *.kapi recipe directly
// in dir, or "" if none exists. Used to detect an already-initialized project.
func existingRecipeName(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == project.RecipeExt {
			return e.Name(), nil
		}
	}
	return "", nil
}

func scaffoldRecipe(name, sourceLocale string, targetLocales []string, content []scaffoldContent) []byte {
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
