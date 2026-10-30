package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// desktopLauncher starts Kapi Desktop: it finds the installed app and runs
// the command that opens a target in it. Tests replace it to run on any
// operating system without launching anything.
type desktopLauncher struct {
	env host.DesktopEnv
	run func(ctx context.Context, argv []string) error
}

// startDesktop runs argv. `open -a` exits as soon as macOS has handed the
// path to the app, so it is waited for and its error reported. The app's own
// binary keeps running after kapi exits, so it is started and let go, on a
// context kapi's exit does not cancel.
func startDesktop(ctx context.Context, argv []string) error {
	if argv[0] == "open" {
		out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
		if err != nil && len(out) > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
		return err
	}
	c := exec.CommandContext(context.WithoutCancel(ctx), argv[0], argv[1:]...)
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

// NewOpenCmd creates `kapi open`: Kapi Desktop on this folder.
func NewOpenCmd(a *App) *cobra.Command {
	return newOpenCmd(a, desktopLauncher{run: startDesktop})
}

func newOpenCmd(_ *App, launch desktopLauncher) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "open [path]",
		Short:   "Open this folder in Kapi Desktop",
		GroupID: "work",
		Long: `Open the project in Kapi Desktop: the project kapi finds from here, or the
folder or kapi.yaml you name. A window that is already open shows it in a new
tab.

When Kapi Desktop is not installed, open says where to get it and exits with
status 1. --dry-run prints the command it would run.`,
		Example: "  kapi open\n" +
			"  kapi open ~/work/site\n" +
			"  kapi open --dry-run",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := openDir(cmd, args)
			if err != nil {
				return err
			}
			target, err := host.DesktopTarget(dir)
			if err != nil {
				return err
			}
			goos := launch.env.GOOS
			if goos == "" {
				goos = runtime.GOOS
			}
			app, err := host.FindDesktop(launch.env)
			if errors.Is(err, host.ErrDesktopNotInstalled) {
				return host.DesktopNotInstalledError(goos)
			}
			if err != nil {
				return err
			}
			argv := host.DesktopOpenCommand(goos, app, target)
			if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), shellLine(argv))
				return err
			}
			if err := launch.run(CmdContext(cmd), argv); err != nil {
				return fmt.Errorf("open Kapi Desktop: %w", err)
			}
			folder := target
			if filepath.Base(target) == project.RecipeFileName {
				folder = filepath.Dir(target)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Opened %s in Kapi Desktop\n", folder)
			return err
		},
	}
	cmd.Flags().Bool("dry-run", false, "print the command that opens Kapi Desktop, and run nothing")
	AddProjectFlag(cmd)
	return cmd
}

// openDir is the folder `kapi open` opens: the path given, the project kapi
// resolves, or the working directory.
func openDir(cmd *cobra.Command, args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return "", err
	}
	if recipe != "" {
		return recipe, nil
	}
	return os.Getwd()
}

// shellLine renders argv the way a person would type it, quoting each word
// that holds a space.
func shellLine(argv []string) string {
	words := make([]string, len(argv))
	for i, w := range argv {
		if strings.ContainsAny(w, " \t'\"") {
			w = "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
		}
		words[i] = w
	}
	return strings.Join(words, " ")
}
