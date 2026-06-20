package cli

import (
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/cli/config"
)

// NewConfigCmd creates the `kapi config` command for reading and writing kapi's
// app configuration (the global ~/.config/kapi/kapi.yaml). Its most common use is
// setting a default AI provider/model so the `--provider` flag can be omitted:
//
//	kapi config set ai.provider gemma
//	kapi config set ai.model gemma-4-e2b
//	kapi ai-translate input.json --target-lang fr   # uses the default
func (a *App) NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Get or set kapi configuration",
		GroupID: "management",
		Long: "Read and write kapi's app configuration (the global config file, " +
			"typically ~/.config/kapi/kapi.yaml).\n\n" +
			"Common keys:\n" +
			"  ai.provider   default AI provider for ai-translate / ai-qa / brand-voice-check / flows\n" +
			"                (e.g. `gemma` to default to the free, on-device local model)\n" +
			"  ai.model      default model for the AI provider\n\n" +
			"An explicit --provider/--model flag, inline config, or project recipe " +
			"default always overrides these.",
		Example: "  kapi config set ai.provider gemma\n" +
			"  kapi config set ai.model gemma-4-e2b\n" +
			"  kapi config get ai.provider\n" +
			"  kapi config list",
	}
	cmd.AddCommand(a.newConfigGetCmd(), a.newConfigSetCmd(), a.newConfigListCmd(), a.newConfigPathCmd())
	return cmd
}

func (a *App) newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print a single config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.Config == nil {
				return errors.New("config not loaded")
			}
			fmt.Fprintln(cmd.OutOrStdout(), a.Config.GetString(args[0]))
			return nil
		},
	}
}

func (a *App) newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value (persists to the global config file)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.SetGlobalConfig(args[0], args[1]); err != nil {
				return fmt.Errorf("set %s: %w", args[0], err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "set %s = %s\n", args[0], args[1])
			return nil
		},
	}
}

func (a *App) newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured values",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.Config == nil {
				return errors.New("config not loaded")
			}
			keys := a.Config.Viper().AllKeys()
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(cmd.OutOrStdout(), "%s = %s\n", k, a.Config.GetString(k))
			}
			return nil
		},
	}
}

func (a *App) newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the global config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), config.GlobalConfigFilePath())
			return nil
		},
	}
}
