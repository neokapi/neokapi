package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
)

// NewShareCmd creates `kapi share`: how far the project's rules reach, and a
// sync through the place they are shared. It is the person's spelling of
// `kapi context sync`, which stays the assistant's and CI's; --via writes the
// `context:` block the desktop's Share section writes. The assembly is
// host.Share.
func NewShareCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "share [location]",
		Short:   "Share the rules with your team",
		GroupID: "work",
		Long: `Share this project's rules, terms and decisions with the people who work on
it, and read what they shared.

The rules reach as far as one of three rungs:

  On this machine      your rules, your assistants
  With your team       a shared folder or git
  Managed on Bowrain   a team queue, who kept what, the same screens in a browser

With no flags, share syncs through the place kapi.yaml names: it reads what
the others shared, merges it here, and shares what was kept on this machine.
A project that names no place prints the rungs, with the command that moves
it up.

--via names the place and writes it into kapi.yaml, then syncs once:

  kapi share --via git              a ref of this repository, outside every branch
  kapi share --via folder <path>    a folder every machine reaches
  kapi share --via s3 <url>         an S3 bucket, s3://<bucket>/<prefix>
  kapi share --via bowrain <url>    a Bowrain server (needs the kapi-bowrain plugin)

A folder with no kapi.yaml gets the one kapi init writes. Commit kapi.yaml so
every checkout names the same place. --via bowrain runs the plugin's connect,
which prints what it prints, so its output is not JSON under --json.

--status prints the rung and the last sync without syncing. --no-push,
--files-only, --merged, --pr and --merger mean what they mean for
kapi context sync, the spelling an assistant or a CI job uses.

It exits with status 5 when the shared place cannot be reached, and nothing
changes here.`,
		Example: "  kapi share\n" +
			"  kapi share --via git\n" +
			"  kapi share --via folder /Volumes/team/kapi\n" +
			"  kapi share --status",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			recipe, err := shareRecipePath(cmd)
			if err != nil {
				return err
			}
			req := host.ShareRequest{Project: recipe}
			req.Via, _ = cmd.Flags().GetString("via")
			if len(args) == 1 {
				req.Location = args[0]
			}
			req.Status, _ = cmd.Flags().GetBool("status")
			req.Merged, _ = cmd.Flags().GetString("merged")
			req.PR, _ = cmd.Flags().GetInt("pr")
			req.Merger, _ = cmd.Flags().GetString("merger")
			req.NoPush, _ = cmd.Flags().GetBool("no-push")
			req.FilesOnly, _ = cmd.Flags().GetBool("files-only")
			res, err := a.Share(CmdContext(cmd), req)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("via", "", "the place to share through: git, folder <path>, s3 <url> or bowrain <url>")
	cmd.Flags().Bool("status", false, "print the rung and the last sync, without syncing")
	cmd.Flags().Bool("no-push", false, "read what others shared and share nothing back")
	cmd.Flags().Bool("files-only", false, "write the project's rules files (AGENTS.md, CLAUDE.md) from this machine's context and do nothing else")
	cmd.Flags().String("merged", "", "a range of commits that reached the default branch, counted as a person's signal")
	cmd.Flags().Int("pr", 0, "the pull request that merged the range (read from the commit subject when unset)")
	cmd.Flags().String("merger", "", "who merged it (the last commit's committer when unset)")
	cmd.MarkFlagsMutuallyExclusive("status", "merged")
	cmd.MarkFlagsMutuallyExclusive("status", "no-push")
	cmd.MarkFlagsMutuallyExclusive("status", "files-only")
	cmd.MarkFlagsMutuallyExclusive("files-only", "merged")
	cmd.MarkFlagsMutuallyExclusive("files-only", "no-push")
	AddProjectFlag(cmd)
	return cmd
}

// shareRecipePath is the recipe `kapi share` reads and writes: the project
// kapi resolves, or kapi.yaml in the working directory when it resolves none.
// The file may not exist yet. With KAPI_NO_PROJECT set and no -p, it refuses
// as `kapi context sync` does, so an isolated run inside a project never
// reads or syncs that project's recipe.
func shareRecipePath(cmd *cobra.Command) (string, error) {
	recipe, err := ResolveProjectPath(cmd)
	if err != nil || recipe != "" {
		return recipe, err
	}
	if os.Getenv(project.NoProjectEnvVar) != "" {
		return RequireProjectPath(cmd)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, project.RecipeFileName), nil
}
