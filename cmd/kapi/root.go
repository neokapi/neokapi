package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gokapi/gokapi/cmd/kapi/output"
	"github.com/gokapi/gokapi/core/config"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/formats"
	"github.com/gokapi/gokapi/plugin/loader"
	"github.com/spf13/cobra"
)

var (
	cfgFile      string
	verbose      bool
	quiet        bool
	formatFlag   string
	encoding     string
	sourceLang   string
	targetLang   string
	pluginDir    string
	formatReg    *registry.FormatRegistry
	pluginLoader *loader.PluginLoader
)

var rootCmd = &cobra.Command{
	Use:          "kapi",
	Short:        "kapi is a localization and translation toolkit",
	SilenceUsage: true,
	Long: `kapi is a powerful localization toolkit that provides format conversion,
content extraction, translation, and quality assurance for multilingual content.

It supports a wide range of document formats and provides AI-powered
translation and quality checking capabilities.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		formatReg = registry.NewFormatRegistry()
		formats.RegisterAll(formatReg)

		// Load configuration.
		cfg := config.NewAppConfig()
		_ = cfg.Load()

		// Resolve plugin directory: flag > env > config.
		dir := pluginDir
		if dir == "" {
			dir = os.Getenv("KAPI_PLUGIN_DIR")
		}
		if dir == "" {
			dir = cfg.PluginDirectory()
		}

		var logger *log.Logger
		if verbose {
			logger = log.New(os.Stderr, "[plugin] ", log.LstdFlags)
		}

		pluginLoader = loader.NewPluginLoader(dir, logger)
		if err := pluginLoader.LoadAll(formatReg, nil); err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "Warning: plugin loading: %v\n", err)
			}
		}

		// Apply format priority overrides from configuration.
		for name, priority := range cfg.FormatPriorities() {
			formatReg.SetFormatPriority(name, priority)
		}
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if pluginLoader != nil {
			pluginLoader.Shutdown()
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: gokapi.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress output")
	rootCmd.PersistentFlags().StringVarP(&formatFlag, "format", "f", "", "override input format detection")
	rootCmd.PersistentFlags().StringVarP(&encoding, "encoding", "e", "UTF-8", "input encoding")
	rootCmd.PersistentFlags().StringVar(&sourceLang, "source-lang", "en", "source language (BCP 47)")
	rootCmd.PersistentFlags().StringVar(&targetLang, "target-lang", "", "target language (BCP 47)")
	rootCmd.PersistentFlags().StringVar(&pluginDir, "plugin-dir", "",
		"plugin directory (default: $HOME/.kapi/plugins, env: KAPI_PLUGIN_DIR)")

	// Output format flags (--json, --text, --output-format)
	output.AddPersistentFlags(rootCmd)
}
