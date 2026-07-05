package host

import (
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/output"
)

func (a *App) ListTools(cmd Command) error {
	if a.ToolReg == nil {
		return output.Print(cmd, output.ToolsListOutput{})
	}

	t := a.T()
	var tools []output.ToolInfo
	for _, entry := range a.ToolReg.CLITools() {
		source := entry.Info.Source
		if source == registry.SourceBuiltIn {
			source = "builtin"
		}
		name := string(entry.Info.Name)
		scope := "tools." + name
		DisplayName := t.T(i18n.Scope(scope+".DisplayName"), entry.Info.DisplayName)
		desc := t.T(i18n.Scope(scope+".description"), entry.Info.Description)
		if desc == "" {
			desc = DisplayName
		}
		tools = append(tools, output.ToolInfo{
			Name:        name,
			Description: desc,
			Category:    entry.Info.Category,
			Source:      source,
		})
	}

	out := output.ToolsListOutput{
		Tools: tools,
		Total: len(tools),
	}
	return output.Print(cmd, out)
}

func (a *App) ToolSchema(_ Command, name string) error {
	if a.ToolReg != nil {
		if s := a.ToolReg.Schema(registry.ToolID(name)); s != nil {
			return printSchema(s)
		}
	}
	return fmt.Errorf("no schema found for tool %q", name)
}

func printSchema(s any) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
