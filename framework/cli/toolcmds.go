package cli

import (
	"context"
	"fmt"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	provider "github.com/neokapi/neokapi/providers/ai"
	"github.com/spf13/cobra"
)

// ToolCommandDef describes a tool that is exposed as a top-level CLI command.
type ToolCommandDef struct {
	Use               string
	Aliases           []string
	Short             string
	Category          string // e.g. "translation", "quality", "analysis", "text-processing"
	WritesOutput      bool
	DefaultTargetLang string

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

// BuiltinToolCommands lists all tools exposed as top-level CLI commands.
// Internal pipeline tools (layer-processor, span-classify, etc.) are excluded.
var BuiltinToolCommands = []ToolCommandDef{
	// ── Translation ─────────────────────────────────────────────────

	{
		Use:          "ai-translate",
		Aliases:      []string{"translate"},
		Short:        "Translate content using AI/LLM",
		Category:     "translation",
		WritesOutput: true,
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			p, err := providerFromFlags(cmd)
			if err != nil {
				return nil, err
			}
			sourceLang, _ := cmd.Flags().GetString("source-lang")
			return aitools.NewAITranslateTool(p, aitools.AITranslateConfig{
				SourceLocale: model.LocaleID(sourceLang),
				TargetLocale: model.LocaleID(targetLang),
			}), nil
		},
		AddFlags: addProviderFlags,
	},
	{
		Use:               "pseudo-translate",
		Aliases:           []string{"pseudo"},
		Short:             "Generate pseudo-translations for localization testing",
		Category:          "translation",
		WritesOutput:      true,
		DefaultTargetLang: "qps",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			lang := targetLang
			if lang == "" {
				lang = "qps"
			}
			expansion, _ := cmd.Flags().GetInt("expansion")
			return libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
				TargetLocale:     model.LocaleID(lang),
				ExpansionPercent: expansion,
			}), nil
		},
		AddFlags: func(cmd *cobra.Command) {
			cmd.Flags().Int("expansion", 0, "text expansion percentage (0 = none)")
		},
	},
	{
		Use:          "tm-leverage",
		Short:        "Pre-fill translations from translation memory",
		Category:     "translation",
		WritesOutput: true,
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			sourceLang, _ := cmd.Flags().GetString("source-lang")
			return libtools.NewTMLeverageTool(&libtools.TMLeverageConfig{
				SourceLocale:   model.LocaleID(sourceLang),
				TargetLocale:   model.LocaleID(targetLang),
				FuzzyThreshold: 70,
				Provider:       libtools.NullTMProvider{},
			}), nil
		},
		AddFlags: func(cmd *cobra.Command) {
			cmd.Flags().String("tm", "", "named TM for leverage (resolves from KAPI_HOME)")
		},
	},
	{
		Use:          "diff-leverage",
		Short:        "Leverage translations from previous versions using diff analysis",
		Category:     "translation",
		WritesOutput: true,
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewDiffLeverageTool(&libtools.DiffLeverageConfig{
				TargetLocale:  model.LocaleID(targetLang),
				CaseSensitive: true,
				PreviousTexts: map[string]libtools.PreviousBlock{},
			}), nil
		},
	},

	// ── Quality ─────────────────────────────────────────────────────

	{
		Use:          "qa-check",
		Aliases:      []string{"qa"},
		Short:        "Run rule-based quality checks on translations",
		Category:     "quality",
		WritesOutput: true,
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewQACheckTool(libtools.NewQACheckConfig(model.LocaleID(targetLang))), nil
		},
	},
	{
		Use:          "ai-qa",
		Short:        "Check translation quality using AI/LLM",
		Category:     "quality",
		WritesOutput: true,
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			p, err := providerFromFlags(cmd)
			if err != nil {
				return nil, err
			}
			sourceLang, _ := cmd.Flags().GetString("source-lang")
			return aitools.NewAIQACheckTool(p, aitools.AIQAConfig{
				SourceLocale: model.LocaleID(sourceLang),
				TargetLocale: model.LocaleID(targetLang),
			}), nil
		},
		AddFlags: addProviderFlags,
	},
	{
		Use:          "ai-review",
		Short:        "Review translations with scoring using AI/LLM",
		Category:     "quality",
		WritesOutput: true,
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			p, err := providerFromFlags(cmd)
			if err != nil {
				return nil, err
			}
			sourceLang, _ := cmd.Flags().GetString("source-lang")
			return aitools.NewAIReviewTool(p, aitools.AIReviewConfig{
				SourceLocale: model.LocaleID(sourceLang),
				TargetLocale: model.LocaleID(targetLang),
			}), nil
		},
		AddFlags: addProviderFlags,
	},
	{
		Use:      "term-check",
		Short:    "Check terminology consistency across content",
		Category: "quality",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewTermCheckTool(&libtools.TermCheckConfig{
				TargetLocale: model.LocaleID(targetLang),
			}), nil
		},
	},
	{
		Use:      "inconsistency-check",
		Short:    "Detect inconsistent translations of identical source strings",
		Category: "quality",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewInconsistencyCheckTool(libtools.NewInconsistencyCheckConfig(model.LocaleID(targetLang))), nil
		},
	},
	{
		Use:      "length-check",
		Short:    "Validate string length against configured limits",
		Category: "quality",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewLengthCheckTool(&libtools.LengthCheckConfig{
				TargetLocale: model.LocaleID(targetLang),
			}), nil
		},
	},
	{
		Use:      "chars-check",
		Short:    "Check for invalid or unexpected Unicode characters",
		Category: "quality",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewCharsCheckTool(libtools.NewCharsCheckConfig(model.LocaleID(targetLang))), nil
		},
	},
	{
		Use:      "pattern-check",
		Short:    "Validate content against custom regex patterns",
		Category: "quality",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewPatternCheckTool(&libtools.PatternCheckConfig{
				TargetLocale: model.LocaleID(targetLang),
			}), nil
		},
	},

	// ── Analysis ────────────────────────────────────────────────────

	{
		Use:      "word-count",
		Aliases:  []string{"wc"},
		Short:    "Count words in source and target text",
		Category: "analysis",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewWordCountTool(&libtools.WordCountConfig{}), nil
		},
		NewCollector: func() flow.Collector {
			return libtools.NewStreamingWordCountCollector()
		},
	},
	{
		Use:      "char-count",
		Short:    "Count characters in source and target text",
		Category: "analysis",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewCharCountTool(&libtools.CharCountConfig{}), nil
		},
	},
	{
		Use:      "segment-count",
		Short:    "Count translatable segments",
		Category: "analysis",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewSegCountTool(&libtools.SegCountConfig{}), nil
		},
	},
	{
		Use:      "scoping-report",
		Short:    "Generate detailed scoping report (word counts, repetitions, file breakdown)",
		Category: "analysis",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewScopingReportTool(&libtools.ScopingReportConfig{}), nil
		},
	},
	{
		Use:      "repetition-analysis",
		Short:    "Identify repeated segments across files for TM leverage",
		Category: "analysis",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewRepetitionAnalysisTool(&libtools.RepetitionAnalysisConfig{CaseSensitive: true}), nil
		},
	},
	{
		Use:      "chars-listing",
		Short:    "List all distinct characters used in source and/or target",
		Category: "analysis",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewCharsListingTool(&libtools.CharsListingConfig{
				IncludeSource: true,
				IncludeTarget: true,
				TargetLocale:  model.LocaleID(targetLang),
			}).Tool(), nil
		},
	},
	{
		Use:      "translation-comparison",
		Short:    "Compare translations across locales or versions",
		Category: "analysis",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewTranslationComparisonTool(&libtools.TranslationComparisonConfig{}), nil
		},
	},
	{
		Use:      "encoding-detect",
		Short:    "Detect character encoding of source files",
		Category: "analysis",
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewEncodingDetectTool(&libtools.EncodingDetectConfig{}), nil
		},
	},

	// ── Text Processing ─────────────────────────────────────────────

	{
		Use:          "search-replace",
		Short:        "Find and replace patterns (literal or regex)",
		Category:     "text-processing",
		WritesOutput: true,
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewSearchReplaceTool(&libtools.SearchReplaceConfig{
				TargetLocale: model.LocaleID(targetLang),
			}), nil
		},
	},
	{
		Use:          "case-transform",
		Short:        "Transform text case (upper, lower, title)",
		Category:     "text-processing",
		WritesOutput: true,
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			mode, _ := cmd.Flags().GetString("mode")
			cfg := &libtools.CaseTransformConfig{
				ApplySource:  true,
				TargetLocale: model.LocaleID(targetLang),
			}
			switch mode {
			case "upper":
				cfg.Mode = libtools.CaseUpper
			case "title":
				cfg.Mode = libtools.CaseTitle
			default:
				cfg.Mode = libtools.CaseLower
			}
			return libtools.NewCaseTransformTool(cfg), nil
		},
		AddFlags: func(cmd *cobra.Command) {
			cmd.Flags().String("mode", "lower", "case mode: upper, lower, title")
		},
	},
	{
		Use:          "segmentation",
		Short:        "Split source text into sentence-level segments",
		Category:     "text-processing",
		WritesOutput: true,
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			return libtools.NewSegmentationTool(&libtools.SegmentationConfig{
				TargetLocale: model.LocaleID(targetLang),
			}), nil
		},
	},

	// ── Scripting ──────────────────────────────────────────────────

	{
		Use:          "script",
		Short:        "Run a JavaScript processing script on each part",
		Category:     "text-processing",
		WritesOutput: true,
		NewTool: func(cmd *cobra.Command, targetLang string) (tool.Tool, error) {
			code, _ := cmd.Flags().GetString("code")
			scriptFile, _ := cmd.Flags().GetString("script-file")
			if code == "" && scriptFile == "" {
				return nil, fmt.Errorf("either --code or --script-file is required")
			}
			return libtools.NewScriptTool(&libtools.ScriptConfig{Code: code, ScriptFile: scriptFile}), nil
		},
		AddFlags: func(cmd *cobra.Command) {
			cmd.Flags().String("code", "", "inline JavaScript code to execute")
			cmd.Flags().String("script-file", "", "path to .js file")
		},
	},
}

