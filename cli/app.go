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
	cmd.PersistentFlags().StringVar(&a.Lang, "lang", "", "UI language for tool/format/plugin metadata (BCP-47, e.g. fr); falls back to KAPI_LANG / LC_ALL / LANG")

	// --explain-prompts shows exactly what kapi sends to the LLM on your behalf:
	// the rendered prompt (including the voice profile and terms it wove in),
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

// AddCommandGroups registers the root command groups. Each group belongs to a
// tier of the root help (helptiers.go):
//
//   - Person, always shown: "Work:" (init, add, ls, rm, up, status, apply,
//     check, context) and "Assets:" (terms, voice, models, credentials).
//   - Languages, shown when the project declares a target language and in
//     `kapi help --all` and `kapi help languages`: "Languages:" (translate,
//     pseudo-translate, and memory, the content memory they reuse).
//   - Engine, shown only in `kapi help --all`: "Advanced:" (run, flows, exec,
//     tools, extract, merge, pack, unpack, info, resolve, inspect, stats,
//     formats, store, plugin, config, trust, mcp, completion, telemetry).
//
// The tiers filter the help page only. Every command runs, completes and
// appears in the reference whatever its tier. help, version and update stay
// ungrouped under "Additional Commands:", which every view shows. Registry
// tools have no root group: they live under `kapi exec`.
func AddCommandGroups(a *App, cmd *cobra.Command) {
	cmd.AddGroup(
		&cobra.Group{ID: "work", Title: "Work:"},
		&cobra.Group{ID: "languages", Title: "Languages:"},
		&cobra.Group{ID: "assets", Title: "Assets:"},
		&cobra.Group{ID: "advanced", Title: "Advanced:"},
	)
}
