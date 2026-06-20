package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/cli/selfupdate"
	"github.com/neokapi/neokapi/core/version"
)

// NewUpdateCmd creates the `kapi update` command — the kapi binary's own
// updater. How it behaves depends on how kapi was installed (see
// cli/selfupdate): a package-manager install is nudged toward the exact upgrade
// command (and, with --run, that command is executed); a direct/tarball install
// is self-replaced after SHA-256 + cosign verification.
func (a *App) NewUpdateCmd() *cobra.Command {
	var channel string
	var run bool
	var checkOnly bool

	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Update kapi to the latest release",
		GroupID: "management",
		Long: `Update the kapi binary itself.

If kapi was installed via a package manager (Homebrew, winget, apt, …), this
prints the exact upgrade command — pass --run to execute it for you. If kapi was
installed by direct download, it downloads the latest release, verifies its
SHA-256 and cosign signature, and replaces the binary in place.

  kapi update            # check and update (or print the upgrade command)
  kapi update --check    # only report whether an update is available
  kapi update --run      # on a managed install, run the package manager for me`,
		Example: "  kapi update",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			source := selfupdate.Detect()

			// An unset --channel follows the configured update.channel
			// (KAPI_UPDATE_CHANNEL), defaulting to stable.
			if channel == "" && a.Config != nil {
				channel = a.Config.UpdateChannel()
			}

			rel, err := selfupdate.FetchLatest(ctx, channel)
			if err != nil {
				return fmt.Errorf("check for updates: %w", err)
			}
			if !selfupdate.IsNewer(rel.Version) {
				fmt.Fprintf(out, "kapi %s is already the latest %s release.\n", version.Version, channelLabel(channel))
				return nil
			}

			if checkOnly {
				fmt.Fprintln(out, selfupdate.Notice(source, channel, version.Version, rel.Version))
				return nil
			}

			// Managed install: the package manager owns the binary.
			if source.Managed() {
				return runManagedUpgrade(cmd, source, channel, rel.Version, run)
			}

			// Direct install we own: self-replace, but only if writable.
			if !selfupdate.CanSelfReplace(source) {
				fmt.Fprintf(out, "A newer kapi (%s) is available, but this install location isn't writable.\n", rel.Version)
				fmt.Fprintln(out, "Re-download from https://github.com/neokapi/neokapi/releases/latest")
				return nil
			}

			fmt.Fprintf(errOut, "Updating kapi %s → %s …\n", version.Version, rel.Version)
			if err := selfupdate.Apply(ctx, rel, progressFunc(errOut)); err != nil {
				return err
			}
			fmt.Fprintf(out, "\nUpdated kapi to %s. Restart any running kapi processes.\n", rel.Version)
			return nil
		},
	}

	cmd.Flags().StringVar(&channel, "channel", "", "release channel (stable, beta); defaults to update.channel config")
	cmd.Flags().BoolVar(&run, "run", false, "on a managed install, run the package-manager upgrade for me")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only report whether an update is available")
	return cmd
}

func channelLabel(channel string) string {
	if channel == "" {
		return "stable"
	}
	return channel
}

// runManagedUpgrade prints the upgrade command for a package-manager install,
// optionally executing it when --run is set and the manager supports automation.
func runManagedUpgrade(cmd *cobra.Command, source selfupdate.Source, channel, latest string, run bool) error {
	out := cmd.OutOrStdout()
	upgradeCmd := selfupdate.UpgradeCommand(source, channel)

	if !run {
		fmt.Fprintf(out, "kapi %s → %s is available.\n", version.Version, latest)
		fmt.Fprintf(out, "This kapi was installed via %s; update it with:\n\n    %s\n\n", source, upgradeCmd)
		fmt.Fprintln(out, "Re-run with --run to do that now.")
		return nil
	}

	argv := selfupdate.UpgradeArgv(source, channel)
	if len(argv) == 0 {
		// apt/dnf need sudo / a TTY — never run those unattended.
		fmt.Fprintf(out, "Run this to update kapi (needs elevated privileges):\n\n    %s\n", upgradeCmd)
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Running: %s\n", upgradeCmd)
	c := exec.CommandContext(cmd.Context(), argv[0], argv[1:]...)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	c.Stdin = os.Stdin
	if err := c.Run(); err != nil {
		// On Windows the package manager can't replace a running kapi.exe; fall
		// back to printing the command so the user can run it from a new shell.
		fmt.Fprintf(out, "\nCould not run the upgrade automatically (%v).\nRun it yourself:\n\n    %s\n", err, upgradeCmd)
		return nil
	}
	return nil
}

// progressFunc renders a simple percentage to w as the download proceeds.
func progressFunc(w interface{ Write([]byte) (int, error) }) func(downloaded, total int64) {
	return func(downloaded, total int64) {
		if total <= 0 {
			return
		}
		fmt.Fprintf(w, "\rDownloading… %d%%", downloaded*100/total)
	}
}
