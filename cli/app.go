// Package cli provides a shared CLI base for gokapi CLI tools.
// Both kapi and brain build on this package, selecting which commands to expose.
package cli

import (
	"fmt"
	"log"
	"os"

	"github.com/gokapi/gokapi/core/formats"
	"github.com/gokapi/gokapi/core/plugin/loader"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/cli/output"
	"github.com/gokapi/gokapi/cli/config"
	"github.com/spf13/cobra"
)

// App holds shared CLI state that is initialized during PersistentPreRun.
// Both kapi and brain create an App instance and attach shared commands.
type App struct {
	FormatReg    *registry.FormatRegistry
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
	if err := a.PluginLoader.LoadAll(a.FormatReg, nil); err != nil {
		if !a.Quiet {
			fmt.Fprintf(os.Stderr, "Warning: plugin loading: %v\n", err)
		}
	}

	// Apply format priority overrides from configuration.
	for name, priority := range a.Config.FormatPriorities() {
		a.FormatReg.SetFormatPriority(name, priority)
	}
}

// Shutdown cleans up plugin resources.
// Call this in PersistentPostRun.
func (a *App) Shutdown() {
	if a.PluginLoader != nil {
		a.PluginLoader.Shutdown()
	}
}
