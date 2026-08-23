package pluginhost

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A plugin format's config crosses the boundary as flat strings, so how a
// LIST is spelled there is the whole contract for any config that has one.
//
// fmt.Sprint renders a slice as Go's debug form — `[desc caveats]` — which no
// plugin can parse back. The sourcecode format's nodePathPatterns was the first
// list config to cross here, and it failed in the worst way available: the
// daemon parsed the rendering into a single nonsense entry, matched nothing,
// and returned zero blocks with no error. A check reported a pass over content
// it had never read.
func TestListConfigCrossesAsJSON(t *testing.T) {
	c := &mapConfig{format: "sourcecode", params: map[string]string{}}
	require.NoError(t, c.ApplyMap(map[string]any{
		"nodePathPatterns": []any{"desc", "caveats"},
	}))
	assert.JSONEq(t, `["desc","caveats"]`, c.params["nodePathPatterns"])

	// []string is the same declaration arriving from Go rather than from YAML.
	c.Reset()
	require.NoError(t, c.ApplyMap(map[string]any{"nodePathPatterns": []string{"desc"}}))
	assert.JSONEq(t, `["desc"]`, c.params["nodePathPatterns"])
}

// Scalars keep their existing spelling exactly. An okapi-bridge filter
// parameter reading "true" or "42" must not start seeing a quoted JSON string
// because a sibling key happened to be a list.
func TestScalarConfigKeepsItsBareSpelling(t *testing.T) {
	c := &mapConfig{format: "pdf", params: map[string]string{}}
	require.NoError(t, c.ApplyMap(map[string]any{
		"geometry": true,
		"glyphs":   false,
		"pages":    42,
		"mode":     "tier3",
	}))

	assert.Equal(t, "true", c.params["geometry"])
	assert.Equal(t, "false", c.params["glyphs"])
	assert.Equal(t, "42", c.params["pages"])
	assert.Equal(t, "tier3", c.params["mode"], "a string stays bare, not JSON-quoted")
}

func TestResetClearsParams(t *testing.T) {
	c := &mapConfig{format: "sourcecode", params: map[string]string{"stale": "1"}}
	c.Reset()
	assert.Empty(t, c.params)
	assert.Equal(t, "sourcecode", c.FormatName())
	assert.NoError(t, c.Validate())
}
