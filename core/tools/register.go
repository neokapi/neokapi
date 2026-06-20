package tools

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// toolMeta creates a ToolMeta with common tool fields.
func toolMeta(id, displayName, category string, opts ...func(*schema.ToolMeta)) schema.ToolMeta {
	m := schema.ToolMeta{
		ID:          id,
		Category:    category,
		DisplayName: displayName,
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

func withTags(tags ...string) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Tags = tags }
}

// IO-contract shorthands for tool registration. Generic over ~string so overlay
// types, annotation keys, and pseudo-port constants all pass without string().
func srcF[T ~string](t T) schema.IOPort  { return schema.Port(t, model.SideSource) }
func tgtF[T ~string](t T) schema.IOPort  { return schema.Port(t, model.SideTarget) }
func optF(f schema.IOPort) schema.IOPort { f.Optional = true; return f }

func withConsumes(fs ...schema.IOPort) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Consumes = fs }
}

func withRequires(reqs ...string) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Requires = reqs }
}

func withCardinality(c schema.LocaleCardinality) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Cardinality = c }
}

func withDefaultLocale(locale model.LocaleID) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.DefaultLocale = locale }
}

func withProduces(fs ...schema.IOPort) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Produces = fs }
}

func withSideEffects(effects ...schema.SideEffect) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.SideEffects = effects }
}

func withWritesOutput() func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.WritesOutput = true }
}

func withParallelBlocks(n int) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.DefaultParallelBlocks = n }
}

// Keep withParallelBlocks reachable until the first tool adopts it.
// Without this anchor, linters flag the helper as unused even though
// it's part of the builder chain that sits next to withWritesOutput
// / withSideEffects / withAliases.
var _ = withParallelBlocks

func withAliases(aliases ...string) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Aliases = aliases }
}

// toolSchema is a shorthand for generating a tool schema from a config struct.
func toolSchema(cfg any, meta schema.ToolMeta) *schema.ComponentSchema {
	return schema.FromStruct(cfg, meta)
}

