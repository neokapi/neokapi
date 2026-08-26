package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The prior version is reference material, and reference material has three
// obligations: it must reach the model, it must be marked as not-an-answer, and
// it must be in the cache key. The third is the one that is easy to forget and
// expensive to discover.

func TestPriorVersionReachesThePrompt(t *testing.T) {
	t.Parallel()

	c := Context{
		Key: "app.onboarding.cta",
		Prior: &PriorVersion{
			Source: "Get started",
			Target: "Kom i gang",
		},
	}
	require.False(t, c.Empty(), "a block with history has something to say about it")

	var found *Section
	for i, s := range c.sections() {
		if strings.Contains(s.Origin, "content memory") {
			found = &c.sections()[i]
		}
	}
	require.NotNil(t, found, "the previous answer is sent")
	assert.Contains(t, found.Text, "Get started")
	assert.Contains(t, found.Text, "Kom i gang")
	assert.Contains(t, strings.ToLower(found.Heading), "do not return",
		"reference the model reads, not an answer it echoes")
}

func TestPriorVersionIsInTheCacheKey(t *testing.T) {
	t.Parallel()

	base := Context{Key: "app.onboarding.cta"}
	withPrior := Context{
		Key:   "app.onboarding.cta",
		Prior: &PriorVersion{Source: "Get started", Target: "Kom i gang"},
	}
	moved := Context{
		Key:   "app.onboarding.cta",
		Prior: &PriorVersion{Source: "Get started", Target: "Sett i gang"},
	}

	// Key is deliberately excluded from the digest because it travels with the
	// block. A prior version does not: it is a statement about history that can
	// move while the block's own text stands still, so a translation produced
	// under one must not be served after the chain moves.
	assert.Empty(t, base.Digest(), "nothing but the block itself was said")
	assert.NotEmpty(t, withPrior.Digest())
	assert.NotEqual(t, withPrior.Digest(), moved.Digest(),
		"a different previous answer is a different prompt")
}

func TestPriorVersionNeedsBothHalves(t *testing.T) {
	t.Parallel()

	// A target with no source is an anchor with no explanation; a source with no
	// target teaches the model wording it must not reuse. Either alone is worse
	// than neither, so neither is sent.
	for name, prior := range map[string]*PriorVersion{
		"nothing":      nil,
		"no source":    {Target: "Kom i gang"},
		"no target":    {Source: "Get started"},
		"only blanks":  {Source: "   ", Target: "\t"},
		"blank target": {Source: "Get started", Target: " "},
		"blank source": {Source: " ", Target: "Kom i gang"},
	} {
		t.Run(name, func(t *testing.T) {
			c := Context{Prior: prior}
			assert.True(t, c.Empty(), "half a version is not reference")
			assert.Empty(t, c.Digest(), "and it does not move the cache key")
			for _, s := range c.sections() {
				assert.NotContains(t, s.Origin, "content memory")
			}
		})
	}
}
