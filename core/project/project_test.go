package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSaveRoundtrip(t *testing.T) {
	proj := &KapiProject{
		Version:         "v1",
		Name:            "My App",
		SourceLanguage:  "en-US",
		TargetLanguages: []string{"fr-FR", "de-DE"},
		Content: []ContentEntry{
			{Path: "src/locales/en/*.json", Format: "json", Target: "src/locales/{lang}/*.json"},
		},
		Preset:  "nextjs",
		Plugins: []string{"okapi@1.47.0"},
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
		Defaults: Defaults{
			Concurrency:    4,
			ParallelBlocks: 3,
			Encoding:       "utf-8",
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")

	require.NoError(t, Save(path, proj))

	loaded, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "v1", loaded.Version)
	assert.Equal(t, "My App", loaded.Name)
	assert.Equal(t, "en-US", loaded.SourceLanguage)
	assert.Equal(t, []string{"fr-FR", "de-DE"}, loaded.TargetLanguages)
	assert.Len(t, loaded.Content, 1)
	assert.Equal(t, "src/locales/en/*.json", loaded.Content[0].Path)
	assert.Equal(t, "json", loaded.Content[0].Format)
	assert.Equal(t, "src/locales/{lang}/*.json", loaded.Content[0].Target)
	assert.Equal(t, "nextjs", loaded.Preset)
	assert.Equal(t, []string{"okapi@1.47.0"}, loaded.Plugins)
	assert.Len(t, loaded.Flows, 2)
	assert.Len(t, loaded.Flows["translate"].Steps, 1)
	assert.Equal(t, "ai-translate", loaded.Flows["translate"].Steps[0].Tool)
	assert.Len(t, loaded.Flows["translate-and-qa"].Steps, 2)
	assert.Equal(t, 4, loaded.Defaults.Concurrency)
	assert.Equal(t, 3, loaded.Defaults.ParallelBlocks)
	assert.Equal(t, "utf-8", loaded.Defaults.Encoding)
}

func TestLoadFromYAML(t *testing.T) {
	yaml := `version: v1
name: Test Project
source_language: en
target_languages:
  - fr
  - de
content:
  - path: "messages/*.json"
    format: json
flows:
  pseudo:
    steps:
      - tool: pseudo-translate
        config:
          expansion_rate: 1.3
`
	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o644))

	proj, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "Test Project", proj.Name)
	assert.Equal(t, "en", proj.SourceLanguage)
	assert.Equal(t, []string{"fr", "de"}, proj.TargetLanguages)
	assert.Len(t, proj.Content, 1)

	spec := proj.GetFlow("pseudo")
	require.NotNil(t, spec)
	assert.Len(t, spec.Steps, 1)
	assert.Equal(t, "pseudo-translate", spec.Steps[0].Tool)
	assert.Equal(t, 1.3, spec.Steps[0].Config["expansion_rate"])
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
			name: "content without path",
			proj: KapiProject{
				Version: "v1",
				Name:    "test",
				Content: []ContentEntry{{Format: "json"}},
			},
			wantErr: "content[0]: path is required",
		},
		{
			name: "flow without steps",
			proj: KapiProject{
				Version: "v1",
				Name:    "test",
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
				Name:    "test",
				Flows: map[string]*flow.StepsSpec{
					"bad": {Steps: []flow.FlowStep{{}}},
				},
			},
			wantErr: `flow "bad" step[0]: tool is required`,
		},
		{
			name: "valid minimal project",
			proj: KapiProject{
				Version: "v1",
				Name:    "test",
			},
		},
		{
			name: "valid with flows",
			proj: KapiProject{
				Version: "v1",
				Name:    "test",
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
		Name:    "test",
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
		Name:    "test",
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
	proj := &KapiProject{Version: "v1", Name: "test"}
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
		Name:    "test",
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
