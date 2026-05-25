package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/set"
	"gopkg.in/yaml.v3"
)

// Sentinel errors for flow definition validation.
var (
	ErrFlowIDRequired   = errors.New("flow definition id is required")
	ErrFlowNameRequired = errors.New("flow definition name is required")
	ErrNodeIDRequired   = errors.New("node id is required")
)

// NodeType identifies the role of a node in a flow graph.
type NodeType string

const (
	NodeReader NodeType = "reader"
	NodeWriter NodeType = "writer"
	NodeTool   NodeType = "tool"
)

// FlowStage names the pipeline stage a tool node belongs to.
type FlowStage string

const (
	// StageMain is the default stage — the main tool chain.
	StageMain FlowStage = ""
	// StageSourceTransform marks a tool that settles the source/model
	// (redaction, simplification, normalization) ahead of the main tools, so
	// downstream tools see one canonical source. Only source-transform-capable
	// tools (tool.CapTransform) may sit here. See AD-006.
	StageSourceTransform FlowStage = "source-transform"
)

// FlowDefinition is a JSON-serializable flow that can be stored and loaded.
// It captures the visual graph (nodes + edges) as well as the tool configurations
// needed to reconstruct a runnable Flow.
type FlowDefinition struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Nodes       []FlowNode `json:"nodes"`
	Edges       []FlowEdge `json:"edges"`
	Source      string     `json:"source"` // "built-in", "user", or "project"
	CreatedAt   string     `json:"created_at,omitempty"`
	ModifiedAt  string     `json:"modified_at,omitempty"`
}

