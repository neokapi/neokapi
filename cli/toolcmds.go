package cli

import (
	"context"

	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
	libtools "github.com/gokapi/gokapi/core/tools"
	"github.com/spf13/cobra"
)

// ToolCommandDef describes a tool that is exposed as a top-level CLI command.
type ToolCommandDef struct {
	Use               string
	Aliases           []string
	Short             string
	WritesOutput      bool
	DefaultTargetLang string
	NewTool           func(targetLang string, expansion int) (tool.Tool, error)
	NewCollector      func() flow.Collector
}

// BuiltinToolCommands lists all tools exposed as top-level commands.
var BuiltinToolCommands = []ToolCommandDef{
	{
		Use:     "word-count",
		Aliases: []string{"wc"},
		Short:   "Count words in source and target text",
		NewTool: func(targetLang string, expansion int) (tool.Tool, error) {
			return libtools.NewWordCountTool(&libtools.WordCountConfig{}), nil
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
		NewTool: func(targetLang string, expansion int) (tool.Tool, error) {
			lang := targetLang
			if lang == "" {
				lang = "qps"
			}
			return libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
				TargetLocale:     model.LocaleID(lang),
				ExpansionPercent: expansion,
			}), nil
		},
	},
}

// NewToolCommands creates cobra commands for all builtin tool definitions.
func (a *App) NewToolCommands() []*cobra.Command {
	var cmds []*cobra.Command

	for _, def := range BuiltinToolCommands {
		d := def // capture loop variable
		var pseudoExpansion int
		var formatMaps []string

		cmd := &cobra.Command{
			Use:     d.Use + " [files...]",
			Aliases: d.Aliases,
			Short:   d.Short,
			Args:    cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				jsonOut, _ := cmd.Flags().GetBool("json")
				conc, _ := cmd.Flags().GetInt("concurrency")
				failUnknown, _ := cmd.Flags().GetBool("fail-on-unknown")
				strict, _ := cmd.Flags().GetBool("strict")
				failUnknown = failUnknown || strict
				noWarn, _ := cmd.Flags().GetBool("no-warn")
				progress, _ := cmd.Flags().GetBool("progress")

				mappings, err := ParseFormatMappings(formatMaps)
				if err != nil {
					return err
				}

				var outputTmpl string
				if d.WritesOutput {
					outputTmpl, _ = cmd.Flags().GetString("output")
					if outputTmpl == "" {
						outputTmpl = "./out/{name}.{ext}"
					}
				}

				effectiveLang := a.TargetLang
				if effectiveLang == "" && d.DefaultTargetLang != "" {
					effectiveLang = d.DefaultTargetLang
				}

				return a.RunToolOnFiles(context.Background(), ToolRunConfig{
					ToolName:       d.Use,
					Files:          args,
					FormatMappings: mappings,
					Concurrency:    conc,
					JSONOutput:     jsonOut,
					FailOnUnknown:  failUnknown,
					NoWarn:         noWarn,
					Progress:       progress,
					OutputTemplate: outputTmpl,
					TargetLang:     effectiveLang,
					NewTool: func() (tool.Tool, error) {
						return d.NewTool(effectiveLang, pseudoExpansion)
					},
					NewCollector: d.NewCollector,
				})
			},
		}
		a.AddProcessingFlags(cmd)
		cmd.Flags().StringArrayVarP(&formatMaps, "map", "m", nil, "map glob pattern to format (e.g. '*.docx=okf_openxml:test')")
		cmd.Flags().Bool("json", false, "output results as JSON")
		cmd.Flags().IntP("concurrency", "j", 0, "max parallel files (0 = auto)")
		cmd.Flags().Bool("fail-on-unknown", false, "exit with error if any file cannot be processed (default: skip with warning)")
		cmd.Flags().Bool("strict", false, "alias for --fail-on-unknown")
		cmd.Flags().Bool("no-warn", false, "suppress warnings for skipped files")
		cmd.Flags().BoolP("progress", "p", false, "show progress bar")
		if d.WritesOutput {
			cmd.Flags().StringP("output", "o", "", "output path template (variables: {name}, {ext}, {lang})")
		}
		if d.Use == "pseudo-translate" {
			cmd.Flags().IntVar(&pseudoExpansion, "expansion", 0, "text expansion percentage (0 = none)")
		}
		cmds = append(cmds, cmd)
	}

	return cmds
}
