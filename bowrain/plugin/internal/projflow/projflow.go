// Package projflow provides shared helpers for discovering project-defined
// flows in the .kapi/flows/ directory. Used by both the commands and mcp
// sub-packages so they can stay decoupled from each other.
package projflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/bowrain/core/project"
	clioutput "github.com/neokapi/neokapi/cli/output"
	"gopkg.in/yaml.v3"
)

// FlowDefinition mirrors the YAML shape of a flow file under .kapi/flows/.
type FlowDefinition struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Steps       []FlowStep `yaml:"steps"`
}

// FlowStep represents a step in a flow.
type FlowStep struct {
	Tool   string         `yaml:"tool"`
	Input  string         `yaml:"input,omitempty"`
	Output string         `yaml:"output,omitempty"`
	Config map[string]any `yaml:"config,omitempty"`
}

// LoadDefinition reads a flow YAML file from disk.
func LoadDefinition(path string) (*FlowDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read flow file: %w", err)
	}

	var flowDef FlowDefinition
	if err := yaml.Unmarshal(data, &flowDef); err != nil {
		return nil, fmt.Errorf("parse flow YAML: %w", err)
	}

	return &flowDef, nil
}

// List walks the project's .kapi/flows/ directory and returns the flow
// info entries discovered there. Returns nil when no project is found.
func List() []clioutput.FlowInfo {
	proj, err := project.FindProject("")
	if err != nil {
		return nil
	}

	flowsDir := proj.FlowsDirPath()
	entries, err := os.ReadDir(flowsDir)
	if err != nil {
		return nil
	}

	var flows []clioutput.FlowInfo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		flowPath := filepath.Join(flowsDir, e.Name())
		def, err := LoadDefinition(flowPath)
		if err != nil {
			continue
		}
		name := e.Name()[:len(e.Name())-5] // strip .yaml
		flows = append(flows, clioutput.FlowInfo{
			Name:        name,
			Description: def.Description,
			Path:        flowPath,
			Steps:       len(def.Steps),
		})
	}
	return flows
}
