package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A renamed tool id fails with the id that replaced it; a current id and an
// unknown one carry no pointer.
func TestRenamedToolError(t *testing.T) {
	err := RenamedToolError("source-gate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `tool "source-gate" is now "translate-after"`)

	assert.NoError(t, RenamedToolError("translate-after"))
	assert.NoError(t, RenamedToolError("frobnicate"))
}

func TestNewTool_RenamedIDNamesItsReplacement(t *testing.T) {
	reg := NewToolRegistry()
	_, err := reg.NewTool("source-gate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `is now "translate-after"`)

	_, err = reg.NewToolWithConfig("source-gate", nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `is now "translate-after"`)

	_, err = reg.NewTool("frobnicate")
	require.EqualError(t, err, "unknown tool: frobnicate")
}
