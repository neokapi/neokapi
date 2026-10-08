package cli

import (
	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
)

// `kapi store` maintains the store a project's context lives in (AD C-03):
// carrying it in one file, reading context files into it, rebuilding it from
// its log, and reporting how its rows are filed. None of it is part of writing
// or deciding, so it sits apart from `kapi context`.

// NewStoreCmd creates the store maintenance command group.
func NewStoreCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "store",
		Short:   "Maintain this project's context store",
		GroupID: "advanced",
		Long: `Maintain the store this project's context lives in: the terms, the
approved wording, the voice profiles and the history of every decision.

  kapi store import    read context files, or a file written by export, into it
  kapi store export    write it and its history to one file
  kapi store rebuild   rebuild it from its history
  kapi store locales   report the language each stored row is filed under`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newStoreImportCmd(a),
		newStoreExportCmd(a),
		newStoreRebuildCmd(a),
		newStoreLocalesCmd(a),
	)
	return cmd
}

func newStoreImportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [path | file.kpz]",
		Short: "Read context files, or a file written by export, into this project",
		Long: `Read a checkout's context files into this project's store: the terms,
the voice profiles, the wording already approved, and the record of who approved
it.

Name a .kpz written by "kapi store export" to merge it instead, the way
"kapi context sync" merges what others shared: everything it carries that this
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
		Example: "  kapi store import\n" +
			"  kapi store import ../other-project/.kapi\n" +
			"  kapi store import --force\n" +
			"  kapi store import context.kpz",
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

func newStoreRebuildCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild this project's store from its history",
		Long: `Empty this project's terms, content memory and voice profiles, and the rules
it applied to the whole workspace, and write them again from the history of
every change.

Every change to a project's context is recorded in that history with what it
wrote, so the store holds nothing the history does not. Rebuilding gives the
same store the changes left behind. Use it when the store is damaged, or after
changes from another machine were merged in.

With --checkpoint it keeps a copy of the store as it stands afterwards, so the
next rebuild starts there and replays only the changes after it.`,
		Example: "  kapi store rebuild\n" +
			"  kapi store rebuild --checkpoint",
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
	cmd.Flags().Bool("checkpoint", false, "keep a checkpoint of the rebuilt store for the next rebuild to start from")
	AddProjectFlag(cmd)
	return cmd
}

func newStoreExportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write this project's context and its history to one file",
		Long: `Write this project's context to one file: every suggestion, decision,
term, voice profile and approved wording, with the history that produced them,
and a checkpoint so reading it back is fast.

It is the backup, and the way to carry a project's context to a machine that
cannot reach the place it is shared. "kapi store import <file>.kpz" merges it
the way "kapi context sync" merges what others shared, so reading one file
twice, or into a machine that already holds part of it, changes nothing more.

Withheld originals are never in it: they stay on this machine. The rules you
applied to every project are in it, and hold on the machine that reads it.`,
		Example: "  kapi store export -o context.kpz\n" +
			"  kapi store export -o backups/acme-context.kpz --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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

func newStoreLocalesCmd(a *App) *cobra.Command {
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
store rebuild", which rewrites the project's store from its history. The rows
kapi read out of your own files are rebuilt from the files: the report names
the file to delete, and the next "kapi up" reads your files again.`,
		Example: "  kapi store locales\n" +
			"  kapi store locales --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
