package cli

import (
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
	cmd.PersistentFlags().StringVar(&a.Lang, "lang", "", "UI locale for tool/format/plugin metadata (BCP-47, e.g. fr-FR); falls back to KAPI_LANG / LC_ALL / LANG")
	output.AddPersistentFlags(cmd.PersistentFlags())
}

// Porcelain comes first: "Work:" holds the everyday project verbs
// (init, add, up, status, apply, check) and "Assets:" the standing resources
// (tm, termbase, brand, models, credentials). The tool-category groups
// (localization/quality/analysis/text-processing) remain for the top-level
// tool commands, whose GroupID derives from each tool's schema Category (see
// the routing in NewToolCommands). "Advanced:" collects the plumbing (run,
// flows, tools, extract, merge, pack/unpack/info, inspect, stats, formats,
// plugin, config, hook, mcp, registry). Standard commands (version, update,
// completion) stay ungrouped under cobra's "Additional Commands:".
func AddCommandGroups(a *App, cmd *cobra.Command) {
	cmd.AddGroup(
		&cobra.Group{ID: "work", Title: "Work:"},
		&cobra.Group{ID: "assets", Title: "Assets:"},
		// The localization toolchain (translate, recycle, the bilingual
		// checks, pseudo-translate) groups here regardless of each tool's
		// schema Category — see schema.TagL10n and the routing in
		// NewToolCommands.
		&cobra.Group{ID: "localization", Title: "Localization:"},
		&cobra.Group{ID: "quality", Title: "Quality:"},
		&cobra.Group{ID: "analysis", Title: "Analysis:"},
		&cobra.Group{ID: "text-processing", Title: "Text Processing:"},
		&cobra.Group{ID: "advanced", Title: "Advanced:"},
	)
}
