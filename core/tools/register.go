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
}
