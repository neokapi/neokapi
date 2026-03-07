package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_YAML(t *testing.T) {
	data := []byte(`apiVersion: v1
kind: HtmlFormatConfig
metadata:
  name: test-config
spec:
  parser:
    preserveWhitespace: true
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)

	assert.Equal(t, "v1", env.APIVersion)
	assert.Equal(t, Kind("HtmlFormatConfig"), env.Kind)
	assert.Equal(t, "test-config", env.Metadata.Name)
	assert.NotNil(t, env.Spec)
	parser := env.Spec["parser"].(map[string]any)
	assert.Equal(t, true, parser["preserveWhitespace"])
}

func TestParse_JSON(t *testing.T) {
	data := []byte(`{
		"apiVersion": "v1",
		"kind": "JsonFormatConfig",
		"metadata": {"name": "json-test"},
		"spec": {"extractAllPairs": true}
	}`)
	env, err := Parse(data, ".json")
	require.NoError(t, err)

	assert.Equal(t, "v1", env.APIVersion)
	assert.Equal(t, Kind("JsonFormatConfig"), env.Kind)
	assert.Equal(t, true, env.Spec["extractAllPairs"])
}

func TestParse_MissingAPIVersion(t *testing.T) {
	data := []byte(`kind: HtmlFormatConfig
spec: {}
`)
	_, err := Parse(data, ".yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiVersion")
}

func TestParse_MissingKind(t *testing.T) {
	data := []byte(`apiVersion: v1
spec: {}
`)
	_, err := Parse(data, ".yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind")
}

func TestParse_InvalidAPIVersion(t *testing.T) {
	data := []byte(`apiVersion: invalid
kind: HtmlFormatConfig
spec: {}
`)
	_, err := Parse(data, ".yaml")
	require.Error(t, err)
}

func TestParse_InvalidKind(t *testing.T) {
	data := []byte(`apiVersion: v1
kind: UnknownThing
spec: {}
`)
	_, err := Parse(data, ".yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UnknownThing")
}

func TestParse_FlowDefinition(t *testing.T) {
	data := []byte(`apiVersion: v1
kind: FlowDefinition
metadata:
  name: test-flow
spec:
  id: test
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)
	assert.Equal(t, KindFlowDefinition, env.Kind)
}

func TestParse_ProjectConfig(t *testing.T) {
	data := []byte(`apiVersion: v1
kind: ProjectConfig
metadata:
  name: test-project
spec:
  project:
    name: Test
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)
	assert.Equal(t, KindProjectConfig, env.Kind)
}

func TestParse_FormatPreset(t *testing.T) {
	data := []byte(`apiVersion: v1
kind: FormatPreset
metadata:
  name: i18next
spec:
  format: json
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)
	assert.Equal(t, KindFormatPreset, env.Kind)
}

func TestParse_MetadataLabelsAndAnnotations(t *testing.T) {
	data := []byte(`apiVersion: v1
kind: HtmlFormatConfig
metadata:
  name: test
  description: "A test config"
  labels:
    env: prod
  annotations:
    source: generated
spec: {}
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)
	assert.Equal(t, "A test config", env.Metadata.Description)
	assert.Equal(t, "prod", env.Metadata.Labels["env"])
	assert.Equal(t, "generated", env.Metadata.Annotations["source"])
}

func TestParse_OkapiKind(t *testing.T) {
	data := []byte(`apiVersion: v1
kind: OkfHtmlFilterConfig
metadata:
  name: okapi-html
spec:
  quoteMode: 3
`)
	env, err := Parse(data, ".yaml")
	require.NoError(t, err)
	assert.Equal(t, Kind("OkfHtmlFilterConfig"), env.Kind)
}

func TestLoad_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	err := os.WriteFile(path, []byte(`apiVersion: v1
kind: HtmlFormatConfig
metadata:
  name: test
spec:
  parser:
    preserveWhitespace: true
`), 0644)
	require.NoError(t, err)

	env, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "v1", env.APIVersion)
	assert.Equal(t, Kind("HtmlFormatConfig"), env.Kind)
}

func TestLoad_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	err := os.WriteFile(path, []byte(`{
		"apiVersion": "v1",
		"kind": "JsonFormatConfig",
		"metadata": {"name": "test"},
		"spec": {}
	}`), 0644)
	require.NoError(t, err)

	env, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "v1", env.APIVersion)
	assert.Equal(t, Kind("JsonFormatConfig"), env.Kind)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path.yaml")
	assert.Error(t, err)
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := Parse([]byte(":::invalid:::"), ".yaml")
	assert.Error(t, err)
}

func TestParse_InvalidJSON(t *testing.T) {
	_, err := Parse([]byte("{invalid}"), ".json")
	assert.Error(t, err)
}
