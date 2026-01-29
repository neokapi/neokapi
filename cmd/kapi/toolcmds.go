package main

import (
	"context"

	"github.com/asgeirf/gokapi/core/flow"
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
	libtools "github.com/asgeirf/gokapi/lib/tools"
	"github.com/spf13/cobra"
)

// toolCommandDef describes a tool that is exposed as a top-level CLI command.
type toolCommandDef struct {
	Use          string
	Aliases      []string
	Short        string
	NewTool      func() (tool.Tool, error)
	NewCollector func() flow.Collector
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
				progress, _ := cmd.Flags().GetBool("progress")

				return RunToolOnFiles(context.Background(), ToolRunConfig{
					ToolName:      def.Use,
					Files:         args,
					Concurrency:   conc,
					JSONOutput:    jsonOut,
					FailOnUnknown: failUnknown,
					Progress:      progress,
					NewTool:       def.NewTool,
					NewCollector:  def.NewCollector,
				})
			},
		}
		cmd.Flags().Bool("json", false, "output results as JSON")
		cmd.Flags().IntP("concurrency", "j", 0, "max parallel files (0 = auto)")
		cmd.Flags().Bool("fail-on-unknown", false, "fail on files with unrecognized formats (default: skip with warning)")
		cmd.Flags().BoolP("progress", "p", false, "show progress bar")
		rootCmd.AddCommand(cmd)
	}
}