// addProviderFlags registers AI provider flags on a cobra command.
func addProviderFlags(cmd *cobra.Command) {
	cmd.Flags().String("provider", "anthropic", "AI provider (anthropic, openai, ollama)")
	cmd.Flags().String("api-key", "", "API key for the AI provider")
	cmd.Flags().String("model", "", "AI model name")
}

// providerFromFlags creates an LLM provider from command flags.
func providerFromFlags(cmd *cobra.Command) (provider.LLMProvider, error) {
	providerName, _ := cmd.Flags().GetString("provider")
	apiKey, _ := cmd.Flags().GetString("api-key")
	modelName, _ := cmd.Flags().GetString("model")

	cfg := provider.Config{
		APIKey: apiKey,
		Model:  modelName,
	}

	switch providerName {
	case "anthropic":
		return provider.NewAnthropicProvider(cfg), nil
	case "openai":
		return provider.NewOpenAIProvider(cfg), nil
	case "ollama":
		return provider.NewOllamaProvider(cfg), nil
	default:
		return nil, fmt.Errorf("unknown AI provider: %s (supported: anthropic, openai, ollama)", providerName)
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
				if effectiveLang == "" && d.DefaultTargetLang != "" {
					effectiveLang = d.DefaultTargetLang
				}

				tracePath, _ := cmd.Flags().GetString("trace")
				parallelBlocks, _ := cmd.Flags().GetInt("parallel-blocks")

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
					TracePath:      tracePath,
					ParallelBlocks: parallelBlocks,
					NewTool: func() (tool.Tool, error) {
						return d.NewTool(cmd, effectiveLang)
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
		if d.AddFlags != nil {
			d.AddFlags(cmd)
		}
		cmd.Flags().String("trace", "", "write flow trace JSON to file (for flow visualization)")
		cmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
		cmds = append(cmds, cmd)
	}

	return cmds
}
