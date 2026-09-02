package cli

import "github.com/spf13/cobra"

// newModelsSetupCmd builds `kapi models setup` — the interactive first-run
// provider wizard.
func newModelsSetupCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactively pick and verify the default AI provider",
		Long: "Detects the AI options on this machine: the Claude Code CLI (uses your\n" +
			"Claude subscription, no API key), a running Ollama server (local models),\n" +
			"and API keys already present in the environment. Then walks through picking\n" +
			"the default provider, storing an API key when one is needed, and verifying\n" +
			"the choice with a tiny test call. Writes ai.provider / ai.model to the global\n" +
			"config (shared with Kapi Desktop).\n\n" +
			"Requires a terminal; in CI set an API key env var or use `kapi config set`.",
		Example: "  kapi models setup",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := a.RunAISetupWizard(cmd.Context(), a.DefaultAISetupIO(cmd), false)
			return err
		},
	}
	return cmd
}
