package cli

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/providers/ai"
	"github.com/spf13/cobra"
)

// ToolCommandDef describes a tool that is exposed as a top-level CLI command.
type ToolCommandDef struct {
	Use               string
	Aliases           []string
	Short             string
	Category          string // e.g. "translation", "quality", "analysis", "text-processing"
	WritesOutput      bool
	// DefaultParallelBlocks is the number of blocks to process in parallel
	// for IO-bound tools (e.g., AI-powered). 0 means sequential.
	DefaultParallelBlocks int

	// NewTool creates the tool. It receives the resolved target language and
	// the cobra command so it can read tool-specific flags.
	NewTool func(cmd *cobra.Command, targetLang string) (tool.Tool, error)

	// NewToolFromConfig creates the tool from a schema-derived config map.
	// Used when Schema is set (schema-driven flag generation).
	NewToolFromConfig func(config map[string]any, targetLang string) (tool.Tool, error)

	// Schema optionally declares the tool's parameter schema.
	// When set, CLI flags are auto-generated from the schema properties.
	Schema *schema.ComponentSchema

	// NewCollector optionally creates a streaming collector for aggregation tools.
	NewCollector func() flow.Collector

	// AddFlags registers tool-specific flags on the cobra command.
	// Used for legacy tools without Schema. When Schema is set, this is ignored.
	AddFlags func(cmd *cobra.Command)
}

// LookupToolCommand finds a ToolCommandDef by name or alias. Returns nil if not found.
func LookupToolCommand(name string) *ToolCommandDef {
	for i := range BuiltinToolCommands {
		def := &BuiltinToolCommands[i]
		if def.Use == name {
			return def
		}
		for _, alias := range def.Aliases {
			if alias == name {
				return def
			}
		}
	}
	return nil
}

// BuiltinToolCommands lists all tools exposed as top-level CLI commands.
// Internal pipeline tools (layer-processor, span-classify, etc.) are excluded.
var BuiltinToolCommands = []ToolCommandDef{
	// ── Translation ─────────────────────────────────────────────────

	{
		Use:                   "ai-translate",
		Aliases:               []string{"translate"},
		Short:                 "Translate content using AI/LLM",
		Category:              "translation",
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Schema:                aitools.AITranslateSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return aitools.NewAITranslateFromConfig(config, targetLang)
		},
	},
	{
		Use:          "pseudo-translate",
		Aliases:      []string{"pseudo"},
		Short:        "Generate pseudo-translations for localization testing",
		Category:     "translation",
		WritesOutput: true,
		Schema:            libtools.PseudoTranslateSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewPseudoTranslateFromConfig(config, targetLang)
		},
	},
	{
		Use:          "tm-leverage",
		Short:        "Pre-fill translations from translation memory",
		Category:     "translation",
		WritesOutput: true,
		Schema:       libtools.TMLeverageSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewTMLeverageFromConfig(config, targetLang)
		},
	},
	{
		Use:          "diff-leverage",
		Short:        "Leverage translations from previous versions using diff analysis",
		Category:     "translation",
		WritesOutput: true,
		Schema:       libtools.DiffLeverageSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewDiffLeverageFromConfig(config, targetLang)
		},
	},

	// ── Quality ─────────────────────────────────────────────────────

	{
		Use:          "qa-check",
		Aliases:      []string{"qa"},
		Short:        "Run rule-based quality checks on translations",
		Category:     "quality",
		WritesOutput: true,
		Schema:       libtools.QACheckSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewQACheckFromConfig(config, targetLang)
		},
	},
	{
		Use:                   "ai-qa",
		Short:                 "Check translation quality using AI/LLM",
		Category:              "quality",
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Schema:       aitools.AIQASchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return aitools.NewAIQAFromConfig(config, targetLang)
		},
	},
	{
		Use:                   "ai-review",
		Short:                 "Review translations with scoring using AI/LLM",
		Category:              "quality",
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Schema:       aitools.AIReviewSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return aitools.NewAIReviewFromConfig(config, targetLang)
		},
	},
	{
		Use:      "term-check",
		Short:    "Check terminology consistency across content",
		Category: "quality",
		Schema:   libtools.TermCheckSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewTermCheckFromConfig(config, targetLang)
		},
	},
	{
		Use:      "inconsistency-check",
		Short:    "Detect inconsistent translations of identical source strings",
		Category: "quality",
		Schema:   libtools.InconsistencyCheckSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewInconsistencyCheckFromConfig(config, targetLang)
		},
	},
	{
		Use:      "length-check",
		Short:    "Validate string length against configured limits",
		Category: "quality",
		Schema:   libtools.LengthCheckSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewLengthCheckFromConfig(config, targetLang)
		},
	},
	{
		Use:      "chars-check",
		Short:    "Check for invalid or unexpected Unicode characters",
		Category: "quality",
		Schema:   libtools.CharsCheckSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewCharsCheckFromConfig(config, targetLang)
		},
	},
	{
		Use:      "pattern-check",
		Short:    "Validate content against custom regex patterns",
		Category: "quality",
		Schema:   libtools.PatternCheckSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewPatternCheckFromConfig(config, targetLang)
		},
	},

	// ── Analysis ────────────────────────────────────────────────────

	{
		Use:      "word-count",
		Aliases:  []string{"wc"},
		Short:    "Count words in source and target text",
		Category: "analysis",
		Schema:   libtools.WordCountSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewWordCountFromConfig(config, targetLang)
		},
		NewCollector: func() flow.Collector {
			return libtools.NewStreamingWordCountCollector()
		},
	},
	{
		Use:      "char-count",
		Short:    "Count characters in source and target text",
		Category: "analysis",
		Schema:   libtools.CharCountSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewCharCountFromConfig(config, targetLang)
		},
	},
	{
		Use:      "segment-count",
		Short:    "Count translatable segments",
		Category: "analysis",
		Schema:   libtools.SegCountSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewSegCountFromConfig(config, targetLang)
		},
	},
	{
		Use:      "scoping-report",
		Short:    "Generate detailed scoping report (word counts, repetitions, file breakdown)",
		Category: "analysis",
		Schema:   libtools.ScopingReportSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewScopingReportFromConfig(config, targetLang)
		},
	},
	{
		Use:      "repetition-analysis",
		Short:    "Identify repeated segments across files for TM leverage",
		Category: "analysis",
		Schema:   libtools.RepetitionAnalysisSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewRepetitionAnalysisFromConfig(config, targetLang)
		},
	},
	{
		Use:      "chars-listing",
		Short:    "List all distinct characters used in source and/or target",
		Category: "analysis",
		Schema:   libtools.CharsListingSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewCharsListingFromConfig(config, targetLang)
		},
	},
	{
		Use:      "translation-comparison",
		Short:    "Compare translations across locales or versions",
		Category: "analysis",
		Schema:   libtools.TranslationComparisonSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewTranslationComparisonFromConfig(config, targetLang)
		},
	},
	{
		Use:      "encoding-detect",
		Short:    "Detect character encoding of source files",
		Category: "analysis",
		Schema:   libtools.EncodingDetectSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewEncodingDetectFromConfig(config, targetLang)
		},
	},

	// ── Text Processing ─────────────────────────────────────────────

	{
		Use:          "search-replace",
		Short:        "Find and replace patterns (literal or regex)",
		Category:     "text-processing",
		WritesOutput: true,
		Schema:       libtools.SearchReplaceSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewSearchReplaceFromConfig(config, targetLang)
		},
	},
	{
		Use:          "case-transform",
		Short:        "Transform text case (upper, lower, title)",
		Category:     "text-processing",
		WritesOutput: true,
		Schema:       libtools.CaseTransformSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewCaseTransformFromConfig(config, targetLang)
		},
	},
	{
		Use:          "segmentation",
		Short:        "Split source text into sentence-level segments",
		Category:     "text-processing",
		WritesOutput: true,
		Schema:       libtools.SegmentationSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewSegmentationFromConfig(config, targetLang)
		},
	},

	// ── Scripting ──────────────────────────────────────────────────

	{
		Use:          "script",
		Short:        "Run a JavaScript processing script on each part",
		Category:     "text-processing",
		WritesOutput: true,
		Schema:       libtools.ScriptSchema(),
		NewToolFromConfig: func(config map[string]any, targetLang string) (tool.Tool, error) {
			return libtools.NewScriptFromConfig(config, targetLang)
		},
	},
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

