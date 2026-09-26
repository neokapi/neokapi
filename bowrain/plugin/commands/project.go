package commands

import (
	"fmt"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/spf13/cobra"
)

// requireProject resolves and loads the project cmd acts on, by the rule every
// kapi command follows (cli.ResolveProjectPath): -p, then KAPI_NO_PROJECT, then
// KAPI_PROJECT, then the upward walk from the working directory. With
// KAPI_NO_PROJECT set and no -p it refuses, before any server is contacted.
//
// Every command here that reads a recipe resolves it through this one function,
// once, and hands the loaded project to whatever it calls. A command that
// resolved its project and then let a helper look again from the working
// directory could push one project's recipe to another's server.
func requireProject(cmd *cobra.Command) (*project.Project, error) {
	path, err := cli.RequireProjectPath(cmd)
	if err != nil {
		return nil, err
	}
	proj, err := project.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load project %s: %w", path, err)
	}
	return proj, nil
}

// addProjectFlag registers -p/--project on cmd and, as a persistent flag, on
// every subcommand under it.
func addProjectFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("project", "p", "", "path to a kapi.yaml project recipe or its directory (found from the working directory if omitted)")
}
