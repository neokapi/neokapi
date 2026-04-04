package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// NewCompletionCmd creates the "completion" command for generating shell
// completion scripts. Supports bash, zsh, fish, and powershell.
func (a *App) NewCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate a shell completion script for the specified shell.

To load completions:

Bash:
  $ source <(kapi completion bash)
  # Or install permanently:
  $ kapi completion bash > /etc/bash_completion.d/kapi

Zsh:
  $ source <(kapi completion zsh)
  # Or install permanently:
  $ kapi completion zsh > "${fpath[1]}/_kapi"

Fish:
  $ kapi completion fish | source
  # Or install permanently:
  $ kapi completion fish > ~/.config/fish/completions/kapi.fish

PowerShell:
  PS> kapi completion powershell | Out-String | Invoke-Expression
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}
	return cmd
}
