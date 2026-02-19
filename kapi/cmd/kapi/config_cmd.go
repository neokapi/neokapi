package main

import (
	"fmt"
	"path/filepath"

	"github.com/gokapi/gokapi/platform/config"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var configGlobal bool

var configCmd = &cobra.Command{
	Use:   "config [key] [value]",
	Short: "View or set configuration values",
	Long: `View or set configuration values.

With no arguments, shows the path to the config file.
With one argument (key), prints the current value.
With two arguments (key value), sets the value.

Use --global to read/write the global config file (~/.config/kapi/kapi.yaml).
Without --global, reads/writes the project config file (.kapi/config.yaml).

Examples:
  kapi config project.name                       # Print project name
  kapi config project.name "My Project"          # Set project name
  kapi config server.url                         # Print project server URL
  kapi config --global server.url                # Print global server URL
  kapi config --global server.url https://bowrain.example.com  # Set global server URL`,
	Args: cobra.MaximumNArgs(2),
	RunE: runConfig,
}

func runConfig(cmd *cobra.Command, args []string) error {
	if configGlobal {
		return runConfigGlobal(args)
	}
	return runConfigProject(args)
}

func runConfigGlobal(args []string) error {
	if len(args) == 0 {
		fmt.Println(config.GlobalConfigFilePath())
		return nil
	}

	key := args[0]

	if len(args) == 1 {
		cfg := config.NewAppConfig()
		_ = cfg.Load()
		val := cfg.GetString(key)
		if val == "" {
			return fmt.Errorf("key %q is not set", key)
		}
		fmt.Println(val)
		return nil
	}

	value := args[1]
	if err := config.SetGlobalConfig(key, value); err != nil {
		return fmt.Errorf("set config: %w", err)
	}
	fmt.Printf("Set %s = %s in %s\n", key, value, config.GlobalConfigFilePath())
	return nil
}

func runConfigProject(args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return fmt.Errorf("no .kapi/ project found (run 'kapi init' first, or use --global)")
	}

	configPath := filepath.Join(proj.KapiDir, project.ConfigFile)

	if len(args) == 0 {
		fmt.Println(configPath)
		return nil
	}

	key := args[0]

	if len(args) == 1 {
		val := project.GetConfigValue(proj.KapiDir, key)
		if val == "" {
			return fmt.Errorf("key %q is not set in %s", key, configPath)
		}
		fmt.Println(val)
		return nil
	}

	value := args[1]
	if err := project.SetConfigValue(proj.KapiDir, key, value); err != nil {
		return fmt.Errorf("set config: %w", err)
	}
	fmt.Printf("Set %s = %s in %s\n", key, value, configPath)
	return nil
}

func init() {
	configCmd.Flags().BoolVar(&configGlobal, "global", false, "Use global config file (~/.config/kapi/kapi.yaml)")
	rootCmd.AddCommand(configCmd)
}
