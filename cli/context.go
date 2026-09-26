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
		Short:   "Ask what applies before you write a file",
		GroupID: "work",
		Long: `Ask what this project keeps to before you write a file: the voice, the
words to use and to avoid, and what has been suggested but not yet established.

  kapi context <path>          what applies to that file
  kapi context --profile <n>   the same answer for a named profile
  kapi context search <word>   what the project says about one word

The answer is the same text an assistant reads from the context:// resource.
--explain adds how it was reached: the point the file sits at, the recipe line
that bound the voice, and the project and revision that answered. --json gives
all of it as data.

The subcommands record and decide. observe and correct record what you notice,
as suggestions; log lists them; keep, drop, revert and widen are a person's
decisions, and withdraw takes back a suggestion of your own. import, snapshot, export and restore move the context between the
store and files, and locales reports how stored rows are filed.`,
		Example: "  kapi context docs/guide.md\n" +
			"  kapi context docs/guide.md --explain\n" +
			"  kapi context docs/guide.md --json\n" +
			"  kapi context --profile marketing\n" +
			"  kapi context search widget",
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
			if explain, _ := cmd.Flags().GetBool("explain"); explain {
				res.Explain()
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
	cmd.Flags().Int("limit", host.DefaultContextTermsLimit, "max word rules to render")
	cmd.Flags().Bool("explain", false, "add how the answer was reached: point, binding, project, revision and notes")
	// The same project- and store-resolution flags every other project verb
	// carries, so this command resolves the same way `terms` and `memory` do.
	// The output format axis (--json among them) is persistent on the root.
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)

	cmd.AddCommand(
		newContextSearchCmd(a),
		newContextImportCmd(a),
		newContextRebuildCmd(a),
		newContextSnapshotCmd(a),
		newContextExportCmd(a),
		newContextRestoreCmd(a),
		newContextLocalesCmd(a),
		newContextObserveCmd(a),
		newContextCorrectCmd(a),
		newContextLogCmd(a),
		newContextKeepCmd(a),
		newContextDropCmd(a),
		newContextWithdrawCmd(a),
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
		Long: `Read a checkout's context files into this project's store: the terms,
the voice profiles, the wording already approved, and the record of who approved
it.

This is the one command that opens those files. Everything else answers from the
store, so what you read here is in force for every checkout of this project on
this machine, and for everyone else once they run it too. A person runs it: it
is a decision about the project, and it appears in "kapi context log" with each
file and the bytes that were read.

With no path it reads the project's own ".kapi" directory. Name a path to read
another checkout's directory instead, when you are bringing an existing
project's context across.

Running it twice changes nothing. Everything is matched by the identity the file
carries, so a second read finds the store already holding what the file says.
A file whose bytes have not moved since this checkout read it is skipped;
--force reads it anyway.`,
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

func newContextRebuildCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild this project's stores from the context log",
		Long: `Empty this project's terms, content memory and voice profiles, and the rules
it widened to the whole workspace, and write them again from the context log.

Every change to a project's context is recorded in the log with what it wrote,
so the stores hold nothing the log does not. Rebuilding gives the same stores
the changes left behind. Use it when a store is damaged, or after operations from
another machine were merged into the log.

With --checkpoint it keeps a copy of the stores as they stand afterwards, so the
next rebuild starts there and replays only the changes after it.`,
		Example: "  kapi context rebuild\n" +
			"  kapi context rebuild --checkpoint",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			checkpoint, _ := cmd.Flags().GetBool("checkpoint")
			res, err := a.RebuildProjectContext(cmd.Context(), projectPath, checkpoint)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().Bool("checkpoint", false, "keep a checkpoint of the rebuilt stores for the next rebuild to start from")
	AddProjectFlag(cmd)
	return cmd
}

func newContextSnapshotCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Write this project's context out as files",
		Long: `Write the context in force for this project into the directory --out
names: the terms, the voice profiles, the approved wording, and the decision
record, in the layout "kapi context import" reads.

The files it writes are generated. Edit the project's context and snapshot
again rather than editing them by hand, the way you would with any other
generated artifact. A clean checkout that imports what a snapshot wrote governs
its content exactly as the project that wrote it does.

Two things never travel. Withheld originals stay on the machine that redacted
them, and nothing kapi keeps for its own use is written.

The decision record in the snapshot is the project's own, written from what
the ledger holds at the moment of the snapshot.`,
		Example: "  kapi context snapshot --out build/context\n" +
			"  kapi context snapshot --out build/context --json",
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
	cmd.Flags().String("out", "", "directory to write the layout into (required)")
	_ = cmd.MarkFlagRequired("out")
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

The decision record in the file is the project's own, written from what the
ledger holds at the moment of the export.

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

func newContextLocalesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "locales",
		Short: "Report the locale each stored row is filed under",
		Long: `Report the locale spelling every stored row carries, and whether it
is the one lookups ask in.

A row filed under a spelling nothing asks for is never matched. The term, the
approved wording or the overlay it holds reads as absent, which looks exactly
like content nobody has worked on yet.

--fix files the project's context rows under the spelling lookups ask in,
without moving them out of the store. Terms, approved wording and voice
profiles exist there and nowhere else, so nothing is thrown away: a row whose
canonical spelling is free takes it, a row saying exactly what the canonical
row says folds into it, and a row whose canonical spelling already answers
differently is left alone and reported, for you to say which answer is right.

The rows kapi read out of your own files are rebuilt rather than moved. The
report names the file to delete, and the next "kapi up" reads your files
again.`,
		Example: "  kapi context locales\n" +
			"  kapi context locales --json\n" +
			"  kapi context locales --fix",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			fix, _ := cmd.Flags().GetBool("fix")
			res, err := a.ProjectStoreLocales(cmd.Context(), projectPath, fix)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().Bool("fix", false, "file the project's context rows under the spelling lookups ask in")
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
