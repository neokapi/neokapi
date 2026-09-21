package cli

import (
	"errors"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// The context retrieval surface (AD-037), CLI half.
//
// Retrieval is addressed by LOCATION (`kapi context <path>`, the `context://`
// resources) or by CONTENT (`kapi context search`, the `context_search` tool),
// never by store. Each half is two wrappers over one host function. That is
// deliberate: the surfaces drifted before — six CLI retrieval verbs against
// three MCP tools, with no rule for which got exposed — and the agent skill
// drives the CLI, so a CLI-only capability teaches an assistant a surface that
// MCP does not have.

// NewContextCmd creates the context retrieval command group, which is also the
// by-location verb: `kapi context <path>` answers what applies at a place.
func NewContextCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context [path]",
		Short:   "Ask what this project's content context says",
		GroupID: "work",
		Long: `Retrieve the content context this project goes by: the terms, the
voice and the rules that hold, without needing to know which store holds the
answer.

Two questions, and every asset-shaped lookup is one of them:

  kapi context <path>          what applies HERE: the voice in force at that
                               location, its guidance, the terms bound there
  kapi context --profile <n>   the same answer for a named profile, when you
                               have no file in hand
  kapi context search <query>  what we know about THIS: terms, prior wording

Communication is contextual: a legal notice is not a help article. Ask what the
project says before you write, rather than learning it from a failing check
afterwards.

Four more verbs move the context itself: import and snapshot carry it between
the project's files and its store, export and restore carry the whole of it as
one file, and with --workspace those two carry every project you work on here.

Six more grow it. Context accumulates out of ordinary work: propose records a
rule with the evidence behind it, log shows what has been proposed and decided,
and confirm, discard, revert and widen are the decisions. A proposal advises
from the moment it is recorded and no check fails on one; confirming is what
makes it bind.`,
		Example: "  kapi context docs/guide.md\n" +
			"  kapi context docs/guide.md --json\n" +
			"  kapi context --profile marketing\n" +
			"  kapi context search widget\n" +
			"  kapi context snapshot --out build/context",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			profile, _ := cmd.Flags().GetString("profile")
			if path == "" && profile == "" {
				return cmd.Help()
			}

			locale, _ := cmd.Flags().GetString("locale")
			limit, _ := cmd.Flags().GetInt("limit")
			req := host.ContextPointRequest{
				Path:    path,
				Profile: profile,
				Locale:  model.LocaleID(locale),
				Limit:   limit,
			}

			// One assembly for both this verb and the `context://` MCP
			// resources (host.ContextSourcesAt).
			src, cleanup := a.ContextSourcesAt(cmd, req)
			defer cleanup()

			res, err := host.ResolveContextAt(cmd.Context(), src, req)
			if err != nil {
				return err
			}
			// The answer renders itself (host.ContextAnswer implements
			// output.TextFormatter), so the markdown a reader sees here is the
			// body the MCP resource serves, and --json is the same document the
			// resource's application/json rendering carries.
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("profile", "", "answer for a named profile instead of a location")
	cmd.Flags().StringP("locale", "l", "", "narrow the reported terms to one language")
	cmd.Flags().Int("limit", host.DefaultContextTermsLimit, "max terms to render")
	// The same project- and store-resolution flags every other project verb
	// carries, so this command resolves the same way `terms` and `memory` do.
	// The output format axis (--json among them) is persistent on the root.
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)

	cmd.AddCommand(
		newContextSearchCmd(a),
		newContextImportCmd(a),
		newContextSnapshotCmd(a),
		newContextExportCmd(a),
		newContextRestoreCmd(a),
		newContextObserveCmd(a),
		newContextProposeCmd(a),
		newContextCorrectCmd(a),
		newContextLogCmd(a),
		newContextConfirmCmd(a),
		newContextDiscardCmd(a),
		newContextRevertCmd(a),
		newContextWidenCmd(a),
	)
	return cmd
}

// The portability half of the context surface (AD C-03). The store holds the
// context a project goes by; these four verbs move it.
//
// Import and snapshot are the two directions between the store and the `.kapi/`
// files a project commits: import reads them, snapshot writes them. Export and
// restore are the same two directions against one archive, for a backup or a
// move between machines. All four go through the importers and exporters the
// rest of kapi already uses, so a file written here is a file `kapi up` reads.

func newContextImportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [path]",
		Short: "Read committed context files into this project's store",
		Long: `Read a project's committed context into its store: the terms, the
voice profiles, the wording already approved, and the record of who approved it.

With no path it reads the project's own ".kapi" directory, which is what a run
does for you on every "kapi up". Name a path to read another checkout's
directory instead, when you are bringing an existing project's context across.

Running it twice changes nothing. Everything is matched by the identity the file
carries, so a second read finds the store already holding what the file says.
A file whose bytes have not moved since the last read is skipped; --force reads
it anyway.`,
		Example: "  kapi context import\n" +
			"  kapi context import ../other-project/.kapi\n" +
			"  kapi context import --force",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			dir := ""
			if len(args) == 1 {
				dir = args[0]
			}
			force, _ := cmd.Flags().GetBool("force")
			res, err := a.ImportProjectContext(cmd.Context(), projectPath, host.ContextImportRequest{
				Dir:   dir,
				Force: force,
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().Bool("force", false, "read every file, including ones whose bytes have not moved")
	AddProjectFlag(cmd)
	return cmd
}

func newContextSnapshotCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Write this project's context back out as files",
		Long: `Write the context in force for this project into a ".kapi"
directory: the terms, the voice profiles at the paths they are bound from, the
approved wording, and the decision record.

The files it writes are generated. Edit the project's context and snapshot
again rather than editing them by hand, the way you would with any other
generated artifact.

With no --out it writes the project's own ".kapi" directory. A clean clone that
holds only what a snapshot wrote governs its content exactly as the project that
wrote it does.

Two things never travel. Withheld originals stay on the machine that redacted
them, and nothing kapi keeps for its own use is written.

The project's committed record is written first, the same write "kapi commit"
makes, so the record in the snapshot is the project's.`,
		Example: "  kapi context snapshot\n" +
			"  kapi context snapshot --out build/context\n" +
			"  kapi context snapshot --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			out, _ := cmd.Flags().GetString("out")
			res, err := a.SnapshotProjectContext(cmd.Context(), projectPath, host.ContextSnapshotRequest{Out: out})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("out", "", "directory to write the layout into (default: the project's own)")
	AddProjectFlag(cmd)
	return cmd
}

func newContextExportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write this project's whole context to one file",
		Long: `Write everything this project's store holds as context into a
single file: the terms, the voice profiles, the wording already approved, and
the decision record.

It is the backup, and the way to move a project's context to another machine.
Nothing is lost and nothing is flattened: every part travels in the form kapi
reads it back from, and "kapi context restore" puts it into a store with the
same identities it left with.

Withheld originals are never in it. They stay on the machine that redacted them,
and no flag puts them in a file meant to be copied.

The project's committed record is written first, the same write "kapi commit"
makes.

With --workspace it writes every project you have worked on here instead of
this one, which is the backup for the context of a whole machine. Add --dry-run
to see what that file would hold before you write it.`,
		Example: "  kapi context export -o context.kpz\n" +
			"  kapi context export -o backups/acme-context.kpz --json\n" +
			"  kapi context export --workspace --dry-run\n" +
			"  kapi context export --workspace -o backups/everything.kpz",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, _ := cmd.Flags().GetString("output")
			all, _ := cmd.Flags().GetBool("workspace")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			if all {
				res, err := a.ExportWorkspaceContext(cmd.Context(), host.ContextWorkspaceExportRequest{
					Out:    out,
					DryRun: dryRun,
				})
				if err != nil {
					return err
				}
				return output.Print(cmd, res)
			}
			if dryRun {
				return errors.New("--dry-run reports what a whole-workspace export would carry; pass it with --workspace")
			}
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			res, err := a.ExportProjectContext(cmd.Context(), projectPath, out)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().StringP("output", "o", "", "file to write")
	cmd.Flags().Bool("workspace", false, "write every project you have worked on here, not just this one")
	cmd.Flags().Bool("dry-run", false, "with --workspace, report what the file would hold and write nothing")
	AddProjectFlag(cmd)
	return cmd
}

func newContextRestoreCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <bundle>",
		Short: "Read a context file back into this project's store",
		Long: `Read a file written by "kapi context export" into this project's
store, with every identity it left with.

A store that already holds context is left alone until you say what should
happen to it. --merge reads the file over what is there, which is idempotent:
restoring the same file twice leaves the same store. --replace puts the file in
place of what is there, so what is left is the file and nothing else.

With --workspace it reads a whole-machine backup: every project in the file
comes back under the name and identity it had, whether or not you have a
checkout of it here. The same two flags apply, and --replace names every
project whose context it is about to empty before it empties any of them.`,
		Example: "  kapi context restore context.kpz\n" +
			"  kapi context restore context.kpz --merge\n" +
			"  kapi context restore context.kpz --replace\n" +
			"  kapi context restore --workspace backups/everything.kpz",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			merge, _ := cmd.Flags().GetBool("merge")
			replace, _ := cmd.Flags().GetBool("replace")
			if merge && replace {
				return errors.New("--merge and --replace ask for different things; pass one")
			}
			mode := host.RestoreRefuse
			switch {
			case replace:
				mode = host.RestoreReplace
			case merge:
				mode = host.RestoreMerge
			}

			if all, _ := cmd.Flags().GetBool("workspace"); all {
				res, err := a.RestoreWorkspaceContext(cmd.Context(), host.ContextWorkspaceRestoreRequest{
					Bundle: args[0],
					Mode:   mode,
					// The notice a replace prints goes to the error stream, so
					// it reaches a person watching without joining the record
					// --json is read for.
					Notice: cmd.ErrOrStderr(),
				})
				if err != nil {
					return err
				}
				return output.Print(cmd, res)
			}
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			res, err := a.RestoreProjectContext(cmd.Context(), projectPath, args[0], mode)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().Bool("merge", false, "read the file over the context the store already holds")
	cmd.Flags().Bool("replace", false, "put the file in place of the context the store already holds")
	cmd.Flags().Bool("workspace", false, "read a whole-machine backup, every project in it")
	AddProjectFlag(cmd)
	return cmd
}

func newContextSearchCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Ask what the project's context says about a word or phrase",
		Long: `Ask one question and get the answer from every store the project
binds: what this is called and whether it is discouraged, and wording the
project has already approved.

Results are grouped by kind rather than merged into one ranked list, because a
terminology match and a wording match are not scored on comparable things.

Each term also reports how often the project's extracted content uses it. The
count is read from the context graph that extraction writes, so it is as of the
last extraction (the last "kapi up") rather than of the working tree: a term
added to the store since then shows no uses until the next run.`,
		Example: "  kapi context search widget\n" +
			"  kapi context search \"sign in\" --locale en\n" +
			"  kapi context search widget --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			locale, _ := cmd.Flags().GetString("locale")
			limit, _ := cmd.Flags().GetInt("limit")

			// One assembly for both this verb and the context_search MCP tool
			// (host.ContextSearchSourcesFor). The CLI passes no standalone paths,
			// so it takes the store the --name/--file/--local flags resolve to.
			src, cleanup := a.ContextSearchSourcesFor(cmd, "", "")
			defer cleanup()

			res, err := host.SearchContext(cmd.Context(), src, host.ContextSearchRequest{
				Query:  args[0],
				Locale: model.LocaleID(locale),
				Limit:  limit,
			})
			if err != nil {
				return err
			}
			// The result renders itself (host.ContextSearchResult implements
			// output.TextFormatter), so --json emits exactly the shape the MCP
			// tool returns and the text form is defined in one place.
			return output.Print(cmd, res)
		},
	}

	cmd.Flags().StringP("locale", "l", "", "narrow results to one language")
	cmd.Flags().Int("limit", host.DefaultContextSearchLimit, "max results per group")
	// The same project- and store-resolution flags every other project verb
	// carries. Without them this command would resolve stores differently from
	// `terms` and `memory` — a parity break inside the CLI, before MCP even
	// enters it — and it could only ever be pointed at a project by standing in
	// it, which no run under the isolation contract can do.
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)
	return cmd
}
