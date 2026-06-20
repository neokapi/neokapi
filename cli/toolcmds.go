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
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
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
	"word-count":    func() flow.Collector { return coretools.NewStreamingWordCountCollector() },
	"segment-count": func() flow.Collector { return coretools.NewStreamingSegCountCollector() },
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

// toolExamples maps tool names to their cobra Example strings. Each entry is a
// newline-separated list of representative, runnable commands using the bundled
// playground fixtures (messages.json, app.xliff, page.html, etc.) so they work
// in the wasm CLI playground with no uploads.
//
// AI/MT commands use demo mode (no --provider flag needed in the playground).
var toolExamples = map[string]string{
	// ── Analysis ────────────────────────────────────────────────────────
	"word-count": `  kapi word-count messages.json
  kapi word-count app.xliff --json`,
	"char-count": `  kapi char-count messages.json
  kapi char-count page.html`,
	"segment-count": `  kapi segment-count messages.json
  kapi segment-count app.xliff`,
	"scoping-report": `  kapi scoping-report messages.json
  kapi scoping-report app.xliff --json`,
	"repetition-analysis": `  kapi repetition-analysis messages.json
  kapi repetition-analysis app.xliff`,

	// ── Quality ─────────────────────────────────────────────────────────
	"qa": `  kapi qa app.xliff --target-lang fr
  kapi qa app.xliff --target-lang fr --provider anthropic
  kapi qa app.xliff --target-lang de --json`,
	"term-check": `  kapi term-check app.xliff --source-lang en --target-lang fr
  kapi term-check messages.json --source-lang en --target-lang fr`,
	"inconsistency-check": `  kapi inconsistency-check app.xliff --target-lang fr
  kapi inconsistency-check app.xliff --target-lang de`,
	"length-check": `  kapi length-check app.xliff --target-lang fr
  kapi length-check app.xliff --target-lang ja`,
	"chars-check": `  kapi chars-check app.xliff --target-lang fr
  kapi chars-check app.xliff --target-lang zh`,
	"pattern-check": `  kapi pattern-check app.xliff --target-lang fr
  kapi pattern-check app.xliff --target-lang de`,
	"brand-vocab-check": `  kapi brand-vocab-check app.xliff --target-lang fr
  kapi brand-vocab-check messages.json --target-lang de`,

	// ── Translation ─────────────────────────────────────────────────────
	"pseudo-translate": `  kapi pseudo-translate messages.json -o messages.pseudo.json
  kapi pseudo-translate app.xliff -o app.pseudo.xliff --target-lang qps`,
	"translate": `  kapi translate messages.json --target-lang fr
  kapi translate app.xliff --target-lang de --provider deepl
  kapi translate app.xliff --target-lang de -o app.de.xliff`,
	"tm-leverage": `  kapi tm-leverage app.xliff --target-lang fr
  kapi tm-leverage messages.json --target-lang de`,

	// ── Text Processing ─────────────────────────────────────────────────
	"search-replace": `  kapi search-replace messages.json --find "foo" --replace "bar"
  kapi search-replace page.html --find "colour" --replace "color"`,
	"case-transform": `  kapi case-transform messages.json --mode upper
  kapi case-transform messages.json --mode lower`,
	"segmentation": `  kapi segmentation messages.json
  kapi segmentation app.xliff`,

	// ── AI Quality ───────────────────────────────────────────────────────
	"ai-review": `  kapi ai-review app.xliff --target-lang fr
  kapi ai-review messages.json --target-lang de`,
	"brand-voice-check": `  kapi brand-voice-check messages.json --target-lang fr
  kapi brand-voice-check app.xliff --target-lang de`,
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
			Example: toolExamples[toolName],
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
				var defaultLayout bool
				if info.WritesOutput {
					outputTmpl, _ = cmd.Flags().GetString("output")
					outputDir, _ := cmd.Flags().GetString("output-dir")
					switch {
					case outputTmpl != "":
						// Explicit -o template wins.
					case outputDir != "":
						// Root outputs under DIR using a locale-dir layout
						// (DIR/{lang}/<file>), mirroring tsc/babel --out-dir.
						outputTmpl = filepath.Join(outputDir, "{lang}") + string(filepath.Separator)
					case allKLF(args):
						// KLF writers are locale-additive: reading and writing
						// back to the same file accumulates translations, so the
						// natural default is in-place.
						inPlace = true
					default:
						// Locale-aware default, resolved per file in the runner:
						// swap the source locale in the input path if present
						// (locales/en/app.json → locales/fr/app.json), else place
						// the file under a {lang}/ directory beside the input
						// (messages.json → fr/messages.json).
						defaultLayout = true
					}
				}

				effectiveLang := a.TargetLang
				if effectiveLang == "" && info.DefaultLocale != "" {
					effectiveLang = string(info.DefaultLocale)
				}

				tracePath, _ := cmd.Flags().GetString("trace")
				parallelBlocks, _ := cmd.Flags().GetInt("parallel-blocks")

				// Tools that require a TM (e.g. tm-leverage) get a real SQLite
				// TM provider resolved from --tm or the project's .kapi/tm.db,
				// opened once and shared across every input file. Without this
				// the tool's config factory falls back to NullTMProvider and
				// leverages nothing. Mirrors the termbase glossary injection
				// below and reuses the flow path's TM opening logic.
				var tmProvider coretools.TMProvider
				if toolRequires(toolSchema, "tm") {
					p, cleanup, terr := a.openToolTM(cmd)
					if terr != nil {
						return terr
					}
					defer cleanup()
					tmProvider = p
				}

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
					// Drop the flag/schema default provider+model when the user did
					// NOT explicitly set them, so a configured default can take
					// effect (project recipe defaults, then the app-config
					// ai.provider/ai.model applied by the registry preprocessor).
					// Without this, the flag's "anthropic" default would always sit
					// in the config and mask the configured default. When nothing is
					// configured, AI tools still fall back to their schema default
					// downstream, so behavior is unchanged for the no-config case.
					if f := cmd.Flags().Lookup("provider"); f != nil && !cmd.Flags().Changed("provider") {
						delete(config, "provider")
					}
					if f := cmd.Flags().Lookup("model"); f != nil && !cmd.Flags().Changed("model") {
						delete(config, "model")
					}
					credName, _ := cmd.Flags().GetString("credential")
					if credName != "" {
						config["credential"] = credName
					}
					if !jsonOut && isatty.IsTerminal(os.Stderr.Fd()) {
						config["onProgress"] = aiProgressWriter(os.Stderr)
					}
					t, terr := a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), config, effectiveLang)
					if terr != nil {
						return nil, terr
					}
					// The tm-leverage config factory cannot read a non-JSON
					// provider from the config map (Provider is json:"-"), so it
					// defaults to NullTMProvider. Swap in the resolved SQLite TM
					// on the created tool's config so it actually leverages.
					// SourceLocale is also schema-hidden and never populated from
					// --source-lang by the factory, so the SQLite lookup would run
					// with an empty source locale and match nothing — set it from
					// the resolved source language so exact/fuzzy lookups hit.
					if tmProvider != nil {
						if cfg, ok := t.Config().(*coretools.TMLeverageConfig); ok {
							cfg.Provider = tmProvider
							if cfg.SourceLocale.IsEmpty() && a.SourceLang != "" {
								cfg.SourceLocale = model.LocaleID(a.SourceLang)
							}
						}
					}
					return t, nil
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
					Colorize:       output.Colorize(cmd, cmd.OutOrStdout()),
					FailOnUnknown:  failUnknown,
					NoWarn:         noWarn,
					Progress:       progress,
					OutputTemplate: outputTmpl,
					InPlace:        inPlace,
					DefaultLayout:  defaultLayout,
					TargetLang:     effectiveLang,
					TracePath:      tracePath,
					ParallelBlocks: parallelBlocks,
					NewTool:        newTool,
					NewCollector:   collector,
				}
				if p, _ := cmd.Flags().GetBool("pack"); p {
					rc.Pack = true
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
		cmd.Flags().Bool("pack", false, "when transforming a .klz, also eject the result to the .klz (auto-pack)")
		if info.WritesOutput {
			cmd.Flags().StringP("output", "o", "", "output path template (variables: {dir}, {name}, {ext}, {lang})")
			cmd.Flags().String("output-dir", "", "write outputs under DIR/{lang}/ (default: beside the input, mirroring its locale layout)")
		}
		RegisterSchemaFlags(cmd, toolSchema)
		if toolSchema.ToolMeta != nil {
			for _, req := range toolSchema.ToolMeta.Requires {
				switch req {
				case "credentials":
					cmd.Flags().String("credential", "", "saved credential name to use (see 'kapi credentials list')")
				case "termbase":
					cmd.Flags().String("termbase", "", "named termbase or path to a glossary (defaults to the project termbase)")
				case "tm":
					cmd.Flags().String("tm", "", "named TM or path to a .db (defaults to the project TM at .kapi/tm.db)")
				}
			}
		}
		cmd.Flags().String("trace", "", "write flow trace JSON to file (for flow visualization)")
		cmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
		cmds = append(cmds, cmd)
	}

	return cmds
}
