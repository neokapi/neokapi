package cli

import (
	"github.com/spf13/cobra"
)

// NewResolveCmd builds `kapi resolve`: it settles a version of a KPZ's
// document that did not land, the conflict kapi status lists with its write.
func NewResolveCmd(a *App) *cobra.Command {
	var rebase, discard string
	cmd := &cobra.Command{
		Use:     "resolve [flags] DOC",
		Short:   "Rebase or discard a version of a .kpz document that did not land",
		GroupID: "work",
		Long: `Settle a version of a document a .kpz carries that was written from an older
version than the one the document holds now, and so did not land. It happens
when the .kpz on disk is replaced while its working cache holds edits nobody
packed, and when two machines' edits to one document meet. kapi status lists
each such version as a conflict with its write and the command that settles it.

--rebase WRITE carries the version's changes over onto the document as it
stands, block by block, through the same checks kapi apply runs: a block the
version added is added, one it removed is removed, and one whose text it
changed takes the change. A block the document changed too is left for you,
and kapi status lists it; a set_content to it through kapi apply decides it,
keeping the current wording included. --discard WRITE drops the version and
keeps the document as it stands, and after a rebase it keeps the current
wording of every block left.

DOC names the document as kapi status prints it (work.kpz!guide.md). Quote it
in a shell that expands "!".

Exit status: 0 when the version was settled or rebased; 2 when DOC names no
document of a .kpz or WRITE is not a version that did not land; 3 when the
rebase's changes were refused, and nothing was settled.`,
		Example: `  kapi status
  kapi resolve 'work.kpz!guide.md' --rebase <write from kapi status>
  kapi resolve 'work.kpz!guide.md' --discard <write from kapi status>
  kapi resolve 'work.kpz!guide.md' --rebase <write> --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.RunResolve(cmd, args[0], rebase, discard)
		},
	}
	f := cmd.Flags()
	f.StringVar(&rebase, "rebase", "", "carry the version WRITE names over onto the document as it stands")
	f.StringVar(&discard, "discard", "", "drop the version WRITE names and keep the document as it stands")
	AddProjectFlag(cmd)
	return cmd
}
