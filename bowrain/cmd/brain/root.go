package main

import (
	"os"
	"path/filepath"

	"github.com/gokapi/gokapi/bowrain/cmd/brain/output"
	"github.com/gokapi/gokapi/core/preset"
	"github.com/gokapi/gokapi/platform/cli"
	clioutput "github.com/gokapi/gokapi/platform/cli/output"
	"github.com/gokapi/gokapi/platform/config"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var app = &cli.App{}

var rootCmd = &cobra.Command{
	Use:          "brain",
	Short:        "Manage localization projects",
	SilenceUsage: true,
	Long: `brain manages localization projects, syncing content with Bowrain Server.

Initialize a .brain/ project in your repository, then push/pull translations,
run quality checks, and manage terminology.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		app.Config = config.NewAppConfig()
		app.Init()
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		app.Shutdown()
	},
}

func init() {
	app.AddPersistentFlags(rootCmd)

	// Shared commands.
	rootCmd.AddCommand(app.NewFlowCmd(cli.FlowCmdOptions{
		FallbackRunE: projectFlowFallback,
		ExtraFlows:   listProjectFlows,
	}))
	rootCmd.AddCommand(app.NewFormatsCmd())
	rootCmd.AddCommand(app.NewPluginsCmd())
	rootCmd.AddCommand(app.NewRegistryCmd())
	rootCmd.AddCommand(app.NewToolsCmd())

	// Shared presets command + brain-specific validate subcommand.
	presetsCmd := app.NewPresetsCmd()
	presetsCmd.AddCommand(newPresetsValidateCmd())
	rootCmd.AddCommand(presetsCmd)

	rootCmd.AddCommand(app.NewTermbaseCmd())
	rootCmd.AddCommand(app.NewVersionCmd("brain"))

	for _, cmd := range app.NewToolCommands() {
		rootCmd.AddCommand(cmd)
	}
}

// projectFlowFallback is called when a flow name doesn't match a built-in
// flow definition. It checks for a project flow in .brain/flows/.
func projectFlowFallback(cmd *cobra.Command, flowName string, args []string) error {
	proj, err := findProject()
	if err != nil {
		return err
	}
	return runProjectFlow(cmd, proj, flowName, args)
}

// listProjectFlows returns flow info entries from .brain/flows/.
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

// newPresetsValidateCmd creates the brain-specific "presets validate" subcommand.
func newPresetsValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate project preset configuration",
		Long:  `Validate that all preset references in .brain/config.yaml are valid.`,
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
