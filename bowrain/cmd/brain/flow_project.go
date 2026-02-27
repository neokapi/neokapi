package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// FlowDefinition represents a flow YAML file.
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

// findProject wraps project.FindProject to return a simple error.
func findProject() (*project.Project, error) {
	return project.FindProject("")
}

// runProjectFlow executes a flow defined in .brain/flows/.
func runProjectFlow(cmd *cobra.Command, proj *project.Project, flowName string, args []string) error {
	// Load flow definition
	flowPath := filepath.Join(proj.FlowsDirPath(), flowName+".yaml")

	flowDef, err := loadFlowDefinition(flowPath)
	if err != nil {
		return fmt.Errorf("load flow %q: %w", flowName, err)
	}

	fmt.Printf("Executing flow: %s\n", flowDef.Name)
	fmt.Printf("Description: %s\n", flowDef.Description)
	fmt.Printf("Steps: %d\n\n", len(flowDef.Steps))

	// TODO: Implement full flow execution
	// For now, just show what would be executed
	for i, step := range flowDef.Steps {
		fmt.Printf("Step %d: %s\n", i+1, step.Tool)
		if step.Input != "" {
			fmt.Printf("  Input:  %s\n", step.Input)
		}
		if step.Output != "" {
			fmt.Printf("  Output: %s\n", step.Output)
		}
		if len(step.Config) > 0 {
			fmt.Printf("  Config: %v\n", step.Config)
		}
		fmt.Println()
	}

	fmt.Println("Flow execution: Not yet fully implemented")
	fmt.Println()
	fmt.Println("Full implementation requires:")
	fmt.Println("  - Tool registry lookup by name")
	fmt.Println("  - FormatRegistry integration")
	fmt.Println("  - Pipeline execution framework")
	fmt.Println("  - Error handling and rollback")

	return nil
}

// loadFlowDefinition loads a flow definition from a YAML file.
func loadFlowDefinition(path string) (*FlowDefinition, error) {
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
