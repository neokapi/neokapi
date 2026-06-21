package tools

import "github.com/neokapi/neokapi/core/registry"

// Built-in tool identifiers. These constants are the canonical names used
// for tool registration and lookup. Using these instead of raw strings
// provides compile-time safety.
const (
	// Validate
	WordCount             registry.ToolID = "word-count"
	CharCount             registry.ToolID = "char-count"
	SegmentCount          registry.ToolID = "segment-count"
	QACheck               registry.ToolID = "qa"
	InconsistencyCheck    registry.ToolID = "inconsistency-check"
	LengthCheck           registry.ToolID = "length-check"
	CharsCheck            registry.ToolID = "chars-check"
	PatternCheck          registry.ToolID = "pattern-check"
	TermCheck             registry.ToolID = "term-check"
	XMLValidation         registry.ToolID = "xml-validation"
	ContentLint           registry.ToolID = "content-lint"
	TranslationComparison registry.ToolID = "translation-comparison"
	CharsListing          registry.ToolID = "chars-listing"
	ScopingReport         registry.ToolID = "scoping-report"
	RepetitionAnalysis    registry.ToolID = "repetition-analysis"
	BrandVocabCheck       registry.ToolID = "brand-vocab-check"

	// Transform
	PseudoTranslate   registry.ToolID = "pseudo-translate"
	SearchReplace     registry.ToolID = "search-replace"
	CaseTransform     registry.ToolID = "case-transform"
	Segmentation      registry.ToolID = "segmentation"
	CreateTarget      registry.ToolID = "create-target"
	RemoveTarget      registry.ToolID = "remove-target"
	InlineCodesRemove registry.ToolID = "inline-codes-remove"
	PropertiesSet     registry.ToolID = "properties-set"
	WhitespaceCorrect registry.ToolID = "whitespace-correct"
	TagProtect        registry.ToolID = "tag-protect"
	XSLTTransform     registry.ToolID = "xslt-transform"

	// Enrich
	TMLeverage   registry.ToolID = "tm-leverage"
	DiffLeverage registry.ToolID = "diff-leverage"

	// Convert
	EncodingConvert  registry.ToolID = "encoding-convert"
	EncodingDetect   registry.ToolID = "encoding-detect"
	LineBreakConvert registry.ToolID = "linebreak-convert"
	BOMConvert       registry.ToolID = "bom-convert"
	FullWidthConvert registry.ToolID = "fullwidth-convert"
	URIConvert       registry.ToolID = "uri-convert"

	// Pipeline
	SpanClassify       registry.ToolID = "span-classify"
	LayerProcessorTool registry.ToolID = "layer-processor"
	ExternalCommand    registry.ToolID = "external-command"

	// Utility
	Batch  registry.ToolID = "batch"
	Script registry.ToolID = "script"
)
