package cli

import (
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// newContextPlacesCmd is `kapi context places`: a lookup in the project's
// gazetteer, for the place id a geo axis takes (host.ContextPlaces).
func newContextPlacesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "places [query]",
		Short: "Look up the places a geo axis takes",
		Long: `Look up places in this project's gazetteer: the world, the UN M49
regions, the ISO 3166-1 countries and ISO 3166-2 subdivisions, and the places
the recipe declares under places:. A geo axis takes a place's id as its value.

With a query, list the places whose id or name matches it, ignoring case and
accents. With --within, list the places directly below a place. With neither,
list the places the recipe declares. Each line gives the id, the name, the
type and the path from the world down.`,
		Example: "  kapi context places oslo\n" +
			"  kapi context places --within no\n" +
			"  kapi context places\n" +
			"  kapi context places akershus --json",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := host.ContextPlacesRequest{}
			if len(args) == 1 {
				req.Query = args[0]
			}
			req.Within, _ = cmd.Flags().GetString("within")
			projectPath, err := ResolveProjectPath(cmd)
			if err != nil {
				return err
			}
			req.Project = projectPath
			res, err := a.ContextPlaces(cmd.Context(), req)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().String("within", "", "list the places directly below this place, or keep the matches inside it")
	AddProjectFlag(cmd)
	return cmd
}
