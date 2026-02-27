package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/gokapi/gokapi/core/plugin/registry"
	"github.com/gokapi/gokapi/platform/cli/output"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// NewPluginsCmd creates the plugins command group (list, install, update, remove, search).
func (a *App) NewPluginsCmd() *cobra.Command {
	pluginsCmd := &cobra.Command{
		Use:   "plugins",
		Short: "Manage plugins and bundles",
	}

	var availableFlag bool
	pluginsListCmd := &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			if availableFlag {
				return a.listAvailablePlugins(cmd)
			}
			return a.listInstalledPlugins(cmd)
		},
	}
	pluginsListCmd.Flags().BoolVarP(&availableFlag, "available", "a", false, "list available plugins from registry")

	pluginsInstallCmd := &cobra.Command{
		Use:   "install <name[@version]>",
		Short: "Install a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := registry.ParsePluginRef(args[0])
			reg, cleanup := a.newProgressRegistry()
			defer cleanup()

			if !a.Quiet {
				fmt.Fprintf(os.Stderr, "Installing plugin: %s\n", ref)
			}

			result, err := reg.InstallPlugin(ref)
			if err != nil {
				return fmt.Errorf("installing %s: %w", ref, err)
			}

			out := output.PluginInstallOutput{
				Name:        result.Name,
				Version:     result.Version,
				InstallType: result.InstallType,
				Files:       result.Files,
			}
			return output.Print(cmd, out)
		},
	}

	pluginsUpdateCmd := &cobra.Command{
		Use:   "update [name[@version]]",
		Short: "Update plugins",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, cleanup := a.newProgressRegistry()
			defer cleanup()

			if len(args) == 1 {
				ref := registry.ParsePluginRef(args[0])
				if !a.Quiet {
					fmt.Fprintf(os.Stderr, "Updating plugin: %s\n", ref)
				}
				result, err := reg.InstallPlugin(ref)
				if err != nil {
					return fmt.Errorf("updating %s: %w", ref, err)
				}
				out := output.PluginUpdateOutput{
					Updated: []output.PluginUpdateEntry{
						{Name: result.Name, NewVersion: result.Version},
					},
				}
				return output.Print(cmd, out)
			}

			updates, err := reg.CheckUpdates()
			if err != nil {
				return fmt.Errorf("checking for updates: %w", err)
			}

			if len(updates) == 0 {
				out := output.PluginUpdateOutput{UpToDate: true}
				return output.Print(cmd, out)
			}

			var entries []output.PluginUpdateEntry
			for _, u := range updates {
				if !a.Quiet {
					fmt.Fprintf(os.Stderr, "Updating %s: %s \u2192 %s\n", u.Name, u.InstalledVersion, u.AvailableVersion)
				}
				result, err := reg.InstallPlugin(registry.PluginRef{Name: u.Name})
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update %s: %v\n", u.Name, err)
					continue
				}
				entries = append(entries, output.PluginUpdateEntry{
					Name:       result.Name,
					OldVersion: u.InstalledVersion,
					NewVersion: result.Version,
				})
			}

			out := output.PluginUpdateOutput{Updated: entries}
			return output.Print(cmd, out)
		},
	}

	pluginsRemoveCmd := &cobra.Command{
		Use:   "remove <name[@version]>",
		Short: "Remove a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := registry.ParsePluginRef(args[0])
			reg := registry.NewRemoteRegistry(a.Config.RegistryURL(), a.PluginLoader.Dir())

			if !a.Quiet {
				if ref.IsVersioned() {
					fmt.Fprintf(os.Stderr, "Removing plugin: %s\n", ref)
				} else {
					fmt.Fprintf(os.Stderr, "Removing all versions of plugin: %s\n", ref.Name)
				}
			}

			if err := reg.RemovePlugin(ref); err != nil {
				return fmt.Errorf("removing %s: %w", ref, err)
			}

			out := output.PluginRemoveOutput{
				Name:    ref.Name,
				Version: ref.Version,
			}
			return output.Print(cmd, out)
		},
	}

	var (
		searchType   string
		searchMime   string
		searchExt    string
		searchBundle bool
		searchFormat bool
		searchTool   bool
	)

	pluginsSearchCmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search for plugins and bundles",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var query string
			if len(args) > 0 {
				query = args[0]
			}

			hasFilter := searchType != "" || searchMime != "" || searchExt != "" ||
				searchBundle || searchFormat || searchTool

			if query == "" && !hasFilter {
				return fmt.Errorf("provide a search query or use --type, --mime, --ext, --bundle, --format, or --tool flags")
			}

			reg := registry.NewRemoteRegistry(a.Config.RegistryURL(), a.PluginLoader.Dir())

			if hasFilter {
				opts := registry.SearchOptions{
					Query:      query,
					Type:       searchType,
					MimeType:   searchMime,
					Extension:  searchExt,
					BundleOnly: searchBundle,
					FormatOnly: searchFormat,
					ToolOnly:   searchTool,
				}
				return a.searchPluginsAdvanced(cmd, reg, opts)
			}

			results, err := reg.SearchPlugins(query)
			if err != nil {
				return fmt.Errorf("searching plugins: %w", err)
			}

			var entries []output.PluginSearchEntry
			for _, m := range results {
				desc := m.Description
				if len(desc) > 50 {
					desc = desc[:47] + "..."
				}
				entries = append(entries, output.PluginSearchEntry{
					Name:        m.Name,
					Version:     m.Version,
					PluginType:  m.PluginType,
					Description: desc,
				})
			}

			out := output.PluginSearchOutput{
				Plugins: entries,
				Total:   len(entries),
			}
			return output.Print(cmd, out)
		},
	}
	pluginsSearchCmd.Flags().StringVar(&searchType, "type", "", "filter by capability type (e.g., format, tool)")
	pluginsSearchCmd.Flags().StringVar(&searchMime, "mime", "", "filter by MIME type (e.g., text/html)")
	pluginsSearchCmd.Flags().StringVar(&searchExt, "ext", "", "filter by file extension (e.g., .docx)")
	pluginsSearchCmd.Flags().BoolVar(&searchBundle, "bundle", false, "show only bundles")
	pluginsSearchCmd.Flags().BoolVar(&searchFormat, "format", false, "show only plugins providing format capabilities")
	pluginsSearchCmd.Flags().BoolVar(&searchTool, "tool", false, "show only plugins providing tool capabilities")

	pluginsCmd.AddCommand(pluginsListCmd)
	pluginsCmd.AddCommand(pluginsInstallCmd)
	pluginsCmd.AddCommand(pluginsUpdateCmd)
	pluginsCmd.AddCommand(pluginsRemoveCmd)
	pluginsCmd.AddCommand(pluginsSearchCmd)

	return pluginsCmd
}

