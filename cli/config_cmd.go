package cli

import (
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host/config"
)

// NewConfigCmd creates the `kapi config` command. Two surfaces share the
// verb, split by shape:
//
//   - Subcommands read/write kapi's app configuration (the global
//     ~/.config/kapi/kapi.yaml): `kapi config set ai.provider ollama`.
//   - The positional form reads/writes the project recipe:
//     `kapi config name`, `kapi config source_language nb`,
//     `kapi config server.url https://…` — core fields natively, registered
//     extension blocks (server, …) generically, validated before saving.
func NewConfigCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config [key] [value]",
		Short:   "Get or set kapi configuration (app config subcommands, recipe keys positionally)",
		GroupID: "advanced",
		Long: "Read and write configuration.\n\n" +
			"App configuration (the global config file, typically ~/.config/kapi/kapi.yaml)\n" +
			"uses the subcommands. Common keys:\n" +
			"  ai.provider   default AI provider for translate / qa / brand-voice-check / flows\n" +
			"                (e.g. `ollama` to default to a free, on-device local model)\n" +
			"  ai.model      default model for the AI provider\n\n" +
			"An explicit --provider/--model flag, inline config, or project recipe " +
			"default always overrides these.\n\n" +
			"Project recipe fields use the positional form: with one argument the key is\n" +
			"printed, with two it is set and the recipe saved. Core keys: name,\n" +
			"source_language, preset. Dotted keys edit a registered recipe block\n" +
			"(e.g. server.url, server.stream on a connected project).",
		Example: "  kapi config set ai.provider ollama   # app config\n" +
			"  kapi config get ai.provider\n" +
			"  kapi config list\n" +
			"  kapi config name                     # recipe: print the project name\n" +
			"  kapi config name \"My Project\"        # recipe: set the project name\n" +
			"  kapi config server.url https://bowrain.example/acme/app",
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			projectPath, err := ResolveProjectPath(cmd)
			if err != nil {
				return err
			}
			if projectPath == "" {
				return fmt.Errorf("recipe key %q needs a project (run inside a kapi project or pass -p); app configuration uses 'kapi config get/set/list/path'", args[0])
			}
			if len(args) == 1 {
				v, gerr := RecipeConfigGet(projectPath, args[0])
				if gerr != nil {
					return gerr
				}
				fmt.Fprintln(cmd.OutOrStdout(), v)
				return nil
			}
			if err := RecipeConfigSet(projectPath, args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "set %s = %s\n", args[0], args[1])
			return nil
		},
	}
	AddProjectFlag(cmd)
	cmd.AddCommand(newConfigGetCmd(a), newConfigSetCmd(a), newConfigListCmd(a), newConfigPathCmd(a))
	return cmd
}

func newConfigGetCmd(a *App) *cobra.Command {
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

func newConfigSetCmd(a *App) *cobra.Command {
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

func newConfigListCmd(a *App) *cobra.Command {
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

func newConfigPathCmd(a *App) *cobra.Command {
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
