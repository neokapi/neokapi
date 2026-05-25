package flow

import (
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Builder provides a fluent API for constructing Flows.
type Builder struct {
	name                     string
	sourceTransforms         []tool.Tool
	sourceTransformFactories []ToolFactory
	tools                    []tool.Tool
	toolFactories            []ToolFactory
	items                    []*Item
}

// FlowBuilder is a deprecated alias for [Builder].
//
// Deprecated: Use [Builder] instead.
type FlowBuilder = Builder

// NewFlow creates a new Builder with the given name.
func NewFlow(name string) *Builder {
	return &Builder{name: name}
}

// AddTool appends a Tool to the flow's main stage.
func (fb *Builder) AddTool(t tool.Tool) *Builder {
	fb.tools = append(fb.tools, t)
	return fb
}

// AddSourceTransform appends a source-transform to the flow's leading stage —
// tools that settle the source/model (redaction, simplification, normalization)
// before the main tools run. Build rejects any tool here that is not
// source-transform-capable (tool.CapTransform).
func (fb *Builder) AddSourceTransform(t tool.Tool) *Builder {
	fb.sourceTransforms = append(fb.sourceTransforms, t)
	return fb
}

// AddSourceTransformFactory appends a source-transform factory for parallel
// execution; each document gets a fresh instance.
func (fb *Builder) AddSourceTransformFactory(f ToolFactory) *Builder {
	fb.sourceTransformFactories = append(fb.sourceTransformFactories, f)
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

// Build constructs the Flow, validating that the builder has a non-empty name,
// at least one tool or tool factory, and that every tool in the
// source-transform stage is source-transform-capable.
func (fb *Builder) Build() (*Flow, error) {
	if fb.name == "" {
		return nil, errors.New("flow name must not be empty")
	}
	total := len(fb.tools) + len(fb.toolFactories) + len(fb.sourceTransforms) + len(fb.sourceTransformFactories)
	if total == 0 {
		return nil, errors.New("flow must have at least one tool or tool factory")
	}
	for _, t := range fb.sourceTransforms {
		if !tool.IsSourceTransform(t) {
			return nil, fmt.Errorf("flow %q: tool %q in the source-transform stage is not source-transform-capable (it must use a Transform handler)", fb.name, t.Name())
		}
	}
	return &Flow{
		Name:                     fb.name,
		SourceTransforms:         fb.sourceTransforms,
		SourceTransformFactories: fb.sourceTransformFactories,
		Tools:                    fb.tools,
		ToolFactories:            fb.toolFactories,
	}, nil
}

// Items returns the configured batch items.
func (fb *Builder) Items() []*Item {
	return fb.items
}
