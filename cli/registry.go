package cli

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/cli/output"
	"github.com/spf13/cobra"
)

// NewRegistryCmd creates the registry command group (list, add, remove).
func (a *App) NewRegistryCmd() *cobra.Command {
	registryCmd := &cobra.Command{
		Use:     "registry",
		Short:   "Manage plugin registries",
		GroupID: "management",
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
					Name:     r.Name,
					URL:      r.URL,
					Channels: r.Channels,
				})
			}

			out := output.RegistryListOutput{
				Registries: entries,
				Total:      len(entries),
			}
			return output.Print(cmd, out)
		},
	}

	var channelsFlag string
	registryAddCmd := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add a registry to global config",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, url := args[0], args[1]
			var channels []string
			if channelsFlag != "" {
				for ch := range strings.SplitSeq(channelsFlag, ",") {
					if s := strings.TrimSpace(ch); s != "" {
						channels = append(channels, s)
					}
				}
			}
			if err := config.AddGlobalRegistry(name, url, channels); err != nil {
				return err
			}

			out := output.RegistryAddOutput{
				Name:     name,
				URL:      url,
				Channels: channels,
			}
			return output.Print(cmd, out)
		},
	}
	registryAddCmd.Flags().StringVar(&channelsFlag, "channels", "", "available channels (comma-separated, e.g., default,snapshot)")

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
