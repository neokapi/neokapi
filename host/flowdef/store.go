package flowdef

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/flow"
	"gopkg.in/yaml.v3"
)

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
func (s *FlowStore) List() ([]flow.FlowDefinition, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read flow dir: %w", err)
	}
	var defs []flow.FlowDefinition
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
func (s *FlowStore) Get(id string) (*flow.FlowDefinition, error) {
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
func parseFlowFile(data []byte, filename string) (*flow.FlowDefinition, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	if ext == ".yaml" || ext == ".yml" {
		return parseFlowYAML(data)
	}
	// JSON: try envelope first, then bare
	return parseFlowJSON(data)
}

// parseFlowYAML parses a YAML flow file, supporting both envelope and bare formats.
// Detects steps-format (spec.steps) vs graph-format (spec.nodes + spec.edges).
func parseFlowYAML(data []byte) (*flow.FlowDefinition, error) {
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
	var def flow.FlowDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, err
	}
	return &def, nil
}

// parseFlowJSON parses a JSON flow file, supporting both envelope and bare formats.
func parseFlowJSON(data []byte) (*flow.FlowDefinition, error) {
	// Probe for envelope
	var probe struct {
		APIVersion string `json:"apiVersion"`
	}
	_ = json.Unmarshal(data, &probe)

	if probe.APIVersion != "" {
		return parseEnvelopedFlow(data, ".json")
	}

	// Bare JSON flow
	var def flow.FlowDefinition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, err
	}
	return &def, nil
}

// parseEnvelopedFlow parses a flow from an envelope, extracting the spec.
// Supports both the graph format (nodes + edges) and the steps format.
func parseEnvelopedFlow(data []byte, ext string) (*flow.FlowDefinition, error) {
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
	var def flow.FlowDefinition
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
func parseStepsFromSpec(env *config.Envelope) (*flow.FlowDefinition, error) {
	specData, err := yaml.Marshal(env.Spec)
	if err != nil {
		return nil, err
	}
	var spec flow.StepsSpec
	if err := yaml.Unmarshal(specData, &spec); err != nil {
		return nil, fmt.Errorf("parse steps spec: %w", err)
	}

	nodes, edges, err := flow.StepsToGraph(&spec)
	if err != nil {
		return nil, fmt.Errorf("compile steps: %w", err)
	}

	def := &flow.FlowDefinition{
		Name:    env.Metadata.Name,
		Nodes:   nodes,
		Edges:   edges,
		Binding: bindingFromSpec(&spec),
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
func parseStepsFromBare(data []byte) (*flow.FlowDefinition, error) {
	var spec flow.StepsSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	if len(spec.Steps) == 0 {
		return nil, errors.New("no steps found")
	}

	nodes, edges, err := flow.StepsToGraph(&spec)
	if err != nil {
		return nil, err
	}

	return &flow.FlowDefinition{
		Nodes:   nodes,
		Edges:   edges,
		Binding: bindingFromSpec(&spec),
	}, nil
}

// bindingFromSpec builds a FlowBinding from a steps spec's source/sink, or nil
// when the flow declares neither (a binding-agnostic flow).
func bindingFromSpec(spec *flow.StepsSpec) *flow.FlowBinding {
	if spec.Source == "" && spec.Sink == "" {
		return nil
	}
	return &flow.FlowBinding{Source: spec.Source, Sink: spec.Sink}
}

// Save writes a flow definition to the store.
func (s *FlowStore) Save(def *flow.FlowDefinition) error {
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
