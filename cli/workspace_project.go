package cli

import (
	"errors"

	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// NewPackCmd creates the "pack" command: snapshot an in-progress .kapi
// project's working state (block-store overlays + content) into a portable
// .kpz, for hand-off or backup.
func NewPackCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pack -o <snapshot.kpz>",
		Short:   "Snapshot a kapi project's working state into a .kpz",
		GroupID: "advanced",
		Long: `Snapshot a kapi project's working state — the block-store overlays, the
authoritative translation memory, and the termbase — into a portable .kpz.
Regenerable caches and secrets are excluded. Move the snapshot to another
machine and "kapi unpack" it to resume work there.`,
		Example: `  kapi pack -o snapshot.kpz   # a kapi project
  kapi pack work.kpz         # eject a .kpz workspace's cache`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Ad-hoc workspace: `kapi pack work.kpz` ejects the .kpz's working
			// cache into the file (the git-bundle hand-off boundary).
			if len(args) == 1 && IsKpzPath(args[0]) {
				return a.PackKpz(cmd.Context(), args[0])
			}
			return a.RunPack(cmd)
		},
	}
	AddProjectFlag(cmd)
	cmd.Flags().StringP("output", "o", "", "output .kpz snapshot path")
	cmd.Flags().Bool("log", false, "stamp a tamper-evident provenance line into the snapshot's advisory history")
	cmd.Flags().Bool("with-source", false, "embed raw source bytes in the .kpz (default: identity + skeleton only)")
	return cmd
}

// NewInfoCmd creates the "info" command: show a .kpz workspace's state —
// documents, locales, output layout, and whether its working cache is dirty
// (has work not yet packed into the .kpz). Named `info` because the bowrain
// plugin owns `status`.
func NewInfoCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info <work.kpz>",
		Short:   "Show a .kpz workspace's state (dirty?)",
		GroupID: "advanced",
		Example: `  kapi info work.kpz`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !IsKpzPath(args[0]) {
				return errors.New("info: expects a .kpz workspace")
			}
			return a.InfoKpz(cmd, args[0])
		},
	}
	output.AddFlags(cmd.Flags())
	return cmd
}

// NewUnpackCmd creates the "unpack" command: rehydrate a project's working
// state from a .kpz snapshot into the local .kapi/ state dir.
func NewUnpackCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "unpack <snapshot.kpz>",
		Short:   "Rehydrate a project's working state from a .kpz snapshot",
		GroupID: "advanced",
		Example: `  kapi unpack snapshot.kpz`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.RunUnpack(cmd, args[0])
		},
	}
	AddProjectFlag(cmd)
	return cmd
}
