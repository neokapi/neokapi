package main

import (
	"github.com/gokapi/gokapi/kapi/cmd/kapi/output"
	"github.com/gokapi/gokapi/core/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := output.VersionOutput{
			Version:   version.Version,
			Commit:    version.Commit,
			BuildDate: version.BuildDate,
		}
		return output.Print(cmd, out)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