// FlowNode represents a node in the flow graph.
type FlowNode struct {
	ID    string   `json:"id"`
	Type  NodeType `json:"type"` // NodeReader, NodeWriter, or NodeTool
	Name  string   `json:"name"` // tool or format name
	Label string   `json:"label,omitempty"`
	// Stage assigns a tool node to a pipeline stage. Empty (StageMain) is the
	// main chain; StageSourceTransform runs ahead of it. Ignored for reader/
	// writer nodes.
	Stage    FlowStage      `json:"stage,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	Position NodePosition   `json:"position"`
}

// NodePosition holds the x/y coordinates of a node in the visual graph.
type NodePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// FlowEdge represents a directed edge between two nodes.
type FlowEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// Validate checks that the flow definition is well-formed.
func (d *FlowDefinition) Validate() error {
	if d.ID == "" {
		return ErrFlowIDRequired
	}
	if d.Name == "" {
		return ErrFlowNameRequired
	}
	nodeIDs := set.New[string]()
	for _, n := range d.Nodes {
		if n.ID == "" {
			return ErrNodeIDRequired
		}
		if nodeIDs.Contains(n.ID) {
			return fmt.Errorf("duplicate node id: %s", n.ID)
		}
		nodeIDs.Add(n.ID)
		switch n.Type {
		case NodeTool, NodeReader, NodeWriter:
		default:
			return fmt.Errorf("invalid node type %q for node %s", n.Type, n.ID)
		}
		switch n.Stage {
		case StageMain, StageSourceTransform:
		default:
			return fmt.Errorf("invalid stage %q for node %s", n.Stage, n.ID)
		}
		if n.Stage == StageSourceTransform && n.Type != NodeTool {
			return fmt.Errorf("node %s: only tool nodes may be in the %s stage", n.ID, StageSourceTransform)
		}
	}
	for _, e := range d.Edges {
		if !nodeIDs.Contains(e.Source) {
			return fmt.Errorf("edge source %q not found in nodes", e.Source)
		}
		if !nodeIDs.Contains(e.Target) {
			return fmt.Errorf("edge target %q not found in nodes", e.Target)
		}
	}
	return nil
}

// TopologicalOrder returns node IDs in execution order following edges from
// sources to sinks. Returns an error if a cycle is detected.
func (d *FlowDefinition) TopologicalOrder() ([]string, error) {
	adj := make(map[string][]string, len(d.Nodes))
	inDeg := make(map[string]int, len(d.Nodes))
	for _, n := range d.Nodes {
		inDeg[n.ID] = 0
	}
	for _, e := range d.Edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		inDeg[e.Target]++
	}
	queue := make([]string, 0, len(d.Nodes))
	for _, n := range d.Nodes {
		if inDeg[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	order := make([]string, 0, len(d.Nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)
		for _, next := range adj[id] {
			inDeg[next]--
			if inDeg[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if len(order) != len(d.Nodes) {
		return nil, errors.New("cycle detected in flow graph")
	}
	return order, nil
}

// ToolNodeNames returns the names of all tool-type nodes in topological order.
func (d *FlowDefinition) ToolNodeNames() ([]string, error) {
	order, err := d.TopologicalOrder()
	if err != nil {
		return nil, err
	}
	nodeMap := make(map[string]*FlowNode, len(d.Nodes))
	for i := range d.Nodes {
		nodeMap[d.Nodes[i].ID] = &d.Nodes[i]
	}
	var names []string
	for _, id := range order {
		n := nodeMap[id]
		if n.Type == NodeTool {
			names = append(names, n.Name)
		}
	}
	return names, nil
}

// StagedToolNodes returns tool node names split by stage, each in topological
// order: the source-transform stage first, then the main tools. The source
// transforms are also returned interleaved at the front of the combined order,
// which the executor relies on (Parts stream through tools in order).
func (d *FlowDefinition) StagedToolNodes() (sourceTransforms, main []string, err error) {
	order, err := d.TopologicalOrder()
	if err != nil {
		return nil, nil, err
	}
	nodeMap := make(map[string]*FlowNode, len(d.Nodes))
	for i := range d.Nodes {
		nodeMap[d.Nodes[i].ID] = &d.Nodes[i]
	}
	for _, id := range order {
		n := nodeMap[id]
		if n.Type != NodeTool {
			continue
		}
		if n.Stage == StageSourceTransform {
			sourceTransforms = append(sourceTransforms, n.Name)
		} else {
			main = append(main, n.Name)
		}
	}
	return sourceTransforms, main, nil
}

// BuiltInFlows returns the default set of built-in flow definitions.
func BuiltInFlows() []FlowDefinition {
	return []FlowDefinition{
		{
			ID:          "ai-translate",
			Name:        "AI Translate",
			Description: "Translate content using AI/LLM",
			Source:      registry.SourceBuiltIn,
			Nodes: []FlowNode{
				{ID: "reader", Type: NodeReader, Name: "auto", Label: "Input", Position: NodePosition{X: 0, Y: 100}},
				{ID: "ai-translate", Type: NodeTool, Name: "ai-translate", Label: "AI Translate", Position: NodePosition{X: 250, Y: 100}},
				{ID: "writer", Type: NodeWriter, Name: "auto", Label: "Output", Position: NodePosition{X: 500, Y: 100}},
			},
			Edges: []FlowEdge{
				{ID: "e-reader-translate", Source: "reader", Target: "ai-translate"},
				{ID: "e-translate-writer", Source: "ai-translate", Target: "writer"},
			},
		},
		{
			ID:          "ai-translate-qa",
			Name:        "AI Translate + QA",
			Description: "Translate content using AI/LLM then run quality check",
			Source:      registry.SourceBuiltIn,
			Nodes: []FlowNode{
				{ID: "reader", Type: NodeReader, Name: "auto", Label: "Input", Position: NodePosition{X: 0, Y: 100}},
				{ID: "ai-translate", Type: NodeTool, Name: "ai-translate", Label: "AI Translate", Position: NodePosition{X: 250, Y: 100}},
				{ID: "ai-qa", Type: NodeTool, Name: "ai-qa", Label: "QA Check", Position: NodePosition{X: 500, Y: 100}},
				{ID: "writer", Type: NodeWriter, Name: "auto", Label: "Output", Position: NodePosition{X: 750, Y: 100}},
			},
			Edges: []FlowEdge{
				{ID: "e-reader-translate", Source: "reader", Target: "ai-translate"},
				{ID: "e-translate-qa", Source: "ai-translate", Target: "ai-qa"},
				{ID: "e-qa-writer", Source: "ai-qa", Target: "writer"},
			},
		},
		{
			ID:          "pseudo-translate",
			Name:        "Pseudo Translate",
			Description: "Generate pseudo-translations for testing",
			Source:      registry.SourceBuiltIn,
			Nodes: []FlowNode{
				{ID: "reader", Type: NodeReader, Name: "auto", Label: "Input", Position: NodePosition{X: 0, Y: 100}},
				{ID: "pseudo-translate", Type: NodeTool, Name: "pseudo-translate", Label: "Pseudo Translate", Position: NodePosition{X: 250, Y: 100}},
				{ID: "writer", Type: NodeWriter, Name: "auto", Label: "Output", Position: NodePosition{X: 500, Y: 100}},
			},
			Edges: []FlowEdge{
				{ID: "e-reader-pseudo", Source: "reader", Target: "pseudo-translate"},
				{ID: "e-pseudo-writer", Source: "pseudo-translate", Target: "writer"},
			},
		},
		{
			ID:          "qa-check",
			Name:        "QA Check",
			Description: "Run rule-based quality checks on translations",
			Source:      registry.SourceBuiltIn,
			Nodes: []FlowNode{
				{ID: "reader", Type: NodeReader, Name: "auto", Label: "Input", Position: NodePosition{X: 0, Y: 100}},
				{ID: "qa-check", Type: NodeTool, Name: "qa-check", Label: "QA Check", Position: NodePosition{X: 250, Y: 100}},
				{ID: "writer", Type: NodeWriter, Name: "auto", Label: "Output", Position: NodePosition{X: 500, Y: 100}},
			},
			Edges: []FlowEdge{
				{ID: "e-reader-qa", Source: "reader", Target: "qa-check"},
				{ID: "e-qa-writer", Source: "qa-check", Target: "writer"},
			},
		},
		{
			ID:          "tm-leverage",
			Name:        "TM Leverage",
			Description: "Pre-fill translations from translation memory",
			Source:      registry.SourceBuiltIn,
			Nodes: []FlowNode{
				{ID: "reader", Type: NodeReader, Name: "auto", Label: "Input", Position: NodePosition{X: 0, Y: 100}},
				{ID: "tm-leverage", Type: NodeTool, Name: "tm-leverage", Label: "TM Leverage", Position: NodePosition{X: 250, Y: 100}},
				{ID: "writer", Type: NodeWriter, Name: "auto", Label: "Output", Position: NodePosition{X: 500, Y: 100}},
			},
			Edges: []FlowEdge{
				{ID: "e-reader-tm", Source: "reader", Target: "tm-leverage"},
				{ID: "e-tm-writer", Source: "tm-leverage", Target: "writer"},
			},
		},
		{
			ID:          "secure-translate",
			Name:        "Secure Translate",
			Description: "Redact sensitive content, AI-translate, then restore the originals locally",
			Source:      registry.SourceBuiltIn,
			Nodes: []FlowNode{
				{ID: "reader", Type: NodeReader, Name: "auto", Label: "Input", Position: NodePosition{X: 0, Y: 100}},
				{ID: "redact", Type: NodeTool, Name: "redact", Label: "Redact", Stage: StageSourceTransform, Position: NodePosition{X: 250, Y: 100}},
				{ID: "ai-translate", Type: NodeTool, Name: "ai-translate", Label: "AI Translate", Position: NodePosition{X: 500, Y: 100}},
				{ID: "unredact", Type: NodeTool, Name: "unredact", Label: "Unredact", Position: NodePosition{X: 750, Y: 100}},
				{ID: "writer", Type: NodeWriter, Name: "auto", Label: "Output", Position: NodePosition{X: 1000, Y: 100}},
			},
			Edges: []FlowEdge{
				{ID: "e-reader-redact", Source: "reader", Target: "redact"},
				{ID: "e-redact-translate", Source: "redact", Target: "ai-translate"},
				{ID: "e-translate-unredact", Source: "ai-translate", Target: "unredact"},
				{ID: "e-unredact-writer", Source: "unredact", Target: "writer"},
			},
		},
	}
}

// FlowStore manages persistent storage of user flow definitions.
type FlowStore struct {
	dir string
}

// NewFlowStore creates a FlowStore that reads/writes JSON files from the given directory.
func NewFlowStore(dir string) *FlowStore {
	return &FlowStore{dir: dir}
}

// FlowDefinitionAPIVersion is the apiVersion for flow definition envelopes.
const FlowDefinitionAPIVersion = "v1"

// List returns all user flow definitions in the store.
// Supports both JSON (.json) and YAML (.yaml/.yml) files, with or without envelope.
func (s *FlowStore) List() ([]FlowDefinition, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read flow dir: %w", err)
	}
	var defs []FlowDefinition
	for _, e := range entries {
		if e.IsDir() || !isFlowFile(e.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		def, err := parseFlowFile(data, e.Name())
		if err != nil {
			continue
		}
		def.Source = "user"
		defs = append(defs, *def)
	}
	return defs, nil
}

// Get returns a specific flow definition by ID.
// Tries .yaml, .yml, and .json extensions in order.
func (s *FlowStore) Get(id string) (*FlowDefinition, error) {
	for _, ext := range []string{".yaml", ".yml", ".json"} {
		path := filepath.Join(s.dir, id+ext)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		def, err := parseFlowFile(data, id+ext)
		if err != nil {
			return nil, fmt.Errorf("parse flow %q: %w", id, err)
		}
		def.Source = "user"
		return def, nil
	}
	return nil, fmt.Errorf("flow %q not found", id)
}

// isFlowFile reports whether the filename has a supported flow file extension.
func isFlowFile(name string) bool {
	return strings.HasSuffix(name, ".json") ||
		strings.HasSuffix(name, ".yaml") ||
		strings.HasSuffix(name, ".yml")
}

// parseFlowFile parses a flow definition from data, detecting format and envelope.
func parseFlowFile(data []byte, filename string) (*FlowDefinition, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	if ext == ".yaml" || ext == ".yml" {
		return parseFlowYAML(data)
	}
	// JSON: try envelope first, then bare
	return parseFlowJSON(data)
}

// parseFlowYAML parses a YAML flow file, supporting both envelope and bare formats.
// Detects steps-format (spec.steps) vs graph-format (spec.nodes + spec.edges).
func parseFlowYAML(data []byte) (*FlowDefinition, error) {
	// Probe for envelope
	var probe struct {
		APIVersion string `yaml:"apiVersion"`
	}
	_ = yaml.Unmarshal(data, &probe)

	if probe.APIVersion != "" {
		return parseEnvelopedFlow(data, ".yaml")
	}

	// Probe for bare steps format
	var stepsProbe struct {
		Steps []any `yaml:"steps"`
	}
	_ = yaml.Unmarshal(data, &stepsProbe)

	if len(stepsProbe.Steps) > 0 {
		return parseStepsFromBare(data)
	}

	// Bare YAML graph flow
	var def FlowDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, err
	}
	return &def, nil
}

// parseFlowJSON parses a JSON flow file, supporting both envelope and bare formats.
func parseFlowJSON(data []byte) (*FlowDefinition, error) {
	// Probe for envelope
	var probe struct {
		APIVersion string `json:"apiVersion"`
	}
	_ = json.Unmarshal(data, &probe)

	if probe.APIVersion != "" {
		return parseEnvelopedFlow(data, ".json")
	}

	// Bare JSON flow
	var def FlowDefinition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, err
	}
	return &def, nil
}

// parseEnvelopedFlow parses a flow from an envelope, extracting the spec.
// Supports both the graph format (nodes + edges) and the steps format.
func parseEnvelopedFlow(data []byte, ext string) (*FlowDefinition, error) {
	env, err := config.Parse(data, ext)
	if err != nil {
		return nil, fmt.Errorf("parse envelope: %w", err)
	}

	if env.Kind != config.KindFlowDefinition {
		return nil, fmt.Errorf("expected kind %q, got %q", config.KindFlowDefinition, env.Kind)
	}

	if err := config.DefaultMigrations.Upgrade(env); err != nil {
		return nil, fmt.Errorf("migrate flow: %w", err)
	}

	// Check if spec uses the steps format
	if _, hasSteps := env.Spec["steps"]; hasSteps {
		return parseStepsFromSpec(env)
	}

	// Re-marshal the spec and unmarshal into FlowDefinition
	specData, err := yaml.Marshal(env.Spec)
	if err != nil {
		return nil, err
	}
	var def FlowDefinition
	if err := yaml.Unmarshal(specData, &def); err != nil {
		return nil, err
	}

	// Use envelope metadata as fallback for flow fields
	if def.Name == "" && env.Metadata.Name != "" {
		def.Name = env.Metadata.Name
	}
	if def.Description == "" && env.Metadata.Description != "" {
		def.Description = env.Metadata.Description
	}

	return &def, nil
}

// parseStepsFromSpec compiles a steps-format spec into a FlowDefinition.
func parseStepsFromSpec(env *config.Envelope) (*FlowDefinition, error) {
	specData, err := yaml.Marshal(env.Spec)
	if err != nil {
		return nil, err
	}
	var spec StepsSpec
	if err := yaml.Unmarshal(specData, &spec); err != nil {
		return nil, fmt.Errorf("parse steps spec: %w", err)
	}

	nodes, edges, err := StepsToGraph(&spec)
	if err != nil {
		return nil, fmt.Errorf("compile steps: %w", err)
	}

	def := &FlowDefinition{
		Name:  env.Metadata.Name,
		Nodes: nodes,
		Edges: edges,
	}
	if env.Metadata.Description != "" {
		def.Description = env.Metadata.Description
	}
	// Derive ID from name
	if def.Name != "" {
		def.ID = strings.ToLower(strings.ReplaceAll(def.Name, " ", "-"))
	}

	return def, nil
}

// parseStepsFromBare compiles a bare steps-format YAML into a FlowDefinition.
func parseStepsFromBare(data []byte) (*FlowDefinition, error) {
	var spec StepsSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	if len(spec.Steps) == 0 {
		return nil, errors.New("no steps found")
	}

	nodes, edges, err := StepsToGraph(&spec)
	if err != nil {
		return nil, err
	}

	return &FlowDefinition{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// Save writes a flow definition to the store.
func (s *FlowStore) Save(def *FlowDefinition) error {
	if err := def.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create flow dir: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if def.CreatedAt == "" {
		def.CreatedAt = now
	}
	def.ModifiedAt = now
	def.Source = "user"

	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal flow: %w", err)
	}
	path := filepath.Join(s.dir, def.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// Delete removes a flow definition from the store.
func (s *FlowStore) Delete(id string) error {
	path := filepath.Join(s.dir, id+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("flow %q not found", id)
		}
		return err
	}
	return nil
}
