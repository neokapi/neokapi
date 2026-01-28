package flow

import (
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
)

// FlowBuilder provides a fluent API for constructing Flows.
type FlowBuilder struct {
	name  string
	tools []tool.Tool
	items []*FlowItem
}

// NewFlow creates a new FlowBuilder with the given name.
func NewFlow(name string) *FlowBuilder {
	return &FlowBuilder{name: name}
}

// AddTool appends a Tool to the flow.
func (fb *FlowBuilder) AddTool(t tool.Tool) *FlowBuilder {
	fb.tools = append(fb.tools, t)
	return fb
}

// AddItem adds a batch item to process.
func (fb *FlowBuilder) AddItem(input *model.RawDocument, outputPath string, targetLocale model.LocaleID) *FlowBuilder {
	fb.items = append(fb.items, &FlowItem{
		Input:        input,
		OutputPath:   outputPath,
		TargetLocale: targetLocale,
	})
	return fb
}

// Build constructs the Flow.
func (fb *FlowBuilder) Build() *Flow {
	return &Flow{
		Name:  fb.name,
		Tools: fb.tools,
	}
}

// Items returns the configured batch items.
func (fb *FlowBuilder) Items() []*FlowItem {
	return fb.items
}
