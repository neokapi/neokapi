package commands

import (
	"errors"

	"github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/spf13/cobra"
)

var (
	tokenName       string
	tokenExpireDays int
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

The token is displayed once — save it immediately.
Requires a .bowrain/ project with a configured workspace.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, err := loadAuth()
		if err != nil {
			return errors.New("not authenticated — run: kapi auth login")
		}

		proj, err := project.FindProject("")
		if err != nil {
			return errors.New("no kapi project found — run: kapi init")
		}
		if !proj.Recipe.HasServer() || proj.Recipe.Server.Workspace() == "" {
			return errors.New("no workspace configured in the project recipe")
		}

		resp, err := client.CreateToken(stored.ServerURL, stored.AccessToken, proj.Recipe.Server.Workspace(), tokenName, tokenExpireDays)
		if err != nil {
			return err
		}

		return output.Print(cmd, output.TokenCreateOutput{
			ID:          resp.ID,
			Name:        resp.Name,
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
		stored, err := loadAuth()
		if err != nil {
			return errors.New("not authenticated — run: kapi auth login")
		}

		proj, err := project.FindProject("")
		if err != nil {
			return errors.New("no kapi project found — run: kapi init")
		}
		if !proj.Recipe.HasServer() || proj.Recipe.Server.Workspace() == "" {
			return errors.New("no workspace configured in the project recipe")
		}

		tokens, err := client.ListTokens(stored.ServerURL, stored.AccessToken, proj.Recipe.Server.Workspace())
		if err != nil {
			return err
		}

		entries := make([]output.TokenEntry, len(tokens))
		for i, t := range tokens {
			entries[i] = output.TokenEntry{
				ID:          t.ID,
				Name:        t.Name,
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
		stored, err := loadAuth()
		if err != nil {
			return errors.New("not authenticated — run: kapi auth login")
		}

		proj, err := project.FindProject("")
		if err != nil {
			return errors.New("no kapi project found — run: kapi init")
		}
		if !proj.Recipe.HasServer() || proj.Recipe.Server.Workspace() == "" {
			return errors.New("no workspace configured in the project recipe")
		}

		tokenID := args[0]
		if err := client.DeleteToken(stored.ServerURL, stored.AccessToken, proj.Recipe.Server.Workspace(), tokenID); err != nil {
			return err
		}

		return output.Print(cmd, output.TokenDeleteOutput{ID: tokenID})
	},
}

func init() {
	authTokenCreateCmd.Flags().StringVar(&tokenName, "name", "", "token name (required)")
	authTokenCreateCmd.Flags().IntVar(&tokenExpireDays, "expire-days", 0, "days until expiration (0 = never)")
	_ = authTokenCreateCmd.MarkFlagRequired("name")

	authTokenCmd.AddCommand(authTokenCreateCmd, authTokenListCmd, authTokenDeleteCmd)
	authCmd.AddCommand(authTokenCmd)
}
