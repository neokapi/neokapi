package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storedBlock is a small helper to build a StoredBlock around a source-only
// model.Block, mirroring what the content store hands the worker.
func storedBlock(id, source string) *store.StoredBlock {
	b := model.NewBlock(id, source)
	b.SourceLocale = "en"
	return &store.StoredBlock{Block: b}
}

// seedMemoryEntry adds a single (en→fr) exact pair to the in-memory content memory.
func seedMemoryEntry(t *testing.T, tm memory.Store, source, target string) {
	t.Helper()
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "seed-" + source,
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: source}}},
			"fr": {{Text: &model.TextRun{Text: target}}},
		},
		HintSrcLang: "en",
	}))
}

// TestRecycleBlocks_FillsFromMemoryAndLeavesRemainder proves the content memory-first split
// (verification (a)): a convergence recycle pass fills the blocks that have an
// exact content-memory match and leaves only the genuinely-new blocks for AI.
func TestRecycleBlocks_FillsFromMemoryAndLeavesRemainder(t *testing.T) {
	ctx := t.Context()
	tm := memory.NewInMemoryStore()
	// "Hello" has a content-memory match; "Brand new string" does not.
	seedMemoryEntry(t, tm, "Hello", "Bonjour")

	blocks := []*store.StoredBlock{
		storedBlock("b1", "Hello"),
		storedBlock("b2", "Brand new string"),
	}

	res, err := recycleBlocks(ctx, tm, blocks, "en", "fr", 1.0)
	require.NoError(t, err)

	assert.Equal(t, 1, res.memoryCount, "one block should be filled from content memory")
	require.Len(t, res.filled, 1)
	require.Len(t, res.remainder, 1)

	assert.Equal(t, "b1", res.filled[0].ID)
	assert.Equal(t, "Bonjour", res.filled[0].TargetText("fr"), "Memory fill lands on the target")

	assert.Equal(t, "b2", res.remainder[0].ID, "only the unmatched block goes to AI")
	assert.Empty(t, res.remainder[0].TargetText("fr"))
}

// TestRecycleBlocks_SkipsAlreadyTranslated confirms a block that already carries
// a target is neither recycled nor sent to AI — it's already done.
func TestRecycleBlocks_SkipsAlreadyTranslated(t *testing.T) {
	ctx := t.Context()
	tm := memory.NewInMemoryStore()
	seedMemoryEntry(t, tm, "Hello", "Bonjour")

	done := storedBlock("b1", "Hello")
	done.Block.Targets = map[model.VariantKey]*model.Target{
		model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Salut"}}}},
	}
	res, err := recycleBlocks(ctx, tm, []*store.StoredBlock{done}, "en", "fr", 1.0)
	require.NoError(t, err)
	assert.Zero(t, res.memoryCount)
	assert.Empty(t, res.filled)
	assert.Empty(t, res.remainder, "an already-translated block is excluded from both buckets")
}

// TestSeedMemoryFromBlocks_SeedsAndDeduplicates proves verification (b): ingesting a
// block that arrives with a target seeds the project content memory, and re-seeding the same
// pair is idempotent (content-hash keyed IDs upsert, never duplicate).
func TestSeedMemoryFromBlocks_SeedsAndDeduplicates(t *testing.T) {
	ctx := t.Context()
	tm := memory.NewInMemoryStore()

	b := model.NewBlock("b1", "Hello")
	b.Targets = map[model.VariantKey]*model.Target{
		model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
	}

	n := seedMemoryFromBlocks(ctx, tm, []*model.Block{b}, "proj", "en", "fr", "push", "")
	assert.Equal(t, 1, n, "one target-carrying block should seed one entry")

	count, err := tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// The seeded pair must now recycle for free.
	matches, err := tm.Lookup(ctx, model.NewBlock("q", "Hello"), "en", "fr", memory.LookupOptions{MinScore: 1.0, MaxResults: 1})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, "Bonjour", matches[0].Entry.VariantText("fr"))

	// Re-seeding the identical pair must not duplicate (idempotent upsert).
	n2 := seedMemoryFromBlocks(ctx, tm, []*model.Block{b}, "proj", "en", "fr", "push", "")
	assert.Equal(t, 1, n2)
	count, err = tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "re-ingesting the same translation must not duplicate the content-memory entry")
}

