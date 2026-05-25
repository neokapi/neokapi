package flow

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// ToolFactory creates a fresh Tool instance. Used for parallel execution
// where each document needs its own tool chain to avoid shared state.
type ToolFactory func() (tool.Tool, error)

// Flow represents a configured sequence of Tools that Parts stream through.
//
// A Flow has an optional leading source-transform stage: tools that rewrite the
// source/model (redaction, a simplifier, normalization) to settle a single
// canonical model before the main tools run. The executor runs the
// source-transform stage ahead of the main tools, so downstream tools
// (segmentation, terminology, translation, QA) all see the same settled source.
// Only source-transform-capable tools (tool.CapTransform) may sit in this stage.
type Flow struct {
	Name string

	// SourceTransforms is the leading stage: source-settling transforms that
	// run before Tools. For parallel execution, SourceTransformFactories
	// supplies per-document instances instead.
	SourceTransforms         []tool.Tool
	SourceTransformFactories []ToolFactory

	Tools         []tool.Tool   // for single-doc / sequential execution
	ToolFactories []ToolFactory // for parallel: creates fresh tool chain per document
}

// Pipeline returns the full ordered tool chain — the source-transform stage
// followed by the main tools. Because Parts stream through tools in order, a
// source transform at the front fully settles a block before any later tool
// sees it.
func (f *Flow) Pipeline() []tool.Tool {
	if len(f.SourceTransforms) == 0 {
		return f.Tools
	}
	out := make([]tool.Tool, 0, len(f.SourceTransforms)+len(f.Tools))
	out = append(out, f.SourceTransforms...)
	return append(out, f.Tools...)
}

// PipelineFactories is the factory equivalent of Pipeline, for parallel
// execution: source-transform factories followed by the main tool factories.
func (f *Flow) PipelineFactories() []ToolFactory {
	if len(f.SourceTransformFactories) == 0 {
		return f.ToolFactories
	}
	out := make([]ToolFactory, 0, len(f.SourceTransformFactories)+len(f.ToolFactories))
	out = append(out, f.SourceTransformFactories...)
	return append(out, f.ToolFactories...)
}

// Item represents a single document to process in a batch.
type Item struct {
	Input          *model.RawDocument
	OutputPath     string
	OutputEncoding string
	TargetLocale   model.LocaleID

	// OutputBlocks holds processed blocks after flow execution, enabling
	// store-backed persistence when a projectID is provided.
	OutputBlocks []*model.Block
}

// FlowItem is a deprecated alias for [Item].
//
// Deprecated: Use [Item] instead.
type FlowItem = Item
