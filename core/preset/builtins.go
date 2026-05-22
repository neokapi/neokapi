package preset

// sourceBuiltIn mirrors sourceBuiltIn to avoid an import cycle
// (preset cannot import registry because registry transitively depends on preset).
const sourceBuiltIn = "built-in"

// KapiReactPresetName is the framework preset for projects using the
// @neokapi/kapi-react stack. Its i18n is managed by the kapi-react bundler
// plugin + `kapi-react extract|compile`, not by a `.kapi` content mapping —
// `kapi init --framework` handles it specially.
const KapiReactPresetName = "kapi-react"

// RegisterBuiltins registers built-in framework presets into the given registry.
// Catalog-based presets carry recipe-ready mappings (source glob → target glob
// with the `{lang}` placeholder), so `kapi init --framework <name>` can scaffold
// a working content block directly.
func RegisterBuiltins(reg *PresetRegistry) {
	reg.RegisterFrameworkPreset(KapiReactPresetName, kapiReactPreset())
	reg.RegisterFrameworkPreset("react-i18next", reactI18nextPreset())
	reg.RegisterFrameworkPreset("react-intl", reactIntlPreset())
	reg.RegisterFrameworkPreset("nextjs", nextjsPreset())
	reg.RegisterFrameworkPreset("vue-i18n", vueI18nPreset())
	reg.RegisterFrameworkPreset("flutter", flutterPreset())
	reg.RegisterFrameworkPreset("angular", angularPreset())
}

// kapiReactPreset represents a React project using the @neokapi/kapi-react stack:
// the bundler plugin extracts plain JSX to a KLF directory, kapi translates it,
// and kapi-react compiles per-locale runtime dicts. Detection-oriented — the
// mapping is informational (kapi-react owns the i18n/ directory).
func kapiReactPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        KapiReactPresetName,
		Description: "React with the @neokapi/kapi-react stack (zero-wrapper, KLF extraction)",
		Detect:      []string{"package.json:@neokapi/kapi-react"},
		Mappings: []MappingTemplate{
			{Local: "i18n/**/*.klf", Format: "klf", TargetPath: "i18n/**/*.klf"},
		},
		Exclude: []string{"node_modules/**", "dist/**", "build/**"},
		Source:  sourceBuiltIn,
	}
}

// reactI18nextPreset covers react-i18next / i18next, whose default catalog layout
// is public/locales/{lng}/{namespace}.json.
func reactI18nextPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "react-i18next",
		Description: "React/JS with react-i18next or i18next (public/locales JSON)",
		Detect:      []string{"package.json:react-i18next", "package.json:i18next"},
		Mappings: []MappingTemplate{
			{Local: "public/locales/en/*.json", Format: "json", TargetPath: "public/locales/{lang}/*.json"},
		},
		Exclude:       []string{"node_modules/**", "build/**", "dist/**"},
		FormatPresets: map[string]map[string]any{"json": {"extractArrayStrings": false}},
		Source:        sourceBuiltIn,
	}
}

// reactIntlPreset covers React with react-intl (FormatJS): one src/lang/{lng}.json per locale.
func reactIntlPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "react-intl",
		Description: "React with react-intl (FormatJS)",
		Detect:      []string{"package.json:react-intl", "package.json:@formatjs/"},
		Mappings: []MappingTemplate{
			{Local: "src/lang/en.json", Format: "json", TargetPath: "src/lang/{lang}.json"},
		},
		Exclude:       []string{"node_modules/**", "build/**", "dist/**"},
		FormatPresets: map[string]map[string]any{"json": {"extractArrayStrings": false}},
		Source:        sourceBuiltIn,
	}
}

// nextjsPreset covers Next.js App Router with next-intl: messages/{lng}.json.
func nextjsPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "nextjs",
		Description: "Next.js App Router with next-intl (messages JSON)",
		Detect:      []string{"next.config.js", "next.config.mjs", "next.config.ts"},
		Mappings: []MappingTemplate{
			{Local: "messages/en.json", Format: "json", TargetPath: "messages/{lang}.json"},
		},
		Exclude:       []string{"node_modules/**", ".next/**"},
		FormatPresets: map[string]map[string]any{"json": {"extractArrayStrings": false}},
		Source:        sourceBuiltIn,
	}
}

// vueI18nPreset covers Vue with vue-i18n: src/locales/{lng}.json.
func vueI18nPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "vue-i18n",
		Description: "Vue with vue-i18n (src/locales JSON)",
		Detect:      []string{"package.json:vue-i18n"},
		Mappings: []MappingTemplate{
			{Local: "src/locales/en.json", Format: "json", TargetPath: "src/locales/{lang}.json"},
		},
		Exclude:       []string{"node_modules/**", "dist/**"},
		FormatPresets: map[string]map[string]any{"json": {"extractArrayStrings": false}},
		Source:        sourceBuiltIn,
	}
}

// flutterPreset covers Flutter intl/ARB catalogs (lib/l10n/app_{lng}.arb). ARB is
// a JSON dialect, so the JSON reader handles it.
func flutterPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "flutter",
		Description: "Flutter with intl/ARB catalogs (lib/l10n/app_{lang}.arb)",
		Detect:      []string{"pubspec.yaml"},
		Mappings: []MappingTemplate{
			{Local: "lib/l10n/app_en.arb", Format: "json", TargetPath: "lib/l10n/app_{lang}.arb"},
		},
		Exclude:       []string{".dart_tool/**", "build/**"},
		FormatPresets: map[string]map[string]any{"json": {"extractArrayStrings": false}},
		Source:        sourceBuiltIn,
	}
}

// angularPreset covers Angular with @angular/localize XLIFF (src/locale/messages.{lng}.xlf).
func angularPreset() *FrameworkPreset {
	return &FrameworkPreset{
		Name:        "angular",
		Description: "Angular with @angular/localize (XLIFF)",
		Detect:      []string{"angular.json"},
		Mappings: []MappingTemplate{
			{Local: "src/locale/messages.xlf", Format: "xliff", TargetPath: "src/locale/messages.{lang}.xlf"},
		},
		Exclude: []string{"node_modules/**", "dist/**", ".angular/**"},
		Source:  sourceBuiltIn,
	}
}
