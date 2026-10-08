package cli

import (
	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
)

// The sharing half of the context surface (AD C-11). A recipe declares where
// the project's context is shared (`context.backend`); sync merges what other
// machines shared and then shares what this one recorded.

func newContextSyncCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Share the context with the rest of the team",
		Long: `Share this project's context through the place kapi.yaml names: first
read what others shared and merge it here, then share what was recorded on
this machine. Everything travels with its history: every suggestion, decision,
term and approved wording, and the rules you applied to every project, which
hold on every machine that syncs the project. Withheld originals stay on this
machine. Running it twice changes nothing.

The place is declared in kapi.yaml, where every checkout reads it:

  context:
    backend: git      # local, file, git or s3

With --merged it also reads the change a range of commits made on the default
branch and counts it as a person's signal: a suggestion whose preferred wording
the change added, or whose avoided wording it removed, becomes a rule when
nothing contradicts it. Run it in CI after a merge. The same range records the
same evidence, so a second run changes nothing. A project that shares its
context nowhere settles the merge on this machine.

--no-push reads what others shared and shares nothing back, for a check or
a fresh checkout that only needs the context to answer from.

--status says where the context is shared and how far this machine and the
shared copy were apart at the last sync, without contacting it.

Once the context is merged, sync rewrites the project's rules files: the
section kapi keeps in AGENTS.md and CLAUDE.md at the project root, and in each
folder whose rules differ from the root's. --files-only writes those files
from the context this machine holds and does nothing else, which also writes
them for a project that has none yet.

It exits with status 5 when the shared copy cannot be reached, and nothing
changes here.`,
		Example: "  kapi context sync\n" +
			"  kapi context sync --status\n" +
			"  kapi context sync --no-push\n" +
			"  kapi context sync --files-only\n" +
			"  kapi context sync --merged HEAD~1..HEAD\n" +
			"  kapi context sync --merged \"$BEFORE..$AFTER\" --pr 412 --merger asgeir",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			merged, _ := cmd.Flags().GetString("merged")
			pr, _ := cmd.Flags().GetInt("pr")
			merger, _ := cmd.Flags().GetString("merger")
			noPush, _ := cmd.Flags().GetBool("no-push")
			filesOnly, _ := cmd.Flags().GetBool("files-only")
			if status, _ := cmd.Flags().GetBool("status"); status {
				res, err := a.ContextBackend(cmd.Context(), projectPath)
				if err != nil {
					return err
				}
				return output.Print(cmd, res)
			}
			res, err := a.SyncProjectContext(cmd.Context(), host.ContextSyncRequest{
				Project:   projectPath,
				Merged:    merged,
				PR:        pr,
				Merger:    merger,
				NoPush:    noPush,
				FilesOnly: filesOnly,
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().Bool("status", false, "say where the context is shared and how far apart this machine is, without syncing")
	cmd.Flags().String("merged", "", "a range of commits that reached the default branch, counted as a person's signal")
	cmd.Flags().Int("pr", 0, "the pull request that merged the range (read from the commit subject when unset)")
	cmd.Flags().String("merger", "", "who merged it (the last commit's committer when unset)")
	cmd.Flags().Bool("no-push", false, "read what others shared and share nothing back")
	cmd.Flags().Bool("files-only", false, "write the project's rules files (AGENTS.md, CLAUDE.md) from this machine's context and do nothing else")
	cmd.MarkFlagsMutuallyExclusive("status", "merged")
	cmd.MarkFlagsMutuallyExclusive("files-only", "status")
	cmd.MarkFlagsMutuallyExclusive("files-only", "merged")
	cmd.MarkFlagsMutuallyExclusive("files-only", "no-push")
	cmd.MarkFlagsMutuallyExclusive("status", "no-push")
	AddProjectFlag(cmd)
	return cmd
}
