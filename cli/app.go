package cli

import (
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// AddPersistentFlags registers global flags on the root command.
func AddPersistentFlags(a *App, cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&a.CfgFile, "config", "c", "", "config file path")
	cmd.PersistentFlags().BoolVarP(&a.Verbose, "verbose", "v", false, "verbose output")
	cmd.PersistentFlags().BoolVarP(&a.Quiet, "quiet", "q", false, "suppress output")
	cmd.PersistentFlags().BoolVarP(&a.AssumeYes, "yes", "y", false, "assume yes for confirmation prompts (e.g. plugin auto-install)")
	cmd.PersistentFlags().StringVar(&a.PluginDir, "plugin-dir", "", "plugin directory")
	cmd.PersistentFlags().StringVar(&a.Lang, "lang", "", "UI locale for tool/format/plugin metadata (BCP-47, e.g. fr); falls back to KAPI_LANG / LC_ALL / LANG")

	// --explain-prompts shows exactly what kapi sends to the LLM on your behalf:
	// the rendered prompt (including the voice profile and glossary it wove in),
	// the model, and the reply. Bare flag prints a transcript to stderr;
	// --explain-prompts=<path> writes the exchanges as JSON. Pair it with
	// `--provider demo` to preview the prompts with no API key and no spend.
	//
	// Distinct from the flow commands' --explain, which reports resolved
	// source → sink bindings and does not run.
	cmd.PersistentFlags().StringVar(&a.Explain, "explain-prompts", "",
		"show the prompts sent to the LLM: bare flag prints to stderr, --explain-prompts=<path> writes JSON")
	cmd.PersistentFlags().Lookup("explain-prompts").NoOptDefVal = host.ExplainStderr

	output.AddPersistentFlags(cmd.PersistentFlags())
}

// Porcelain comes first: "Work:" holds the everyday project verbs
// (init, add, up, status, apply, check), "Localization:" the flow-backed
// produce verbs (translate, pseudo-translate — guardrailed built-in flows,
// not raw tools), and "Assets:" the standing resources (tm, terms,
// brand, models, credentials). "Advanced:" collects the plumbing (run,
// flows, exec, tools, extract, merge, pack/unpack/info, inspect, stats,
// formats, plugin, config, hook, mcp). Standard commands (version, update,
// completion) stay ungrouped under cobra's "Additional Commands:". Raw
// registry tools render no root group at all — they live under `kapi exec`.
func AddCommandGroups(a *App, cmd *cobra.Command) {
	cmd.AddGroup(
		&cobra.Group{ID: "work", Title: "Work:"},
		&cobra.Group{ID: "translate", Title: "Translate:"},
		&cobra.Group{ID: "assets", Title: "Assets:"},
		&cobra.Group{ID: "advanced", Title: "Advanced:"},
	)
}
