package cli

import (
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// NewLookCmd creates `kapi look`: the project as a person sees it (the Map),
// or one file with its findings drawn under the words they are about and the
// rules that hold there (the Reader). It is the person's spelling of what
// `kapi context <path>` answers for an assistant. The data assembly is
// host.LookProject and host.LookFile, so the desktop can draw the same.
func NewLookCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "look [path]",
		Short:   "See the project, or one file with its rules and findings in place",
		GroupID: "work",
		Long: `See where the project stands, or read one file with what applies to it.

With no path, look prints the project: the files kapi reads, the blocks in
them, the rules in force, the findings kapi check reports, and the
suggestions waiting for you in kapi context review. Inside a project, kapi
with no command prints the same.

With a path, look prints the file's blocks in order, each one named the way
kapi check and kapi inspect name it. A finding is marked with carets under the
words it is about, and the rules that hold in the file follow the blocks. A
file the project does not read as content is shown too, without rules.

--json gives the same as data.`,
		Example: "  kapi look\n" +
			"  kapi look docs/guide.md\n" +
			"  kapi look docs/guide.md --json",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				reader, err := a.LookFile(cmd, args[0])
				if err != nil {
					return err
				}
				return output.Print(cmd, reader)
			}
			m, err := a.LookProject(cmd)
			if err != nil {
				return err
			}
			return output.Print(cmd, m)
		},
	}
	AddProjectFlag(cmd)
	return cmd
}
