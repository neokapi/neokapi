package format_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyMapViaJSON(t *testing.T) {
	t.Parallel()

	type cfg struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Count   int    `json:"count"`
	}

	t.Run("applies values", func(t *testing.T) {
		t.Parallel()
		c := &cfg{Name: "default", Enabled: false, Count: 0}
		err := format.ApplyMapViaJSON(c, map[string]any{
			"name":    "updated",
			"enabled": true,
			"count":   42,
		})
		require.NoError(t, err)
		assert.Equal(t, "updated", c.Name)
		assert.True(t, c.Enabled)
		assert.Equal(t, 42, c.Count)
	})

	t.Run("partial update preserves existing", func(t *testing.T) {
		t.Parallel()
		c := &cfg{Name: "original", Enabled: true, Count: 10}
		err := format.ApplyMapViaJSON(c, map[string]any{
			"count": 20,
		})
		require.NoError(t, err)
		assert.Equal(t, "original", c.Name)
		assert.True(t, c.Enabled)
		assert.Equal(t, 20, c.Count)
	})

	t.Run("unknown keys produce error", func(t *testing.T) {
		t.Parallel()
		c := &cfg{}
		err := format.ApplyMapViaJSON(c, map[string]any{
			"name":    "test",
			"unknown": "bad",
		})
		assert.Error(t, err)
	})

	t.Run("wrong type produces error", func(t *testing.T) {
		t.Parallel()
		c := &cfg{}
		err := format.ApplyMapViaJSON(c, map[string]any{
			"enabled": "not-a-bool",
		})
		assert.Error(t, err)
	})
}
