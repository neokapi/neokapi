package manifest_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// A format entry declares the attributes its writer writes and the code
// types it writes as new codes, and the embedded schema describes both.
func TestParseFormatEditDeclarations(t *testing.T) {
	raw := []byte(`{
		"manifest_version": "1",
		"plugin": "okapi-bridge",
		"version": "1.48.0",
		"binary": "kapi-okapi-bridge",
		"capabilities": {
			"formats": [{
				"name": "okf_html",
				"capabilities": ["read", "write"],
				"writable_attrs": {"link:hyperlink": ["href"], "*": ["id"]},
				"synthesizes": ["fmt:bold"]
			}]
		},
		"daemon": {}
	}`)
	m, err := manifest.Parse(raw)
	require.NoError(t, err)
	f := m.Capabilities.Formats[0]
	assert.Equal(t, map[string][]string{"link:hyperlink": {"href"}, "*": {"id"}}, f.WritableAttrs)
	assert.Equal(t, []string{"fmt:bold"}, f.Synthesizes)

	var schema struct {
		Properties struct {
			Capabilities struct {
				Properties struct {
					Formats struct {
						Items struct {
							Properties map[string]json.RawMessage `json:"properties"`
						} `json:"items"`
					} `json:"formats"`
				} `json:"properties"`
			} `json:"capabilities"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(manifest.SchemaJSON(), &schema))
	props := schema.Properties.Capabilities.Properties.Formats.Items.Properties
	assert.Contains(t, props, "writable_attrs")
	assert.Contains(t, props, "synthesizes")
}
