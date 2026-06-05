package flow

import (
	"errors"
	"fmt"
)

// FlowStep represents a single step in the human-authored steps format.
// Steps are sequential by default. Use Parallel for fan-out branches.
type FlowStep struct {
	Tool     string         `json:"tool,omitempty" yaml:"tool,omitempty"`
	Config   map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
	Label    string         `json:"label,omitempty" yaml:"label,omitempty"`
	Parallel []FlowStep     `json:"parallel,omitempty" yaml:"parallel,omitempty"`
}

// StepsSpec is the steps-based flow format that humans author.
// It compiles to a FlowDefinition (nodes + edges) for execution.
type StepsSpec struct {
	// Source and Sink declare the flow's I/O bindings (AD-026). They hold a
	// binding locator — a scheme such as "store" or "none", optionally with a
	// path. A flow sets these only when a binding is intrinsic to what it is
	// (e.g. Sink "none" for an analysis flow); otherwise the binding is supplied
	// at invocation (the -i/-o flags, the project, or auto-detection). The flow
	// never names a concrete file path here — locations come from the run.
	Source string `json:"source,omitempty" yaml:"source,omitempty"`
	Sink   string `json:"sink,omitempty" yaml:"sink,omitempty"`

	// Input and Output name the reader/writer formats for the file binding. They
	// are the format-hint shorthand for `source`/`sink` over a file; an explicit
	// Source/Sink takes precedence.
	Input  string `json:"input,omitempty" yaml:"input,omitempty"`
	Output string `json:"output,omitempty" yaml:"output,omitempty"`
	// SourceTransforms is the leading source-transform stage: tools that settle
	// the source/model (redaction, simplification, normalization) before the
	// main steps run. Each must be a source-transform-capable tool. See AD-006.
	SourceTransforms []FlowStep `json:"sourceTransforms,omitempty" yaml:"source_transforms,omitempty"`
	Steps            []FlowStep `json:"steps" yaml:"steps"`
}

// SourceLocator returns the flow's declared source binding and true when the
// flow names one; false means the source is supplied at invocation.
func (s *StepsSpec) SourceLocator() (Locator, bool) {
	if s.Source == "" {
		return Locator{}, false
	}
	return ParseLocator(s.Source), true
}

// SinkLocator returns the flow's declared sink binding and true when the flow
// names one; false means the sink is supplied at invocation.
func (s *StepsSpec) SinkLocator() (Locator, bool) {
	if s.Sink == "" {
		return Locator{}, false
	}
	return ParseLocator(s.Sink), true
}

// StepsToGraph compiles a steps-based spec into FlowDefinition nodes and edges.
// It auto-generates reader/writer nodes and chains steps sequentially,
// with fan-out for parallel blocks.
func StepsToGraph(spec *StepsSpec) ([]FlowNode, []FlowEdge, error) {
	if len(spec.Steps) == 0 && len(spec.SourceTransforms) == 0 {
		return nil, nil, errors.New("flow has no steps")
	}

	inputFormat := spec.Input
	if inputFormat == "" {
		inputFormat = "auto"
	}
	outputFormat := spec.Output
	if outputFormat == "" {
		outputFormat = "auto"
	}

	var nodes []FlowNode
	var edges []FlowEdge
	nodeCounter := 0

	nextID := func(prefix string) string {
		nodeCounter++
		return fmt.Sprintf("%s-%d", prefix, nodeCounter)
	}

	// Reader node
	readerID := "reader"
	nodes = append(nodes, FlowNode{
		ID:       readerID,
		Type:     NodeReader,
		Name:     inputFormat,
		Label:    "Input",
		Position: NodePosition{X: 0, Y: 100},
	})

	prevIDs := []string{readerID}
	xPos := 250.0

	// Source-transform stage: sequential tools that settle the source/model,
	// emitted between the reader and the main steps and marked with the stage.
	for _, step := range spec.SourceTransforms {
		id := nextID("tool")
		label := step.Label
		if label == "" {
			label = step.Tool
		}
		nodes = append(nodes, FlowNode{
			ID:       id,
			Type:     NodeTool,
			Name:     step.Tool,
			Label:    label,
			Stage:    StageSourceTransform,
			Config:   step.Config,
			Position: NodePosition{X: xPos, Y: 100},
		})
		for _, prev := range prevIDs {
			edges = append(edges, FlowEdge{
				ID:     fmt.Sprintf("e-%s-%s", prev, id),
				Source: prev,
				Target: id,
			})
		}
		prevIDs = []string{id}
		xPos += 250
	}

	for _, step := range spec.Steps {
		if len(step.Parallel) > 0 {
			// Fan-out: create parallel branches
			var branchEndIDs []string
			yPos := 0.0
			for _, branch := range step.Parallel {
				id := nextID("tool")
				label := branch.Label
				if label == "" {
					label = branch.Tool
				}
				nodes = append(nodes, FlowNode{
					ID:       id,
					Type:     NodeTool,
					Name:     branch.Tool,
					Label:    label,
					Config:   branch.Config,
					Position: NodePosition{X: xPos, Y: yPos},
				})
				// Each branch connects from all prev nodes
				for _, prev := range prevIDs {
					edges = append(edges, FlowEdge{
						ID:     fmt.Sprintf("e-%s-%s", prev, id),
						Source: prev,
						Target: id,
					})
				}
				branchEndIDs = append(branchEndIDs, id)
				yPos += 150
			}
			prevIDs = branchEndIDs
			xPos += 250
		} else {
			// Sequential step
			id := nextID("tool")
			label := step.Label
			if label == "" {
				label = step.Tool
			}
			nodes = append(nodes, FlowNode{
				ID:       id,
				Type:     NodeTool,
				Name:     step.Tool,
				Label:    label,
				Config:   step.Config,
				Position: NodePosition{X: xPos, Y: 100},
			})
			for _, prev := range prevIDs {
				edges = append(edges, FlowEdge{
					ID:     fmt.Sprintf("e-%s-%s", prev, id),
					Source: prev,
					Target: id,
				})
			}
			prevIDs = []string{id}
			xPos += 250
		}
	}

	// Writer node
	writerID := "writer"
	nodes = append(nodes, FlowNode{
		ID:       writerID,
		Type:     NodeWriter,
		Name:     outputFormat,
		Label:    "Output",
		Position: NodePosition{X: xPos, Y: 100},
	})
	for _, prev := range prevIDs {
		edges = append(edges, FlowEdge{
			ID:     fmt.Sprintf("e-%s-%s", prev, writerID),
			Source: prev,
			Target: writerID,
		})
	}

	return nodes, edges, nil
}
