package tools

import (
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/core/tool"
)

// RegisterAll registers all built-in tools in the given ToolRegistry.
// Each registration is a factory that creates a new tool instance with default config.
func RegisterAll(reg *registry.ToolRegistry) {
	reg.Register("word-count", func() tool.Tool {
		return NewWordCountTool(&WordCountConfig{})
	})

	reg.Register("char-count", func() tool.Tool {
		return NewCharCountTool(&CharCountConfig{})
	})

	reg.Register("pseudo-translate", func() tool.Tool {
		return NewPseudoTranslateTool(&PseudoConfig{
			Prefix:       "[",
			Suffix:       "]",
			TargetLocale: "qps",
		})
	})

	reg.Register("search-replace", func() tool.Tool {
		return NewSearchReplaceTool(&SearchReplaceConfig{})
	})

	reg.Register("segment-count", func() tool.Tool {
		return NewSegCountTool(&SegCountConfig{})
	})

	reg.Register("case-transform", func() tool.Tool {
		return NewCaseTransformTool(&CaseTransformConfig{
			Mode:        CaseLower,
			ApplySource: true,
		})
	})

	reg.Register("xslt-transform", func() tool.Tool {
		return NewXSLTTransformTool(&XSLTTransformConfig{})
	})

	reg.Register("encoding-detect", func() tool.Tool {
		return NewEncodingDetectTool(&EncodingDetectConfig{})
	})

	reg.Register("xml-validation", func() tool.Tool {
		return NewXMLValidationTool(&XMLValidationConfig{CheckSource: true, WrapRoot: true})
	})

	reg.Register("tag-protect", func() tool.Tool {
		return NewTagProtectTool(&TagProtectConfig{})
	})

	reg.Register("term-check", func() tool.Tool {
		return NewTermCheckTool(&TermCheckConfig{
			TargetLocale: model.LocaleEnglish,
		})
	})

	reg.Register("segmentation", func() tool.Tool {
		return NewSegmentationTool(&SegmentationConfig{})
	})

	reg.Register("tm-leverage", func() tool.Tool {
		return NewTMLeverageTool(&TMLeverageConfig{
			FuzzyThreshold: 70,
			Provider:       NullTMProvider{},
		})
	})

	reg.Register("qa-check", func() tool.Tool {
		return NewQACheckTool(NewQACheckConfig(model.LocaleEnglish))
	})

	reg.Register("span-classify", func() tool.Tool {
		return NewSpanClassifyTool(&SpanClassifyConfig{})
	})

	reg.Register("layer-processor", func() tool.Tool {
		return NewLayerProcessorTool(&LayerProcessorConfig{})
	})

	reg.Register("create-target", func() tool.Tool {
		return NewCreateTargetTool(&CreateTargetConfig{})
	})

	reg.Register("remove-target", func() tool.Tool {
		return NewRemoveTargetTool(&RemoveTargetConfig{})
	})

	reg.Register("linebreak-convert", func() tool.Tool {
		return NewLineBreakConvertTool(&LineBreakConvertConfig{
			Mode:        LineBreakLF,
			ApplySource: true,
			ApplyTarget: true,
		})
	})

	reg.Register("bom-convert", func() tool.Tool {
		return NewBOMConvertTool(&BOMConvertConfig{})
	})

	reg.Register("fullwidth-convert", func() tool.Tool {
		return NewFullWidthConvertTool(&FullWidthConvertConfig{
			Mode:        FullWidthToHalf,
			ApplyTarget: true,
		})
	})

	reg.Register("uri-convert", func() tool.Tool {
		return NewURIConvertTool(&URIConvertConfig{
			Mode:        URIDecode,
			ApplyTarget: true,
		})
	})

	reg.Register("inline-codes-remove", func() tool.Tool {
		return NewInlineCodesRemoveTool(&InlineCodesRemoveConfig{
			ApplyTarget: true,
		})
	})

	reg.Register("properties-set", func() tool.Tool {
		return NewPropertiesSetTool(&PropertiesSetConfig{
			Overwrite:        true,
			OnlyTranslatable: true,
		})
	})

	reg.Register("translation-comparison", func() tool.Tool {
		return NewTranslationComparisonTool(&TranslationComparisonConfig{})
	})

	reg.Register("repetition-analysis", func() tool.Tool {
		return NewRepetitionAnalysisTool(&RepetitionAnalysisConfig{CaseSensitive: true})
	})

	reg.Register("whitespace-correct", func() tool.Tool {
		return NewWhitespaceCorrectTool(&WhitespaceCorrectConfig{
			TargetLocale:          model.LocaleEnglish,
			NormalizeSpaces:       true,
			MatchSourceWhitespace: true,
			RemoveZeroWidthChars:  true,
		})
	})

	reg.Register("encoding-convert", func() tool.Tool {
		return NewEncodingConvertTool(&EncodingConvertConfig{
			ApplyTarget: true,
		})
	})

	reg.Register("inconsistency-check", func() tool.Tool {
		return NewInconsistencyCheckTool(NewInconsistencyCheckConfig(model.LocaleEnglish))
	})

	reg.Register("length-check", func() tool.Tool {
		return NewLengthCheckTool(&LengthCheckConfig{
			TargetLocale: model.LocaleEnglish,
		})
	})

	reg.Register("chars-check", func() tool.Tool {
		return NewCharsCheckTool(NewCharsCheckConfig(model.LocaleEnglish))
	})

	reg.Register("diff-leverage", func() tool.Tool {
		return NewDiffLeverageTool(&DiffLeverageConfig{
			CaseSensitive: true,
			PreviousTexts: map[string]PreviousBlock{},
		})
	})

	reg.Register("external-command", func() tool.Tool {
		return NewExternalCommandTool(&ExternalCommandConfig{
			ApplyTarget: true,
			SendAsStdin: true,
			Timeout:     30,
		})
	})

	reg.Register("pattern-check", func() tool.Tool {
		return NewPatternCheckTool(&PatternCheckConfig{
			TargetLocale: model.LocaleEnglish,
		})
	})

	reg.Register("chars-listing", func() tool.Tool {
		return NewCharsListingTool(&CharsListingConfig{
			IncludeSource: true,
			IncludeTarget: true,
			TargetLocale:  model.LocaleEnglish,
		}).Tool()
	})

	reg.Register("scoping-report", func() tool.Tool {
		return NewScopingReportTool(&ScopingReportConfig{})
	})
}
