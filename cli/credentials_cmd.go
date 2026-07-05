package cli

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/cli/credentials"
	"github.com/neokapi/neokapi/cli/output"
	"github.com/spf13/cobra"
)

// NewCredentialsCmd creates the "credentials" command group for managing
// saved AI provider credentials.
func NewCredentialsCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "credentials",
		Aliases: []string{"creds"},
		Short:   "Manage saved AI provider credentials",
		GroupID: "assets",
	}

	cmd.AddCommand(newCredentialsAddCmd(a))
	cmd.AddCommand(newCredentialsListCmd(a))
	cmd.AddCommand(newCredentialsRemoveCmd(a))
	cmd.AddCommand(newCredentialsTestCmd(a))

	return cmd
}

func newCredentialsAddCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Save a named AI provider credential",
		Long: `Save an AI provider credential to the OS keychain.

The credential name is used to reference it in flows and tool commands:
  kapi translate --credential my-openai -i file.json --target-lang fr

If only one credential is saved, tools will auto-detect it without --credential.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			providerType, _ := cmd.Flags().GetString("provider")
			apiKey, _ := cmd.Flags().GetString("api-key")
			model, _ := cmd.Flags().GetString("model")
			baseURL, _ := cmd.Flags().GetString("base-url")

			if providerType == "" {
				return fmt.Errorf("--provider is required (one of: %s)", strings.Join(KnownProviderTypes(), ", "))
			}
			if err := ValidateProviderType(providerType); err != nil {
				return err
			}
			if apiKey == "" && !credentials.Keyless(providerType) {
				return fmt.Errorf("--api-key is required for %s provider", providerType)
			}

			cfg, err := a.Credentials.Upsert(credentials.ProviderConfig{
				Name:         name,
				ProviderType: providerType,
				Model:        model,
				BaseURL:      baseURL,
			})
			if err != nil {
				return fmt.Errorf("save provider config: %w", err)
			}

			if apiKey != "" {
				if err := a.Credentials.SetAPIKey(cfg.ID, apiKey); err != nil {
					return fmt.Errorf("store API key: %w", err)
				}
			}

			return output.Print(cmd, CredentialSavedOutput{
				Name:     name,
				Provider: providerType,
				ID:       cfg.ID,
			})
		},
	}

	cmd.Flags().String("provider", "", "AI provider type (anthropic, openai, gemini, ollama)")
	cmd.Flags().String("api-key", "", "API key for the provider")
	cmd.Flags().String("model", "", "default model name (optional)")
	cmd.Flags().String("base-url", "", "custom API base URL (optional)")
	_ = cmd.MarkFlagRequired("provider")

	return cmd
}

func newCredentialsListCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			configs := a.Credentials.List()

			var rows []CredentialRow
			for _, c := range configs {
				_, err := a.Credentials.GetAPIKey(c.ID)
				rows = append(rows, CredentialRow{
					Name:     c.Name,
					Provider: c.ProviderType,
					Model:    c.Model,
					ID:       c.ID,
					HasKey:   err == nil,
				})
			}

			return output.Print(cmd, CredentialListOutput{
				Credentials: rows,
				Total:       len(rows),
			})
		},
	}
}

func newCredentialsRemoveCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a saved credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := a.Credentials.GetByName(name)
			if err != nil {
				return fmt.Errorf("credential %q not found", name)
			}

			_ = a.Credentials.DeleteAPIKey(cfg.ID) // ignore keychain errors
			if err := a.Credentials.Remove(cfg.ID); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Credential %q removed\n", name)
			return nil
		},
	}
}

func newCredentialsTestCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "test <name>",
		Short: "Test that a credential's API key is accessible",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := a.Credentials.GetByName(name)
			if err != nil {
				return fmt.Errorf("credential %q not found", name)
			}

			key, err := a.Credentials.GetAPIKey(cfg.ID)
			if err != nil {
				return fmt.Errorf("API key not found in keychain: %w", err)
			}

			status := "accessible"
			if len(key) == 0 {
				status = "empty"
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Credential %q (%s): API key %s\n", name, cfg.ProviderType, status)
			return nil
		},
	}
}
