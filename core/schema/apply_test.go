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

func TestApplyConfigReadsSnakeCaseKeys(t *testing.T) {
	type rule struct {
		Pattern string
		IsRegex bool
	}
	type cfg struct {
		FillTarget          bool
		FillTargetThreshold int `json:"fill_target_threshold"`
		TextUnitIDs         []string
		Rules               []rule
	}

	var c cfg
	require.NoError(t, ApplyConfig(map[string]any{
		"fill_target":           true,
		"fill_target_threshold": 95,
		"text_unit_ids":         []any{"a"},
		"rules":                 []any{map[string]any{"pattern": "x", "is_regex": true}},
	}, &c))
	assert.True(t, c.FillTarget)
	assert.Equal(t, 95, c.FillTargetThreshold)
	assert.Equal(t, []string{"a"}, c.TextUnitIDs)
	assert.Equal(t, []rule{{Pattern: "x", IsRegex: true}}, c.Rules)

	// A camelCase or Go-cased spelling names no field.
	var old cfg
	require.NoError(t, ApplyConfig(map[string]any{"fillTarget": true, "FillTarget": true, "fillTargetThreshold": 95}, &old))
	assert.Equal(t, cfg{}, old)
}

func TestRenamedKeyError(t *testing.T) {
	type cfg struct {
		FillTargetThreshold int
		Rules               []struct{ IsRegex bool }
	}
	props := FromStruct(&cfg{}, ToolMeta{ID: "t"}).Properties

	err := RenamedKeyError(props, map[string]any{"fillTargetThreshold": 100}, "flows.translate.steps[1].config")
	require.Error(t, err)
	assert.Equal(t, "flows.translate.steps[1].config: fillTargetThreshold is now fill_target_threshold", err.Error())

	err = RenamedKeyError(props, map[string]any{"rules": []any{map[string]any{"isRegex": true}}}, "c")
	require.Error(t, err)
	assert.Equal(t, "c.rules[0]: isRegex is now is_regex", err.Error())

	assert.NoError(t, RenamedKeyError(props, map[string]any{"fill_target_threshold": 100, "onProgress": 1}, "c"))
}
