package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/internal/projflow"
	clioutput "github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/spf13/cobra"
)

// findProject wraps project.FindProject to return a simple error.
func findProject() (*project.Project, error) {
	return project.FindProject("")
}

// projectFlowFallback is called when a flow name doesn't match a built-in
// flow definition. It checks for a project flow in .kapi/flows/.
func projectFlowFallback(cmd *cobra.Command, flowName string, args []string) error {
	proj, err := findProject()
	if err != nil {
		return err
	}
	return runProjectFlow(cmd, proj, flowName, args)
}

// listProjectFlows returns flow info entries from .kapi/flows/.
func listProjectFlows() []clioutput.FlowInfo {
	return projflow.List()
}

// runProjectFlow executes a flow defined in .kapi/flows/.
func runProjectFlow(cmd *cobra.Command, proj *project.Project, flowName string, args []string) error {
	// Load flow definition
	flowPath := filepath.Join(proj.FlowsDirPath(), flowName+".yaml")

	flowDef, err := projflow.LoadDefinition(flowPath)
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
		t, err := toolReg.NewTool(registry.ToolID(step.Tool))
		if err != nil {
			return fmt.Errorf("tool %q: %w", step.Tool, err)
		}
		tools = append(tools, t)
	}

	// Build Items from step inputs or CLI args.
	var inputPaths []string
	for _, step := range flowDef.Steps {
		if step.Input != "" {
			inputPaths = append(inputPaths, filepath.Join(proj.Root, step.Input))
		}
	}
	if len(inputPaths) == 0 {
		inputPaths = args
	}

	var items []*flow.Item
	for _, p := range inputPaths {
		f, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("open input %q: %w", p, err)
		}
		items = append(items, &flow.Item{
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

	executor := flow.NewExecutor(flow.WithFailFast(true))
	fmt.Fprintf(cmd.OutOrStdout(), "Executing flow: %s (%d steps, %d items)\n", flowDef.Name, len(tools), len(items))
	if err := executor.Execute(cmd.Context(), fl, items); err != nil {
		return fmt.Errorf("flow execution failed: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Flow completed successfully.")

	return nil
}
