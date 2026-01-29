package main

import (
	"context"

	"github.com/asgeirf/gokapi/core/flow"
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
	libtools "github.com/asgeirf/gokapi/lib/tools"
	"github.com/spf13/cobra"
)

// pseudoExpansion is the expansion percentage for pseudo-translate.
var pseudoExpansion int

// toolCommandDef describes a tool that is exposed as a top-level CLI command.
type toolCommandDef struct {
	Use               string
	Aliases           []string
	Short             string
	WritesOutput      bool   // tool produces file output via format writers
	DefaultTargetLang string // fallback target language when --target-lang is not set
	NewTool           func() (tool.Tool, error)
	NewCollector      func() flow.Collector
}

// builtinToolCommands lists all tools exposed as top-level commands.
// tools_cmd.go reads this for `kapi tools` output.
var builtinToolCommands = []toolCommandDef{
	{
		Use:     "word-count",
		Aliases: []string{"wc"},
		Short:   "Count words in source and target text",
		NewTool: func() (tool.Tool, error) {
			return libtools.NewWordCountTool(&libtools.WordCountConfig{
				Locale: model.LocaleID(targetLang),
			}), nil
		},
		NewCollector: func() flow.Collector {
			return libtools.NewWordCountCollector()
		},
	},
	{
		Use:               "pseudo-translate",
		Aliases:           []string{"pseudo"},
		Short:             "Generate pseudo-translations for localization testing",
		WritesOutput:      true,
		DefaultTargetLang: "qps",
		NewTool: func() (tool.Tool, error) {
			lang := targetLang
			if lang == "" {
				lang = "qps"
			}
			return libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
				TargetLocale:     model.LocaleID(lang),
				ExpansionPercent: pseudoExpansion,
			}), nil
		},
	},
}

func init() {
	for _, def := range builtinToolCommands {
		def := def // capture
		cmd := &cobra.Command{
			Use:     def.Use + " [files...]",
			Aliases: def.Aliases,
			Short:   def.Short,
			Args:    cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				jsonOut, _ := cmd.Flags().GetBool("json")
				conc, _ := cmd.Flags().GetInt("concurrency")
				failUnknown, _ := cmd.Flags().GetBool("fail-on-unknown")
				noWarn, _ := cmd.Flags().GetBool("no-warn")
				progress, _ := cmd.Flags().GetBool("progress")

				var outputTmpl string
				if def.WritesOutput {
					outputTmpl, _ = cmd.Flags().GetString("output")
					if outputTmpl == "" {
						outputTmpl = "./out/{name}.{ext}"
					}
				}

				effectiveLang := targetLang
				if effectiveLang == "" && def.DefaultTargetLang != "" {
					effectiveLang = def.DefaultTargetLang
				}

				return RunToolOnFiles(context.Background(), ToolRunConfig{
					ToolName:       def.Use,
					Files:          args,
					Concurrency:    conc,
					JSONOutput:     jsonOut,
					FailOnUnknown:  failUnknown,
					NoWarn:         noWarn,
					Progress:       progress,
					OutputTemplate: outputTmpl,
					TargetLang:     effectiveLang,
					NewTool:        def.NewTool,
					NewCollector:   def.NewCollector,
				})
			},
		}
		cmd.Flags().Bool("json", false, "output results as JSON")
		cmd.Flags().IntP("concurrency", "j", 0, "max parallel files (0 = auto)")
		cmd.Flags().Bool("fail-on-unknown", false, "fail on files with unrecognized formats (default: skip with warning)")
		cmd.Flags().Bool("no-warn", false, "suppress warnings for skipped files")
		cmd.Flags().BoolP("progress", "p", false, "show progress bar")
		if def.WritesOutput {
			cmd.Flags().StringP("output", "o", "", "output path template (variables: {name}, {ext}, {lang})")
		}
		if def.Use == "pseudo-translate" {
			cmd.Flags().IntVar(&pseudoExpansion, "expansion", 0, "text expansion percentage (0 = none)")
		}
		rootCmd.AddCommand(cmd)
	}
}
