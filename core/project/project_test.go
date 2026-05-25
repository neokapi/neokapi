package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLoadSaveRoundtrip(t *testing.T) {
	proj := &KapiProject{
		Version: "v1",
		Name:    "My App",
		Plugins: map[string]PluginSpec{
			"okapi": {FrameworkVersion: "^1.47.0", FormatPriority: 200},
		},
		Defaults: Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
			Concurrency:     4,
			ParallelBlocks:  3,
			Encoding:        "utf-8",
			Formats: map[string]FormatDefaults{
				"okf_html": {Preset: "strict-extraction"},
				"json":     {Config: map[string]any{"indent": 2}},
			},
		},
		Content: []ContentCollection{
			{Path: "src/locales/en/*.json", Format: &FormatSpec{Name: "json"}, Target: "src/locales/{lang}/*.json"},
			{
				Name:            "Marketing",
				TargetLanguages: []model.LocaleID{"fr-FR"},
				Items: []ContentItem{
					{Path: "marketing/**/*.html", Format: &FormatSpec{Name: "okf_html", Preset: "lenient"}, Target: "marketing/{lang}/**/*.html"},
					{Path: "marketing/**/*.json", Target: "marketing/{lang}/**/*.json"},
				},
			},
		},
		Preset: "nextjs",
		Flows: map[string]*flow.StepsSpec{
			"translate": {
				Steps: []flow.FlowStep{
					{Tool: "ai-translate", Config: map[string]any{"provider": "anthropic"}},
				},
			},
			"translate-and-qa": {
				Steps: []flow.FlowStep{
					{Tool: "ai-translate", Config: map[string]any{"provider": "anthropic"}},
					{Tool: "qa-check"},
				},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")

	require.NoError(t, Save(path, proj))

	loaded, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "v1", loaded.Version)
	assert.Equal(t, "My App", loaded.Name)
	assert.Equal(t, model.LocaleID("en-US"), loaded.Defaults.SourceLanguage)
	assert.Equal(t, []model.LocaleID{"fr-FR", "de-DE"}, loaded.Defaults.TargetLanguages)
	assert.Equal(t, 4, loaded.Defaults.Concurrency)
	assert.Equal(t, 3, loaded.Defaults.ParallelBlocks)
	assert.Equal(t, "utf-8", loaded.Defaults.Encoding)
	assert.Equal(t, "strict-extraction", loaded.Defaults.Formats["okf_html"].Preset)
	assert.Equal(t, 2, loaded.Defaults.Formats["json"].Config["indent"])

	// Plugins.
	require.Contains(t, loaded.Plugins, "okapi")
	assert.Equal(t, "^1.47.0", loaded.Plugins["okapi"].FrameworkVersion)
	assert.Equal(t, 200, loaded.Plugins["okapi"].FormatPriority)

	// Content.
	require.Len(t, loaded.Content, 2)

	// Bare entry.
	bare := loaded.Content[0]
	assert.True(t, bare.IsBareEntry())
	assert.Equal(t, "src/locales/en/*.json", bare.Path)
	assert.Equal(t, "json", bare.Format.Name)
	assert.Equal(t, "src/locales/{lang}/*.json", bare.Target)

	// Collection.
	coll := loaded.Content[1]
	assert.False(t, coll.IsBareEntry())
	assert.Equal(t, "Marketing", coll.Name)
	assert.Equal(t, []model.LocaleID{"fr-FR"}, coll.TargetLanguages)
	require.Len(t, coll.Items, 2)
	assert.Equal(t, "marketing/**/*.html", coll.Items[0].Path)
	assert.Equal(t, "okf_html", coll.Items[0].Format.Name)
	assert.Equal(t, "lenient", coll.Items[0].Format.Preset)

	// Flows.
	assert.Equal(t, "nextjs", loaded.Preset)
	assert.Len(t, loaded.Flows, 2)
	assert.Len(t, loaded.Flows["translate"].Steps, 1)
	assert.Equal(t, "ai-translate", loaded.Flows["translate"].Steps[0].Tool)
	assert.Len(t, loaded.Flows["translate-and-qa"].Steps, 2)
}

func TestLoadFromYAML(t *testing.T) {
	yamlContent := `version: v1
name: Test Project
defaults:
  source_language: en
  target_languages:
    - fr
    - de
  formats:
    okf_html:
      preset: strict-extraction
      priority: 200
plugins:
  okapi: "^1.47.0"
  my-tool:
    version: "^2.0.0"
content:
  - path: "messages/*.json"
    format: json
  - name: Docs
    source_language: zh-CN
    target_languages: [en-US]
    items:
      - path: "docs/*.html"
        format:
          name: okf_html
          preset: lenient
          config:
            preserveWhitespace: true
        target: "docs/{lang}/*.html"
flows:
  pseudo:
    steps:
      - tool: pseudo-translate
        config:
          expansion_rate: 1.3
`
	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")
	require.NoError(t, os.WriteFile(path, []byte(yamlContent), 0o644))

	proj, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "Test Project", proj.Name)
	assert.Equal(t, model.LocaleID("en"), proj.Defaults.SourceLanguage)
	assert.Equal(t, []model.LocaleID{"fr", "de"}, proj.Defaults.TargetLanguages)

	// Format defaults.
	require.Contains(t, proj.Defaults.Formats, "okf_html")
	assert.Equal(t, "strict-extraction", proj.Defaults.Formats["okf_html"].Preset)
	assert.Equal(t, 200, proj.Defaults.Formats["okf_html"].Priority)

	// Plugins — short form.
	require.Contains(t, proj.Plugins, "okapi")
	assert.Equal(t, "^1.47.0", proj.Plugins["okapi"].Version)
	assert.Empty(t, proj.Plugins["okapi"].FrameworkVersion)

	// Plugins — long form.
	require.Contains(t, proj.Plugins, "my-tool")
	assert.Equal(t, "^2.0.0", proj.Plugins["my-tool"].Version)

	// Content — bare entry with short format.
	require.Len(t, proj.Content, 2)
	assert.True(t, proj.Content[0].IsBareEntry())
	assert.Equal(t, "json", proj.Content[0].Format.Name)

	// Content — collection with long format.
	coll := proj.Content[1]
	assert.False(t, coll.IsBareEntry())
	assert.Equal(t, "Docs", coll.Name)
	assert.Equal(t, model.LocaleID("zh-CN"), coll.SourceLanguage)
	assert.Equal(t, []model.LocaleID{"en-US"}, coll.TargetLanguages)
	require.Len(t, coll.Items, 1)
	assert.Equal(t, "okf_html", coll.Items[0].Format.Name)
	assert.Equal(t, "lenient", coll.Items[0].Format.Preset)
	assert.Equal(t, true, coll.Items[0].Format.Config["preserveWhitespace"])

	// Flows.
	spec := proj.GetFlow("pseudo")
	require.NotNil(t, spec)
	assert.Len(t, spec.Steps, 1)
	assert.Equal(t, "pseudo-translate", spec.Steps[0].Tool)
	assert.Equal(t, 1.3, spec.Steps[0].Config["expansion_rate"])
}

func TestFormatSpecUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantName string
		wantPre  string
	}{
		{"short form", `format: okf_html`, "okf_html", ""},
		{"long form", "format:\n  name: okf_html\n  preset: lenient", "okf_html", "lenient"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target struct {
				Format *FormatSpec `yaml:"format,omitempty"`
			}
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), &target))
			require.NotNil(t, target.Format)
			assert.Equal(t, tt.wantName, target.Format.Name)
			assert.Equal(t, tt.wantPre, target.Format.Preset)
		})
	}
}

func TestPluginSpecUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantVer string
		wantFV  string
		wantFP  int
	}{
		{"short form", `"^2.0.0"`, "^2.0.0", "", 0},
		{"long form", "version: \"^0.38.0\"\nframework_version: \"^1.47.0\"\nformat_priority: 200", "^0.38.0", "^1.47.0", 200},
		{"framework only", "framework_version: \"^1.47.0\"", "", "^1.47.0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var spec PluginSpec
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), &spec))
			assert.Equal(t, tt.wantVer, spec.Version)
			assert.Equal(t, tt.wantFV, spec.FrameworkVersion)
			assert.Equal(t, tt.wantFP, spec.FormatPriority)
		})
	}
}

func TestContentCollectionBareVsCollection(t *testing.T) {
	yamlContent := `
- path: "src/**/*"
  target: "output/{lang}/**/*"
- name: Marketing
  target_languages: [fr-FR]
  items:
    - path: "marketing/*.html"
      format: okf_html
`
	var content []ContentCollection
	require.NoError(t, yaml.Unmarshal([]byte(yamlContent), &content))
	require.Len(t, content, 2)

	// Bare entry.
	assert.True(t, content[0].IsBareEntry())
	items := content[0].EffectiveItems()
	require.Len(t, items, 1)
	assert.Equal(t, "src/**/*", items[0].Path)
	assert.Equal(t, "output/{lang}/**/*", items[0].Target)

	// Collection.
	assert.False(t, content[1].IsBareEntry())
	assert.Equal(t, "Marketing", content[1].Name)
	items = content[1].EffectiveItems()
	require.Len(t, items, 1)
	assert.Equal(t, "marketing/*.html", items[0].Path)
}

