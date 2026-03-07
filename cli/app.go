// Package cli provides a shared CLI base for gokapi CLI tools.
// Both kapi and bowrain build on this package, selecting which commands to expose.
package cli

import (
	"fmt"
	"log"
	"os"

	gokapiconfig "github.com/gokapi/gokapi/core/config"
	"github.com/gokapi/gokapi/core/format/schema"
	"github.com/gokapi/gokapi/core/formats"
	"github.com/gokapi/gokapi/core/plugin/loader"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/cli/output"
	"github.com/gokapi/gokapi/cli/config"
	"github.com/spf13/cobra"
)

// App holds shared CLI state that is initialized during PersistentPreRun.
// Both kapi and bowrain create an App instance and attach shared commands.
type App struct {
	FormatReg    *registry.FormatRegistry
	SchemaReg    *schema.SchemaRegistry
	PluginLoader *loader.PluginLoader
	Config       *config.AppConfig

	// Flags bound by AddPersistentFlags.
	Verbose   bool
	Quiet     bool
	CfgFile   string
	PluginDir string

	// Processing flags bound by AddProcessingFlags.
	FormatFlag string
	Encoding   string
	SourceLang string
	TargetLang string

	// RegistryResolver is an optional hook for resolving plugin registries.
	// When set, it is called before falling back to the config-based registries.
	// Brain sets this to resolve registries from .bowrain/ project config.
	RegistryResolver func() []config.RegistryEntry
}

// AddPersistentFlags registers global flags on the root command.
func (a *App) AddPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&a.CfgFile, "config", "c", "", "config file path")
	cmd.PersistentFlags().BoolVarP(&a.Verbose, "verbose", "v", false, "verbose output")
	cmd.PersistentFlags().BoolVarP(&a.Quiet, "quiet", "q", false, "suppress output")
	cmd.PersistentFlags().StringVar(&a.PluginDir, "plugin-dir", "", "plugin directory")
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
func (a *App) Init() {
	a.FormatReg = registry.NewFormatRegistry()
	formats.RegisterAll(a.FormatReg)

	// Create unified schema registry and collect native format schemas.
	a.SchemaReg = schema.NewSchemaRegistry()
	a.FormatReg.CollectNativeSchemas(a.SchemaReg)

	// Register config envelope decoders for all native formats.
	a.FormatReg.CollectNativeDecoders(gokapiconfig.DefaultRegistry)

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
	if err := a.PluginLoader.ScanMetadata(a.FormatReg); err != nil {
		if !a.Quiet {
			fmt.Fprintf(os.Stderr, "Warning: plugin scan: %v\n", err)
		}
	}

	// Merge bridge schemas into the unified registry so that CLI commands
	// (formats info, formats schema, presets list) see both native and bridge schemas.
	for _, id := range a.PluginLoader.Schemas().FilterIDs() {
		if s, ok := a.PluginLoader.Schemas().GetSchema(id); ok {
			if !a.SchemaReg.HasSchema(id) {
				a.SchemaReg.RegisterSchema(id, s)
			}
		}
	}

	// Extract native format presets (bridge presets are already extracted by ScanMetadata).
	a.SchemaReg.ExtractPresets(a.PluginLoader.Presets())

	// Apply format priority overrides from configuration.
	for name, priority := range a.Config.FormatPriorities() {
		a.FormatReg.SetFormatPriority(name, priority)
	}

	// Register lazy bridge loading: bridges start only when a non-built-in
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

// Shutdown cleans up plugin resources (stops bridge processes, etc.).
// Must be called before the process exits — typically from main() after
// Execute() returns, to ensure cleanup runs even when RunE returns an error.
func (a *App) Shutdown() {
	if a.PluginLoader != nil {
		a.PluginLoader.Shutdown()
	}
}
