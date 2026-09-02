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
		Long: `Snapshot a kapi project's working state (the block-store overlays, the
authoritative content memory, and the terms store) into a portable .kpz.
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
	output.AddFlags(cmd.Flags())
	return cmd
}

// NewInfoCmd creates the "info" command: report on a project's packable state,
// or on a .kpz archive.
//
// One verb, two subjects, because they are two views of the same thing: `kapi
// info` in a project lists the parts a snapshot would capture (and what it
// deliberately omits), and `kapi info work.kpz` lists what an archive actually
// holds plus whether its working cache has drifted from it. One verb answers
// "what would I be handing over?" both before and after `kapi pack`. Named
// `info` rather than `status` because the bowrain plugin owns `status`.
func NewInfoCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info [work.kpz]",
		Short:   "Report a project's packable state, or a .kpz archive's contents",
		GroupID: "advanced",
		Long: `With no argument, reports the project in scope: its languages, how many files
it tracks, and the parts a ` + "`kapi pack`" + ` snapshot would capture, each with
whether it is present and how many items it holds, plus what the snapshot
deliberately leaves out (regenerable caches, credentials, sync tokens).

With a .kpz argument, reports that archive. A project snapshot is reported as the
same parts, in the same words, so comparing the two is the round-trip check. A
workspace .kpz (the ad-hoc parcel with a working cache) instead reports its
documents, locales, and whether the cache holds work not yet packed back.

Both forms support --output-format json|yaml.`,
		Example: `  kapi info              # the project: what a snapshot would capture
  kapi info handoff.kpz  # the archive: what it holds`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return a.RunProjectInfo(cmd)
			}
			if !IsKpzPath(args[0]) {
				return errors.New("info: expects a .kpz archive, or no argument to report the project")
			}
			return a.InfoKpz(cmd, args[0])
		},
	}
	AddProjectFlag(cmd)
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
	output.AddFlags(cmd.Flags())
	return cmd
}