func (a *App) listInstalledPlugins(cmd *cobra.Command) error {
	plugins := a.PluginLoader.Plugins()
	reg := registry.NewRemoteRegistry(a.Config.RegistryURL(), a.PluginLoader.Dir())
	installed, _ := reg.ListInstalled()

	if len(plugins) == 0 && len(installed) == 0 {
		out := output.PluginsListOutput{
			Plugins: []output.PluginInfo{},
			Total:   0,
		}
		if output.GetFormat(cmd) == output.FormatJSON {
			return output.Print(cmd, out)
		}
		fmt.Fprintf(os.Stderr, "No plugins installed.\n")
		fmt.Fprintf(os.Stderr, "Plugin directory: %s\n", a.PluginLoader.Dir())
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Use 'plugins search <query>' to find plugins and bundles,")
		fmt.Fprintln(os.Stderr, "or 'plugins list -a' to see all available plugins and bundles.")
		return nil
	}

	type installedInfo struct {
		installType string
		pluginType  string
		formats     int
	}
	installedByKey := make(map[string]installedInfo)
	for _, iv := range installed {
		installedByKey[iv.Name+"/"+iv.Version] = installedInfo{
			installType: iv.InstallType,
			pluginType:  iv.PluginType,
			formats:     iv.FormatCount(),
		}
	}

	byName := make(map[string][]string)
	var nameOrder []string
	for _, iv := range installed {
		if _, exists := byName[iv.Name]; !exists {
			nameOrder = append(nameOrder, iv.Name)
		}
		byName[iv.Name] = append(byName[iv.Name], iv.Version)
	}

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

	var pluginInfos []output.PluginInfo
	for _, name := range nameOrder {
		versions := byName[name]
		sort.Slice(versions, func(i, j int) bool {
			return registry.CompareSemver(versions[i], versions[j]) > 0
		})
		for _, v := range versions {
			info := output.PluginInfo{
				Name:    name,
				Version: v,
				Status:  "installed",
				Path:    a.PluginLoader.Dir(),
			}
			if ii, ok := installedByKey[name+"/"+v]; ok {
				info.PluginType = ii.pluginType
				info.Formats = ii.formats
				if info.PluginType == "" {
					info.PluginType = ii.installType
				}
			}
			pluginInfos = append(pluginInfos, info)
		}
	}

	out := output.PluginsListOutput{
		Plugins: pluginInfos,
		Total:   len(pluginInfos),
	}
	return output.Print(cmd, out)
}

