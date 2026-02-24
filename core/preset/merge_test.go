package preset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepMerge(t *testing.T) {
	tests := []struct {
		name     string
		dst      map[string]any
		src      map[string]any
		expected map[string]any
	}{
		{
			name:     "simple scalar override",
			dst:      map[string]any{"a": 1, "b": 2},
			src:      map[string]any{"b": 3},
			expected: map[string]any{"a": 1, "b": 3},
		},
		{
			name: "nested map merging",
			dst: map[string]any{
				"top": map[string]any{"a": 1, "b": 2},
			},
			src: map[string]any{
				"top": map[string]any{"b": 3, "c": 4},
			},
			expected: map[string]any{
				"top": map[string]any{"a": 1, "b": 3, "c": 4},
			},
		},
		{
			name:     "nil dst",
			dst:      nil,
			src:      map[string]any{"a": 1},
			expected: map[string]any{"a": 1},
		},
		{
			name:     "nil src",
			dst:      map[string]any{"a": 1},
			src:      nil,
			expected: map[string]any{"a": 1},
		},
		{
			name:     "both nil",
			dst:      nil,
			src:      nil,
			expected: map[string]any{},
		},
		{
			name:     "slice replacement not merging",
			dst:      map[string]any{"tags": []any{"a", "b"}},
			src:      map[string]any{"tags": []any{"c"}},
			expected: map[string]any{"tags": []any{"c"}},
		},
		{
			name: "src map replaces dst scalar",
			dst:  map[string]any{"x": "scalar"},
			src:  map[string]any{"x": map[string]any{"nested": true}},
			expected: map[string]any{
				"x": map[string]any{"nested": true},
			},
		},
		{
			name: "src scalar replaces dst map",
			dst:  map[string]any{"x": map[string]any{"nested": true}},
			src:  map[string]any{"x": "scalar"},
			expected: map[string]any{
				"x": "scalar",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeepMerge(tt.dst, tt.src)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeepMerge_DoesNotMutateSrc(t *testing.T) {
	dst := map[string]any{"a": 1}
	src := map[string]any{"b": map[string]any{"c": 2}}
	result := DeepMerge(dst, src)

	// Mutate result
	result["b"].(map[string]any)["c"] = 99

	// src must be unchanged
	assert.Equal(t, 2, src["b"].(map[string]any)["c"])
}

func TestDeepMerge_DoesNotMutateDst(t *testing.T) {
	dst := map[string]any{"a": map[string]any{"b": 1}}
	src := map[string]any{"a": map[string]any{"c": 2}}
	result := DeepMerge(dst, src)

	// Mutate result
	result["a"].(map[string]any)["b"] = 99

	// dst must be unchanged
	assert.Equal(t, 1, dst["a"].(map[string]any)["b"])
}

func TestMergeConfig(t *testing.T) {
	tests := []struct {
		name      string
		defaults  map[string]any
		preset    map[string]any
		overrides map[string]any
		expected  map[string]any
	}{
		{
			name:      "three layer merge",
			defaults:  map[string]any{"a": 1, "b": 2, "c": 3},
			preset:    map[string]any{"b": 20, "d": 40},
			overrides: map[string]any{"c": 300},
			expected:  map[string]any{"a": 1, "b": 20, "c": 300, "d": 40},
		},
		{
			name: "nested three layer merge",
			defaults: map[string]any{
				"fmt": map[string]any{"inline": true, "wrap": 80},
			},
			preset: map[string]any{
				"fmt": map[string]any{"wrap": 120, "escape": false},
			},
			overrides: map[string]any{
				"fmt": map[string]any{"inline": false},
			},
			expected: map[string]any{
				"fmt": map[string]any{"inline": false, "wrap": 120, "escape": false},
			},
		},
		{
			name:      "all nil",
			defaults:  nil,
			preset:    nil,
			overrides: nil,
			expected:  map[string]any{},
		},
		{
			name:      "nil defaults and preset",
			defaults:  nil,
			preset:    nil,
			overrides: map[string]any{"a": 1},
			expected:  map[string]any{"a": 1},
		},
		{
			name:      "nil overrides",
			defaults:  map[string]any{"a": 1},
			preset:    map[string]any{"b": 2},
			overrides: nil,
			expected:  map[string]any{"a": 1, "b": 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeConfig(tt.defaults, tt.preset, tt.overrides)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeepCopy(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		assert.Nil(t, DeepCopy(nil))
	})

	t.Run("does not share references", func(t *testing.T) {
		original := map[string]any{
			"nested": map[string]any{"key": "value"},
			"slice":  []any{1, 2, 3},
		}
		copied := DeepCopy(original)
		require.Equal(t, original, copied)

		// Mutate the copy
		copied["nested"].(map[string]any)["key"] = "changed"
		copied["slice"].([]any)[0] = 99

		// Original must be unchanged
		assert.Equal(t, "value", original["nested"].(map[string]any)["key"])
		assert.Equal(t, 1, original["slice"].([]any)[0])
	})

	t.Run("deeply nested", func(t *testing.T) {
		original := map[string]any{
			"l1": map[string]any{
				"l2": map[string]any{
					"l3": []any{"a", "b"},
				},
			},
		}
		copied := DeepCopy(original)
		copied["l1"].(map[string]any)["l2"].(map[string]any)["l3"].([]any)[0] = "z"
		assert.Equal(t, "a", original["l1"].(map[string]any)["l2"].(map[string]any)["l3"].([]any)[0])
	})
}
