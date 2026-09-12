package check

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextTargetContextPathIsNotAFileClaim(t *testing.T) {
	target := Target{Kind: "text", Blocks: 1, ContextPath: "docs/new-page.json"}
	body, err := json.Marshal(target)
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"text","blocks":1,"context_path":"docs/new-page.json"}`, string(body))
	var restored Target
	require.NoError(t, json.Unmarshal(body, &restored))
	assert.Equal(t, target, restored)
	body, err = json.Marshal(Target{Kind: "file", File: "docs/page.json", Blocks: 1})
	require.NoError(t, err)
	assert.NotContains(t, string(body), "context_path")
}
