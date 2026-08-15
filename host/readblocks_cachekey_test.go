package host

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The read path's document-cache key has to separate two reads of the same file
// that ask different questions. A key that collides serves one run's blocks as
// the other's, and the failure is invisible: the blocks look like a real parse,
// because they were one — of a different configuration.
func TestReadBlocksConfigKey_SeparatesConfigurations(t *testing.T) {
	withPatterns := map[string]any{"keyPathPatterns": []string{"title", "narration.*.text"}}
	withoutPatterns := map[string]any(nil)

	assert.NotEqual(t,
		readBlocksConfigKey("yaml", withPatterns, "en"),
		readBlocksConfigKey("yaml", withoutPatterns, "en"),
		"the same YAML read with and without keyPathPatterns yields different blocks")

	assert.NotEqual(t,
		readBlocksConfigKey("yaml", withPatterns, "en"),
		readBlocksConfigKey("yaml", map[string]any{"keyPathPatterns": []string{"title"}}, "en"),
		"a narrower pattern set is a different question")

	assert.NotEqual(t,
		readBlocksConfigKey("mdx", nil, "en"),
		readBlocksConfigKey("markdown", nil, "en"),
		"the format still separates keys")

	assert.NotEqual(t,
		readBlocksConfigKey("yaml", nil, "en"),
		readBlocksConfigKey("yaml", nil, "nb"),
		"the source language still separates keys")
}

// Go randomizes map iteration order, so a digest that walked the map as it came
// would give one config two keys and turn every read into a miss — a cache that
// silently never fills, which presents as nothing but an inexplicably slow
// project.
func TestFormatConfigDigest_IsStableAcrossIterationOrder(t *testing.T) {
	cfg := map[string]any{
		"translateFrontMatter": true,
		"frontMatterKeys":      []string{"title", "description", "sidebar_label"},
		"extractAllPairs":      false,
		"useKeyAsName":         true,
		"extractionRules":      "(displayName|description|title|label)$",
	}
	want := formatConfigDigest(cfg)
	for range 50 {
		assert.Equal(t, want, formatConfigDigest(cfg))
	}

	// An equal config built as a separate map digests the same.
	same := map[string]any{}
	maps.Copy(same, cfg)
	assert.Equal(t, want, formatConfigDigest(same))

	assert.Empty(t, formatConfigDigest(nil), "no config is the reader's defaults")
	assert.Empty(t, formatConfigDigest(map[string]any{}))
}

// A value change has to move the digest, not only a key change — the config that
// selects one key path and the config that selects another share every key.
func TestFormatConfigDigest_TracksValues(t *testing.T) {
	assert.NotEqual(t,
		formatConfigDigest(map[string]any{"keyPathPatterns": []string{"title"}}),
		formatConfigDigest(map[string]any{"keyPathPatterns": []string{"command"}}))

	assert.NotEqual(t,
		formatConfigDigest(map[string]any{"translateFrontMatter": true}),
		formatConfigDigest(map[string]any{"translateFrontMatter": false}))
}
