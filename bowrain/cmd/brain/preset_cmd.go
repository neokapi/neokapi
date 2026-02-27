package main

import (
	"github.com/gokapi/gokapi/bowrain/cmd/brain/output"
	"github.com/gokapi/gokapi/core/preset"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var presetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "Manage format and framework presets",
}

var presetsValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate project preset configuration",
	Long:  `Validate that all preset references in .brain/config.yaml are valid.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.FindProject("")
		if err != nil {
			return err
		}

		presetReg := pluginLoader.Presets()
		preset.RegisterBuiltins(presetReg)

		// Convert project local presets to resolver format.
		localPresets := make(map[string]preset.LocalFormatPreset)
		for name, lp := range proj.Config.FormatPresets {
			localPresets[name] = preset.LocalFormatPreset{
				Description: lp.Description,
				Base:        lp.Base,
				Config:      lp.Config,
			}
		}

		resolver := preset.NewConfigResolver(presetReg, pluginLoader.Schemas())
		errors := resolver.ValidateAllPresets(localPresets, "")

		out := output.PresetsValidateOutput{
			Valid:  len(errors) == 0,
			Errors: errors,
		}
		return output.Print(cmd, out)
	},
}

func init() {
	presetsCmd.AddCommand(presetsValidateCmd)
	rootCmd.AddCommand(presetsCmd)
}
