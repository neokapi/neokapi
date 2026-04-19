package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
)

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
	)
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Scaffold a new kapi project in the current directory",
		GroupID: "content",
		Long: `Create a new kapi project with a {name}.kapi recipe and an
adjacent .kapi/ state directory.

By default the project id is the current directory's basename and
source/target locales are en / (none). Override with --name,
--source-locale, --target-locale (repeatable).`,
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

			recipePath := filepath.Join(root, name+project.RecipeExt)
			stateDir := filepath.Join(root, project.StateDirName)

			if _, err := os.Stat(recipePath); err == nil {
				return fmt.Errorf("refusing to overwrite existing %s", recipePath)
			}
			if _, err := os.Stat(stateDir); err == nil {
				return fmt.Errorf("refusing to overwrite existing %s", stateDir)
			}

			recipe := scaffoldRecipe(name, sourceLocale, targetLocale)
			if err := os.WriteFile(recipePath, recipe, 0o644); err != nil {
				return fmt.Errorf("write recipe: %w", err)
			}

			layout := project.Layout{Root: root, RecipePath: recipePath, StateDir: stateDir}
			if err := project.EnsureLayout(layout); err != nil {
				return fmt.Errorf("create state dir: %w", err)
			}
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
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Directory to scaffold in (default: current directory)")
	cmd.Flags().StringVar(&name, "name", "", "Project id/name (default: directory basename)")
	cmd.Flags().StringVar(&sourceLocale, "source-locale", "en", "Source locale (BCP-47)")
	cmd.Flags().StringSliceVar(&targetLocale, "target-locale", nil, "Target locale (repeatable)")
	return cmd
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

func scaffoldRecipe(name, sourceLocale string, targetLocales []string) []byte {
	var b strings.Builder
	b.WriteString("version: v1\n")
	b.WriteString("id: ")
	b.WriteString(name)
	b.WriteString("\nname: ")
	b.WriteString(name)
	b.WriteString("\nsourceLocale: ")
	b.WriteString(sourceLocale)
	b.WriteByte('\n')
	if len(targetLocales) > 0 {
		b.WriteString("targetLocales:\n")
		for _, t := range targetLocales {
			b.WriteString("  - ")
			b.WriteString(t)
			b.WriteByte('\n')
		}
	}
	b.WriteString(`
# Define content collections and flows. Kapi tools read
# authored content from the declared source globs and write
# generated translations to the declared writer paths. Runtime
# block state is persisted in .kapi/cache.db.
#
# content:
#   - collection: ui
#     items:
#       - src: src/**/*.{tsx,jsx}
#     writers:
#       - format: json
#         out: i18n/{locale}.json
#
# flows:
#   translate:
#     steps:
#       - extract
#       - ai-translate
content: []
flows: {}
`)
	return []byte(b.String())
}
