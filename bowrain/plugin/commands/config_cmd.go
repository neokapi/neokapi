package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/core/model"
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

Use --global to read/write the global config file (~/.config/bowrain/bowrain.yaml).
Without --global, reads/writes the project recipe (<project>/<name>.kapi).

Project keys (no --global):
  project.name
  source_language
  server.url
  server.stream
  preset

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
		return runConfigGlobal(cmd, args)
	}
	return runConfigProject(cmd, args)
}

func runConfigGlobal(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return output.Print(cmd, output.ConfigOutput{
			Path:   config.GlobalConfigFilePath("bowrain"),
			Action: "path",
		})
	}

	key := args[0]

	if len(args) == 1 {
		cfg := newBowrainAppConfig()
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
	if err := config.SetGlobalConfig(key, value, "bowrain"); err != nil {
		return fmt.Errorf("set config: %w", err)
	}
	return output.Print(cmd, output.ConfigOutput{
		Path:   config.GlobalConfigFilePath("bowrain"),
		Key:    key,
		Value:  value,
		Action: "set",
	})
}

func runConfigProject(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return errors.New("no kapi project found (run 'kapi init' first, or use --global)")
	}

	recipePath := proj.RecipePath()

	if len(args) == 0 {
		return output.Print(cmd, output.ConfigOutput{
			Path:   recipePath,
			Action: "path",
		})
	}

	key := args[0]

	if len(args) == 1 {
		val, ok := getRecipeValue(proj.Recipe, key)
		if !ok || val == "" {
			return fmt.Errorf("key %q is not set in %s", key, recipePath)
		}
		return output.Print(cmd, output.ConfigOutput{
			Key:    key,
			Value:  val,
			Action: "get",
		})
	}

	value := args[1]
	if err := setRecipeValue(proj.Recipe, key, value); err != nil {
		return fmt.Errorf("set config: %w", err)
	}
	if err := proj.Save(); err != nil {
		return fmt.Errorf("save recipe: %w", err)
	}
	return output.Print(cmd, output.ConfigOutput{
		Path:   recipePath,
		Key:    key,
		Value:  value,
		Action: "set",
	})
}

// getRecipeValue reads a top-level recipe field by dotted key. Returns the
// value as a string and a found flag.
func getRecipeValue(r *project.Recipe, key string) (string, bool) {
	switch strings.ToLower(key) {
	case "project.name", "name":
		return r.Name, true
	case "source_language", "defaults.source_language":
		return string(r.Defaults.SourceLanguage), true
	case "server.url":
		if r.Server == nil {
			return "", true
		}
		return r.Server.URL, true
	case "server.stream":
		if r.Server == nil {
			return "", true
		}
		return r.Server.Stream, true
	case "preset":
		return r.Preset, true
	default:
		return "", false
	}
}

// setRecipeValue writes a top-level recipe field by dotted key.
func setRecipeValue(r *project.Recipe, key, value string) error {
	switch strings.ToLower(key) {
	case "project.name", "name":
		r.Name = value
		return nil
	case "source_language", "defaults.source_language":
		r.Defaults.SourceLanguage = model.LocaleID(value)
		return nil
	case "server.url":
		if r.Server == nil {
			r.Server = &project.ServerSpec{}
		}
		r.Server.URL = value
		requireBowrain(r)
		return nil
	case "server.stream":
		if r.Server == nil {
			r.Server = &project.ServerSpec{}
		}
		r.Server.Stream = value
		requireBowrain(r)
		return nil
	case "preset":
		r.Preset = value
		return nil
	default:
		return fmt.Errorf("unknown recipe key %q", key)
	}
}

func init() {
	configCmd.Flags().BoolVar(&configGlobal, "global", false, "Use global config file (~/.config/bowrain/bowrain.yaml)")
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(configCmd) })
}
