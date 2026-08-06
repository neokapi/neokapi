package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
)

// NewInitCmd returns `kapi init` — scaffold a new kapi project in
// the current directory (or `--dir <path>`). Creates `kapi.yaml`
// + `.kapi/` adjacent to it. Idempotent; re-running on an existing
// project adopts it rather than erroring.
func NewInitCmd(a *App) *cobra.Command {
	var (
		dir          string
		name         string
		sourceLocale string
		targetLocale []string
		framework    string
		presetName   string
		listPresets  bool
	)
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Scaffold a new kapi project in the current directory",
		GroupID: "work",
		Long: `Create a new kapi project with a kapi.yaml recipe and an
adjacent .kapi/ state directory.

By default kapi init scaffolds a content project that keeps your source on
brand — a brand-voice profile, the project terms store, and a check flow, with no
target languages. Pass --target-locale (or --framework) to make it a
translation project instead.

The project id defaults to the current directory's basename and the source
locale to en. Override with --name, --source-locale, --target-locale
(repeatable).

--preset <name> (alias: --framework) pre-fills the content mapping for a known
stack's i18n catalogs: react-i18next, react-intl, nextjs, vue-i18n, flutter,
angular — and scaffolds the translation project. List every preset (framework
scaffolds plus per-format parsing presets) with --list-presets.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --list-presets: print the preset catalog and exit (absorbs the
			// former `kapi presets list`, #1078 C1).
			if listPresets {
				return PrintPresetList(cmd)
			}
			// --preset is the porcelain spelling of --framework.
			if presetName != "" {
				framework = presetName
			}
			root, err := ResolveDir(dir)
			if err != nil {
				return err
			}
			if name == "" {
				name = filepath.Base(root)
			}
			if sourceLocale == "" {
				sourceLocale = "en"
			}

			content, err := FrameworkContent(framework)
			if err != nil {
				return err
			}
			voiceProfile, termsSource := FrameworkBindings(framework)

			// `kapi init` is idempotent: re-running it (or running it on a
			// project that already has a recipe) is not an error. This lets
			// plugin contributions (e.g. connecting an existing kapi project to
			// a server) run on top of `kapi init` without a separate command —
			// `kapi init --server …` on an existing project just connects it.
			//
			// The recipe filename is fixed (kapi.yaml), so re-running can never
			// scaffold a second recipe: when one is already here we adopt it and
			// leave it untouched. The project label lives in the recipe's name:
			// field; --name only affects a fresh scaffold.
			recipeExists, err := RecipeExists(root)
			if err != nil {
				return fmt.Errorf("check for existing project: %w", err)
			}

			recipePath := filepath.Join(root, project.RecipeFileName)
			stateDir := filepath.Join(root, project.StateDirName)

			if recipeExists {
				fmt.Fprintf(cmd.OutOrStdout(), "kapi project already initialized: %s\n", recipePath)
			} else {
				// On-brand content is the default. --target-locale or
				// --framework opts into the translation scaffold; otherwise
				// scaffold the content project (voice profile + terms + check).
				var recipe []byte
				if len(targetLocale) > 0 || framework != "" {
					recipe = ScaffoldRecipe(name, sourceLocale, targetLocale, content, voiceProfile, termsSource)
				} else {
					recipe = ScaffoldContentRecipe(name, sourceLocale)
				}
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
	cmd.Flags().StringVar(&framework, "framework", "", "Pre-fill content mapping for a known stack (see 'kapi init --list-presets'); scaffolds a translation project")
	cmd.Flags().StringVar(&presetName, "preset", "", "Scaffold from a named framework preset (see 'kapi init --list-presets'); alias of --framework")
	cmd.Flags().BoolVar(&listPresets, "list-presets", false, "List available presets (framework scaffolds and per-format parsing presets) and exit")
	cmd.MarkFlagsMutuallyExclusive("preset", "framework")
	return cmd
}
