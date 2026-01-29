package main

import (
	"github.com/asgeirf/gokapi/core/registry"
	"github.com/asgeirf/gokapi/formats"
	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	verbose    bool
	quiet      bool
	formatFlag string
	encoding   string
	sourceLang string
	targetLang string
	formatReg  *registry.FormatRegistry
)

var rootCmd = &cobra.Command{
	Use:   "kapi",
	Short: "kapi is a localization and translation toolkit",
	Long: `kapi is a powerful localization toolkit that provides format conversion,
content extraction, translation, and quality assurance for multilingual content.

It supports a wide range of document formats and provides AI-powered
translation and quality checking capabilities.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		formatReg = registry.NewFormatRegistry()
		formats.RegisterAll(formatReg)
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
}
