package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The authored corpus must survive untouched, or every run already published stops
// being comparable to anything measured later.
func TestScalingPreservesTheAuthoredCorpus(t *testing.T) {
	t.Parallel()

	base := Corpus()
	assert.Equal(t, base.Digest(), CorpusN(30).Digest(), "the 30-block corpus must keep its digest")
	assert.Equal(t, base.Digest(), CorpusN(10).Digest(), "asking for fewer blocks must not silently invent a new experiment")

	big := CorpusN(500)
	require.Len(t, big.Cases, 500)
	for i, c := range base.Cases {
		assert.Equal(t, c, big.Cases[i], "the authored cases must remain the first cases, unchanged")
	}
	assert.NotEqual(t, base.Digest(), big.Digest(), "a bigger corpus is a different experiment and must say so")
}

// Duplicate source text would let a model answer one segment and copy it into the
// next — manufacturing the very failure the eval hunts for. The only duplicates
// allowed are the deliberate ambiguous ones (same word, different key).
func TestGeneratedBlocksAreDistinct(t *testing.T) {
	t.Parallel()

	c := CorpusN(500)
	keys := map[string]bool{}
	seen := map[string]int{}
	for _, tc := range c.Cases {
		assert.Falsef(t, keys[tc.Key], "duplicate key %q — the key must identify the block", tc.Key)
		keys[tc.Key] = true
		if tc.Kind != "ui-ambiguous" {
			seen[tc.Source]++
		}
	}
	for src, n := range seen {
		assert.LessOrEqualf(t, n, 1, "source text repeated %d times: %q", n, src)
	}
}

// A corpus of nothing but short strings would flatter every N. The mix must hold as
// it scales — long prose above all, since that is where batching is documented to
// hurt most.
func TestScaledCorpusKeepsTheStressorMix(t *testing.T) {
	t.Parallel()

	c := CorpusN(500)
	kinds := map[string]int{}
	longest := 0
	for _, tc := range c.Cases {
		kinds[tc.Kind]++
		if len(tc.Source) > longest {
			longest = len(tc.Source)
		}
	}
	for _, k := range []string{"ui", "ui-ambiguous", "placeholder", "inline-tag", "prose"} {
		assert.Greaterf(t, kinds[k], 20, "too few %q blocks at scale — degradation the corpus cannot express reads as zero", k)
	}
	assert.Greater(t, longest, 200, "long items must survive scaling")
	assert.Greater(t, c.Words(), 3000, "a 500-block corpus must carry real volume")
}
