package tools

import "github.com/neokapi/neokapi/core/registry"

// Built-in tool identifiers. These constants are the canonical names used
// for tool registration and lookup. Using these instead of raw strings
// provides compile-time safety.
const (
	// Validate
	WordCount          registry.ToolID = "word-count"
	CharCount          registry.ToolID = "char-count"
	SegmentCount       registry.ToolID = "segment-count"
	QACheck            registry.ToolID = "qa"
	InconsistencyCheck registry.ToolID = "inconsistency-check"
	LengthCheck        registry.ToolID = "length-check"
	CharsCheck         registry.ToolID = "chars-check"
	PatternCheck       registry.ToolID = "pattern-check"
	TermCheck          registry.ToolID = "term-check"
	XMLValidation      registry.ToolID = "xml-validation"
	ContentLint        registry.ToolID = "content-lint"
	ScopingReport      registry.ToolID = "scoping-report"
	RepetitionAnalysis registry.ToolID = "repetition-analysis"
	BrandVocabCheck    registry.ToolID = "brand-vocab-check"

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

	// Enrich
	// TMLeverage's id is "recycle" (the canonical command). The Go const
	// name is unchanged.
	TMLeverage   registry.ToolID = "recycle"
	DiffLeverage registry.ToolID = "diff-leverage"

	// Analyze
	EncodingDetect registry.ToolID = "encoding-detect"

	// Pipeline
	SpanClassify       registry.ToolID = "span-classify"
	LayerProcessorTool registry.ToolID = "layer-processor"
	ExternalCommand    registry.ToolID = "external-command"

	// Utility
	Batch  registry.ToolID = "batch"
	Script registry.ToolID = "script"
)
