package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the Bowrain desktop app",
	Long:  `Launch the Bowrain desktop application (GUI).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return launchDesktopApp()
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
}

func launchDesktopApp() error {
	switch runtime.GOOS {
	case "darwin":
		return launchOrError(exec.Command("open", "-a", "Bowrain")) //nolint:noctx // launch desktop app
	case "linux":
		// Try common locations.
		for _, name := range []string{"Bowrain", "bowrain"} {
			if path, err := exec.LookPath(name); err == nil {
				return launchOrError(exec.Command(path)) //nolint:noctx // launch desktop app
			}
		}
		return errors.New("bowrain desktop app not found in PATH; install it from https://github.com/neokapi/neokapi/releases")
	case "windows":
		for _, name := range []string{"Bowrain.exe", "bowrain.exe"} {
			if path, err := exec.LookPath(name); err == nil {
				return launchOrError(exec.Command(path)) //nolint:noctx // launch desktop app
			}
		}
		return errors.New("bowrain desktop app not found in PATH; install it from https://github.com/neokapi/neokapi/releases")
	default:
		return fmt.Errorf("unsupported platform %s; download Bowrain from https://github.com/neokapi/neokapi/releases", runtime.GOOS)
	}
}

func launchOrError(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch bowrain: %w", err)
	}
	return nil
}
