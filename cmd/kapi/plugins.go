package main

import (
	"fmt"
	"sort"
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
	plugins := pluginLoader.Plugins()

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

	// Group installed versions by name.
	byName := make(map[string][]string)
	var nameOrder []string
	for _, iv := range installed {
		if _, exists := byName[iv.Name]; !exists {
			nameOrder = append(nameOrder, iv.Name)
		}
		byName[iv.Name] = append(byName[iv.Name], iv.Version)
	}

	// Include loaded plugins that may lack version tracking.
	for _, p := range plugins {
		if _, exists := byName[p.Name]; !exists {
			nameOrder = append(nameOrder, p.Name)
			v := p.Version
			if v == "" {
				v = "0.0.0"
			}
			byName[p.Name] = []string{v}
		}
	}

	sort.Strings(nameOrder)

	var items []string
	for _, name := range nameOrder {
		versions := byName[name]
		sort.Slice(versions, func(i, j int) bool {
			return registry.CompareSemver(versions[i], versions[j]) > 0
		})
		// Latest version shown as bare name.
		items = append(items, name+" \u2714")
		// Older versions shown as name@version.
		for _, v := range versions[1:] {
			items = append(items, name+"@"+v+" \u2714")
		}
	}

	fmt.Print(formatColumns(items))
	fmt.Printf("\nPlugin directory: %s\n", pluginLoader.Dir())
	return nil
}

func listAvailablePlugins() error {
	cfg := config.NewAppConfig()
	_ = cfg.Load()
	reg := registry.NewRemoteRegistry(cfg.RegistryURL(), pluginLoader.Dir())

	groups, err := reg.ListAvailableGrouped()
	if err != nil {
		return fmt.Errorf("fetching available plugins: %w", err)
	}

	if len(groups) == 0 {
		fmt.Println("No plugins available in the registry.")
		return nil
	}

	// Build installed version set.
	installed, _ := reg.ListInstalled()
	installedSet := make(map[string]bool)
	for _, iv := range installed {
		installedSet[iv.Name+"/"+iv.Version] = true
	}

	// Build display items: bare name for latest, name@version for others.
	var items []string
	for _, g := range groups {
		label := g.Name
		if installedSet[g.Name+"/"+g.Latest.Version] {
			label += " \u2714"
		}
		items = append(items, label)

		for _, v := range g.Versions[1:] {
			vLabel := g.Name + "@" + v.Version
			if installedSet[g.Name+"/"+v.Version] {
				vLabel += " \u2714"
			}
			items = append(items, vLabel)
		}
	}

	fmt.Print(formatColumns(items))
	return nil
}

// formatColumns arranges items in a multi-column grid layout, similar to
// homebrew's search output. Returns the formatted string.
func formatColumns(items []string) string {
	if len(items) == 0 {
		return ""
	}

	const termWidth = 80

	maxWidth := 0
	for _, item := range items {
		w := len([]rune(item))
		if w > maxWidth {
			maxWidth = w
		}
	}

	colWidth := maxWidth + 2
	cols := termWidth / colWidth
	if cols < 1 {
		cols = 1
	}

	var sb strings.Builder
	for i, item := range items {
		if i > 0 && i%cols == 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(item)
		pad := colWidth - len([]rune(item))
		if pad > 0 {
			sb.WriteString(strings.Repeat(" ", pad))
		}
	}
	sb.WriteByte('\n')
	return sb.String()
}

var pluginsInstallCmd = &cobra.Command{
	Use:   "install <name[@version]>",
	Short: "Install a plugin from the registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := registry.ParsePluginRef(args[0])

		cfg := config.NewAppConfig()
		_ = cfg.Load()
		reg := registry.NewRemoteRegistry(cfg.RegistryURL(), pluginLoader.Dir())

		if !quiet {
			fmt.Printf("Installing plugin: %s\n", ref)
		}

		result, err := reg.InstallPlugin(ref)
		if err != nil {
			return fmt.Errorf("installing %s: %w", ref, err)
		}

		if !quiet {
			fmt.Printf("Installed %s v%s (%s)\n", result.Name, result.Version, result.InstallType)
			for _, f := range result.Files {
				fmt.Printf("  \u2192 %s\n", f)
			}
		}
		return nil
	},
}

var pluginsUpdateCmd = &cobra.Command{
	Use:   "update [name[@version]]",
	Short: "Update installed plugins",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.NewAppConfig()
		_ = cfg.Load()
		reg := registry.NewRemoteRegistry(cfg.RegistryURL(), pluginLoader.Dir())

		if len(args) == 1 {
			// Update a specific plugin (installs latest side-by-side).
			ref := registry.ParsePluginRef(args[0])
			if !quiet {
				fmt.Printf("Updating plugin: %s\n", ref)
			}
			result, err := reg.InstallPlugin(ref)
			if err != nil {
				return fmt.Errorf("updating %s: %w", ref, err)
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
				fmt.Printf("Updating %s: %s \u2192 %s\n", u.Name, u.InstalledVersion, u.AvailableVersion)
			}
			result, err := reg.InstallPlugin(registry.PluginRef{Name: u.Name})
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update %s: %v\n", u.Name, err)
				continue
			}
			fmt.Printf("Updated %s to v%s\n", result.Name, result.Version)
		}
		return nil
	},
}

var pluginsRemoveCmd = &cobra.Command{
	Use:   "remove <name[@version]>",
	Short: "Remove an installed plugin",
	Long: `Remove an installed plugin.

  kapi plugins remove okapi@1.46.0   Remove a specific version
  kapi plugins remove okapi           Remove all versions`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := registry.ParsePluginRef(args[0])

		cfg := config.NewAppConfig()
		_ = cfg.Load()
		reg := registry.NewRemoteRegistry(cfg.RegistryURL(), pluginLoader.Dir())

		if !quiet {
			if ref.IsVersioned() {
				fmt.Printf("Removing plugin: %s\n", ref)
			} else {
				fmt.Printf("Removing all versions of plugin: %s\n", ref.Name)
			}
		}

		if err := reg.RemovePlugin(ref); err != nil {
			return fmt.Errorf("removing %s: %w", ref, err)
		}

		if !quiet {
			fmt.Printf("Removed %s\n", ref)
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
	pluginsCmd.AddCommand(pluginsRemoveCmd)
	pluginsCmd.AddCommand(pluginsSearchCmd)
	rootCmd.AddCommand(pluginsCmd)
}
