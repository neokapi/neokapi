package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// NewSnapshotCmd returns `kapi snapshot` — zip the project (recipe +
// .kapi/) into a `.klz` archive.
func (a *App) NewSnapshotCmd() *cobra.Command {
	var (
		recipeFlag     string
		outPath        string
		includeSources bool
		sourceRoots    []string
		excludeCacheDB bool
	)
	cmd := &cobra.Command{
		Use:     "snapshot",
		Short:   "Zip the project into a .klz archive",
		GroupID: "content",
		Long: `kapi snapshot packages {project}.kapi plus the adjacent .kapi/
state folder into a single .klz archive. The archive's internal
layout is canonical (manifest.yaml at zip root, cache.db at zip
root, collections/** at zip root), independent of the project
folder's user-facing layout.

By default sources are NOT included. Pass --include-sources with
one or more --source-root paths to embed authored content for
fully-self-contained handoff.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			layout, err := resolveLayoutFromFlag(recipeFlag)
			if err != nil {
				return err
			}
			if outPath == "" {
				outPath = filepath.Base(layout.RecipePath)
				outPath = outPath[:len(outPath)-len(project.RecipeExt)] + ".klz"
			}

			f, err := os.Create(outPath)
			if err != nil {
				return fmt.Errorf("create %s: %w", outPath, err)
			}
			defer f.Close()

			opts := project.SnapshotOptions{
				IncludeSources: includeSources,
				SourceRoots:    sourceRoots,
				ExcludeCacheDB: excludeCacheDB,
			}
			if err := project.Snapshot(layout, f, opts); err != nil {
				return fmt.Errorf("snapshot: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", outPath)
			return nil
		},
	}
	cmd.Flags().StringVarP(&recipeFlag, "project", "p", "", "Path to the .kapi recipe (default: walk up)")
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "Output .klz path (default: <project-id>.klz)")
	cmd.Flags().BoolVar(&includeSources, "include-sources", false, "Embed declared source roots in the archive")
	cmd.Flags().StringSliceVar(&sourceRoots, "source-root", nil, "Source directory to include (repeatable, requires --include-sources)")
	cmd.Flags().BoolVar(&excludeCacheDB, "exclude-cache-db", false, "Omit .kapi/cache.db from the archive")
	return cmd
}

// NewOpenCmd returns `kapi open` — unpack a `.klz` archive into a
// fresh working project directory.
func (a *App) NewOpenCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:     "open <archive.klz>",
		Short:   "Unpack a .klz archive into a working project directory",
		GroupID: "content",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			archivePath := args[0]

			info, err := os.Stat(archivePath)
			if err != nil {
				return fmt.Errorf("stat %s: %w", archivePath, err)
			}
			if info.IsDir() {
				return fmt.Errorf("%s is a directory, not a .klz file", archivePath)
			}

			if target == "" {
				// Default target: derive from archive filename.
				base := filepath.Base(archivePath)
				stem := base
				if ext := filepath.Ext(base); ext != "" {
					stem = base[:len(base)-len(ext)]
				}
				target = stem
			}
			absTarget, err := filepath.Abs(target)
			if err != nil {
				return fmt.Errorf("resolve target: %w", err)
			}

			f, err := os.Open(archivePath)
			if err != nil {
				return fmt.Errorf("open %s: %w", archivePath, err)
			}
			defer f.Close()

			layout, err := project.Open(f, info.Size(), absTarget)
			if err != nil {
				return fmt.Errorf("open archive: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Extracted to %s\n", layout.Root)
			fmt.Fprintf(cmd.OutOrStdout(), "  recipe: %s\n", layout.RecipePath)
			fmt.Fprintf(cmd.OutOrStdout(), "  state:  %s\n", layout.StateDir)
			return nil
		},
	}
	cmd.Flags().StringVarP(&target, "into", "C", "", "Target directory (default: archive filename stem)")
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

func resolveLayoutFromFlag(flag string) (project.Layout, error) {
	if flag != "" {
		return project.LayoutFor(flag)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return project.Layout{}, err
	}
	layout, err := project.ResolveLayout(cwd)
	if err != nil {
		if errors.Is(err, project.ErrAmbiguousLayout) {
			return project.Layout{}, errors.New("multiple *.kapi recipes in this directory — pass -p <path> to choose one")
		}
		return project.Layout{}, err
	}
	return layout, nil
}

func scaffoldRecipe(name, sourceLocale string, targetLocales []string) []byte {
	out := "version: v1\n"
	out += "id: " + name + "\n"
	out += "name: " + name + "\n"
	out += "sourceLocale: " + sourceLocale + "\n"
	if len(targetLocales) > 0 {
		out += "targetLocales:\n"
		for _, t := range targetLocales {
			out += "  - " + t + "\n"
		}
	}
	out += `
# Define content collections — sources kapi will extract from, and
# writer outputs for translated files. See docs/kapi-project-model
# for the full schema.
#
# content:
#   - collection: ui
#     store:
#       type: klzdb
#       path: .kapi/cache.db
#     items:
#       - src: src/**/*.{tsx,jsx}
#         format:
#           name: exec
#           config:
#             command: vp kapi-react extract --stream
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
`
	return []byte(out)
}
