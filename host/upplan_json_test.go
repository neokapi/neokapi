package host

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpPlanScope_JSONNamesTheContentMemory pins the `kapi up --plan --json`
// key for exact content-memory hits: `memoryExact`, as the CLI contract
// records it.
func TestUpPlanScope_JSONNamesTheContentMemory(t *testing.T) {
	raw, err := json.Marshal(UpPlanScope{Locale: "fr", MemoryExact: 4})
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	assert.Equal(t, float64(4), doc["memoryExact"])
	assert.NotContains(t, doc, "tmExact")
}
