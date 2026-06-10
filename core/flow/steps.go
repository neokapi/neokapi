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

	// SourceTransforms is the retired source-transform stage (AD-006 removed
	// it; transformers are ordinary ordered steps). The field exists only so a
	// flow that still uses it gets an actionable error from StepsToGraph
	// instead of a silently dropped stage.
	SourceTransforms []FlowStep `json:"sourceTransforms,omitempty" yaml:"source_transforms,omitempty"`

	Steps []FlowStep `json:"steps" yaml:"steps"`
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
// The graph is tool nodes only (AD-026): steps chained sequentially, with
// fan-out for parallel blocks. A flow's I/O ends are bindings resolved at run
// time, not nodes — so no reader or writer node is emitted, and the first tool
// has no incoming edge.
func StepsToGraph(spec *StepsSpec) ([]FlowNode, []FlowEdge, error) {
	if len(spec.SourceTransforms) > 0 {
		return nil, nil, errors.New("flow uses the removed source_transforms stage (AD-006): list transformers as ordered steps — the placement pass validates their position")
	}
	if len(spec.Steps) == 0 {
		return nil, nil, errors.New("flow has no steps")
	}

	var nodes []FlowNode
	var edges []FlowEdge
	nodeCounter := 0

	nextID := func(prefix string) string {
		nodeCounter++
		return fmt.Sprintf("%s-%d", prefix, nodeCounter)
	}

	// prevIDs is the set of node IDs the next tool connects from; empty for the
	// first tool (it is a graph root — content arrives via the source binding).
	var prevIDs []string

	addTool := func(step FlowStep, x, y float64) string {
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
			Position: NodePosition{X: x, Y: y},
		})
		for _, prev := range prevIDs {
			edges = append(edges, FlowEdge{
				ID:     fmt.Sprintf("e-%s-%s", prev, id),
				Source: prev,
				Target: id,
			})
		}
		return id
	}

	xPos := 0.0

	for _, step := range spec.Steps {
		if len(step.Parallel) > 0 {
			// Fan-out: each branch connects from the same predecessors.
			var branchEndIDs []string
			yPos := 0.0
			for _, branch := range step.Parallel {
				id := addTool(branch, xPos, yPos)
				branchEndIDs = append(branchEndIDs, id)
				yPos += 150
			}
			prevIDs = branchEndIDs
			xPos += 250
		} else {
			id := addTool(step, xPos, 100)
			prevIDs = []string{id}
			xPos += 250
		}
	}

	return nodes, edges, nil
}