func TestContentCollectionExecFormat(t *testing.T) {
	yamlContent := `
- name: ui
  items:
    - path: "src/**/*.tsx"
      format:
        name: exec
        config:
          command: "vp kapi-react extract --stream"
`
	var content []ContentCollection
	require.NoError(t, yaml.Unmarshal([]byte(yamlContent), &content))
	require.Len(t, content, 1)
	require.Len(t, content[0].Items, 1)
	require.NotNil(t, content[0].Items[0].Format)
	assert.Equal(t, "exec", content[0].Items[0].Format.Name)
	assert.Equal(t, "vp kapi-react extract --stream",
		content[0].Items[0].Format.Config["command"])

	out, err := yaml.Marshal(content)
	require.NoError(t, err)
	var back []ContentCollection
	require.NoError(t, yaml.Unmarshal(out, &back))
	assert.Equal(t, content, back, "exec format spec round-trips cleanly")
}

func TestLanguageResolution(t *testing.T) {
	defaults := Defaults{
		SourceLanguage:  "en-US",
		TargetLanguages: []model.LocaleID{"fr-FR", "de-DE", "ja-JP"},
	}

	coll := &ContentCollection{
		Name:            "China",
		SourceLanguage:  "zh-CN",
		TargetLanguages: []model.LocaleID{"en-US"},
	}

	t.Run("item inherits from defaults", func(t *testing.T) {
		item := &ContentItem{Path: "src/**/*"}
		assert.Equal(t, model.LocaleID("en-US"), item.ResolvedSourceLanguage(nil, defaults))
		assert.Equal(t, []model.LocaleID{"fr-FR", "de-DE", "ja-JP"}, item.ResolvedTargetLanguages(nil, defaults))
	})

	t.Run("item inherits from collection", func(t *testing.T) {
		item := &ContentItem{Path: "china/**/*"}
		assert.Equal(t, model.LocaleID("zh-CN"), item.ResolvedSourceLanguage(coll, defaults))
		assert.Equal(t, []model.LocaleID{"en-US"}, item.ResolvedTargetLanguages(coll, defaults))
	})

	t.Run("item overrides collection", func(t *testing.T) {
		item := &ContentItem{
			Path:            "special/*",
			SourceLanguage:  "ko-KR",
			TargetLanguages: []model.LocaleID{"ja-JP"},
		}
		assert.Equal(t, model.LocaleID("ko-KR"), item.ResolvedSourceLanguage(coll, defaults))
		assert.Equal(t, []model.LocaleID{"ja-JP"}, item.ResolvedTargetLanguages(coll, defaults))
	})

	t.Run("partial override — source from collection, targets from item", func(t *testing.T) {
		item := &ContentItem{
			Path:            "mixed/*",
			TargetLanguages: []model.LocaleID{"de-DE"},
		}
		assert.Equal(t, model.LocaleID("zh-CN"), item.ResolvedSourceLanguage(coll, defaults))
		assert.Equal(t, []model.LocaleID{"de-DE"}, item.ResolvedTargetLanguages(coll, defaults))
	})
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		proj    KapiProject
		wantErr string
	}{
		{
			name:    "missing version",
			proj:    KapiProject{Name: "test"},
			wantErr: "version is required",
		},
		{
			name:    "unsupported version",
			proj:    KapiProject{Version: "v99", Name: "test"},
			wantErr: "unsupported version",
		},
		{
			name: "collection without items",
			proj: KapiProject{
				Version: "v1",
				Content: []ContentCollection{
					{Name: "Empty"},
				},
			},
			wantErr: `collection "Empty" must have at least one item`,
		},
		{
			name: "collection item without path",
			proj: KapiProject{
				Version: "v1",
				Content: []ContentCollection{
					{Name: "Bad", Items: []ContentItem{{}}},
				},
			},
			wantErr: "content[0].items[0]: path is required",
		},
		{
			name: "flow without steps",
			proj: KapiProject{
				Version: "v1",
				Flows: map[string]*flow.StepsSpec{
					"empty": {Steps: nil},
				},
			},
			wantErr: `flow "empty": at least one step is required`,
		},
		{
			name: "flow step without tool",
			proj: KapiProject{
				Version: "v1",
				Flows: map[string]*flow.StepsSpec{
					"bad": {Steps: []flow.FlowStep{{}}},
				},
			},
			wantErr: `flow "bad" step[0]: tool is required`,
		},
		{
			name: "valid minimal project",
			proj: KapiProject{Version: "v1"},
		},
		{
			name: "valid bare entry",
			proj: KapiProject{
				Version: "v1",
				Content: []ContentCollection{
					{Path: "src/**/*"},
				},
			},
		},
		{
			name: "valid collection",
			proj: KapiProject{
				Version: "v1",
				Content: []ContentCollection{
					{Name: "Docs", Items: []ContentItem{{Path: "docs/*.md"}}},
				},
			},
		},
		{
			name: "valid with flows",
			proj: KapiProject{
				Version: "v1",
				Flows: map[string]*flow.StepsSpec{
					"translate": {Steps: []flow.FlowStep{{Tool: "ai-translate"}}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.proj.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetFlow(t *testing.T) {
	proj := &KapiProject{
		Version: "v1",
		Flows: map[string]*flow.StepsSpec{
			"translate": {Steps: []flow.FlowStep{{Tool: "ai-translate"}}},
		},
	}

	assert.NotNil(t, proj.GetFlow("translate"))
	assert.Nil(t, proj.GetFlow("nonexistent"))
}

func TestFlowNames(t *testing.T) {
	proj := &KapiProject{
		Version: "v1",
		Flows: map[string]*flow.StepsSpec{
			"a": {Steps: []flow.FlowStep{{Tool: "x"}}},
			"b": {Steps: []flow.FlowStep{{Tool: "y"}}},
		},
	}

	names := proj.FlowNames()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "a")
	assert.Contains(t, names, "b")
}

func TestFlowNamesEmpty(t *testing.T) {
	proj := &KapiProject{Version: "v1"}
	assert.Empty(t, proj.FlowNames())
}

func TestSaveSetsDefaultVersion(t *testing.T) {
	proj := &KapiProject{Name: "test"}
	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")

	require.NoError(t, Save(path, proj))
	assert.Equal(t, CurrentVersion, proj.Version)
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path.kapi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read project file")
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.kapi")
	require.NoError(t, os.WriteFile(path, []byte("{{invalid yaml"), 0o644))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse project file")
}

func TestParallelSteps(t *testing.T) {
	proj := &KapiProject{
		Version: "v1",
		Flows: map[string]*flow.StepsSpec{
			"parallel-qa": {
				Steps: []flow.FlowStep{
					{
						Parallel: []flow.FlowStep{
							{Tool: "qa-check"},
							{Tool: "ai-qa"},
						},
					},
				},
			},
		},
	}

	require.NoError(t, proj.Validate())
	spec := proj.GetFlow("parallel-qa")
	require.NotNil(t, spec)
	assert.Len(t, spec.Steps[0].Parallel, 2)
}

func TestEffectiveItems(t *testing.T) {
	t.Run("bare entry wraps as single item", func(t *testing.T) {
		c := &ContentCollection{
			Path:   "src/**/*",
			Format: &FormatSpec{Name: "json"},
			Target: "output/{lang}/**/*",
		}
		items := c.EffectiveItems()
		require.Len(t, items, 1)
		assert.Equal(t, "src/**/*", items[0].Path)
		assert.Equal(t, "json", items[0].Format.Name)
		assert.Equal(t, "output/{lang}/**/*", items[0].Target)
	})

	t.Run("collection returns items directly", func(t *testing.T) {
		c := &ContentCollection{
			Name: "Test",
			Items: []ContentItem{
				{Path: "a/*"},
				{Path: "b/*"},
			},
		}
		items := c.EffectiveItems()
		assert.Len(t, items, 2)
	})
}

// Bilingual interop defaults — AD-017 (issue #414).

func TestDefaults_MergeTMSegmentation_RoundTrip(t *testing.T) {
	proj := &KapiProject{
		Version: "v1",
		Name:    "Interop",
		Defaults: Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
			Merge:           MergeDefaults{ConflictPolicy: ConflictPolicyExistingWins},
			TM: TMDefaults{
				FuzzyThreshold: 80,
				Read:           []string{"/opt/corporate-en-fr.tmx", "./legacy.tmx"},
			},
			Segmentation: SegmentationDefaults{Source: true, SRX: "rules.srx"},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")
	require.NoError(t, Save(path, proj))

	loaded, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, ConflictPolicyExistingWins, loaded.Defaults.Merge.ConflictPolicy)
	assert.Equal(t, 80, loaded.Defaults.TM.FuzzyThreshold)
	assert.Equal(t, []string{"/opt/corporate-en-fr.tmx", "./legacy.tmx"}, loaded.Defaults.TM.Read)
	assert.True(t, loaded.Defaults.Segmentation.Source)
	assert.Equal(t, "rules.srx", loaded.Defaults.Segmentation.SRX)
}

func TestDefaults_MergeTM_Defaults(t *testing.T) {
	var m MergeDefaults
	assert.Equal(t, ConflictPolicyTranslatorWins, m.ResolvedConflictPolicy())
	m.ConflictPolicy = ConflictPolicyNewestWins
	assert.Equal(t, ConflictPolicyNewestWins, m.ResolvedConflictPolicy())

	var tm TMDefaults
	assert.Equal(t, DefaultFuzzyThreshold, tm.ResolvedFuzzyThreshold())
	tm.FuzzyThreshold = 60
	assert.Equal(t, 60, tm.ResolvedFuzzyThreshold())
}

func TestValidate_MergeConflictPolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  string
		wantErr bool
	}{
		{"empty is ok (defaults apply)", "", false},
		{"translator-wins", ConflictPolicyTranslatorWins, false},
		{"existing-wins", ConflictPolicyExistingWins, false},
		{"newest-wins", ConflictPolicyNewestWins, false},
		{"unknown rejected", "translator-loses", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proj := &KapiProject{Version: "v1", Defaults: Defaults{Merge: MergeDefaults{ConflictPolicy: tc.policy}}}
			err := proj.Validate()
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "conflict_policy")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidate_TMFuzzyThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold int
		wantErr   bool
	}{
		{"zero is ok (default applies)", 0, false},
		{"min", 1, false},
		{"mid", 75, false},
		{"max", 100, false},
		{"negative rejected", -1, true},
		{"over 100 rejected", 101, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proj := &KapiProject{Version: "v1", Defaults: Defaults{TM: TMDefaults{FuzzyThreshold: tc.threshold}}}
			err := proj.Validate()
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "fuzzy_threshold")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLoad_YAMLInteropSections(t *testing.T) {
	yamlText := `version: v1
name: YAML Interop
defaults:
  source_language: en
  target_languages: [fr, de]
  merge:
    conflict_policy: newest-wins
  tm:
    fuzzy_threshold: 70
    read:
      - /shared/corp.tmx
  segmentation:
    source: true
`
	dir := t.TempDir()
	path := filepath.Join(dir, "recipe.kapi")
	require.NoError(t, os.WriteFile(path, []byte(yamlText), 0o644))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, ConflictPolicyNewestWins, loaded.Defaults.Merge.ConflictPolicy)
	assert.Equal(t, 70, loaded.Defaults.TM.FuzzyThreshold)
	assert.Equal(t, []string{"/shared/corp.tmx"}, loaded.Defaults.TM.Read)
	assert.True(t, loaded.Defaults.Segmentation.Source)
}

func TestDefaults_BrandVoiceTermbase_Parse(t *testing.T) {
	yamlText := `version: v1
name: brandy
defaults:
  source_language: en
  target_languages: [fr]
  brand_voice:
    profile_file: brand.yaml
  termbase: glossary.db
`
	dir := t.TempDir()
	path := filepath.Join(dir, "brandy.kapi")
	require.NoError(t, os.WriteFile(path, []byte(yamlText), 0o644))

	loaded, err := Load(path)
	require.NoError(t, err)
	require.NotNil(t, loaded.Defaults.BrandVoice)
	assert.Equal(t, "brand.yaml", loaded.Defaults.BrandVoice.ProfileFile)
	assert.Empty(t, loaded.Defaults.BrandVoice.Profile)
	assert.Empty(t, loaded.Defaults.BrandVoice.Pack)
	assert.Equal(t, "glossary.db", loaded.Defaults.Termbase)
}

func TestDefaults_BrandVoiceTermbase_RoundTrip(t *testing.T) {
	proj := &KapiProject{
		Version: "v1",
		Name:    "Branded",
		Defaults: Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			BrandVoice:      &BrandVoiceBinding{Profile: "house-style"},
			Termbase:        ".kapi/termbase.db",
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "branded.kapi")
	require.NoError(t, Save(path, proj))

	loaded, err := Load(path)
	require.NoError(t, err)
	require.NotNil(t, loaded.Defaults.BrandVoice)
	assert.Equal(t, "house-style", loaded.Defaults.BrandVoice.Profile)
	assert.Equal(t, ".kapi/termbase.db", loaded.Defaults.Termbase)
}

func TestBrandVoiceBinding_Validate(t *testing.T) {
	tests := []struct {
		name    string
		binding *BrandVoiceBinding
		wantErr bool
	}{
		{"nil is ok", nil, false},
		{"profile_file only", &BrandVoiceBinding{ProfileFile: "brand.yaml"}, false},
		{"profile only", &BrandVoiceBinding{Profile: "house"}, false},
		{"pack only", &BrandVoiceBinding{Pack: "professional-b2b"}, false},
		{"none set", &BrandVoiceBinding{}, true},
		{"two set", &BrandVoiceBinding{ProfileFile: "brand.yaml", Pack: "p"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.binding.validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestBowrainTopLevelBrandVoice_RoundTrips guards that the framework's
// `defaults.brand_voice` binding does not regress bowrain's distinct
// top-level `brand_voice:` extension. The framework alone (no bowrain
// extension registered) must round-trip the top-level key verbatim via
// Extras while still decoding the defaults binding into its typed field.
func TestBowrainTopLevelBrandVoice_RoundTrips(t *testing.T) {
	yamlText := `version: v1
name: dual
defaults:
  source_language: en
  target_languages: [fr]
  brand_voice:
    pack: professional-b2b
brand_voice:
  profile: house-style
  channel: web
`
	dir := t.TempDir()
	path := filepath.Join(dir, "dual.kapi")
	require.NoError(t, os.WriteFile(path, []byte(yamlText), 0o644))

	loaded, err := Load(path)
	require.NoError(t, err)

	// Framework binding under defaults decodes into the typed field.
	require.NotNil(t, loaded.Defaults.BrandVoice)
	assert.Equal(t, "professional-b2b", loaded.Defaults.BrandVoice.Pack)

	// Bowrain's top-level brand_voice survives in Extras (no extension
	// registered in the framework test binary).
	node, ok := loaded.Extras["brand_voice"]
	require.True(t, ok, "top-level brand_voice should be captured in Extras")
	var topLevel struct {
		Profile string `yaml:"profile"`
		Channel string `yaml:"channel"`
	}
	require.NoError(t, node.Decode(&topLevel))
	assert.Equal(t, "house-style", topLevel.Profile)
	assert.Equal(t, "web", topLevel.Channel)

	// Save and reload — both keys must survive a full round-trip.
	out := filepath.Join(dir, "dual-out.kapi")
	require.NoError(t, Save(out, loaded))
	reloaded, err := Load(out)
	require.NoError(t, err)
	require.NotNil(t, reloaded.Defaults.BrandVoice)
	assert.Equal(t, "professional-b2b", reloaded.Defaults.BrandVoice.Pack)
	_, ok = reloaded.Extras["brand_voice"]
	assert.True(t, ok, "top-level brand_voice should survive round-trip in Extras")
}
