package tools

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// toolSchema is a shorthand for generating a tool schema from a config struct.
func toolSchema(cfg any, id, displayName, category string) *schema.ComponentSchema {
	return schema.FromStruct(cfg, schema.ComponentMeta{
		ID:          id,
		Type:        "tool",
		Category:    category,
		DisplayName: displayName,
	})
}

// RegisterAll registers all built-in tools in the given ToolRegistry.
// Each registration includes a factory and an auto-generated parameter schema.
func RegisterAll(reg *registry.ToolRegistry) {
	// ── Validate ────────────────────────────────────────────────────

	reg.RegisterWithSchema("word-count", func() tool.Tool {
		return NewWordCountTool(&WordCountConfig{})
	}, toolSchema(&WordCountConfig{}, "word-count", "Word Count", "validate"))

	reg.RegisterWithSchema("char-count", func() tool.Tool {
		return NewCharCountTool(&CharCountConfig{})
	}, toolSchema(&CharCountConfig{}, "char-count", "Character Count", "validate"))

	reg.RegisterWithSchema("segment-count", func() tool.Tool {
		return NewSegCountTool(&SegCountConfig{})
	}, toolSchema(&SegCountConfig{}, "segment-count", "Segment Count", "validate"))

	reg.RegisterWithSchema("qa-check", func() tool.Tool {
		return NewQACheckTool(NewQACheckConfig(model.LocaleEnglish))
	}, toolSchema(NewQACheckConfig(model.LocaleEnglish), "qa-check", "QA Check", "validate"))

	reg.RegisterWithSchema("inconsistency-check", func() tool.Tool {
		return NewInconsistencyCheckTool(NewInconsistencyCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewInconsistencyCheckConfig(model.LocaleEnglish), "inconsistency-check", "Inconsistency Check", "validate"))

	reg.RegisterWithSchema("length-check", func() tool.Tool {
		return NewLengthCheckTool(&LengthCheckConfig{TargetLocale: model.LocaleEnglish})
	}, toolSchema(&LengthCheckConfig{}, "length-check", "Length Check", "validate"))

	reg.RegisterWithSchema("chars-check", func() tool.Tool {
		return NewCharsCheckTool(NewCharsCheckConfig(model.LocaleEnglish))
	}, toolSchema(NewCharsCheckConfig(model.LocaleEnglish), "chars-check", "Characters Check", "validate"))

	reg.RegisterWithSchema("pattern-check", func() tool.Tool {
		return NewPatternCheckTool(&PatternCheckConfig{TargetLocale: model.LocaleEnglish})
	}, toolSchema(&PatternCheckConfig{}, "pattern-check", "Pattern Check", "validate"))

	reg.RegisterWithSchema("term-check", func() tool.Tool {
		return NewTermCheckTool(&TermCheckConfig{TargetLocale: model.LocaleEnglish})
	}, toolSchema(&TermCheckConfig{}, "term-check", "Terminology Check", "validate"))

	reg.RegisterWithSchema("xml-validation", func() tool.Tool {
		return NewXMLValidationTool(&XMLValidationConfig{CheckSource: true, WrapRoot: true})
	}, toolSchema(&XMLValidationConfig{CheckSource: true, WrapRoot: true}, "xml-validation", "XML Validation", "validate"))

	reg.RegisterWithSchema("translation-comparison", func() tool.Tool {
		return NewTranslationComparisonTool(&TranslationComparisonConfig{})
	}, toolSchema(&TranslationComparisonConfig{}, "translation-comparison", "Translation Comparison", "validate"))

	reg.RegisterWithSchema("chars-listing", func() tool.Tool {
		return NewCharsListingTool(&CharsListingConfig{
			IncludeSource: true, IncludeTarget: true, TargetLocale: model.LocaleEnglish,
		}).Tool()
	}, toolSchema(&CharsListingConfig{IncludeSource: true, IncludeTarget: true}, "chars-listing", "Characters Listing", "validate"))

	reg.RegisterWithSchema("scoping-report", func() tool.Tool {
		return NewScopingReportTool(&ScopingReportConfig{})
	}, toolSchema(&ScopingReportConfig{}, "scoping-report", "Scoping Report", "validate"))

	reg.RegisterWithSchema("repetition-analysis", func() tool.Tool {
		return NewRepetitionAnalysisTool(&RepetitionAnalysisConfig{CaseSensitive: true})
	}, toolSchema(&RepetitionAnalysisConfig{CaseSensitive: true}, "repetition-analysis", "Repetition Analysis", "validate"))

	// ── Transform ───────────────────────────────────────────────────

	reg.RegisterWithSchema("pseudo-translate", func() tool.Tool {
		return NewPseudoTranslateTool(&PseudoConfig{Prefix: "[", Suffix: "]", TargetLocale: "qps"})
	}, toolSchema(&PseudoConfig{Prefix: "[", Suffix: "]"}, "pseudo-translate", "Pseudo Translate", "transform"))

	reg.RegisterWithSchema("search-replace", func() tool.Tool {
		return NewSearchReplaceTool(&SearchReplaceConfig{})
	}, toolSchema(&SearchReplaceConfig{}, "search-replace", "Search and Replace", "transform"))

	reg.RegisterWithSchema("case-transform", func() tool.Tool {
		return NewCaseTransformTool(&CaseTransformConfig{Mode: CaseLower, ApplySource: true})
	}, toolSchema(&CaseTransformConfig{Mode: CaseLower, ApplySource: true}, "case-transform", "Case Transform", "transform"))

	reg.RegisterWithSchema("segmentation", func() tool.Tool {
		return NewSegmentationTool(&SegmentationConfig{})
	}, toolSchema(&SegmentationConfig{}, "segmentation", "Segmentation", "transform"))

	reg.RegisterWithSchema("create-target", func() tool.Tool {
		return NewCreateTargetTool(&CreateTargetConfig{})
	}, toolSchema(&CreateTargetConfig{}, "create-target", "Create Target", "transform"))

	reg.RegisterWithSchema("remove-target", func() tool.Tool {
		return NewRemoveTargetTool(&RemoveTargetConfig{})
	}, toolSchema(&RemoveTargetConfig{}, "remove-target", "Remove Target", "transform"))

	reg.RegisterWithSchema("inline-codes-remove", func() tool.Tool {
		return NewInlineCodesRemoveTool(&InlineCodesRemoveConfig{ApplyTarget: true})
	}, toolSchema(&InlineCodesRemoveConfig{ApplyTarget: true}, "inline-codes-remove", "Inline Codes Remove", "transform"))

	reg.RegisterWithSchema("properties-set", func() tool.Tool {
		return NewPropertiesSetTool(&PropertiesSetConfig{Overwrite: true, OnlyTranslatable: true})
	}, toolSchema(&PropertiesSetConfig{Overwrite: true, OnlyTranslatable: true}, "properties-set", "Properties Set", "transform"))

	reg.RegisterWithSchema("whitespace-correct", func() tool.Tool {
		return NewWhitespaceCorrectTool(&WhitespaceCorrectConfig{
			TargetLocale: model.LocaleEnglish, NormalizeSpaces: true,
			MatchSourceWhitespace: true, RemoveZeroWidthChars: true,
		})
	}, toolSchema(&WhitespaceCorrectConfig{NormalizeSpaces: true, MatchSourceWhitespace: true, RemoveZeroWidthChars: true},
		"whitespace-correct", "Whitespace Correct", "transform"))

	reg.RegisterWithSchema("tag-protect", func() tool.Tool {
		return NewTagProtectTool(&TagProtectConfig{})
	}, toolSchema(&TagProtectConfig{}, "tag-protect", "Tag Protect", "transform"))

	reg.RegisterWithSchema("xslt-transform", func() tool.Tool {
		return NewXSLTTransformTool(&XSLTTransformConfig{})
	}, toolSchema(&XSLTTransformConfig{}, "xslt-transform", "XSLT Transform", "transform"))

	// ── Enrich ──────────────────────────────────────────────────────

	reg.RegisterWithSchema("tm-leverage", func() tool.Tool {
		return NewTMLeverageTool(&TMLeverageConfig{FuzzyThreshold: 70, Provider: NullTMProvider{}})
	}, toolSchema(&TMLeverageConfig{FuzzyThreshold: 70}, "tm-leverage", "TM Leverage", "enrich"))

	reg.RegisterWithSchema("diff-leverage", func() tool.Tool {
		return NewDiffLeverageTool(&DiffLeverageConfig{CaseSensitive: true, PreviousTexts: map[string]PreviousBlock{}})
	}, toolSchema(&DiffLeverageConfig{CaseSensitive: true}, "diff-leverage", "Diff Leverage", "enrich"))

	// ── Convert ─────────────────────────────────────────────────────

	reg.RegisterWithSchema("encoding-convert", func() tool.Tool {
		return NewEncodingConvertTool(&EncodingConvertConfig{ApplyTarget: true})
	}, toolSchema(&EncodingConvertConfig{ApplyTarget: true}, "encoding-convert", "Encoding Convert", "convert"))

	reg.RegisterWithSchema("encoding-detect", func() tool.Tool {
		return NewEncodingDetectTool(&EncodingDetectConfig{})
	}, toolSchema(&EncodingDetectConfig{}, "encoding-detect", "Encoding Detect", "convert"))

	reg.RegisterWithSchema("linebreak-convert", func() tool.Tool {
		return NewLineBreakConvertTool(&LineBreakConvertConfig{Mode: LineBreakLF, ApplySource: true, ApplyTarget: true})
	}, toolSchema(&LineBreakConvertConfig{Mode: LineBreakLF, ApplySource: true, ApplyTarget: true},
		"linebreak-convert", "Line Break Convert", "convert"))

	reg.RegisterWithSchema("bom-convert", func() tool.Tool {
		return NewBOMConvertTool(&BOMConvertConfig{})
	}, toolSchema(&BOMConvertConfig{}, "bom-convert", "BOM Convert", "convert"))

	reg.RegisterWithSchema("fullwidth-convert", func() tool.Tool {
		return NewFullWidthConvertTool(&FullWidthConvertConfig{Mode: FullWidthToHalf, ApplyTarget: true})
	}, toolSchema(&FullWidthConvertConfig{Mode: FullWidthToHalf, ApplyTarget: true},
		"fullwidth-convert", "Full Width Convert", "convert"))

	reg.RegisterWithSchema("uri-convert", func() tool.Tool {
		return NewURIConvertTool(&URIConvertConfig{Mode: URIDecode, ApplyTarget: true})
	}, toolSchema(&URIConvertConfig{Mode: URIDecode, ApplyTarget: true}, "uri-convert", "URI Convert", "convert"))

	// ── Pipeline ────────────────────────────────────────────────────

	reg.RegisterWithSchema("span-classify", func() tool.Tool {
		return NewSpanClassifyTool(&SpanClassifyConfig{})
	}, toolSchema(&SpanClassifyConfig{}, "span-classify", "Span Classify", "pipeline"))

	reg.RegisterWithSchema("layer-processor", func() tool.Tool {
		return NewLayerProcessorTool(&LayerProcessorConfig{})
	}, nil) // LayerProcessorConfig has interface fields, skip schema

	reg.RegisterWithSchema("external-command", func() tool.Tool {
		return NewExternalCommandTool(&ExternalCommandConfig{ApplyTarget: true, SendAsStdin: true, Timeout: 30})
	}, toolSchema(&ExternalCommandConfig{ApplyTarget: true, SendAsStdin: true, Timeout: 30},
		"external-command", "External Command", "pipeline"))

	reg.RegisterWithSchema("brand-vocab-check", func() tool.Tool {
		return NewBrandVocabCheckTool(nil, nil)
	}, nil) // BrandVocabConfig has interface fields, skip schema

	// ── Utility ─────────────────────────────────────────────────────

	reg.RegisterWithSchema("batch", func() tool.Tool {
		return NewBatchTool(&BatchConfig{Size: 10})
	}, toolSchema(&BatchConfig{Size: 10}, "batch", "Batch Collector", "pipeline"))

	reg.RegisterWithSchema("script", func() tool.Tool {
		return NewScriptTool(&ScriptConfig{})
	}, toolSchema(&ScriptConfig{}, "script", "Script", "pipeline"))
}
