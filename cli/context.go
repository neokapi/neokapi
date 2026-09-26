package cli

import (
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
decisions, and withdraw takes back a suggestion of your own. pull and push
share the context through the backend kapi.yaml declares; export and import
carry it in one file; locales reports how stored rows are filed.`,
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
		newContextPullCmd(a),
		newContextPushCmd(a),
		newContextBackendCmd(a),
		newContextExportCmd(a),
		newRetiredContextCmd("snapshot", "the project's context is shared with `kapi context push` and read with `kapi context pull`, "+
			"or carried in one file with `kapi context export -o <file>.kpz`"),
		newRetiredContextCmd("restore", "read a file written by `kapi context export` with `kapi context import <file>.kpz`"),
		newContextLocalesCmd(a),
		newContextObserveCmd(a),
		newContextCorrectCmd(a),
		newContextLogCmd(a),
		newContextDigestCmd(a),
		newContextKeepCmd(a),
		newContextDropCmd(a),
		newContextWithdrawCmd(a),
		newContextRevertCmd(a),
		newContextWidenCmd(a),
		newContextSettleCmd(a),
	)
	return cmd
}

// The portability half of the context surface (AD C-03). Import reads context
// files a person wrote (a terms bundle, a voice profile) into operations; import
// of a .kpz and export move the whole log in one transfer file.

func newContextImportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [path | file.kpz]",
		Short: "Read context files, or a context file, into this project",
		Long: `Read a checkout's context files into this project's store: the terms,
the voice profiles, the wording already approved, and the record of who approved
it.

Name a .kpz written by "kapi context export" to merge it instead, the way
"kapi context pull" merges a backend: every operation it carries that this
project does not hold, with its history.

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
			"  kapi context import --force\n" +
			"  kapi context import context.kpz",
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
			if host.IsKpzPath(dir) {
				res, err := a.ImportContextFile(cmd.Context(), projectPath, dir)
				if err != nil {
					return err
				}
				return output.Print(cmd, res)
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
		Args: cobra.NoArgs,
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

func newContextExportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write this project's context and its history to one file",
		Long: `Write this project's context to one file: every suggestion, decision,
term, voice profile and approved wording, with the history that produced them,
and a checkpoint so reading it back is fast.

It is the backup, and the way to carry a project's context to a machine that
cannot reach its context backend. "kapi context import <file>.kpz" merges it
the way "kapi context pull" merges a backend, so reading one file twice, or
into a machine that already holds part of it, changes nothing more.

Withheld originals are never in it, and neither are the rules you widened to
every project: both stay on this machine.`,
		Example: "  kapi context export -o context.kpz\n" +
			"  kapi context export -o backups/acme-context.kpz --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			out, _ := cmd.Flags().GetString("output")
			res, err := a.ExportProjectContext(cmd.Context(), projectPath, out)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().StringP("output", "o", "", "file to write (required)")
	_ = cmd.MarkFlagRequired("output")
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

kapi files every row under the spelling lookups ask in as it writes it. A
context row under another spelling is written again canonically by "kapi
context rebuild", which rewrites the project's stores from the operation log.
The rows kapi read out of your own files are rebuilt from the files: the report
names the file to delete, and the next "kapi up" reads your files again.`,
		Example: "  kapi context locales\n" +
			"  kapi context locales --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			res, err := a.ProjectStoreLocales(cmd.Context(), projectPath)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
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
