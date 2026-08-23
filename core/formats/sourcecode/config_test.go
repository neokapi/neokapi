package sourcecode_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/sourcecode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The config lives in core precisely so a recipe naming `sourcecode` is
// validated whether or not the plugin is installed. Without that, a typo in a
// node path would only surface on a machine that happened to have the reader.
func TestUnknownParameterIsRefused(t *testing.T) {
	var c sourcecode.Config
	err := c.ApplyMap(map[string]any{"nodePaths": []string{"desc"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nodePaths")
}

func TestNodePathPatternsAcceptsBothShapes(t *testing.T) {
	// A recipe decoded from YAML hands over []any; a caller building the config
	// in Go hands over []string. Both are the same declaration.
	var fromYAML sourcecode.Config
	require.NoError(t, fromYAML.ApplyMap(map[string]any{
		"nodePathPatterns": []any{"desc", "caveats"},
	}))
	assert.Equal(t, []string{"desc", "caveats"}, fromYAML.NodePathPatterns)

	var fromGo sourcecode.Config
	require.NoError(t, fromGo.ApplyMap(map[string]any{
		"nodePathPatterns": []string{"desc"},
	}))
	assert.Equal(t, []string{"desc"}, fromGo.NodePathPatterns)
}

// An empty node path can only come from a typo or a stray comma. Dropping it
// silently would narrow what the collection governs without saying so — the
// failure mode where a check reports a pass over content it never read.
func TestEmptyNodePathIsRefusedRatherThanIgnored(t *testing.T) {
	var c sourcecode.Config
	err := c.ApplyMap(map[string]any{"nodePathPatterns": []any{"desc", ""}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nodePathPatterns[1]")
}

func TestWrongTypesAreRefused(t *testing.T) {
	var c sourcecode.Config
	require.Error(t, c.ApplyMap(map[string]any{"nodePathPatterns": "desc"}))
	require.Error(t, c.ApplyMap(map[string]any{"nodePathPatterns": []any{1}}))
	require.Error(t, c.ApplyMap(map[string]any{"comments": "true"}))
}

func TestResetRestoresDefaults(t *testing.T) {
	c := sourcecode.Config{NodePathPatterns: []string{"desc"}, Comments: true}
	c.Reset()
	assert.Nil(t, c.NodePathPatterns)
	assert.False(t, c.Comments, "comments are opt-in, so the zero value must be off")
	assert.Equal(t, "sourcecode", c.FormatName())
}
