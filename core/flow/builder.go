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

// AddSourceTransform appends a tool to the flow's leading source-transform
// ("settle") stage — tools that settle the source/model (redaction,
// simplification, normalization) before the main tools run. The stage may also
// hold annotators (e.g. ai-entity-extract) ahead of the transform so their
// overlays can drive it; Build rejects a Translate tool here and requires the
// stage to contain at least one source-transform (tool.CapTransform).
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
	if err := validateSettleStage(fb.name, fb.sourceTransforms); err != nil {
		return nil, err
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

// validateSettleStage checks the leading source-transform ("settle") stage:
// every tool must be Annotate- or Transform-capable (a Translate tool is
// rejected — translation belongs in the main stage, after the source settles),
// and a non-empty stage must contain at least one source-transform
// (tool.CapTransform), the step that actually settles the source. Annotators may
// precede the transform so their overlays drive it (AD-006).
func validateSettleStage(flowName string, stage []tool.Tool) error {
	if len(stage) == 0 {
		return nil
	}
	hasTransform := false
	for _, t := range stage {
		if !tool.CanPrecedeSettle(t) {
			return fmt.Errorf("flow %q: tool %q in the source-transform stage must be an annotator or a source-transform (it may not write targets)", flowName, t.Name())
		}
		if tool.IsSourceTransform(t) {
			hasTransform = true
		}
	}
	if !hasTransform {
		return fmt.Errorf("flow %q: the source-transform stage must contain at least one source-transform tool to settle the source", flowName)
	}
	return nil
}
