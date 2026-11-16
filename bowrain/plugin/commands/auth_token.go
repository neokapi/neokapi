package commands

import (
	"cmp"
	"errors"

	"github.com/neokapi/neokapi/host/venue/config"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/host/venue/client"
	"github.com/spf13/cobra"
)

var (
	tokenName       string
	tokenExpireDays int
	tokenTool       string
)

var authTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage API tokens",
	Long:  "Create, list, and delete API tokens for the current workspace.",
}

var authTokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an API token",
	Long: `Create a new API token for the current workspace.

With --tool the token belongs to a tool, such as a CI workflow: the server
records what the tool does as the tool's, issued by you, and you can review
that work. A tool token cannot review. Revoking it stops the tool and leaves
your own sign-in alone.

The token is displayed once, so save it immediately.
Requires a project connected to a server workspace.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, err := config.LoadAuth()
		if err != nil {
			return errors.New("not authenticated. Run: kapi auth login")
		}

		proj, err := requireProject(cmd)
		if err != nil {
			return err
		}
		if !proj.Recipe.HasServer() || proj.Recipe.Server.Workspace() == "" {
			return errors.New("no workspace configured in the project recipe")
		}

		resp, err := client.CreateToken(cmd.Context(), stored.ServerURL, stored.AccessToken, proj.Recipe.Server.Workspace(),
			client.CreateTokenRequest{Name: tokenName, ExpireDays: tokenExpireDays, ToolName: tokenTool})
		if err != nil {
			return err
		}

		return output.Print(cmd, output.TokenCreateOutput{
			ID:          resp.ID,
			Name:        resp.Name,
			Tool:        resp.ToolName,
			Token:       resp.Token,
			TokenPrefix: resp.TokenPrefix,
			ExpiresAt:   resp.ExpiresAt,
		})
	},
}

var authTokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List API tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, err := config.LoadAuth()
		if err != nil {
			return errors.New("not authenticated. Run: kapi auth login")
		}

		proj, err := requireProject(cmd)
		if err != nil {
			return err
		}
		if !proj.Recipe.HasServer() || proj.Recipe.Server.Workspace() == "" {
			return errors.New("no workspace configured in the project recipe")
		}

		tokens, err := client.ListTokens(cmd.Context(), stored.ServerURL, stored.AccessToken, proj.Recipe.Server.Workspace())
		if err != nil {
			return err
		}

		entries := make([]output.TokenEntry, len(tokens))
		for i, t := range tokens {
			entries[i] = output.TokenEntry{
				ID:          t.ID,
				Name:        t.Name,
				Tool:        t.ToolName,
				IssuedBy:    cmp.Or(t.IssuerName, t.IssuedBy),
				TokenPrefix: t.TokenPrefix,
				LastUsedAt:  t.LastUsedAt,
				ExpiresAt:   t.ExpiresAt,
				CreatedAt:   t.CreatedAt,
			}
		}

		return output.Print(cmd, output.TokenListOutput{Tokens: entries})
	},
}

var authTokenDeleteCmd = &cobra.Command{
	Use:   "delete <token-id>",
	Short: "Delete an API token",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, err := config.LoadAuth()
		if err != nil {
			return errors.New("not authenticated. Run: kapi auth login")
		}

		proj, err := requireProject(cmd)
		if err != nil {
			return err
		}
		if !proj.Recipe.HasServer() || proj.Recipe.Server.Workspace() == "" {
			return errors.New("no workspace configured in the project recipe")
		}

		tokenID := args[0]
		if err := client.DeleteToken(cmd.Context(), stored.ServerURL, stored.AccessToken, proj.Recipe.Server.Workspace(), tokenID); err != nil {
			return err
		}

		return output.Print(cmd, output.TokenDeleteOutput{ID: tokenID})
	},
}

func init() {
	authTokenCreateCmd.Flags().StringVar(&tokenName, "name", "", "token name (required)")
	authTokenCreateCmd.Flags().IntVar(&tokenExpireDays, "expire-days", 0, "days until expiration (0 = never)")
	authTokenCreateCmd.Flags().StringVar(&tokenTool, "tool", "", "make a tool token for the named tool, such as dogfood-sync (lowercase letters, digits, - and _)")
	_ = authTokenCreateCmd.MarkFlagRequired("name")

	addProjectFlag(authTokenCmd)
	authTokenCmd.AddCommand(authTokenCreateCmd, authTokenListCmd, authTokenDeleteCmd)
	authCmd.AddCommand(authTokenCmd)
}
