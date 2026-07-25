package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host/output"
)

// NewConfigCmd creates `kapi config` — the one configuration verb. Two scopes
// share it, split by shape:
//
//   - Subcommands read and write kapi's app configuration (the global
//     ~/.config/kapi/kapi.yaml): `kapi config set ai.provider ollama`. A key
//     whose first segment names an installed plugin's declared namespace is
//     routed to that plugin's own config file — `kapi config set
//     bowrain.server.url …` — so plugins never ship a competing config command.
//   - The positional form reads and writes the project recipe:
//     `kapi config name`, `kapi config source_language nb`,
//     `kapi config server.url https://…` — core fields natively, registered
//     extension blocks (server, …) generically, validated before saving.
//
// The split is deliberate: app config is per-machine and uncommitted, the
// recipe is the project's committed configuration.
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
			"Installed plugins claim their own key namespaces, reached through the same\n" +
			"subcommands (e.g. `kapi config set bowrain.server.url https://…`); there is no\n" +
			"separate per-plugin config command. `kapi config list` shows every namespace.\n\n" +
			"Project recipe fields use the positional form: with one argument the key is\n" +
			"printed, with two it is set and the recipe saved. Core keys: name,\n" +
			"source_language, preset. Dotted keys edit a registered recipe block\n" +
			"(e.g. server.url, server.stream on a connected project).",
		Example: "  kapi config set ai.provider ollama   # app config\n" +
			"  kapi config get ai.provider\n" +
			"  kapi config unset ai.model\n" +
			"  kapi config list\n" +
			"  kapi config set bowrain.server.url https://bowrain.example  # plugin namespace\n" +
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
				return fmt.Errorf("recipe key %q needs a project (run inside a kapi project or pass -p); app configuration uses 'kapi config get/set/unset/list/path'", args[0])
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
	cmd.AddCommand(
		newConfigGetCmd(a),
		newConfigSetCmd(a),
		newConfigUnsetCmd(a),
		newConfigListCmd(a),
		newConfigPathCmd(a),
	)
	return cmd
}

func newConfigGetCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Print a single config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := a.ConfigGet(args[0])
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	output.AddFlags(cmd.Flags())
	return cmd
}

func newConfigSetCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value (persists to the global config file, or a plugin's when the key is namespaced)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := a.ConfigSet(args[0], args[1])
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	output.AddFlags(cmd.Flags())
	return cmd
}

func newConfigUnsetCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove a config value, restoring the built-in default",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := a.ConfigUnset(args[0])
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	output.AddFlags(cmd.Flags())
	return cmd
}

func newConfigListCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every configured value, including plugin namespaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := a.ConfigList()
			if err != nil {
				return err
			}
			return output.Print(cmd, out)
		},
	}
	output.AddFlags(cmd.Flags())
	return cmd
}

func newConfigPathCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path [namespace]",
		Short: "Print the config file path (of kapi, or of a plugin namespace)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ns := ""
			if len(args) == 1 {
				ns = args[0]
			}
			return output.Print(cmd, a.ConfigPath(ns))
		},
	}
	output.AddFlags(cmd.Flags())
	return cmd
}
