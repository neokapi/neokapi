package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/host/credentials"
	"github.com/neokapi/neokapi/host/output"
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

// promptForAPIKey asks for the key `kapi credentials add` was not given.
//
// A key passed as --api-key lands in shell history and, for the life of the
// process, in every local process listing. The flag stays — scripts and the
// non-interactive path need it, and it is part of the CLI contract — but a
// person at a terminal should not have to choose that just to save a
// credential. There is no third prompt here: this is the wizard's own
// AISetupPrompter, which the CLI registers globally at init with
// huh.EchoModePassword, for the reason `kapi models setup` already records —
// the earlier prompt echoed the key, which is the wrong default for a secret.
//
// With no terminal (or no prompter, as in the js/wasm build) the original
// actionable error is unchanged, so scripts and CI see exactly what they did
// before.
func promptForAPIKey(a *App, providerType string) (string, error) {
	missing := fmt.Errorf("--api-key is required for %s provider", providerType)
	if a.AISetupPrompter == nil || !isatty.IsTerminal(os.Stdin.Fd()) {
		return "", missing
	}
	key, err := a.AISetupPrompter.APIKey(providerType)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(key) == "" {
		return "", missing
	}
	return key, nil
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
				key, err := promptForAPIKey(a, providerType)
				if err != nil {
					return err
				}
				apiKey = key
			}

			cfg, err := SaveCredential(a.Credentials, credentials.ProviderConfig{
				Name:         name,
				ProviderType: providerType,
				Model:        model,
				BaseURL:      baseURL,
			}, apiKey)
			if err != nil {
				return err
			}

			return output.Print(cmd, CredentialSavedOutput{
				Name:     name,
				Provider: providerType,
				ID:       cfg.ID,
			})
		},
	}

	cmd.Flags().String("provider", "", "AI provider type (anthropic, openai, gemini, ollama)")
	cmd.Flags().String("api-key", "", "API key for the provider (omit on a terminal to be prompted, unechoed)")
	cmd.Flags().String("model", "", "default model name (optional)")
	// The usage string carries the rule because this is where a user meets it.
	// A custom endpoint is part of the saved credential and reaches a provider
	// only via --credential: an inline --api-key, or a key from the
	// environment, resolves before an endpoint is attached (AD-012).
	cmd.Flags().String("base-url", "", "custom API base URL, e.g. a self-hosted endpoint (applies when used with --credential, not with an inline --api-key)")
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

			if err := RemoveCredential(a.Credentials, cfg.ID); err != nil {
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

			usable, err := TestCredential(a.Credentials, cfg)
			if err != nil {
				return err
			}

			status := "empty"
			if usable {
				status = "accessible"
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Credential %q (%s): API key %s\n", name, cfg.ProviderType, status)
			return nil
		},
	}
}
