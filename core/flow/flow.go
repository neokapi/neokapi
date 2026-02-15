package flow

import (
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// ToolFactory creates a fresh Tool instance. Used for parallel execution
// where each document needs its own tool chain to avoid shared state.
type ToolFactory func() (tool.Tool, error)

// Flow represents a configured sequence of Tools that Parts stream through.
type Flow struct {
	Name          string
	Tools         []tool.Tool   // for single-doc / sequential execution
	ToolFactories []ToolFactory // for parallel: creates fresh tool chain per document
}

// FlowItem represents a single document to process in a batch.
type FlowItem struct {
	Input          *model.RawDocument
	OutputPath     string
	OutputEncoding string
	TargetLocale   model.LocaleID
}
