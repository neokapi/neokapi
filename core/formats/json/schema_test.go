package json

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaMatchesApplyMap verifies that every key accepted by ApplyMap
// has a corresponding entry in the Schema properties, and vice versa.
// This detects drift between the schema and the actual config implementation.
func TestSchemaMatchesApplyMap(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	cfg.Reset()
	s := cfg.Schema()
	require.NotNil(t, s)

	// All ApplyMap keys that Config accepts.
	applyMapKeys := map[string]bool{
		"extract_all_pairs":                true,
		"exceptions":                       true,
		"extract_isolated_strings":         true,
		"extract_array_strings":            true, // alias for extract_isolated_strings
		"extract_non_translatable_content": true,
		"use_key_as_name":                  true,
		"use_full_key_path":                true,
		"use_leading_slash_on_key_path":    true,
		"escape_forward_slashes":           true,
		"subfilters":                       true,
		"subfilter":                        true,
		"subfilter_rules":                  true,
		"note_rules":                       true,
		"id_rules":                         true,
		"use_id_stack":                     true,
		"generic_meta_rules":               true,
		"extraction_rules":                 true,
		"maxwidth_rules":                   true,
		"maxwidth_size_unit":               true,
		"use_code_finder":                  true,
		"code_finder_rules":                true,
	}

	// Every schema property must be accepted by ApplyMap.
	for propName := range s.Properties {
		assert.True(t, applyMapKeys[propName],
			"schema property %q is not accepted by ApplyMap", propName)
	}

	// Every ApplyMap key (except aliases) should have a schema property.
	aliases := map[string]bool{"extract_array_strings": true}
	for key := range applyMapKeys {
		if aliases[key] {
			continue
		}
		_, ok := s.Properties[key]
		assert.True(t, ok, "ApplyMap key %q has no schema property", key)
	}
}

func TestSchemaHasPresets(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	s := cfg.Schema()
	require.NotNil(t, s)
	assert.GreaterOrEqual(t, len(s.Presets), 2,
		"expected at least i18next and chrome-extension presets")
}

func TestSchemaHasGroups(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	s := cfg.Schema()
	require.NotNil(t, s)
	assert.NotEmpty(t, s.Groups)

	// All group fields should reference valid properties.
	for _, g := range s.Groups {
		for _, field := range g.Fields {
			_, ok := s.Properties[field]
			assert.True(t, ok, "group %q references unknown property %q", g.ID, field)
		}
	}
}
