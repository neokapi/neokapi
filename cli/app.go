// Package cli provides a shared CLI base for neokapi CLI tools.
// CLI tools build on this package, selecting which commands to expose.
package cli

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/cli/output"
	neokapiconfig "github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/plugin/loader"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/spf13/cobra"
)

// App holds shared CLI state that is initialized during PersistentPreRun.
// CLI tools create an App instance and attach shared commands.
type App struct {
	FormatReg    *registry.FormatRegistry
	ToolReg      *registry.ToolRegistry
	SchemaReg    *schema.SchemaRegistry
	PluginLoader *loader.PluginLoader
	Config       *config.AppConfig

	// Flags bound by AddPersistentFlags.
	Verbose        bool
	Quiet          bool
	CfgFile        string
	PluginDir      string
	DisablePlugins string // comma-separated plugin names to skip

	// Processing flags bound by AddProcessingFlags.
	FormatFlag string
	Encoding   string
	SourceLang string
	TargetLang string

	// RegistryResolver is an optional hook for resolving plugin registries.
	// When set, it is called before falling back to the config-based registries.
	RegistryResolver func() []config.RegistryEntry

	// projectContext is set temporarily by runFromProject so that downstream
	// methods (reader creation, writer setup) can apply project format defaults.
	projectContext *project.ProjectContext

	// projectFlowTools is set temporarily by runProjectSteps to override
	// buildFlowTools for project-defined flows.
	projectFlowTools []tool.Tool
}

// AddPersistentFlags registers global flags on the root command.
func (a *App) AddPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&a.CfgFile, "config", "c", "", "config file path")
	cmd.PersistentFlags().BoolVarP(&a.Verbose, "verbose", "v", false, "verbose output")
	cmd.PersistentFlags().BoolVarP(&a.Quiet, "quiet", "q", false, "suppress output")
	cmd.PersistentFlags().StringVar(&a.PluginDir, "plugin-dir", "", "plugin directory")
	cmd.PersistentFlags().StringVar(&a.DisablePlugins, "disable-plugins", "", "comma-separated plugin names to skip (e.g. okapi)")
	output.AddPersistentFlags(cmd)
}

// AddProcessingFlags adds file-processing flags to a command.
func (a *App) AddProcessingFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&a.FormatFlag, "format", "f", "", "override input format detection")
	cmd.Flags().StringVarP(&a.Encoding, "encoding", "e", "UTF-8", "input file encoding")
	cmd.Flags().StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	cmd.Flags().StringVar(&a.TargetLang, "target-lang", "", "target language (e.g. fr, de-DE)")
}

// Init initializes the format registry and loads plugins.
// Call this in PersistentPreRun after setting a.Config.
//
// Built-in formats are registered with static metadata — no reader/writer
// instances are created. Schemas and config decoders are registered in the
// same pass for formats that support them. Plugin metadata is loaded from
// a pre-computed cache file ({plugin_dir}/plugin-cache.json) built at
// install time, avoiding directory scanning and manifest parsing.
func (a *App) Init() {
	a.FormatReg = registry.NewFormatRegistry()
	a.SchemaReg = schema.NewSchemaRegistry()

	// Single-pass registration: formats, schemas, and config decoders.
	formats.RegisterAll(a.FormatReg, formats.RegisterOptions{
		SchemaReg: a.SchemaReg,
		ConfigReg: neokapiconfig.DefaultRegistry,
	})

	a.ToolReg = registry.NewToolRegistry()
	libtools.RegisterAll(a.ToolReg)
	aitools.RegisterAll(a.ToolReg)

	if a.Config == nil {
		a.Config = config.NewAppConfig()
	}
	_ = a.Config.Load()

	// Resolve plugin directory: flag > env > config.
	dir := a.PluginDir
	if dir == "" {
		dir = os.Getenv("KAPI_PLUGIN_DIR")
	}
	if dir == "" {
		dir = a.Config.PluginDirectory()
	}

	var logger *log.Logger
	if a.Verbose {
		logger = log.New(os.Stderr, "[plugin] ", log.LstdFlags)
	}

	a.PluginLoader = loader.NewPluginLoader(dir, logger)

	disabled := a.disabledPluginSet()
	if len(disabled) > 0 {
		a.PluginLoader.SetDisabledPlugins(disabled)
	}

	if err := a.PluginLoader.ScanMetadata(a.FormatReg); err != nil {
		if !a.Quiet {
			fmt.Fprintf(os.Stderr, "Warning: plugin scan: %v\n", err)
		}
	}

	// Merge plugin schemas into the unified registry.
	for _, id := range a.PluginLoader.Schemas().FormatIDs() {
		if s, ok := a.PluginLoader.Schemas().GetSchema(id); ok {
			if !a.SchemaReg.HasSchema(id) {
				a.SchemaReg.RegisterSchema(id, s)
			}
		}
	}

	// Extract presets from plugin schemas.
	a.SchemaReg.ExtractPresets(a.PluginLoader.Presets())

	// Apply format priority overrides from configuration.
	a.applyFormatPriorities(a.Config.FormatPriorities())

	// Lazy bridge loading: bridges start only when a non-built-in
	// format is requested for the first time.
	a.FormatReg.SetOnMiss(func() {
		a.EnsureBridgesLoaded()
	})
}

