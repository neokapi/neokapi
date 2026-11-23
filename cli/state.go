package cli

import (
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// NewStateCmd creates the state command group: where a checkout's own state
// lives, and the two ways a person brings state into it.
func NewStateCmd(a *App) *cobra.Command {
	stateCmd := &cobra.Command{
		Use:     "state",
		Short:   "Show where a checkout's state lives, and bring state into it",
		GroupID: "advanced",
		// Reached from the message that names the exact command after an
		// upgrade or a move, not from the root help.
		Hidden: true,
		Long: `Show where a checkout's own state lives, and bring state into it.

kapi keeps each checkout's state outside the checkout, in a directory of its
own under the data root: the project store and caches, which kapi rebuilds,
and the redaction vault, the sync state and the saved filters, which it
cannot. The directory is found by the checkout's location and the identity of
its folder, never by anything the checkout contains, so a repository cannot
reach another project's state, and kapi reads nothing a checkout carries as
state.

A checkout moved or renamed on its volume keeps its state. Any other move,
a restored backup, or a fresh clone starts without it; kapi rebuilds what it
can, and a person brings the rest across:

  kapi state import   takes the state an earlier kapi left inside the
                      checkout's .kapi/ folder
  kapi state relink   takes the state kapi recorded for another location
  kapi state path     prints the checkout's state directory and its parse cache

Both import and relink show every file they would take, with its size and
the server any sync state was issued by, ask at the terminal (or take --yes),
and take exactly what they showed. An agent or an MCP call cannot run them.`,
		Example: `  kapi state path -p ./app
  kapi state import -p ./app
  kapi state relink --from ~/old/app -p ./app`,
	}

	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Take the state an earlier kapi left inside the checkout",
		Long: `Take the state an earlier kapi left inside the checkout's .kapi/ folder (the
redaction vault, the sync state and the saved filters) into the checkout's
state directory. The files are shown first and confirmed at the terminal.
With no terminal attached nothing is taken unless --yes is given.

The copies inside the checkout stay where they are unless --remove is given;
kapi no longer reads them either way.`,
		Example: `  kapi state import -p ./app
  kapi state import -p ./app --yes --remove`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			recipePath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			out, err := a.StateImport(cmd, recipePath, BoolFlag(cmd, "remove"))
			if err != nil {
				return err
			}
			return output.Print(cmd, out)
		},
	}
	AddProjectFlag(importCmd)
	importCmd.Flags().Bool("remove", false, "remove the copies inside the checkout once they are taken")

	relinkCmd := &cobra.Command{
		Use:   "relink",
		Short: "Take the state kapi recorded for the checkout's earlier location",
		Long: `Take the state kapi recorded for a checkout at another location into this
checkout's state directory: for a checkout moved to another volume, restored
from a backup, or cloned again. The files are shown first and confirmed at the
terminal. With no terminal attached nothing is taken unless --yes is given.
The work directory is rebuilt here rather than taken, and the state directory
the files came from is removed once they are in place.`,
		Example: `  kapi state relink --from ~/old/app -p ./app`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			recipePath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			from, _ := cmd.Flags().GetString("from")
			out, err := a.StateRelink(cmd, recipePath, from)
			if err != nil {
				return err
			}
			return output.Print(cmd, out)
		},
	}
	AddProjectFlag(relinkCmd)
	relinkCmd.Flags().String("from", "", "the location the checkout's state was recorded for")
	_ = relinkCmd.MarkFlagRequired("from")

	pathCmd := &cobra.Command{
		Use:   "path",
		Short: "Print the directory that holds the checkout's state",
		Long: `Print the directory that holds the checkout's own state. With --json the
answer also names the parse cache directory, parse_cache: work/cache/docs in
the state directory, or the directory KAPI_PARSE_CACHE_DIR names.

A CI workflow caches the parse cache only. It sets KAPI_PARSE_CACHE_DIR to a
fixed directory and caches that, because the state directory changes with
every fresh checkout and also holds the sync state and the redaction vault.`,
		Example: "  kapi state path -p .\n  kapi state path -p . --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			recipePath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			out, err := a.StatePath(recipePath)
			if err != nil {
				return err
			}
			return output.Print(cmd, out)
		},
	}
	AddProjectFlag(pathCmd)

	stateCmd.AddCommand(importCmd, relinkCmd, pathCmd)
	// Hidden from the root help, and listed in the command reference.
	return ListInReference(stateCmd)
}
