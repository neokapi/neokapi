package cli

import (
	"github.com/neokapi/neokapi/cli/output"
	"github.com/spf13/cobra"
)

// NewFlowsCmd creates the "flows" management command (list flows).
// Bare invocation lists flows; "flows list" is an explicit alias.
func (a *App) NewFlowsCmd(opts FlowCmdOptions) *cobra.Command {
	flowsCmd := &cobra.Command{
		Use:   "flows",
		Short: "List available flows",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.listFlows(cmd, opts)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available flows",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.listFlows(cmd, opts)
		},
	}

	flowsCmd.AddCommand(listCmd)
	return flowsCmd
}

// AllBuiltinFlowInfos returns flow info for all built-in flows (composed + single-tool).
// Used by the deprecated "flow list" command for backward compatibility.
func AllBuiltinFlowInfos() []output.FlowInfo {
	return []output.FlowInfo{
		{Name: "ai-translate", Description: "Translate content using AI/LLM"},
		{Name: "ai-translate-qa", Description: "Translate + quality check using AI/LLM"},
		{Name: "pseudo-translate", Description: "Generate pseudo-translations for testing"},
		{Name: "qa-check", Description: "Run rule-based quality checks on translations"},
		{Name: "tm-leverage", Description: "Pre-fill translations from translation memory"},
		{Name: "segmentation", Description: "Split source text into sentence segments"},
	}
}
