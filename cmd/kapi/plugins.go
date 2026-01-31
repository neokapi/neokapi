package main

import (
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/config"
	"github.com/gokapi/gokapi/plugin/registry"
	"github.com/spf13/cobra"
)

var pluginsCmd = &cobra.Command{
	Use:   "plugins",
	Short: "Manage plugins",
}

var availableFlag bool

var pluginsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		if availableFlag {
			return listAvailablePlugins()
		}
		return listInstalledPlugins()
	},
}

func listInstalledPlugins() error {
	// Show locally loaded plugins (discovered by the plugin loader).
	plugins := pluginLoader.Plugins()

	// Also show version-tracked installations.
	cfg := config.NewAppConfig()
	_ = cfg.Load()
	reg := registry.NewRemoteRegistry(cfg.RegistryURL(), pluginLoader.Dir())
	installed, _ := reg.ListInstalled()

	if len(plugins) == 0 && len(installed) == 0 {
		fmt.Printf("No plugins installed.\n")
		fmt.Printf("Plugin directory: %s\n", pluginLoader.Dir())
		fmt.Println()
		fmt.Println("Use 'kapi plugins search <query>' to find plugins,")
		fmt.Println("or 'kapi plugins list -a' to see all available plugins.")
		return nil
	}

	if len(plugins) > 0 {
		fmt.Printf("  %-20s %-10s %-30s %s\n", "NAME", "TYPE", "FORMATS", "SOURCE")
		fmt.Printf("  %-20s %-10s %-30s %s\n", "----", "----", "-------", "------")
		for _, p := range plugins {
			fmts := strings.Join(p.Formats, ", ")
			fmt.Printf("  %-20s %-10s %-30s %s\n", p.Name, p.Type, fmts, p.Source)
		}
		fmt.Printf("\n%d plugin(s) loaded from %s\n", len(plugins), pluginLoader.Dir())
	}

	if len(installed) > 0 {
		if len(plugins) > 0 {
			fmt.Println()
		}
		fmt.Println("Installed plugins (version tracked):")
		fmt.Printf("  %-25s %-10s %-10s %s\n", "NAME", "VERSION", "TYPE", "INSTALLED")
		fmt.Printf("  %-25s %-10s %-10s %s\n", "----", "-------", "----", "---------")
		for _, vf := range installed {
			fmt.Printf("  %-25s %-10s %-10s %s\n", vf.Name, vf.Version, vf.InstallType, vf.InstalledAt)
		}
	}

	return nil
}

func listAvailablePlugins() error {
	cfg := config.NewAppConfig()
	_ = cfg.Load()
	reg := registry.NewRemoteRegistry(cfg.RegistryURL(), pluginLoader.Dir())

	available, err := reg.ListAvailable()
	if err != nil {
		return fmt.Errorf("fetching available plugins: %w", err)
	}

	if len(available) == 0 {
		fmt.Println("No plugins available in the registry.")
		return nil
	}

	fmt.Printf("  %-25s %-10s %-10s %s\n", "NAME", "VERSION", "TYPE", "DESCRIPTION")
	fmt.Printf("  %-25s %-10s %-10s %s\n", "----", "-------", "----", "-----------")
	for _, m := range available {
		desc := m.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		fmt.Printf("  %-25s %-10s %-10s %s\n", m.Name, m.Version, m.PluginType, desc)
	}
	fmt.Printf("\n%d plugin(s) available\n", len(available))
	return nil
}

var pluginsInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Install a plugin from the registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg := config.NewAppConfig()
		_ = cfg.Load()
		reg := registry.NewRemoteRegistry(cfg.RegistryURL(), pluginLoader.Dir())

		if !quiet {
			fmt.Printf("Installing plugin: %s\n", name)
		}

		result, err := reg.InstallPlugin(name)
		if err != nil {
			return fmt.Errorf("installing %s: %w", name, err)
		}

		if !quiet {
			fmt.Printf("Installed %s v%s (%s)\n", result.Name, result.Version, result.InstallType)
			for _, f := range result.Files {
				fmt.Printf("  → %s\n", f)
			}
		}
		return nil
	},
}

var pluginsUpdateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update installed plugins",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.NewAppConfig()
		_ = cfg.Load()
		reg := registry.NewRemoteRegistry(cfg.RegistryURL(), pluginLoader.Dir())

		if len(args) == 1 {
			// Update a specific plugin.
			name := args[0]
			if !quiet {
				fmt.Printf("Updating plugin: %s\n", name)
			}
			result, err := reg.InstallPlugin(name)
			if err != nil {
				return fmt.Errorf("updating %s: %w", name, err)
			}
			fmt.Printf("Updated %s to v%s\n", result.Name, result.Version)
			return nil
		}

		// Check for all updates.
		updates, err := reg.CheckUpdates()
		if err != nil {
			return fmt.Errorf("checking for updates: %w", err)
		}

		if len(updates) == 0 {
			fmt.Println("All plugins are up to date.")
			return nil
		}

		for _, u := range updates {
			if !quiet {
				fmt.Printf("Updating %s: %s → %s\n", u.Name, u.InstalledVersion, u.AvailableVersion)
			}
			result, err := reg.InstallPlugin(u.Name)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update %s: %v\n", u.Name, err)
				continue
			}
			fmt.Printf("Updated %s to v%s\n", result.Name, result.Version)
		}
		return nil
	},
}

