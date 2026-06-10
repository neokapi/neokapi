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
// Transformers (redaction, normalization) are ordinary ordered steps — there
// is no separate structural stage (AD-006). Because the applier mutates inline
// and in order, each transformer settles the source before later steps observe
// it; ordering safety is the placement pass (ValidatePlacement), which runs
// beside the data-flow contract at build/load.
type Flow struct {
	Name string

	Tools         []tool.Tool   // for single-doc / sequential execution
	ToolFactories []ToolFactory // for parallel: creates fresh tool chain per document
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
