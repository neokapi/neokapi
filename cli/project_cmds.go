package cli

import (
	"fmt"

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
		noPointer    bool
	)
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Scaffold a new kapi project in the current directory",
		GroupID: "work",
		Long: `Create a new kapi project with a kapi.yaml recipe and an
adjacent .kapi/ state directory.

By default kapi init scaffolds a content project that keeps your source in
voice: a voice profile, the project terms store, and a check flow, with no
target languages. Pass --target-locale (or --framework) to make it a
translation project instead.

The project id defaults to the current directory's basename and the source
locale to en. Override with --name, --source-locale, --target-locale
(repeatable).

--preset <name> (alias: --framework) pre-fills the content mapping for a known
stack's i18n catalogs: react-i18next, react-intl, nextjs, vue-i18n, flutter,
angular, and scaffolds the translation project. List every preset (framework
scaffolds plus per-format parsing presets) with --list-presets.

When the project binds a voice profile, kapi init also writes a short section
into the project's assistant file, so an assistant working in the tree knows
the voice is held by kapi and retrieves it with 'kapi voice guide' before
writing. An existing AGENTS.md or CLAUDE.md at the root takes the section
(AGENTS.md when both exist); with neither, kapi init creates AGENTS.md, which a
CLAUDE.md picks up through an import line (@AGENTS.md). Re-running replaces
the section in place and leaves the rest of the file alone. --no-pointer skips
it; 'kapi voice pointer' writes it later.`,
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

			// `kapi init` is idempotent: re-running it (or running it on a
			// project that already has a recipe) is not an error. This lets
			// plugin contributions (e.g. connecting an existing kapi project to
			// a server) run on top of `kapi init` without a separate command —
			// `kapi init --server …` on an existing project just connects it.
			//
			// The composition (write recipe → EnsureLayout → SaveState) lives in
			// host.InitProject so Kapi Desktop creates projects the same way.
			res, err := InitProject(root, InitOptions{
				Name:          name,
				SourceLocale:  sourceLocale,
				TargetLocales: targetLocale,
				Framework:     framework,
			})
			if err != nil {
				return err
			}

			if res.AlreadyInitialized {
				fmt.Fprintf(cmd.OutOrStdout(), "kapi project already initialized: %s\n", res.RecipePath)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized kapi project %q\n", res.Name)
				fmt.Fprintf(cmd.OutOrStdout(), "  recipe: %s\n", res.RecipePath)
				fmt.Fprintf(cmd.OutOrStdout(), "  state:  %s\n", res.StateDir)
			}
			if noPointer {
				return nil
			}

			// The pointer is what tells an assistant standing in this tree
			// that the project has a voice. It follows the scaffold rather
			// than being part of it because naming the voice may need the
			// project store, which InitProject has no reason to open. A
			// pointer that cannot be written is reported and does not undo
			// an init that succeeded.
			ptr, perr := a.WriteVoicePointer(CmdContext(cmd), root)
			if perr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: voice pointer: %v\n", perr)
				return nil
			}
			printInitPointer(cmd, ptr, res.AlreadyInitialized)
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
	cmd.Flags().BoolVar(&noPointer, "no-pointer", false, "Do not write the voice pointer into AGENTS.md or CLAUDE.md")
	cmd.MarkFlagsMutuallyExclusive("preset", "framework")
	return cmd
}

// printInitPointer reports what init did to the assistant file. On a fresh
// project every outcome that wrote something is listed beside the recipe and
// the state directory; on a re-run only a change is worth a line.
func printInitPointer(cmd *cobra.Command, ptr *VoicePointerResult, rerun bool) {
	w := cmd.OutOrStdout()
	switch ptr.Action {
	case VoicePointerCreated:
		fmt.Fprintf(w, "  agents: %s (voice pointer for assistants; a CLAUDE.md picks it up with @AGENTS.md)\n", ptr.File)
	case VoicePointerUpdated:
		fmt.Fprintf(w, "  agents: %s (voice pointer written)\n", ptr.File)
	case VoicePointerUnchanged:
		if !rerun {
			fmt.Fprintf(w, "  agents: %s (voice pointer current)\n", ptr.File)
		}
	case VoicePointerRemoved:
		fmt.Fprintf(w, "  agents: %s (voice pointer removed: no voice bound)\n", ptr.File)
	}
	if ptr.Warning != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: voice pointer could not name the voice: %s\n", ptr.Warning)
	}
}
