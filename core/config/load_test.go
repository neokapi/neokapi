package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_YAML(t *testing.T) {
	data := []byte(`apiVersion: gokapi/html-v1
kind: FormatConfig
metadata:
  name: my-html-config
  description: "Custom HTML extraction rules"
spec:
  parser:
    preserveWhitespace: true
  elements:
    pre:
      ruleTypes: [EXCLUDE]
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)

	assert.Equal(t, "gokapi/html-v1", env.APIVersion)
	assert.Equal(t, KindFormatConfig, env.Kind)
	assert.Equal(t, "my-html-config", env.Metadata.Name)
	assert.Equal(t, "Custom HTML extraction rules", env.Metadata.Description)
	assert.NotNil(t, env.Spec)

	parser, ok := env.Spec["parser"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, parser["preserveWhitespace"])
}

func TestParse_JSON(t *testing.T) {
	data := []byte(`{
		"apiVersion": "gokapi/json-v1",
		"kind": "FormatConfig",
		"metadata": {"name": "json-config"},
		"spec": {"extractAllPairs": true}
	}`)
	env, err := Parse(data, ".json")
	require.NoError(t, err)

	assert.Equal(t, "gokapi/json-v1", env.APIVersion)
	assert.Equal(t, KindFormatConfig, env.Kind)
	assert.Equal(t, "json-config", env.Metadata.Name)
	assert.Equal(t, true, env.Spec["extractAllPairs"])
}

func TestParse_MissingAPIVersion(t *testing.T) {
	data := []byte(`kind: FormatConfig
metadata:
  name: test
spec: {}
`)
	_, err := Parse(data, ".yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiVersion")
}

func TestParse_MissingKind(t *testing.T) {
	data := []byte(`apiVersion: gokapi/html-v1
metadata:
  name: test
spec: {}
`)
	_, err := Parse(data, ".yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind")
}

func TestParse_InvalidAPIVersion(t *testing.T) {
	data := []byte(`apiVersion: bad-version
kind: FormatConfig
metadata:
  name: test
spec: {}
`)
	_, err := Parse(data, ".yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace")
}

func TestParse_InvalidKind(t *testing.T) {
	data := []byte(`apiVersion: gokapi/html-v1
kind: UnknownKind
metadata:
  name: test
spec: {}
`)
	_, err := Parse(data, ".yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown kind")
}

func TestParse_FlowDefinition(t *testing.T) {
	data := []byte(`apiVersion: gokapi/flow-v1
kind: FlowDefinition
metadata:
  name: pseudo-translate
  description: "Generate pseudo-translations for testing"
spec:
  nodes:
    - id: pseudo
      tool: pseudo-translate
  edges: []
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)
	assert.Equal(t, KindFlowDefinition, env.Kind)
	assert.Equal(t, "pseudo-translate", env.Metadata.Name)
}

func TestParse_ProjectConfig(t *testing.T) {
	data := []byte(`apiVersion: gokapi/project-v1
kind: ProjectConfig
metadata:
  name: my-app
spec:
  project:
    name: my-app
    source_locale: en
    target_locales: [fr, de]
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)
	assert.Equal(t, KindProjectConfig, env.Kind)
	assert.Equal(t, "my-app", env.Metadata.Name)
}

func TestParse_FormatPreset(t *testing.T) {
	data := []byte(`apiVersion: gokapi/preset-v1
kind: FormatPreset
metadata:
  name: i18next
  description: "i18next JSON format"
spec:
  format: json
  isDefault: false
  parameters:
    useFullKeyPath: true
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)
	assert.Equal(t, KindFormatPreset, env.Kind)
	assert.Equal(t, "i18next", env.Metadata.Name)
}

func TestParse_MetadataLabelsAndAnnotations(t *testing.T) {
	data := []byte(`apiVersion: gokapi/html-v1
kind: FormatConfig
metadata:
  name: test
  labels:
    env: production
    team: l10n
  annotations:
    source: generated
spec: {}
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)
	assert.Equal(t, "production", env.Metadata.Labels["env"])
	assert.Equal(t, "l10n", env.Metadata.Labels["team"])
	assert.Equal(t, "generated", env.Metadata.Annotations["source"])
}

func TestParse_OkapiNamespace(t *testing.T) {
	data := []byte(`apiVersion: okapi/html-v1
kind: FormatConfig
metadata:
  name: well-formed
spec:
  parser:
    preserveWhitespace: true
    assumeWellformed: true
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)
	assert.Equal(t, "okapi/html-v1", env.APIVersion)
}

func TestLoad_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(`apiVersion: gokapi/html-v1
kind: FormatConfig
metadata:
  name: test
spec:
  parser:
    preserveWhitespace: true
`), 0o644)
	require.NoError(t, err)

	env, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "gokapi/html-v1", env.APIVersion)
	assert.Equal(t, KindFormatConfig, env.Kind)
}

func TestLoad_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	err := os.WriteFile(path, []byte(`{
		"apiVersion": "gokapi/json-v1",
		"kind": "FormatConfig",
		"metadata": {"name": "test"},
		"spec": {"extractAllPairs": false}
	}`), 0o644)
	require.NoError(t, err)

	env, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "gokapi/json-v1", env.APIVersion)
	assert.Equal(t, false, env.Spec["extractAllPairs"])
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	require.Error(t, err)
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := Parse([]byte(":::invalid"), ".yaml")
	require.Error(t, err)
}

func TestParse_InvalidJSON(t *testing.T) {
	_, err := Parse([]byte("{invalid json}"), ".json")
	require.Error(t, err)
}
