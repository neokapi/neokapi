package tools

import "github.com/neokapi/neokapi/core/registry"

// Built-in tool identifiers. These constants are the canonical names used
// for tool registration and lookup. Using these instead of raw strings
// provides compile-time safety.
const (
	// Validate
	WordCount       registry.ToolID = "word-count"
	CharCount       registry.ToolID = "char-count"
	SegmentCount    registry.ToolID = "segment-count"
	RuleCheck       registry.ToolID = "qa"
	TermCheck       registry.ToolID = "term-check"
	XMLValidation   registry.ToolID = "xml-validation"
	VoiceVocabCheck registry.ToolID = "voice-vocab-check"

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
	// MemoryLeverage's id is "recycle" (the canonical command). The Go const
	// name is unchanged.
	MemoryLeverage registry.ToolID = "recycle"
	DiffLeverage   registry.ToolID = "diff-leverage"

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
