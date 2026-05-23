package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/spf13/cobra"
)

// allKLF returns true when every positional input path carries the
// `.klf` extension. Used to decide whether a tool run defaults to
// in-place output (the KLF writer is locale-additive — accumulates
// target translations on each block) or the sibling `./out/...`
// template (every other format).
func allKLF(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, p := range paths {
		if !strings.EqualFold(filepath.Ext(p), ".klf") {
			return false
		}
	}
	return true
}

// CollectorFactories maps tool names to streaming collector factories.
// Only tools that aggregate results across files need a collector.
var CollectorFactories = map[string]func() flow.Collector{
	"word-count": func() flow.Collector { return tools.NewStreamingWordCountCollector() },
}

// aiProgressWriter returns a ProgressEvent callback that writes a single
// rewriting status line to w. Thinking summaries and block counters are
// shown while running; the line is cleared when the final block completes.
func aiProgressWriter(w *os.File) func(aiprovider.ProgressEvent) {
	return func(e aiprovider.ProgressEvent) {
		if e.Done && e.Thinking == "" {
			// Block done — update counter.
			if e.TotalBlocks > 0 {
				fmt.Fprintf(w, "\r\033[K  Translating [%d/%d]", e.Block, e.TotalBlocks)
			} else {
				fmt.Fprintf(w, "\r\033[K  Translating [%d]", e.Block)
			}
			return
		}
		if e.Thinking != "" {
			// Truncate long thinking summaries to fit a terminal line.
			think := e.Thinking
			if len(think) > 60 {
				think = think[:57] + "..."
			}
			if e.TotalBlocks > 0 {
				fmt.Fprintf(w, "\r\033[K  [%d/%d] thinking: %s", e.Block, e.TotalBlocks, think)
			} else {
				fmt.Fprintf(w, "\r\033[K  [%d] thinking: %s", e.Block, think)
			}
		}
	}
}

// NewToolCommands creates cobra commands from all CLI-visible tools in the
// ToolRegistry. This replaces the old hardcoded BuiltinToolCommands list —
// the registry is the single source of truth for tool metadata.
func (a *App) NewToolCommands() []*cobra.Command {
	if a.ToolReg == nil {
		return nil
	}

	entries := a.ToolReg.CLITools()

	// Sort by category then name for stable command ordering.
	slices.SortFunc(entries, func(a, b registry.CLIToolEntry) int {
		if a.Info.Category != b.Info.Category {
			if a.Info.Category < b.Info.Category {
				return -1
			}
			return 1
		}
		if a.Info.Name < b.Info.Name {
			return -1
		}
		if a.Info.Name > b.Info.Name {
			return 1
		}
		return 0
	})

	var cmds []*cobra.Command
	for _, entry := range entries {
		toolName := string(entry.Info.Name)
		info := entry.Info
		toolSchema := entry.Schema
		var formatMaps []string

		short := info.Description
		if short == "" {
			short = info.DisplayName
		}

		cmd := &cobra.Command{
			Use:     toolName + " [files...]",
			Aliases: info.Aliases,
			Short:   short,
			GroupID: info.Category,
			Args:    cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				jsonOut, _ := cmd.Flags().GetBool("json")
				jqFilter, _ := cmd.Flags().GetString("jq")
				jsonOut = jsonOut || jqFilter != "" // --jq implies JSON
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
				var inPlace bool
				if info.WritesOutput {
					outputTmpl, _ = cmd.Flags().GetString("output")
					if outputTmpl == "" {
						// KLF writers are locale-additive: reading and
						// writing back to the same file accumulates
						// translations. For that case the natural default
						// is in-place. Other formats keep the sibling-dir
						// template so input and output can't collide.
						if allKLF(args) {
							inPlace = true
						} else {
							outputTmpl = "./out/{name}.{ext}"
						}
					}
				}

				effectiveLang := a.TargetLang
				if effectiveLang == "" && info.DefaultLocale != "" {
					effectiveLang = string(info.DefaultLocale)
				}

				tracePath, _ := cmd.Flags().GetString("trace")
				parallelBlocks, _ := cmd.Flags().GetInt("parallel-blocks")

				newTool := func() (tool.Tool, error) {
					config := ReadAllSchemaFlags(cmd, toolSchema)
					// Tools that require a termbase (e.g. term-check) get the
					// project's bound glossary injected when no glossary was
					// supplied programmatically. This makes `kapi term-check
					// fr.json` enforce the project termbase with no flag.
					if toolRequires(toolSchema, "termbase") {
						if _, ok := config["glossary"]; !ok {
							glossary, gerr := a.resolveProjectGlossary(cmd, effectiveLang)
							if gerr != nil {
								return nil, gerr
							}
							if len(glossary) > 0 {
								config["glossary"] = glossary
							}
						}
					}
					credName, _ := cmd.Flags().GetString("credential")
					if credName != "" {
						config["credential"] = credName
						// When a named credential is given and --provider was not
						// explicitly set by the user, drop the schema default so
						// ResolveCredentials can inject the credential's own
						// provider_type (fixes #637).
						if !cmd.Flags().Changed("provider") {
							delete(config, "provider")
						}
					}
					if !jsonOut && isatty.IsTerminal(os.Stderr.Fd()) {
						config["onProgress"] = aiProgressWriter(os.Stderr)
					}
					return a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), config, effectiveLang)
				}

				var collector func() flow.Collector
				if cf, ok := CollectorFactories[toolName]; ok {
					collector = cf
				}

				rc := ToolRunConfig{
					ToolName:       toolName,
					Files:          args,
					FormatMappings: mappings,
					Concurrency:    conc,
					JSONOutput:     jsonOut,
					JQ:             jqFilter,
					Colorize:       output.Colorize(cmd),
					FailOnUnknown:  failUnknown,
					NoWarn:         noWarn,
					Progress:       progress,
					OutputTemplate: outputTmpl,
					InPlace:        inPlace,
					TargetLang:     effectiveLang,
					TracePath:      tracePath,
					ParallelBlocks: parallelBlocks,
					NewTool:        newTool,
					NewCollector:   collector,
				}

				if !jsonOut && isatty.IsTerminal(os.Stderr.Fd()) {
					rc.AfterTool = func() {
						fmt.Fprint(os.Stderr, "\r\033[K")
					}
				}

				return a.RunToolOnFiles(cmd.Context(), rc)
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
		if info.WritesOutput {
			cmd.Flags().StringP("output", "o", "", "output path template (variables: {name}, {ext}, {lang})")
		}
		RegisterSchemaFlags(cmd, toolSchema)
		if toolSchema.ToolMeta != nil {
			for _, req := range toolSchema.ToolMeta.Requires {
				switch req {
				case "credentials":
					cmd.Flags().String("credential", "", "saved credential name to use (see 'kapi credentials list')")
				case "termbase":
					cmd.Flags().String("termbase", "", "named termbase or path to a glossary (defaults to the project termbase)")
				}
			}
		}
		cmd.Flags().String("trace", "", "write flow trace JSON to file (for flow visualization)")
		cmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
		cmds = append(cmds, cmd)
	}

	return cmds
}
