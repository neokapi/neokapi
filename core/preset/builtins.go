package preset

// sourceBuiltIn mirrors sourceBuiltIn to avoid an import cycle
// (preset cannot import registry because registry transitively depends on preset).
const sourceBuiltIn = "built-in"

// RegisterBuiltins registers built-in framework presets into the given registry.
func RegisterBuiltins(reg *PresetRegistry) {
	reg.RegisterFrameworkPreset("nextjs", nextjsPreset())
	reg.RegisterFrameworkPreset("react-intl", reactIntlPreset())
	reg.RegisterFrameworkPreset("angular", angularPreset())
}

func nextjsPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "nextjs",
		Description: "Next.js App Router with next-intl",
		Detect:      []string{"next.config.js", "next.config.mjs", "next.config.ts"},
		Mappings: []MappingTemplate{
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
		Source: sourceBuiltIn,
	}
}

func reactIntlPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "react-intl",
		Description: "React with react-intl (FormatJS)",
		Detect:      []string{"package.json:react-intl", "package.json:@formatjs/"},
		Mappings: []MappingTemplate{
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
		Source: sourceBuiltIn,
	}
}

func angularPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "angular",
		Description: "Angular with @angular/localize",
		Detect:      []string{"angular.json"},
		Mappings: []MappingTemplate{
			{
				Local:      "src/locale/*.xlf",
				Format:     "xliff",
				TargetPath: "src/locale/messages.{locale}.xlf",
			},
		},
		Exclude: []string{"node_modules/**", "dist/**", ".angular/**"},
		Source:  sourceBuiltIn,
	}
}
