package commands

import (
	"errors"
	"strings"

	clivenue "github.com/neokapi/neokapi/cli/venue"

	"github.com/neokapi/neokapi/host/venue/config"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/host/venue/client"
	"github.com/spf13/cobra"
)

var (
	workspaceServerURL  string
	workspaceCreateName string
	workspaceCreateSlug string
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "List and create workspaces on the server",
	Long:  "Manage the workspaces you can access on a Bowrain server. Requires authentication (run 'kapi auth login' first).",
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspaces you can access",
	RunE: func(cmd *cobra.Command, _ []string) error {
		serverURL, token, err := workspaceAuth()
		if err != nil {
			return err
		}
		workspaces, err := client.ListWorkspaces(cmd.Context(), serverURL, token)
		if err != nil {
			return err
		}
		out := output.WorkspaceListOutput{Server: serverURL}
		for _, ws := range workspaces {
			out.Workspaces = append(out.Workspaces, output.WorkspaceItem{
				ID:   ws.ID,
				Name: ws.Name,
				Slug: ws.Slug,
				Type: ws.Type,
			})
		}
		return output.Print(cmd, out)
	},
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new team workspace",
	RunE: func(cmd *cobra.Command, _ []string) error {
		serverURL, token, err := workspaceAuth()
		if err != nil {
			return err
		}
		name := strings.TrimSpace(workspaceCreateName)
		if name == "" {
			return errors.New("workspace name required. Use --name")
		}
		slug := strings.TrimSpace(workspaceCreateSlug)
		if slug == "" {
			slug = toSlug(name)
		}
		ws, err := client.CreateWorkspace(cmd.Context(), serverURL, token, name, slug)
		if err != nil {
			return err
		}
		return output.Print(cmd, output.WorkspaceCreateOutput{
			ID:   ws.ID,
			Name: ws.Name,
			Slug: ws.Slug,
		})
	},
}

// workspaceAuth resolves the server URL and bearer token, requiring login.
func workspaceAuth() (serverURL, token string, err error) {
	stored, err := config.LoadAuth()
	if err != nil || stored == nil {
		return "", "", errors.New("not authenticated. Run: kapi auth login")
	}
	// resolveServerURLFrom already prefers the stored login's server; the
	// hosted default only applies when nothing is configured anywhere.
	serverURL = clivenue.ResolveServerURLOrDefault(workspaceServerURL)
	return serverURL, stored.AccessToken, nil
}

func init() {
	workspaceCmd.PersistentFlags().StringVar(&workspaceServerURL, "server", "", "server URL")
	workspaceCreateCmd.Flags().StringVar(&workspaceCreateName, "name", "", "workspace name (required)")
	workspaceCreateCmd.Flags().StringVar(&workspaceCreateSlug, "slug", "", "workspace slug (derived from --name if omitted)")
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceCreateCmd)
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(workspaceCmd) })
}
