package cli

import (
	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
)

// NewVersionCmd creates a version command for the named program.
func (a *App) NewVersionCmd(program string) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Show version information",
		GroupID: "management",
		Example: "  kapi version",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.VersionOutput{
				Program:   program,
				Version:   version.Version,
				Channel:   version.Channel(),
				Commit:    version.Commit,
				BuildDate: version.BuildDate,
			}
			return output.Print(cmd, out)
		},
	}
}
