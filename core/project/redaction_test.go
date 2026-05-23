package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRedactionSpec_Validate(t *testing.T) {
	p := &KapiProject{
		Version:  "v1",
		Name:     "app",
		Defaults: Defaults{Redaction: &RedactionSpec{Enabled: true, Rules: ".kapi/redaction.yaml", Detectors: []string{"rules", "entities"}}},
	}
	require.NoError(t, p.Validate())

	p.Defaults.Redaction.Detectors = []string{"bogus"}
	assert.Error(t, p.Validate())
}

func TestRedactionSpec_YAMLRoundtrip(t *testing.T) {
	const doc = `
version: v1
name: app
defaults:
  source_language: en
  target_languages: [fr]
  redaction:
    enabled: true
    rules: .kapi/redaction.yaml
    detectors: [rules]
    placeholder: "[REDACTED:{category}]"
`
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(doc), &p))
	require.NoError(t, p.Validate())
	require.NotNil(t, p.Defaults.Redaction)
	assert.True(t, p.Defaults.Redaction.Enabled)
	assert.Equal(t, ".kapi/redaction.yaml", p.Defaults.Redaction.Rules)
	assert.Equal(t, []string{"rules"}, p.Defaults.Redaction.Detectors)
}
