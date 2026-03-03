package main

import (
	"fmt"
	"path/filepath"

	"github.com/gokapi/gokapi/bowrain-cli/cmd/bowrain/output"
	"github.com/gokapi/gokapi/cli/config"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var configGlobal bool

var configCmd = &cobra.Command{
	Use:   "config [key] [value]",
	Short: "View or change settings",
	Long: `View or set configuration values.

With no arguments, shows the path to the config file.
With one argument (key), prints the current value.
With two arguments (key value), sets the value.

Use --global to read/write the global config file (~/.config/kapi/kapi.yaml).
Without --global, reads/writes the project config file (.bowrain/config.yaml).

Examples:
  bowrain config project.name                       # Print project name
  bowrain config project.name "My Project"          # Set project name
  bowrain config server.url                         # Print project server URL
  bowrain config --global server.url                # Print global server URL
  bowrain config --global server.url https://bowrain.example.com  # Set global server URL`,
	Args: cobra.MaximumNArgs(2),
	RunE: runConfig,
}

func runConfig(cmd *cobra.Command, args []string) error {
	if configGlobal {
		return runConfigGlobal(cmd, args)
	}
	return runConfigProject(cmd, args)
}

func runConfigGlobal(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return output.Print(cmd, output.ConfigOutput{
			Path:   config.GlobalConfigFilePath(),
			Action: "path",
		})
	}

	key := args[0]

	if len(args) == 1 {
		cfg := config.NewAppConfig()
		_ = cfg.Load()
		val := cfg.GetString(key)
		if val == "" {
			return fmt.Errorf("key %q is not set", key)
		}
		return output.Print(cmd, output.ConfigOutput{
			Key:    key,
			Value:  val,
			Action: "get",
		})
	}

	value := args[1]
	if err := config.SetGlobalConfig(key, value); err != nil {
		return fmt.Errorf("set config: %w", err)
	}
	return output.Print(cmd, output.ConfigOutput{
		Path:   config.GlobalConfigFilePath(),
		Key:    key,
		Value:  value,
		Action: "set",
	})
}

func runConfigProject(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return fmt.Errorf("no .bowrain/ project found (run 'bowrain init' first, or use --global)")
	}

	configPath := filepath.Join(proj.ConfigDir, project.ConfigFile)

	if len(args) == 0 {
		return output.Print(cmd, output.ConfigOutput{
			Path:   configPath,
			Action: "path",
		})
	}

	key := args[0]

	if len(args) == 1 {
		val := project.GetConfigValue(proj.ConfigDir, key)
		if val == "" {
			return fmt.Errorf("key %q is not set in %s", key, configPath)
		}
		return output.Print(cmd, output.ConfigOutput{
			Key:    key,
			Value:  val,
			Action: "get",
		})
	}

	value := args[1]
	if err := project.SetConfigValue(proj.ConfigDir, key, value); err != nil {
		return fmt.Errorf("set config: %w", err)
	}
	return output.Print(cmd, output.ConfigOutput{
		Path:   configPath,
		Key:    key,
		Value:  value,
		Action: "set",
	})
}

func init() {
	configCmd.Flags().BoolVar(&configGlobal, "global", false, "Use global config file (~/.config/kapi/kapi.yaml)")
	rootCmd.AddCommand(configCmd)
}