// TestSeedMemoryFromBlocks_SkipsUntranslated confirms a source-only block seeds
// nothing (there is no target to learn from).
func TestSeedMemoryFromBlocks_SkipsUntranslated(t *testing.T) {
	ctx := t.Context()
	tm := memory.NewInMemoryStore()
	n := seedMemoryFromBlocks(ctx, tm, []*model.Block{model.NewBlock("b1", "Hello")}, "proj", "en", "fr", "push", "")
	assert.Zero(t, n)
	count, err := tm.Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

// TestSeedMemoryFromBlockTargets_MultiLocaleFanOut proves the ingest seeding path
// (verification (b), ingest variant): a pushed block carrying targets in several
// locales seeds one content-memory entry per (source, target) pair, and a source-only block
// contributes nothing.
func TestSeedMemoryFromBlockTargets_MultiLocaleFanOut(t *testing.T) {
	ctx := t.Context()
	tm := memory.NewInMemoryStore()

	withTargets := model.NewBlock("b1", "Hello")
	withTargets.Targets = map[model.VariantKey]*model.Target{
		model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
		model.Variant("de"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Hallo"}}}},
	}
	sourceOnly := model.NewBlock("b2", "Untranslated")

	seedMemoryFromBlockTargets(ctx, tm, []*model.Block{withTargets, sourceOnly}, "proj", "en")

	count, err := tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "one entry per (en→fr) and (en→de) pair; source-only block seeds nothing")

	frMatches, err := tm.Lookup(ctx, model.NewBlock("q", "Hello"), "en", "fr", memory.LookupOptions{MinScore: 1.0, MaxResults: 1})
	require.NoError(t, err)
	require.NotEmpty(t, frMatches)
	assert.Equal(t, "Bonjour", frMatches[0].Entry.VariantText("fr"))
}

// TestSeedMemoryFromBlockTargets_NilMemoryNoPanic guards the disabled path.
func TestSeedMemoryFromBlockTargets_NilMemoryNoPanic(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.Targets = map[model.VariantKey]*model.Target{
		model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
	}
	// nil content memory and empty source locale must both be no-ops.
	seedMemoryFromBlockTargets(t.Context(), nil, []*model.Block{b}, "proj", "en")
	seedMemoryFromBlockTargets(t.Context(), memory.NewInMemoryStore(), []*model.Block{b}, "proj", "")
}

// TestProjectMemoryMinScore checks the recipe threshold mapping that gates content memory
// leverage: default fuzzy at the framework's canonical threshold (0.7, matching
// the CLI recycle flow), with an explicit `tm_fuzzy_threshold` overriding in
// either direction — including 100 to restore strict exact-only.
func TestProjectMemoryMinScore(t *testing.T) {
	assert.Equal(t, defaultMemoryMinScore, projectMemoryMinScore(nil))
	assert.Equal(t, defaultMemoryMinScore, projectMemoryMinScore(&store.Project{}))
	assert.Equal(t, defaultMemoryMinScore, projectMemoryMinScore(&store.Project{Properties: map[string]string{"tm_fuzzy_threshold": "bad"}}))
	assert.Equal(t, defaultMemoryMinScore, projectMemoryMinScore(&store.Project{Properties: map[string]string{"tm_fuzzy_threshold": "0"}}))
	assert.InDelta(t, 0.85, projectMemoryMinScore(&store.Project{Properties: map[string]string{"tm_fuzzy_threshold": "85"}}), 1e-9)
	// A project can still opt back into exact-only leverage.
	assert.Equal(t, 1.0, projectMemoryMinScore(&store.Project{Properties: map[string]string{"tm_fuzzy_threshold": "100"}}))
	assert.Equal(t, 1.0, projectMemoryMinScore(&store.Project{Properties: map[string]string{"tm_fuzzy_threshold": "150"}}))
}

// TestRecycleBlocks_DefaultFuzzyFillsNearExact proves the default threshold
// change (content memory-1): with no explicit minScore, near-exact matches — an exact text
// match demoted to 0.99 by the ambiguity rule — now pre-fill instead of being
// discarded by the old exact-only (1.0) default, while a genuinely fuzzy match
// still never fills (only exact-tier match types set a target) and its block
// stays in the AI remainder.
func TestRecycleBlocks_DefaultFuzzyFillsNearExact(t *testing.T) {
	ctx := t.Context()
	tm := memory.NewInMemoryStore()
	// Two entries with the same source but differing targets: both demote to
	// ScoreNearExact (0.99, still exact-tier MatchType) under the ambiguity rule.
	seedMemoryEntry(t, tm, "Hello", "Bonjour")
	require.NoError(t, tm.Add(ctx, memory.Entry{
		ID: "seed-ambiguous",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello"}}},
			"fr": {{Text: &model.TextRun{Text: "Salut"}}},
		},
		HintSrcLang: "en",
	}))
	// A near-miss for fuzzy matching only (Levenshtein < 1.0, > 0.7).
	seedMemoryEntry(t, tm, "Hello there friends", "Salut les amis")

	blocks := []*store.StoredBlock{
		storedBlock("b1", "Hello"),
		storedBlock("b2", "Hello there friend"),
	}

	// minScore 0 → the default (fuzzy at defaultMemoryMinScore).
	res, err := recycleBlocks(ctx, tm, blocks, "en", "fr", 0)
	require.NoError(t, err)

	require.Len(t, res.filled, 1, "the near-exact (demoted ambiguous) match must fill")
	assert.Equal(t, "b1", res.filled[0].ID)
	assert.NotEmpty(t, res.filled[0].TargetText("fr"))

	require.Len(t, res.remainder, 1, "a fuzzy-only match must not fill; the block goes to AI")
	assert.Equal(t, "b2", res.remainder[0].ID)
	assert.Empty(t, res.remainder[0].TargetText("fr"))

	// Explicit exact-only (1.0) still excludes the demoted near-exact match:
	// the project setting overrides the fuzzy default in both directions.
	strict, err := recycleBlocks(ctx, tm, []*store.StoredBlock{storedBlock("b1", "Hello")}, "en", "fr", 1.0)
	require.NoError(t, err)
	assert.Empty(t, strict.filled, "exact-only must not fill an ambiguous near-exact match")
	require.Len(t, strict.remainder, 1)
}
