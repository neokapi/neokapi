package preset

// sourceBuiltIn mirrors sourceBuiltIn to avoid an import cycle
// (preset cannot import registry because registry transitively depends on preset).
const sourceBuiltIn = "built-in"

// RegisterBuiltins registers built-in framework presets into the given registry.
func RegisterBuiltins(reg *PresetRegistry) {
	reg.RegisterFrameworkPreset("kapi-react", kapiReactPreset())
	reg.RegisterFrameworkPreset("react-i18next", reactI18nextPreset())
	reg.RegisterFrameworkPreset("react-intl", reactIntlPreset())
	reg.RegisterFrameworkPreset("nextjs", nextjsPreset())
	reg.RegisterFrameworkPreset("vue-i18n", vueI18nPreset())
	reg.RegisterFrameworkPreset("flutter", flutterPreset())
	reg.RegisterFrameworkPreset("angular", angularPreset())
}

// kapiReactPreset represents a React project using the @neokapi/kapi-react stack:
// the bundler plugin extracts plain JSX to a KLF directory, kapi translates it,
// and kapi-react compiles per-locale runtime dicts.
func kapiReactPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "kapi-react",
		Description: "React with the @neokapi/kapi-react stack (zero-wrapper, KLF extraction)",
		Detect:      []string{"package.json:@neokapi/kapi-react"},
		Mappings: []MappingTemplate{
			{
				Local:      "i18n/**/*.klf",
				Format:     "klf",
				TargetPath: "i18n/{path}",
			},
		},
		Exclude: []string{"node_modules/**", "dist/**", "build/**"},
		Flows: map[string]map[string]any{
			"translate": {"ai_provider": "anthropic"},
		},
		Source: sourceBuiltIn,
	}
}

// reactI18nextPreset covers the react-i18next / i18next ecosystem, whose default
// catalog layout is public/locales/{lng}/{namespace}.json.
func reactI18nextPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "react-i18next",
		Description: "React/JS with react-i18next or i18next (public/locales JSON)",
		Detect:      []string{"package.json:react-i18next", "package.json:i18next"},
		Mappings: []MappingTemplate{
			{
				Local:      "public/locales/en/*.json",
				Format:     "json",
				TargetPath: "public/locales/{locale}/{name}.json",
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

// vueI18nPreset covers Vue projects using vue-i18n with src/locales/{locale}.json.
func vueI18nPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "vue-i18n",
		Description: "Vue with vue-i18n (src/locales JSON)",
		Detect:      []string{"package.json:vue-i18n"},
		Mappings: []MappingTemplate{
			{
				Local:      "src/locales/en.json",
				Format:     "json",
				TargetPath: "src/locales/{locale}.json",
			},
		},
		Exclude: []string{"node_modules/**", "dist/**"},
		FormatPresets: map[string]map[string]any{
			"json": {"extractArrayStrings": false},
		},
		Flows: map[string]map[string]any{
			"translate": {"ai_provider": "anthropic"},
		},
		Source: sourceBuiltIn,
	}
}

// flutterPreset covers Flutter projects using ARB catalogs (lib/l10n/app_{locale}.arb).
// ARB is a JSON dialect, so the JSON reader handles it.
func flutterPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "flutter",
		Description: "Flutter with intl/ARB catalogs (lib/l10n/app_{locale}.arb)",
		Detect:      []string{"pubspec.yaml"},
		Mappings: []MappingTemplate{
			{
				Local:      "lib/l10n/app_en.arb",
				Format:     "json",
				TargetPath: "lib/l10n/app_{locale}.arb",
			},
		},
		Exclude: []string{".dart_tool/**", "build/**"},
		FormatPresets: map[string]map[string]any{
			"json": {"extractArrayStrings": false},
		},
		Flows: map[string]map[string]any{
			"translate": {"ai_provider": "anthropic"},
		},
		Source: sourceBuiltIn,
	}
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
