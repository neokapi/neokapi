package cli

import (
	"errors"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/spf13/cobra"
)

// NewPackCmd creates the "pack" command: snapshot an in-progress .kapi
// project's working state (block-store overlays + content) into a portable
// .klz, for hand-off or backup.
func NewPackCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pack -o <snapshot.klz>",
		Short:   "Snapshot a .kapi project's working state into a .klz",
		GroupID: "advanced",
		Long: `Snapshot a .kapi project's working state — the block-store overlays, the
authoritative translation memory, and the termbase — into a portable .klz.
Regenerable caches and secrets are excluded. Move the snapshot to another
machine and "kapi unpack" it to resume work there.`,
		Example: `  kapi pack -o snapshot.klz   # a .kapi project
  kapi pack work.klz         # eject a .klz workspace's cache`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Ad-hoc workspace: `kapi pack work.klz` ejects the .klz's working
			// cache into the file (the git-bundle hand-off boundary).
			if len(args) == 1 && IsKlzPath(args[0]) {
				return a.PackKlz(cmd.Context(), args[0])
			}
			return a.RunPack(cmd)
		},
	}
	AddProjectFlag(cmd)
	cmd.Flags().StringP("output", "o", "", "output .klz snapshot path")
	cmd.Flags().Bool("log", false, "stamp a tamper-evident provenance line into the snapshot's advisory history")
	cmd.Flags().Bool("with-source", false, "embed raw source bytes in the .klz (default: identity + skeleton only)")
	return cmd
}

// NewInfoCmd creates the "info" command: show a .klz workspace's state —
// documents, locales, output layout, and whether its working cache is dirty
// (has work not yet packed into the .klz). Named `info` because the bowrain
// plugin owns `status`.
func NewInfoCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info <work.klz>",
		Short:   "Show a .klz workspace's state (dirty?)",
		GroupID: "advanced",
		Example: `  kapi info work.klz`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !IsKlzPath(args[0]) {
				return errors.New("info: expects a .klz workspace")
			}
			return a.InfoKlz(cmd, args[0])
		},
	}
	output.AddFlags(cmd.Flags())
	return cmd
}

// NewUnpackCmd creates the "unpack" command: rehydrate a project's working
// state from a .klz snapshot into the local .kapi/ state dir.
func NewUnpackCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "unpack <snapshot.klz>",
		Short:   "Rehydrate a project's working state from a .klz snapshot",
		GroupID: "advanced",
		Example: `  kapi unpack snapshot.klz`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.RunUnpack(cmd, args[0])
		},
	}
	AddProjectFlag(cmd)
	return cmd
}