// EnsureBridgesLoaded starts bridge plugin processes if not already started.
// Call this before any file-processing command that may use plugin formats.
func (a *App) EnsureBridgesLoaded() {
	if a.PluginLoader == nil || a.PluginLoader.BridgesLoaded() {
		return
	}
	if err := a.PluginLoader.LoadBridges(a.FormatReg, nil); err != nil {
		if !a.Quiet {
			fmt.Fprintf(os.Stderr, "Warning: bridge loading: %v\n", err)
		}
	}
}

// InstalledPluginList returns the currently loaded plugins as project.InstalledPlugin
// values, suitable for passing to project.CheckPlugins or project.PopulatePlugins.
func (a *App) InstalledPluginList() []project.InstalledPlugin {
	if a.PluginLoader == nil {
		return nil
	}
	plugins := a.PluginLoader.Plugins()
	result := make([]project.InstalledPlugin, len(plugins))
	for i, p := range plugins {
		result[i] = project.InstalledPlugin{
			Name:             p.Name,
			Version:          p.Version,
			FrameworkVersion: p.FrameworkVersion,
		}
	}
	return result
}

// applyFormatPriorities applies priority overrides to the format registry.
// Keys can be exact format names or glob patterns (e.g. "okf_*").
func (a *App) applyFormatPriorities(priorities map[string]int) {
	for pattern, priority := range priorities {
		if isGlobPattern(pattern) {
			// Glob pattern — match against all registered format infos.
			for _, info := range a.FormatReg.FormatInfos() {
				if matched, _ := filepath.Match(pattern, info.Name); matched {
					a.FormatReg.SetFormatPriority(info.Name, priority)
				}
			}
		} else {
			a.FormatReg.SetFormatPriority(pattern, priority)
		}
	}
}

// isGlobPattern returns true if the string contains glob metacharacters.
func isGlobPattern(s string) bool {
	for _, c := range s {
		if c == '*' || c == '?' || c == '[' {
			return true
		}
	}
	return false
}

// disabledPluginSet returns the set of plugin names to skip.
// Resolved from: --disable-plugins flag > KAPI_DISABLE_PLUGINS env > config.
func (a *App) disabledPluginSet() map[string]bool {
	raw := a.DisablePlugins
	if raw == "" {
		raw = os.Getenv("KAPI_DISABLE_PLUGINS")
	}
	if raw == "" && a.Config != nil {
		raw = a.Config.GetString("plugins.disabled")
	}
	if raw == "" {
		return nil
	}
	set := make(map[string]bool)
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			set[name] = true
		}
	}
	return set
}

// AddCommandGroups registers Cobra command groups on the root command for
// sectioned --help output. Group IDs match the Category field on ToolCommandDef
// and plugin metadata.
func (a *App) AddCommandGroups(cmd *cobra.Command) {
	cmd.AddGroup(
		&cobra.Group{ID: "processing", Title: "Processing:"},
		&cobra.Group{ID: "translation", Title: "Translation:"},
		&cobra.Group{ID: "quality", Title: "Quality:"},
		&cobra.Group{ID: "analysis", Title: "Analysis:"},
		&cobra.Group{ID: "text-processing", Title: "Text Processing:"},
		&cobra.Group{ID: "management", Title: "Info & Management:"},
	)
}

// Shutdown cleans up plugin resources (stops bridge processes, etc.).
// Must be called before the process exits — typically from main() after
// Execute() returns, to ensure cleanup runs even when RunE returns an error.
func (a *App) Shutdown() {
	if a.PluginLoader != nil {
		a.PluginLoader.Shutdown()
	}
}
