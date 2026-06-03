package odf

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Config must satisfy the optional SchemaProvider interface so the CLI/UI
// and reference docs can introspect its parameters.
var _ format.SchemaProvider = (*Config)(nil)

// TestSchemaMatchesApplyMap verifies that the schema exposes exactly the keys
// accepted by ApplyMap — no more, no less. This keeps CLI introspection,
// schema-based validation, and reference docs in sync with the config.
func TestSchemaMatchesApplyMap(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()
	s := cfg.Schema()
	require.NotNil(t, s)

	// Keys accepted by ApplyMap.
	applyMapKeys := map[string]bool{
		"translateNotes":         true,
		"translateHiddenContent": true,
	}

	// Every schema property must be accepted by ApplyMap.
	for propName := range s.Properties {
		assert.True(t, applyMapKeys[propName],
			"schema property %q is not accepted by ApplyMap", propName)
		// Each accepted key should round-trip through ApplyMap without error.
		require.NoError(t, cfg.ApplyMap(map[string]any{propName: true}),
			"ApplyMap rejected schema key %q", propName)
	}

	// Every ApplyMap key must have a schema property.
	for key := range applyMapKeys {
		_, ok := s.Properties[key]
		assert.True(t, ok, "ApplyMap key %q has no schema property", key)
	}
}

// TestSchemaDefaultsMatchReset verifies the schema defaults agree with the
// values produced by Config.Reset, so reference docs reflect real behavior.
func TestSchemaDefaultsMatchReset(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()
	s := cfg.Schema()
	require.NotNil(t, s)

	assert.Equal(t, cfg.TranslateNotes, s.Properties["translateNotes"].Default,
		"translateNotes default diverges from Reset")
	assert.Equal(t, cfg.TranslateHiddenContent, s.Properties["translateHiddenContent"].Default,
		"translateHiddenContent default diverges from Reset")
}

// TestSchemaGroupsReferenceKnownFields verifies every group field references a
// declared property and that FormatMeta carries the format identity.
func TestSchemaGroupsReferenceKnownFields(t *testing.T) {
	cfg := &Config{}
	s := cfg.Schema()
	require.NotNil(t, s)

	assert.Equal(t, "odf", s.FormatMeta.ID)
	assert.NotEmpty(t, s.FormatMeta.Extensions)
	assert.NotEmpty(t, s.FormatMeta.MimeTypes)

	require.NotEmpty(t, s.Groups)
	for _, g := range s.Groups {
		for _, field := range g.Fields {
			_, ok := s.Properties[field]
			assert.True(t, ok, "group %q references unknown property %q", g.ID, field)
		}
	}
}
