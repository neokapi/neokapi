package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyConfig(t *testing.T) {
	type cfg struct {
		Prefix string `json:"prefix"`
		Count  int    `json:"count"`
	}

	t.Run("populates from map", func(t *testing.T) {
		var c cfg
		require.NoError(t, ApplyConfig(map[string]any{"prefix": "» ", "count": 3}, &c))
		assert.Equal(t, "» ", c.Prefix)
		assert.Equal(t, 3, c.Count)
	})

	// The CLI injects sidecar callbacks (e.g. onProgress) into the config map
	// for tools that consume them; tools decoding via ApplyConfig must ignore
	// such non-serializable values rather than fail the JSON round-trip.
	t.Run("ignores func and chan sidecar values", func(t *testing.T) {
		var c cfg
		config := map[string]any{
			"prefix":     "x",
			"onProgress": func(int) {},
			"events":     make(chan int),
		}
		require.NoError(t, ApplyConfig(config, &c))
		assert.Equal(t, "x", c.Prefix)
		// The original map is left untouched (we copy before stripping).
		assert.Contains(t, config, "onProgress")
		assert.Contains(t, config, "events")
	})
}