// NewToolCommands creates cobra commands for all builtin tool definitions.
func (a *App) NewToolCommands() []*cobra.Command {
	var cmds []*cobra.Command

	for _, def := range BuiltinToolCommands {
		d := def // capture loop variable
		var formatMaps []string

		cmd := &cobra.Command{
			Use:     d.Use + " [files...]",
			Aliases: d.Aliases,
			Short:   d.Short,
			GroupID: d.Category,
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
				if effectiveLang == "" && a.ToolReg != nil {
					if info := a.ToolReg.GetToolInfo(registry.ToolID(d.Use)); info != nil && info.DefaultLocale != "" {
						effectiveLang = info.DefaultLocale
					}
				}

				tracePath, _ := cmd.Flags().GetString("trace")
				parallelBlocks, _ := cmd.Flags().GetInt("parallel-blocks")

				// Build the tool factory: prefer schema-driven path over legacy.
				newTool := func() (tool.Tool, error) {
					if d.Schema != nil && d.NewToolFromConfig != nil {
						config := ReadAllSchemaFlags(cmd, d.Schema)
						// Inject --credential flag value into config for credential resolution.
						if credName, _ := cmd.Flags().GetString("credential"); credName != "" {
							config["credential"] = credName
						}
						// Inject progress callback for AI tools on a TTY.
						if !jsonOut && isatty.IsTerminal(os.Stderr.Fd()) {
							config["onProgress"] = aiProgressWriter(os.Stderr)
						}
						return a.ToolReg.NewToolWithConfig(registry.ToolID(d.Use), config, effectiveLang)
					}
					return d.NewTool(cmd, effectiveLang)
				}

				rc := ToolRunConfig{
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
					TracePath:      tracePath,
					ParallelBlocks: parallelBlocks,
					NewTool:        newTool,
					NewCollector:   d.NewCollector,
				}

				// Clear the AI progress status line after tool execution.
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
		if d.WritesOutput {
			cmd.Flags().StringP("output", "o", "", "output path template (variables: {name}, {ext}, {lang})")
		}
		if d.Schema != nil {
			RegisterSchemaFlags(cmd, d.Schema)
			// Add --credential flag for tools that require credentials.
			if d.Schema.ToolMeta != nil {
				for _, req := range d.Schema.ToolMeta.Requires {
					if req == "credentials" {
						cmd.Flags().String("credential", "", "saved credential name to use (see 'kapi credentials list')")
						break
					}
				}
			}
		} else if d.AddFlags != nil {
			d.AddFlags(cmd)
		}
		cmd.Flags().String("trace", "", "write flow trace JSON to file (for flow visualization)")
		cmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
		cmds = append(cmds, cmd)
	}

	return cmds
}
