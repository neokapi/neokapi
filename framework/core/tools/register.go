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

func withInputs(parts ...string) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Inputs = parts }
}

func withTags(tags ...string) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Tags = tags }
}

func withRequires(reqs ...string) func(*schema.ToolMeta) {
	return func(m *schema.ToolMeta) { m.Requires = reqs }
}

// toolSchema is a shorthand for generating a tool schema from a config struct.
func toolSchema(cfg any, meta schema.ToolMeta) *schema.ComponentSchema {
	return schema.FromStruct(cfg, meta)
}

// RegisterAll registers all built-in tools in the given ToolRegistry.
// Each registration includes a factory and an auto-generated parameter schema.
func RegisterAll(reg *registry.ToolRegistry) {
	B := schema.PartTypeBlock

	// ── Validate ────────────────────────────────────────────────────

	reg.RegisterWithSchema("word-count", func() tool.Tool {
		return NewWordCountTool(&WordCountConfig{})
	}, toolSchema(&WordCountConfig{}, toolMeta("word-count", "Word Count", "validate",
		withInputs(B), withTags("analysis"))))

	reg.RegisterWithSchema("char-count", func() tool.Tool {
		return NewCharCountTool(&CharCountConfig{})
	}, toolSchema(&CharCountConfig{}, toolMeta("char-count", "Character Count", "validate",
		withInputs(B), withTags("analysis"))))

	reg.RegisterWithSchema("segment-count", func() tool.Tool {
		return NewSegCountTool(&SegCountConfig{})
	}, toolSchema(&SegCountConfig{}, toolMeta("segment-count", "Segment Count", "validate",
		withInputs(B), withTags("analysis"))))

	reg.RegisterWithSchema("qa-check", func() tool.Tool {
		return NewQACheckTool(NewQACheckConfig(model.LocaleEnglish))
	}, toolSchema(NewQACheckConfig(model.LocaleEnglish), toolMeta("qa-check", "QA Check", "validate",
		withInputs(B), withTags("quality"), withRequires("target-language"))))

	reg.RegisterWithSchema("inconsistency-check", func() tool.Tool {
		return NewInconsistencyCheckTool(NewInconsistencyCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewInconsistencyCheckConfig(model.LocaleEnglish), toolMeta("inconsistency-check", "Inconsistency Check", "validate",
		withInputs(B), withTags("quality"), withRequires("target-language"))))

	reg.RegisterWithSchema("length-check", func() tool.Tool {
		return NewLengthCheckTool(&LengthCheckConfig{TargetLocale: model.LocaleEnglish})
	}, toolSchema(&LengthCheckConfig{}, toolMeta("length-check", "Length Check", "validate",
		withInputs(B), withTags("quality"), withRequires("target-language"))))

	reg.RegisterWithSchema("chars-check", func() tool.Tool {
		return NewCharsCheckTool(NewCharsCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewCharsCheckConfig(model.LocaleEnglish), toolMeta("chars-check", "Characters Check", "validate",
		withInputs(B), withTags("quality"), withRequires("target-language"))))

	reg.RegisterWithSchema("pattern-check", func() tool.Tool {
		return NewPatternCheckTool(&PatternCheckConfig{TargetLocale: model.LocaleEnglish})
	}, toolSchema(&PatternCheckConfig{}, toolMeta("pattern-check", "Pattern Check", "validate",
		withInputs(B), withTags("quality", "regex"), withRequires("target-language"))))

	reg.RegisterWithSchema("term-check", func() tool.Tool {
		return NewTermCheckTool(&TermCheckConfig{TargetLocale: model.LocaleEnglish})
	}, toolSchema(&TermCheckConfig{}, toolMeta("term-check", "Terminology Check", "validate",
		withInputs(B), withTags("quality"), withRequires("target-language", "termbase"))))

	reg.RegisterWithSchema("xml-validation", func() tool.Tool {
		return NewXMLValidationTool(&XMLValidationConfig{CheckSource: true, WrapRoot: true})
	}, toolSchema(&XMLValidationConfig{CheckSource: true, WrapRoot: true}, toolMeta("xml-validation", "XML Validation", "validate",
		withInputs(B), withTags("quality"))))

	reg.RegisterWithSchema("translation-comparison", func() tool.Tool {
		return NewTranslationComparisonTool(&TranslationComparisonConfig{})
	}, toolSchema(&TranslationComparisonConfig{}, toolMeta("translation-comparison", "Translation Comparison", "validate",
		withInputs(B), withTags("quality"), withRequires("target-language"))))

	reg.RegisterWithSchema("chars-listing", func() tool.Tool {
		return NewCharsListingTool(&CharsListingConfig{
			IncludeSource: true, IncludeTarget: true, TargetLocale: model.LocaleEnglish,
		}).Tool()
	}, toolSchema(&CharsListingConfig{IncludeSource: true, IncludeTarget: true}, toolMeta("chars-listing", "Characters Listing", "validate",
		withInputs(B), withTags("analysis"))))

	reg.RegisterWithSchema("scoping-report", func() tool.Tool {
		return NewScopingReportTool(&ScopingReportConfig{})
	}, toolSchema(&ScopingReportConfig{}, toolMeta("scoping-report", "Scoping Report", "validate",
		withInputs(B), withTags("analysis"))))

	reg.RegisterWithSchema("repetition-analysis", func() tool.Tool {
		return NewRepetitionAnalysisTool(&RepetitionAnalysisConfig{CaseSensitive: true})
	}, toolSchema(&RepetitionAnalysisConfig{CaseSensitive: true}, toolMeta("repetition-analysis", "Repetition Analysis", "validate",
		withInputs(B), withTags("analysis"))))

	// ── Transform ───────────────────────────────────────────────────

	reg.RegisterWithSchema("pseudo-translate", func() tool.Tool {
		return NewPseudoTranslateTool(&PseudoConfig{Prefix: "[", Suffix: "]", TargetLocale: "qps"})
	}, toolSchema(&PseudoConfig{Prefix: "[", Suffix: "]"}, toolMeta("pseudo-translate", "Pseudo Translate", "transform",
		withInputs(B), withTags("translation"), withRequires("target-language"))))

	reg.RegisterWithSchema("search-replace", func() tool.Tool {
		return NewSearchReplaceTool(&SearchReplaceConfig{})
	}, toolSchema(&SearchReplaceConfig{}, toolMeta("search-replace", "Search and Replace", "transform",
		withInputs(B), withTags("regex", "configurable"))))

	reg.RegisterWithSchema("case-transform", func() tool.Tool {
		return NewCaseTransformTool(&CaseTransformConfig{Mode: CaseLower, ApplySource: true})
	}, toolSchema(&CaseTransformConfig{Mode: CaseLower, ApplySource: true}, toolMeta("case-transform", "Case Transform", "transform",
		withInputs(B), withTags("text-processing"))))

	reg.RegisterWithSchema("segmentation", func() tool.Tool {
		return NewSegmentationTool(&SegmentationConfig{})
	}, toolSchema(&SegmentationConfig{}, toolMeta("segmentation", "Segmentation", "transform",
		withInputs(B), withTags("text-processing"))))

	reg.RegisterWithSchema("create-target", func() tool.Tool {
		return NewCreateTargetTool(&CreateTargetConfig{})
	}, toolSchema(&CreateTargetConfig{}, toolMeta("create-target", "Create Target", "transform",
		withInputs(B), withRequires("target-language"))))

	reg.RegisterWithSchema("remove-target", func() tool.Tool {
		return NewRemoveTargetTool(&RemoveTargetConfig{})
	}, toolSchema(&RemoveTargetConfig{}, toolMeta("remove-target", "Remove Target", "transform",
		withInputs(B), withRequires("target-language"))))

	reg.RegisterWithSchema("inline-codes-remove", func() tool.Tool {
		return NewInlineCodesRemoveTool(&InlineCodesRemoveConfig{ApplyTarget: true})
	}, toolSchema(&InlineCodesRemoveConfig{ApplyTarget: true}, toolMeta("inline-codes-remove", "Inline Codes Remove", "transform",
		withInputs(B), withTags("text-processing"))))

	reg.RegisterWithSchema("properties-set", func() tool.Tool {
		return NewPropertiesSetTool(&PropertiesSetConfig{Overwrite: true, OnlyTranslatable: true})
	}, toolSchema(&PropertiesSetConfig{Overwrite: true, OnlyTranslatable: true}, toolMeta("properties-set", "Properties Set", "transform",
		withInputs(B), withTags("configurable"))))

	reg.RegisterWithSchema("whitespace-correct", func() tool.Tool {
		return NewWhitespaceCorrectTool(&WhitespaceCorrectConfig{
			TargetLocale: model.LocaleEnglish, NormalizeSpaces: true,
			MatchSourceWhitespace: true, RemoveZeroWidthChars: true,
		})
	}, toolSchema(&WhitespaceCorrectConfig{NormalizeSpaces: true, MatchSourceWhitespace: true, RemoveZeroWidthChars: true},
		toolMeta("whitespace-correct", "Whitespace Correct", "transform",
			withInputs(B), withTags("text-processing"), withRequires("target-language"))))

	reg.RegisterWithSchema("tag-protect", func() tool.Tool {
		return NewTagProtectTool(&TagProtectConfig{})
	}, toolSchema(&TagProtectConfig{}, toolMeta("tag-protect", "Tag Protect", "transform",
		withInputs(B), withTags("regex", "configurable"))))

	reg.RegisterWithSchema("xslt-transform", func() tool.Tool {
		return NewXSLTTransformTool(&XSLTTransformConfig{})
	}, toolSchema(&XSLTTransformConfig{}, toolMeta("xslt-transform", "XSLT Transform", "transform",
		withInputs(B, schema.PartTypeData), withTags("configurable"))))

	// ── Enrich ──────────────────────────────────────────────────────

	reg.RegisterWithSchema("tm-leverage", func() tool.Tool {
		return NewTMLeverageTool(&TMLeverageConfig{FuzzyThreshold: 70, Provider: NullTMProvider{}})
	}, toolSchema(&TMLeverageConfig{FuzzyThreshold: 70}, toolMeta("tm-leverage", "TM Leverage", "enrich",
		withInputs(B), withTags("translation"), withRequires("target-language", "tm"))))

	reg.RegisterWithSchema("diff-leverage", func() tool.Tool {
		return NewDiffLeverageTool(&DiffLeverageConfig{CaseSensitive: true, PreviousTexts: map[string]PreviousBlock{}})
	}, toolSchema(&DiffLeverageConfig{CaseSensitive: true}, toolMeta("diff-leverage", "Diff Leverage", "enrich",
		withInputs(B), withTags("translation"))))

	// ── Convert ─────────────────────────────────────────────────────

	reg.RegisterWithSchema("encoding-convert", func() tool.Tool {
		return NewEncodingConvertTool(&EncodingConvertConfig{ApplyTarget: true})
	}, toolSchema(&EncodingConvertConfig{ApplyTarget: true}, toolMeta("encoding-convert", "Encoding Convert", "convert",
		withInputs(B, schema.PartTypeData))))

	reg.RegisterWithSchema("encoding-detect", func() tool.Tool {
		return NewEncodingDetectTool(&EncodingDetectConfig{})
	}, toolSchema(&EncodingDetectConfig{}, toolMeta("encoding-detect", "Encoding Detect", "convert",
		withInputs(B, schema.PartTypeData))))

	reg.RegisterWithSchema("linebreak-convert", func() tool.Tool {
		return NewLineBreakConvertTool(&LineBreakConvertConfig{Mode: LineBreakLF, ApplySource: true, ApplyTarget: true})
	}, toolSchema(&LineBreakConvertConfig{Mode: LineBreakLF, ApplySource: true, ApplyTarget: true},
		toolMeta("linebreak-convert", "Line Break Convert", "convert",
			withInputs(B))))

	reg.RegisterWithSchema("bom-convert", func() tool.Tool {
		return NewBOMConvertTool(&BOMConvertConfig{})
	}, toolSchema(&BOMConvertConfig{}, toolMeta("bom-convert", "BOM Convert", "convert",
		withInputs(B, schema.PartTypeData))))

	reg.RegisterWithSchema("fullwidth-convert", func() tool.Tool {
		return NewFullWidthConvertTool(&FullWidthConvertConfig{Mode: FullWidthToHalf, ApplyTarget: true})
	}, toolSchema(&FullWidthConvertConfig{Mode: FullWidthToHalf, ApplyTarget: true},
		toolMeta("fullwidth-convert", "Full Width Convert", "convert",
			withInputs(B), withTags("text-processing"))))

	reg.RegisterWithSchema("uri-convert", func() tool.Tool {
		return NewURIConvertTool(&URIConvertConfig{Mode: URIDecode, ApplyTarget: true})
	}, toolSchema(&URIConvertConfig{Mode: URIDecode, ApplyTarget: true}, toolMeta("uri-convert", "URI Convert", "convert",
		withInputs(B), withTags("text-processing"))))

	// ── Pipeline ────────────────────────────────────────────────────

	reg.RegisterWithSchema("span-classify", func() tool.Tool {
		return NewSpanClassifyTool(&SpanClassifyConfig{})
	}, toolSchema(&SpanClassifyConfig{}, toolMeta("span-classify", "Span Classify", "pipeline",
		withInputs(B), withTags("text-processing"))))

	reg.RegisterWithSchema("layer-processor", func() tool.Tool {
		return NewLayerProcessorTool(&LayerProcessorConfig{})
	}, nil) // LayerProcessorConfig has interface fields, skip schema

	reg.RegisterWithSchema("external-command", func() tool.Tool {
		return NewExternalCommandTool(&ExternalCommandConfig{ApplyTarget: true, SendAsStdin: true, Timeout: 30})
	}, toolSchema(&ExternalCommandConfig{ApplyTarget: true, SendAsStdin: true, Timeout: 30},
		toolMeta("external-command", "External Command", "pipeline",
			withInputs(B), withTags("configurable"))))

	reg.RegisterWithSchema("brand-vocab-check", func() tool.Tool {
		return NewBrandVocabCheckTool(nil, nil)
	}, nil) // BrandVocabConfig has interface fields, skip schema

	// ── Utility ─────────────────────────────────────────────────────

	reg.RegisterWithSchema("batch", func() tool.Tool {
		return NewBatchTool(&BatchConfig{Size: 10})
	}, toolSchema(&BatchConfig{Size: 10}, toolMeta("batch", "Batch Collector", "pipeline",
		withInputs(B), withTags("batch"))))

	reg.RegisterWithSchema("script", func() tool.Tool {
		return NewScriptTool(&ScriptConfig{})
	}, toolSchema(&ScriptConfig{}, toolMeta("script", "Script", "pipeline",
		withInputs(B, schema.PartTypeData), withTags("configurable"))))
}
