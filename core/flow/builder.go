package flow

import (
	"errors"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Builder provides a fluent API for constructing Flows.
type Builder struct {
	name          string
	tools         []tool.Tool
	toolFactories []ToolFactory
	items         []*Item
}

// FlowBuilder is a deprecated alias for [Builder].
//
// Deprecated: Use [Builder] instead.
type FlowBuilder = Builder

// NewFlow creates a new Builder with the given name.
func NewFlow(name string) *Builder {
	return &Builder{name: name}
}

// AddTool appends a Tool to the flow's ordered tool chain. Transformers are
// ordinary steps (AD-006); ordering safety is the placement pass
// (FlowDefinition.ValidatePlacement), not a structural stage.
func (fb *Builder) AddTool(t tool.Tool) *Builder {
	fb.tools = append(fb.tools, t)
	return fb
}

// AddToolFactory appends a ToolFactory for parallel execution.
// When the flow is executed in parallel, each document gets fresh tool
// instances created by the registered factories.
func (fb *Builder) AddToolFactory(f ToolFactory) *Builder {
	fb.toolFactories = append(fb.toolFactories, f)
	return fb
}

// AddItem adds a batch item to process.
func (fb *Builder) AddItem(input *model.RawDocument, outputPath string, targetLocale model.LocaleID) *Builder {
	fb.items = append(fb.items, &Item{
		Input:        input,
		OutputPath:   outputPath,
		TargetLocale: targetLocale,
	})
	return fb
}

// Build constructs the Flow, validating that the builder has a non-empty name
// and at least one tool or tool factory.
func (fb *Builder) Build() (*Flow, error) {
	if fb.name == "" {
		return nil, errors.New("flow name must not be empty")
	}
	if len(fb.tools)+len(fb.toolFactories) == 0 {
		return nil, errors.New("flow must have at least one tool or tool factory")
	}
	return &Flow{
		Name:          fb.name,
		Tools:         fb.tools,
		ToolFactories: fb.toolFactories,
	}, nil
}

// Items returns the configured batch items.
func (fb *Builder) Items() []*Item {
	return fb.items
}
