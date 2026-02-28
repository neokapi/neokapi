package cli

import (
	"fmt"

	"github.com/gokapi/gokapi/platform/cli/output"
	"github.com/gokapi/gokapi/platform/config"
	"github.com/spf13/cobra"
)

// NewRegistryCmd creates the registry command group (list, add, remove).
func (a *App) NewRegistryCmd() *cobra.Command {
	registryCmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage plugin registries",
	}

	registryListCmd := &cobra.Command{
		Use:   "list",
		Short: "List configured registries",
		RunE: func(cmd *cobra.Command, args []string) error {
			regs, err := config.ListGlobalRegistries()
			if err != nil {
				return fmt.Errorf("listing registries: %w", err)
			}

			var entries []output.RegistryInfo
			for _, r := range regs {
				entries = append(entries, output.RegistryInfo{
					Name: r.Name,
					URL:  r.URL,
				})
			}

			out := output.RegistryListOutput{
				Registries: entries,
				Total:      len(entries),
			}
			return output.Print(cmd, out)
		},
	}

	registryAddCmd := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add a registry to global config",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, url := args[0], args[1]
			if err := config.AddGlobalRegistry(name, url); err != nil {
				return err
			}

			out := output.RegistryAddOutput{
				Name: name,
				URL:  url,
			}
			return output.Print(cmd, out)
		},
	}

	registryRemoveCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a registry from global config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := config.RemoveGlobalRegistry(name); err != nil {
				return err
			}

			out := output.RegistryRemoveOutput{
				Name: name,
			}
			return output.Print(cmd, out)
		},
	}

	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryAddCmd)
	registryCmd.AddCommand(registryRemoveCmd)

	return registryCmd
}
