package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gokapi/gokapi/core/formats"
	"github.com/gokapi/gokapi/core/plugin/loader"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/kapi/cmd/kapi/output"
	"github.com/gokapi/gokapi/platform/config"
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
	Short:        "A localization and translation toolkit",
	SilenceUsage: true,
	Long: `kapi helps you manage multilingual content — convert document formats,
translate with AI, and run quality checks across a wide range of file types.`,
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
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress output")
	rootCmd.PersistentFlags().StringVar(&pluginDir, "plugin-dir", "",
		"plugin directory")

	// Output format flags (--json, --text, --output-format)
	output.AddPersistentFlags(rootCmd)
}

// addProcessingFlags adds file-processing flags (--format, --encoding,
// --source-lang, --target-lang) to a command. These only apply to commands
// that process files (flow run, tool commands).
func addProcessingFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&formatFlag, "format", "f", "", "override input format detection")
	cmd.Flags().StringVarP(&encoding, "encoding", "e", "UTF-8", "input file encoding")
	cmd.Flags().StringVar(&sourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	cmd.Flags().StringVar(&targetLang, "target-lang", "", "target language (e.g. fr, de-DE)")
}