var (
	searchType string
	searchMime string
	searchExt  string
)

var pluginsSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for plugins in the registry",
	Long: `Search for plugins by text query, capability type, MIME type, or file extension.

When --type, --mime, or --ext flags are provided, the query argument is optional.
All filters are combined with AND logic.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var query string
		if len(args) > 0 {
			query = args[0]
		}

		// Require at least a query or one filter flag.
		if query == "" && searchType == "" && searchMime == "" && searchExt == "" {
			return fmt.Errorf("provide a search query or use --type, --mime, or --ext flags")
		}

		cfg := config.NewAppConfig()
		_ = cfg.Load()
		reg := registry.NewRemoteRegistry(cfg.RegistryURL(), pluginLoader.Dir())

		// Use advanced search when any flag is set.
		if searchType != "" || searchMime != "" || searchExt != "" {
			return searchPluginsAdvanced(reg, query)
		}

		results, err := reg.SearchPlugins(query)
		if err != nil {
			return fmt.Errorf("searching plugins: %w", err)
		}

		if len(results) == 0 {
			fmt.Printf("No plugins found matching %q.\n", query)
			return nil
		}

		fmt.Printf("  %-25s %-10s %-10s %s\n", "NAME", "VERSION", "TYPE", "DESCRIPTION")
		fmt.Printf("  %-25s %-10s %-10s %s\n", "----", "-------", "----", "-----------")
		for _, m := range results {
			desc := m.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			fmt.Printf("  %-25s %-10s %-10s %s\n", m.Name, m.Version, m.PluginType, desc)
		}
		fmt.Printf("\n%d plugin(s) found\n", len(results))
		return nil
	},
}

func searchPluginsAdvanced(reg *registry.RemoteRegistry, query string) error {
	opts := registry.SearchOptions{
		Query:     query,
		Type:      searchType,
		MimeType:  searchMime,
		Extension: searchExt,
	}

	results, err := reg.SearchPluginsAdvanced(opts)
	if err != nil {
		return fmt.Errorf("searching plugins: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No plugins found matching the given criteria.")
		return nil
	}

	fmt.Printf("  %-25s %-10s %-10s %s\n", "NAME", "VERSION", "TYPE", "DESCRIPTION")
	fmt.Printf("  %-25s %-10s %-10s %s\n", "----", "-------", "----", "-----------")
	for _, m := range results {
		desc := m.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}

		// Show matching capability display names when filtering.
		capInfo := matchingCapabilities(m, opts)
		if capInfo != "" {
			desc = capInfo
		}

		fmt.Printf("  %-25s %-10s %-10s %s\n", m.Name, m.Version, m.PluginType, desc)
	}
	fmt.Printf("\n%d plugin(s) found\n", len(results))
	return nil
}

// matchingCapabilities returns a summary of capabilities that match the active filters.
func matchingCapabilities(m registry.PluginManifest, opts registry.SearchOptions) string {
	if len(m.Capabilities) == 0 {
		return ""
	}

	var names []string
	for _, cap := range m.Capabilities {
		if opts.Type != "" && !strings.EqualFold(cap.Type, opts.Type) {
			continue
		}
		display := cap.DisplayName
		if display == "" {
			display = cap.Name
		}
		names = append(names, display)
	}

	if len(names) == 0 {
		return ""
	}
	if len(names) > 3 {
		return fmt.Sprintf("%s (+%d more)", strings.Join(names[:3], ", "), len(names)-3)
	}
	return strings.Join(names, ", ")
}

func init() {
	pluginsListCmd.Flags().BoolVarP(&availableFlag, "available", "a", false, "list available plugins from registry")
	pluginsSearchCmd.Flags().StringVar(&searchType, "type", "", "filter by capability type (e.g., format, tool)")
	pluginsSearchCmd.Flags().StringVar(&searchMime, "mime", "", "filter by MIME type (e.g., text/html)")
	pluginsSearchCmd.Flags().StringVar(&searchExt, "ext", "", "filter by file extension (e.g., .docx)")
	pluginsCmd.AddCommand(pluginsListCmd)
	pluginsCmd.AddCommand(pluginsInstallCmd)
	pluginsCmd.AddCommand(pluginsUpdateCmd)
	pluginsCmd.AddCommand(pluginsSearchCmd)
	rootCmd.AddCommand(pluginsCmd)
}
