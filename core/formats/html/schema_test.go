package html

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaMatchesApplyMap verifies that every flat parameter accepted by
// ApplyMap has a corresponding entry in the schema's FlatProperties, and
// vice versa. The schema uses a hierarchical structure (e.g., parser.preserveWhitespace)
// that maps to flat ApplyMap keys via x-flattenPath.
func TestSchemaMatchesApplyMap(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()
	s := cfg.Schema()
	require.NotNil(t, s)

	// All flat parameter names accepted by the config. "preserveWhitespace"
	// lives under the "parser" section but its flat name is still used here
	// because the schema uses x-flattenPath to map it.
	applyMapKeys := map[string]bool{
		"preserveWhitespace":            true,
		"extractNonTranslatableContent": true,
		"useCodeFinder":                 true,
		"codeFinderRules":               true,
		"elements":                      true,
		"attributes":                    true,
	}

	// Build flat properties the same way SchemaRegistry.RegisterSchema does.
	flatProps := make(map[string]bool)
	for sectionKey, section := range s.Properties {
		if section.Type == "object" && len(section.Properties) > 0 {
			for _, prop := range section.Properties {
				if prop.FlattenPath != "" {
					flatProps[prop.FlattenPath] = true
				}
			}
		} else if section.FlattenPath != "" {
			flatProps[section.FlattenPath] = true
		} else {
			flatProps[sectionKey] = true
		}
	}

	// Every flat schema property must be accepted by ApplyMap.
	for propName := range flatProps {
		assert.True(t, applyMapKeys[propName],
			"schema flat property %q is not accepted by ApplyMap", propName)
	}

	// Every ApplyMap key should have a schema flat property.
	for key := range applyMapKeys {
		assert.True(t, flatProps[key],
			"ApplyMap key %q has no schema flat property", key)
	}
}

// TestApplyMapHierarchicalParser verifies that parser settings are applied
// via the hierarchical "parser" section, matching the okf_html config structure.
func TestApplyMapHierarchicalParser(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	// Bridge-style hierarchical params.
	err := cfg.ApplyMap(map[string]any{
		"parser": map[string]any{
			"preserveWhitespace": true,
		},
	})
	require.NoError(t, err)
	assert.True(t, cfg.PreserveWhitespace)
}

func TestSchemaHasGroups(t *testing.T) {
	cfg := &Config{}
	s := cfg.Schema()
	require.NotNil(t, s)
	assert.Len(t, s.Groups, 3)

	// All group fields should reference valid top-level properties.
	for _, g := range s.Groups {
		for _, field := range g.Fields {
			_, ok := s.Properties[field]
			assert.True(t, ok, "group %q references unknown property %q", g.ID, field)
		}
	}
}