// RegisterAll registers all built-in tools in the given ToolRegistry.
// Each registration includes a factory and an auto-generated parameter schema.
func RegisterAll(reg *registry.ToolRegistry) {

	// ── Validate ────────────────────────────────────────────────────

	reg.RegisterWithSchema("word-count", func() tool.Tool {
		cfg := &WordCountConfig{}
		cfg.Reset()
		return NewWordCountTool(cfg)
	}, toolSchema(&WordCountConfig{CountSource: true, CountTarget: true}, toolMeta("word-count", "Word Count", schema.CategoryAnalysis,
		withTags("analysis"), withAliases("wc"), withCardinality(schema.Monolingual), withProduces(srcF(model.AnnoWordCount)))))

	reg.RegisterWithSchema("char-count", func() tool.Tool {
		cfg := &CharCountConfig{}
		cfg.Reset()
		return NewCharCountTool(cfg)
	}, toolSchema(&CharCountConfig{CountSource: true, CountTarget: true}, toolMeta("char-count", "Character Count", schema.CategoryAnalysis,
		withTags("analysis"), withCardinality(schema.Monolingual), withProduces(srcF(model.AnnoCharCount)))))

	reg.RegisterWithSchema("segment-count", func() tool.Tool {
		return NewSegCountTool(&SegCountConfig{})
	}, toolSchema(&SegCountConfig{}, toolMeta("segment-count", "Segment Count", schema.CategoryAnalysis,
		withTags("analysis"), withCardinality(schema.Monolingual), withProduces(srcF(model.AnnoSegCount)))))

	reg.RegisterWithSchema("qa", func() tool.Tool {
		return NewQACheckTool(NewQACheckConfig(model.LocaleEnglish))
	}, toolSchema(NewQACheckConfig(model.LocaleEnglish), toolMeta("qa", "Quality Check", schema.CategoryQuality,
		withTags("quality"), withWritesOutput(), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("inconsistency-check", func() tool.Tool {
		return NewInconsistencyCheckTool(NewInconsistencyCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewInconsistencyCheckConfig(model.LocaleEnglish), toolMeta("inconsistency-check", "Inconsistency Check", schema.CategoryQuality,
		withTags("quality"), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("length-check", func() tool.Tool {
		cfg := &LengthCheckConfig{TargetLocale: model.LocaleEnglish}
		cfg.Reset()
		cfg.TargetLocale = model.LocaleEnglish
		return NewLengthCheckTool(cfg)
	}, toolSchema(&LengthCheckConfig{CheckMaxCharLength: true, MaxCharLengthBreak: 20, MaxCharLengthAbove: 200, MaxCharLengthBelow: 350, CheckMinCharLength: true, MinCharLengthBreak: 20, MinCharLengthAbove: 45, MinCharLengthBelow: 30},
		toolMeta("length-check", "Length Check", schema.CategoryQuality,
			withTags("quality"), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("chars-check", func() tool.Tool {
		return NewCharsCheckTool(NewCharsCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewCharsCheckConfig(model.LocaleEnglish), toolMeta("chars-check", "Characters Check", schema.CategoryQuality,
		withTags("quality"), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("pattern-check", func() tool.Tool {
		return NewPatternCheckTool(&PatternCheckConfig{TargetLocale: model.LocaleEnglish})
	}, toolSchema(&PatternCheckConfig{}, toolMeta("pattern-check", "Pattern Check", schema.CategoryQuality,
		withTags("quality", "regex"), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("dnt-check", func() tool.Tool {
		return NewDNTCheckTool(NewDNTCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewDNTCheckConfig(model.LocaleEnglish), toolMeta("dnt-check", "Do-Not-Translate Check", schema.CategoryQuality,
		withTags("quality"), withAliases("dnt"), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("placeholder-check", func() tool.Tool {
		return NewPlaceholderCheckTool(NewPlaceholderCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewPlaceholderCheckConfig(model.LocaleEnglish), toolMeta("placeholder-check", "Placeholder Check", schema.CategoryQuality,
		withTags("quality"), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("term-check", func() tool.Tool {
		return NewTermCheckTool(&TermCheckConfig{TargetLocale: model.LocaleEnglish})
	}, toolSchema(&TermCheckConfig{}, toolMeta("term-check", "Terminology Check", schema.CategoryQuality,
		withTags("quality"), withRequires("target-language", "termbase"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(srcF(model.OverlayTerm)), withSideEffects(schema.SideEffectTermbaseRead))))

	reg.RegisterWithSchema("xml-validation", func() tool.Tool {
		return NewXMLValidationTool(&XMLValidationConfig{CheckSource: true, WrapRoot: true})
	}, toolSchema(&XMLValidationConfig{CheckSource: true, WrapRoot: true}, toolMeta("xml-validation", "XML Validation", schema.CategoryQuality,
		withTags("quality"), withCardinality(schema.Monolingual), withProduces(tgtF(model.OverlayQA)))))

	reg.RegisterWithSchema("translation-comparison", func() tool.Tool {
		cfg := &TranslationComparisonConfig{}
		cfg.Reset()
		return NewTranslationComparisonTool(cfg)
	}, toolSchema(&TranslationComparisonConfig{CaseSensitive: true, WhitespaceSensitive: true, PunctuationSensitive: true, Document1Label: "Trans1", Document2Label: "Trans2", GenericCodes: true},
		toolMeta("translation-comparison", "Translation Comparison", schema.CategoryAnalysis,
			withTags("quality"), withRequires("target-language"), withCardinality(schema.Bilingual), withConsumes(tgtF(schema.PortTarget)), withProduces(tgtF(model.AnnoComparison)))))

	reg.RegisterWithSchema("chars-listing", func() tool.Tool {
		return NewCharsListingTool(&CharsListingConfig{
			IncludeSource: true, IncludeTarget: true, TargetLocale: model.LocaleEnglish,
		}).Tool()
	}, toolSchema(&CharsListingConfig{IncludeSource: true, IncludeTarget: true}, toolMeta("chars-listing", "Characters Listing", schema.CategoryAnalysis,
		withTags("analysis"), withCardinality(schema.Monolingual), withProduces(srcF(model.AnnoCharCount)))))

	reg.RegisterWithSchema("scoping-report", func() tool.Tool {
		return NewScopingReportTool(&ScopingReportConfig{})
	}, toolSchema(&ScopingReportConfig{}, toolMeta("scoping-report", "Scoping Report", schema.CategoryAnalysis,
		withTags("analysis"), withCardinality(schema.Monolingual), withProduces(srcF(model.AnnoScopingReport)))))

	reg.RegisterWithSchema("repetition-analysis", func() tool.Tool {
		return NewRepetitionAnalysisTool(&RepetitionAnalysisConfig{CaseSensitive: true})
	}, toolSchema(&RepetitionAnalysisConfig{CaseSensitive: true}, toolMeta("repetition-analysis", "Repetition Analysis", schema.CategoryAnalysis,
		withTags("analysis"), withCardinality(schema.Monolingual), withProduces(srcF(model.AnnoRepetition)))))

	// ── Transform ───────────────────────────────────────────────────

	reg.RegisterWithSchema("pseudo-translate", func() tool.Tool {
		return NewPseudoTranslateTool(&PseudoConfig{Prefix: "\u2592 ", Suffix: " \u2592", TargetLocale: "qps"})
	}, toolSchema(&PseudoConfig{Prefix: "\u2592 ", Suffix: " \u2592"}, toolMeta("pseudo-translate", "Pseudo Translate", schema.CategoryTranslation,
		withTags("translation"), withAliases("pseudo"), withWritesOutput(), withRequires("target-language"), withCardinality(schema.Bilingual), withDefaultLocale(model.LocaleID("qps")), withProduces(tgtF(schema.PortTarget)))))

	reg.RegisterWithSchema("search-replace", func() tool.Tool {
		return NewSearchReplaceTool(&SearchReplaceConfig{})
	}, toolSchema(&SearchReplaceConfig{}, toolMeta("search-replace", "Search and Replace", schema.CategoryTextProcessing,
		withTags("regex", "configurable"), withWritesOutput(), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("case-transform", func() tool.Tool {
		return NewCaseTransformTool(&CaseTransformConfig{Mode: CaseLower, ApplySource: true})
	}, toolSchema(&CaseTransformConfig{Mode: CaseLower, ApplySource: true}, toolMeta("case-transform", "Case Transform", schema.CategoryTextProcessing,
		withTags("text-processing"), withWritesOutput(), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("subtitle-filter", func() tool.Tool {
		return NewSubtitleFilterTool(&SubtitleFilterConfig{})
	}, toolSchema(&SubtitleFilterConfig{}, toolMeta("subtitle-filter", "Subtitle Filter", schema.CategoryTextProcessing,
		withTags("media", "subtitle"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("segmentation", func() tool.Tool {
		return NewSegmentationTool(&SegmentationConfig{})
	}, toolSchema(&SegmentationConfig{}, toolMeta("segmentation", "Segmentation", schema.CategoryTextProcessing,
		withTags("text-processing"), withAliases("segment"), withWritesOutput(), withCardinality(schema.Monolingual),
		withProduces(srcF(model.OverlaySegmentation), tgtF(model.OverlaySegmentation)))))

	reg.RegisterWithSchema("create-target", func() tool.Tool {
		return NewCreateTargetTool(&CreateTargetConfig{CreateOnNonTranslatable: true})
	}, toolSchema(&CreateTargetConfig{CreateOnNonTranslatable: true}, toolMeta("create-target", "Create Target", schema.CategoryTextProcessing,
		withRequires("target-language"), withCardinality(schema.Bilingual))))

	reg.RegisterWithSchema("remove-target", func() tool.Tool {
		return NewRemoveTargetTool(&RemoveTargetConfig{FilterByIDs: true})
	}, toolSchema(&RemoveTargetConfig{FilterByIDs: true}, toolMeta("remove-target", "Remove Target", schema.CategoryTextProcessing,
		withRequires("target-language"), withCardinality(schema.Bilingual))))

	reg.RegisterWithSchema("inline-codes-remove", func() tool.Tool {
		return NewInlineCodesRemoveTool(&InlineCodesRemoveConfig{ApplyTarget: true, IncludeNonTranslatable: true})
	}, toolSchema(&InlineCodesRemoveConfig{ApplyTarget: true, IncludeNonTranslatable: true}, toolMeta("inline-codes-remove", "Inline Codes Remove", schema.CategoryTextProcessing,
		withTags("text-processing"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("properties-set", func() tool.Tool {
		return NewPropertiesSetTool(&PropertiesSetConfig{Overwrite: true, OnlyTranslatable: true})
	}, toolSchema(&PropertiesSetConfig{Overwrite: true, OnlyTranslatable: true}, toolMeta("properties-set", "Properties Set", schema.CategoryTextProcessing,
		withTags("configurable"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("whitespace-correct", func() tool.Tool {
		cfg := &WhitespaceCorrectConfig{}
		cfg.Reset()
		cfg.TargetLocale = model.LocaleEnglish
		return NewWhitespaceCorrectTool(cfg)
	}, toolSchema(&WhitespaceCorrectConfig{NormalizeSpaces: true, MatchSourceWhitespace: true, RemoveZeroWidthChars: true, CorrectFullStop: true, CorrectComma: true, CorrectExclamation: true, CorrectQuestion: true, IncludeVerticalWS: true, IncludeHorizontalWS: true},
		toolMeta("whitespace-correct", "Whitespace Correct", schema.CategoryTextProcessing,
			withTags("text-processing"), withRequires("target-language"), withCardinality(schema.Bilingual))))

	reg.RegisterWithSchema("tag-protect", func() tool.Tool {
		return NewTagProtectTool(&TagProtectConfig{})
	}, toolSchema(&TagProtectConfig{}, toolMeta("tag-protect", "Tag Protect", schema.CategoryTextProcessing,
		withTags("regex", "configurable"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("redact", func() tool.Tool {
		t, _ := NewRedactTool(&RedactConfig{Detectors: []string{DetectRules}})
		return t
	}, RedactSchema())

	reg.RegisterWithSchema("unredact", func() tool.Tool {
		t, _ := NewUnredactTool(&UnredactConfig{})
		return t
	}, UnredactSchema())

	reg.RegisterWithSchema("xslt-transform", func() tool.Tool {
		cfg := &XSLTTransformConfig{}
		cfg.Reset()
		return NewXSLTTransformTool(cfg)
	}, toolSchema(&XSLTTransformConfig{ApplySource: true, PassOnOutput: true}, toolMeta("xslt-transform", "XSLT Transform", schema.CategoryTextProcessing,
		withTags("configurable"), withCardinality(schema.Monolingual))))

	// ── Enrich ──────────────────────────────────────────────────────

	reg.RegisterWithSchema("tm-leverage", func() tool.Tool {
		return NewTMLeverageTool(&TMLeverageConfig{FuzzyThreshold: 70, Provider: NullTMProvider{}})
	}, toolSchema(&TMLeverageConfig{FuzzyThreshold: 70}, toolMeta("tm-leverage", "TM Leverage", schema.CategoryTranslation,
		withTags("translation"), withWritesOutput(), withRequires("target-language", "tm"), withCardinality(schema.Bilingual), withConsumes(optF(srcF(model.OverlaySegmentation))), withProduces(srcF(model.AnnoTMMatch), srcF(model.AnnoAltTranslation), tgtF(schema.PortTarget)), withSideEffects(schema.SideEffectTMRead))))

	reg.RegisterWithSchema("diff-leverage", func() tool.Tool {
		return NewDiffLeverageTool(&DiffLeverageConfig{CaseSensitive: true, PreviousTexts: map[string]PreviousBlock{}})
	}, toolSchema(&DiffLeverageConfig{CaseSensitive: true}, toolMeta("diff-leverage", "Diff Leverage", schema.CategoryTranslation,
		withTags("translation"), withWritesOutput(), withCardinality(schema.Bilingual), withProduces(srcF(model.AnnoAltTranslation), tgtF(schema.PortTarget)))))

	// ── Convert ─────────────────────────────────────────────────────

	reg.RegisterWithSchema("encoding-convert", func() tool.Tool {
		cfg := &EncodingConvertConfig{}
		cfg.Reset()
		return NewEncodingConvertTool(cfg)
	}, toolSchema(&EncodingConvertConfig{ApplyTarget: true, UnescapeNCR: true, UnescapeCER: true, UnescapeJava: true, ReportUnsupported: true},
		toolMeta("encoding-convert", "Encoding Convert", schema.CategoryTextProcessing,
			withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("encoding-detect", func() tool.Tool {
		return NewEncodingDetectTool(&EncodingDetectConfig{})
	}, toolSchema(&EncodingDetectConfig{}, toolMeta("encoding-detect", "Encoding Detect", schema.CategoryAnalysis,
		withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("linebreak-convert", func() tool.Tool {
		return NewLineBreakConvertTool(&LineBreakConvertConfig{Mode: LineBreakLF, ApplySource: true, ApplyTarget: true})
	}, toolSchema(&LineBreakConvertConfig{Mode: LineBreakLF, ApplySource: true, ApplyTarget: true},
		toolMeta("linebreak-convert", "Line Break Convert", schema.CategoryTextProcessing,
			withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("bom-convert", func() tool.Tool {
		return NewBOMConvertTool(&BOMConvertConfig{})
	}, toolSchema(&BOMConvertConfig{}, toolMeta("bom-convert", "BOM Convert", schema.CategoryTextProcessing,
		withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("fullwidth-convert", func() tool.Tool {
		return NewFullWidthConvertTool(&FullWidthConvertConfig{Mode: FullWidthToHalf, ApplyTarget: true})
	}, toolSchema(&FullWidthConvertConfig{Mode: FullWidthToHalf, ApplyTarget: true},
		toolMeta("fullwidth-convert", "Full Width Convert", schema.CategoryTextProcessing,
			withTags("text-processing"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("uri-convert", func() tool.Tool {
		return NewURIConvertTool(&URIConvertConfig{Mode: URIDecode, ApplyTarget: true})
	}, toolSchema(&URIConvertConfig{Mode: URIDecode, ApplyTarget: true}, toolMeta("uri-convert", "URI Convert", schema.CategoryTextProcessing,
		withTags("text-processing"), withCardinality(schema.Monolingual))))

	// ── Pipeline ────────────────────────────────────────────────────

	reg.RegisterWithSchema("span-classify", func() tool.Tool {
		return NewSpanClassifyTool(&SpanClassifyConfig{})
	}, toolSchema(&SpanClassifyConfig{}, toolMeta("span-classify", "Span Classify", schema.CategoryTextProcessing,
		withTags("text-processing"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("layer-processor", func() tool.Tool {
		return NewLayerProcessorTool(&LayerProcessorConfig{})
	}, &schema.ComponentSchema{ToolMeta: &schema.ToolMeta{
		ID: "layer-processor", DisplayName: "Layer Processor", Category: schema.CategoryTextProcessing,
		Cardinality: schema.Monolingual,
	}})

	reg.RegisterWithSchema("external-command", func() tool.Tool {
		return NewExternalCommandTool(&ExternalCommandConfig{ApplyTarget: true, SendAsStdin: true, Timeout: 30})
	}, toolSchema(&ExternalCommandConfig{ApplyTarget: true, SendAsStdin: true, Timeout: 30},
		toolMeta("external-command", "External Command", schema.CategoryTextProcessing,
			withTags("configurable"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("brand-vocab-check", func() tool.Tool {
		return NewBrandVocabCheckTool(nil, nil)
	}, &schema.ComponentSchema{ToolMeta: &schema.ToolMeta{
		ID: "brand-vocab-check", DisplayName: "Brand Vocabulary Check", Category: schema.CategoryQuality,
		Cardinality: schema.Bilingual,
		Consumes:    []schema.IOPort{{Type: schema.PortTarget, Side: model.SideTarget}},
		Produces:    []schema.IOPort{{Type: model.AnnoBrandVoice, Side: model.SideTarget}},
		Requires:    []string{"target-language"},
	}})

	// ── Utility ─────────────────────────────────────────────────────

	reg.RegisterWithSchema("batch", func() tool.Tool {
		return NewBatchTool(&BatchConfig{Size: 10})
	}, toolSchema(&BatchConfig{Size: 10}, toolMeta("batch", "Batch Collector", schema.CategoryTextProcessing,
		withTags("batch"), withCardinality(schema.Monolingual))))

	reg.RegisterWithSchema("script", func() tool.Tool {
		return NewScriptTool(&ScriptConfig{})
	}, toolSchema(&ScriptConfig{}, toolMeta("script", "Script", schema.CategoryTextProcessing,
		withTags("configurable"), withWritesOutput(), withCardinality(schema.Monolingual))))

	// Register config factories for all tools that support NewToolFromConfig.
	// This enables project flows to create tools with step-level config.
	registerConfigFactories(reg)
}

func registerConfigFactories(reg *registry.ToolRegistry) {
	reg.SetConfigFactory("word-count", NewWordCountFromConfig)
	reg.SetConfigFactory("char-count", NewCharCountFromConfig)
	reg.SetConfigFactory("segment-count", NewSegCountFromConfig)
	reg.SetConfigFactory("qa", NewQACheckFromConfig)
	reg.SetConfigFactory("inconsistency-check", NewInconsistencyCheckFromConfig)
	reg.SetConfigFactory("length-check", NewLengthCheckFromConfig)
	reg.SetConfigFactory("chars-check", NewCharsCheckFromConfig)
	reg.SetConfigFactory("pattern-check", NewPatternCheckFromConfig)
	reg.SetConfigFactory("term-check", NewTermCheckFromConfig)
	reg.SetConfigFactory("scoping-report", NewScopingReportFromConfig)
	reg.SetConfigFactory("repetition-analysis", NewRepetitionAnalysisFromConfig)
	reg.SetConfigFactory("chars-listing", NewCharsListingFromConfig)
	reg.SetConfigFactory("translation-comparison", NewTranslationComparisonFromConfig)
	reg.SetConfigFactory("encoding-detect", NewEncodingDetectFromConfig)
	reg.SetConfigFactory("pseudo-translate", NewPseudoTranslateFromConfig)
	reg.SetConfigFactory("redact", NewRedactFromConfig)
	// Entity detection makes the upstream entity overlay a required input (so a
	// misconfigured flow fails fast instead of silently leaving PII unredacted).
	reg.SetContractResolver("redact", ResolveRedactContract)
	reg.SetConfigFactory("unredact", NewUnredactFromConfig)
	reg.SetConfigFactory("search-replace", NewSearchReplaceFromConfig)
	reg.SetConfigFactory("case-transform", NewCaseTransformFromConfig)
	reg.SetConfigFactory("segmentation", NewSegmentationFromConfig)
	reg.SetConfigFactory("tm-leverage", NewTMLeverageFromConfig)
	reg.SetConfigFactory("diff-leverage", NewDiffLeverageFromConfig)
	reg.SetConfigFactory("script", NewScriptFromConfig)
}
