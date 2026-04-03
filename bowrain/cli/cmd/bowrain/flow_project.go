package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/bowrain/core/project"
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

// runProjectFlow executes a flow defined in .bowrain/flows/.
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

	// Build tool registry.
	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)

	// Convert FlowDefinition steps to tool.Tool chain.
	var tools []tool.Tool
	for _, step := range flowDef.Steps {
		t, err := toolReg.NewTool(step.Tool)
		if err != nil {
			return fmt.Errorf("tool %q: %w", step.Tool, err)
		}
		tools = append(tools, t)
	}

	// Build FlowItems from step inputs or CLI args.
	var inputPaths []string
	for _, step := range flowDef.Steps {
		if step.Input != "" {
			inputPaths = append(inputPaths, filepath.Join(proj.Root, step.Input))
		}
	}
	if len(inputPaths) == 0 {
		inputPaths = args
	}

	var items []*flow.FlowItem
	for _, p := range inputPaths {
		f, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("open input %q: %w", p, err)
		}
		items = append(items, &flow.FlowItem{
			Input: &model.RawDocument{
				URI:    p,
				Reader: f,
			},
			OutputPath: p,
		})
	}

	fl := &flow.Flow{
		Name:  flowDef.Name,
		Tools: tools,
	}

	executor := flow.NewFlowExecutor(flow.WithFailFast(true))
	fmt.Fprintf(cmd.OutOrStdout(), "Executing flow: %s (%d steps, %d items)\n", flowDef.Name, len(tools), len(items))
	if err := executor.Execute(cmd.Context(), fl, items); err != nil {
		return fmt.Errorf("flow execution failed: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Flow completed successfully.")

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