func (a *App) listAvailablePlugins(cmd *cobra.Command) error {
	reg := registry.NewRemoteRegistry(a.Config.RegistryURL(), a.PluginLoader.Dir())

	groups, err := reg.ListAvailableGrouped()
	if err != nil {
		return fmt.Errorf("fetching available plugins: %w", err)
	}

	if len(groups) == 0 {
		out := output.PluginsListOutput{
			Plugins: []output.PluginInfo{},
			Total:   0,
		}
		return output.Print(cmd, out)
	}

	installed, _ := reg.ListInstalled()
	installedSet := make(map[string]bool)
	for _, iv := range installed {
		installedSet[iv.Name+"/"+iv.Version] = true
	}

	var pluginInfos []output.PluginInfo
	for _, g := range groups {
		for _, v := range g.Versions {
			status := "available"
			if installedSet[g.Name+"/"+v.Version] {
				status = "installed"
			}
			var formatCount int
			for _, cap := range v.Capabilities {
				if cap.Type == "format" {
					formatCount++
				}
			}
			pluginInfos = append(pluginInfos, output.PluginInfo{
				Name:       g.Name,
				Version:    v.Version,
				PluginType: v.PluginType,
				Status:     status,
				Formats:    formatCount,
			})
		}
	}

	out := output.PluginsListOutput{
		Plugins: pluginInfos,
		Total:   len(pluginInfos),
	}
	return output.Print(cmd, out)
}

func (a *App) searchPluginsAdvanced(cmd *cobra.Command, reg *registry.RemoteRegistry, opts registry.SearchOptions) error {
	results, err := reg.SearchPluginsAdvanced(opts)
	if err != nil {
		return fmt.Errorf("searching plugins: %w", err)
	}

	var entries []output.PluginSearchEntry
	for _, m := range results {
		desc := m.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}

		capInfo := matchingCapabilities(m, opts)
		if capInfo != "" {
			desc = capInfo
		}

		entries = append(entries, output.PluginSearchEntry{
			Name:        m.Name,
			Version:     m.Version,
			PluginType:  m.PluginType,
			Description: desc,
		})
	}

	out := output.PluginSearchOutput{
		Plugins: entries,
		Total:   len(entries),
	}
	return output.Print(cmd, out)
}

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

// barWriter adapts an mpb.Bar to io.Writer for progress tracking.
type barWriter struct {
	bar *mpb.Bar
}

func (w *barWriter) Write(p []byte) (int, error) {
	w.bar.IncrBy(len(p))
	return len(p), nil
}

func (a *App) newProgressRegistry() (reg *registry.RemoteRegistry, cleanup func()) {
	reg = registry.NewRemoteRegistry(a.Config.RegistryURL(), a.PluginLoader.Dir())
	cleanup = func() {}

	if a.Quiet {
		return reg, cleanup
	}

	var progress *mpb.Progress
	var bar *mpb.Bar

	reg.OnProgress = func(totalBytes int64) io.Writer {
		progress = mpb.New(mpb.WithOutput(os.Stderr))
		if totalBytes > 0 {
			bar = progress.New(totalBytes,
				mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Padding(" ").Rbound("]"),
				mpb.PrependDecorators(decor.Counters(decor.SizeB1024(0), "%.1f / %.1f")),
				mpb.AppendDecorators(decor.EwmaSpeed(decor.SizeB1024(0), "% .1f", 30)),
			)
		} else {
			bar = progress.New(0,
				mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Padding(" ").Rbound("]"),
				mpb.PrependDecorators(decor.Counters(decor.SizeB1024(0), "%.1f")),
			)
		}
		return &barWriter{bar: bar}
	}

	cleanup = func() {
		if progress != nil {
			if bar != nil {
				bar.Abort(false)
			}
			progress.Wait()
		}
	}

	return reg, cleanup
}
