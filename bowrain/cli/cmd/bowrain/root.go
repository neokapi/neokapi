package main

import (
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/bowrain/cli/cmd/bowrain/output"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/cli"
	cliconfig "github.com/neokapi/neokapi/cli/config"
	clioutput "github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/spf13/cobra"
)

var app = &cli.App{}

var rootCmd = &cobra.Command{
	Use:          "bowrain",
	Short:        "Manage localization projects",
	SilenceUsage: true,
	Long: `bowrain manages localization projects, syncing content with Bowrain Server.

Initialize a .bowrain/ project in your repository, then push/pull translations,
run quality checks, and manage terminology.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		app.Config = newBowrainAppConfig()
		app.RegistryResolver = func() []cliconfig.RegistryEntry {
			if proj, err := project.FindProject(""); err == nil && len(proj.Config.Registries) > 0 {
				entries := make([]cliconfig.RegistryEntry, len(proj.Config.Registries))
				for i, r := range proj.Config.Registries {
					entries[i] = cliconfig.RegistryEntry{Name: r.Name, URL: r.URL, Channels: r.Channels}
				}
				return entries
			}
			return nil
		}
		app.Init()
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		app.Shutdown()
	},
}

func init() {
	app.AddPersistentFlags(rootCmd)

	// Primary commands.
	rootCmd.AddCommand(app.NewRunCmd(cli.RunCmdOptions{
		FallbackRunE: projectFlowFallback,
	}))

	// Management commands.
	rootCmd.AddCommand(app.NewFlowsCmd(cli.FlowCmdOptions{
		ExtraFlows: listProjectFlows,
	}))
	rootCmd.AddCommand(app.NewToolsCmd())
	rootCmd.AddCommand(app.NewFormatsCmd())
	rootCmd.AddCommand(app.NewPluginsCmd())
	rootCmd.AddCommand(app.NewRegistryCmd())

	// Shared presets command + Bowrain CLI-specific validate subcommand.
	presetsCmd := app.NewPresetsCmd()
	presetsCmd.AddCommand(newPresetsValidateCmd())
	rootCmd.AddCommand(presetsCmd)

	rootCmd.AddCommand(app.NewTermbaseCmd())
	rootCmd.AddCommand(app.NewTMCmd())
	rootCmd.AddCommand(app.NewVersionCmd("bowrain"))

	// Top-level tool commands (declarative opt-in via BuiltinToolCommands).
	for _, cmd := range app.NewToolCommands() {
		rootCmd.AddCommand(cmd)
	}
}

// projectFlowFallback is called when a flow name doesn't match a built-in
// flow definition. It checks for a project flow in .bowrain/flows/.
func projectFlowFallback(cmd *cobra.Command, flowName string, args []string) error {
	proj, err := findProject()
	if err != nil {
		return err
	}
	return runProjectFlow(cmd, proj, flowName, args)
}

// listProjectFlows returns flow info entries from .bowrain/flows/.
func listProjectFlows() []clioutput.FlowInfo {
	proj, err := findProject()
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
		def, err := loadFlowDefinition(flowPath)
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

// newPresetsValidateCmd creates the Bowrain CLI-specific "presets validate" subcommand.
func newPresetsValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate project preset configuration",
		Long:  `Validate that all preset references in .bowrain/config.yaml are valid.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPresetsValidate(cmd)
		},
	}
}

// runPresetsValidate checks that all preset references in the project config resolve.
func runPresetsValidate(cmd *cobra.Command) error {
	proj, err := project.FindProject("")
	if err != nil {
		return err
	}

	presetReg := app.PluginLoader.Presets()
	preset.RegisterBuiltins(presetReg)

	localPresets := make(map[string]preset.LocalFormatPreset)
	for name, lp := range proj.Config.FormatPresets {
		localPresets[name] = preset.LocalFormatPreset{
			Description: lp.Description,
			Base:        lp.Base,
			Config:      lp.Config,
		}
	}

	resolver := preset.NewConfigResolver(presetReg, app.PluginLoader.Schemas())
	errors := resolver.ValidateAllPresets(localPresets, "")

	out := output.PresetsValidateOutput{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
	return output.Print(cmd, out)
}
