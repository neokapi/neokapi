package preset

import (
	"github.com/gokapi/gokapi/core/preset"
)

// RegisterBuiltins registers built-in framework presets into the given registry.
func RegisterBuiltins(reg *preset.PresetRegistry) {
	reg.RegisterFrameworkPreset("nextjs", nextjsPreset())
	reg.RegisterFrameworkPreset("react-intl", reactIntlPreset())
	reg.RegisterFrameworkPreset("angular", angularPreset())
}

func nextjsPreset() *preset.FrameworkPreset {
	return &preset.FrameworkPreset{
		Name:        "nextjs",
		Description: "Next.js App Router with next-intl",
		Mappings: []preset.MappingTemplate{
			{
				Local:      "messages/*.json",
				Format:     "json",
				TargetPath: "messages/{locale}.json",
			},
		},
		Exclude: []string{"node_modules/**", ".next/**"},
		FormatPresets: map[string]map[string]any{
			"json": {"extractArrayStrings": false},
		},
		Flows: map[string]map[string]any{
			"translate": {"ai_provider": "anthropic"},
		},
		Source: "built-in",
	}
}

func reactIntlPreset() *preset.FrameworkPreset {
	return &preset.FrameworkPreset{
		Name:        "react-intl",
		Description: "React with react-intl (FormatJS)",
		Mappings: []preset.MappingTemplate{
			{
				Local:      "src/lang/*.json",
				Format:     "json",
				TargetPath: "src/lang/{locale}.json",
			},
		},
		Exclude: []string{"node_modules/**", "build/**", "dist/**"},
		FormatPresets: map[string]map[string]any{
			"json": {"extractArrayStrings": false},
		},
		Flows: map[string]map[string]any{
			"translate": {"ai_provider": "anthropic"},
		},
		Source: "built-in",
	}
}

func angularPreset() *preset.FrameworkPreset {
	return &preset.FrameworkPreset{
		Name:        "angular",
		Description: "Angular with @angular/localize",
		Mappings: []preset.MappingTemplate{
			{
				Local:      "src/locale/*.xlf",
				Format:     "xliff",
				TargetPath: "src/locale/messages.{locale}.xlf",
			},
		},
		Exclude: []string{"node_modules/**", "dist/**", ".angular/**"},
		Source:  "built-in",
	}
}
