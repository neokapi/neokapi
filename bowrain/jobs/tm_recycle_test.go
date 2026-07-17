package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
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

// seedTMEntry adds a single (en→fr) exact pair to the in-memory TM.
func seedTMEntry(t *testing.T, tm sievepen.TMStore, source, target string) {
	t.Helper()
	require.NoError(t, tm.Add(t.Context(), sievepen.TMEntry{
		ID: "seed-" + source,
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: source}}},
			"fr": {{Text: &model.TextRun{Text: target}}},
		},
		HintSrcLang: "en",
	}))
}

// TestRecycleBlocks_FillsFromTMAndLeavesRemainder proves the TM-first split
// (verification (a)): a convergence recycle pass fills the blocks that have an
// exact TM match and leaves only the genuinely-new blocks for AI.
func TestRecycleBlocks_FillsFromTMAndLeavesRemainder(t *testing.T) {
	ctx := t.Context()
	tm := sievepen.NewInMemoryTM()
	// "Hello" has a TM match; "Brand new string" does not.
	seedTMEntry(t, tm, "Hello", "Bonjour")

	blocks := []*store.StoredBlock{
		storedBlock("b1", "Hello"),
		storedBlock("b2", "Brand new string"),
	}

	res, err := recycleBlocks(ctx, tm, blocks, "en", "fr", 1.0)
	require.NoError(t, err)

	assert.Equal(t, 1, res.tmCount, "one block should be filled from TM")
	require.Len(t, res.filled, 1)
	require.Len(t, res.remainder, 1)

	assert.Equal(t, "b1", res.filled[0].ID)
	assert.Equal(t, "Bonjour", res.filled[0].TargetText("fr"), "TM fill lands on the target")

	assert.Equal(t, "b2", res.remainder[0].ID, "only the unmatched block goes to AI")
	assert.Empty(t, res.remainder[0].TargetText("fr"))
}

// TestRecycleBlocks_SkipsAlreadyTranslated confirms a block that already carries
// a target is neither recycled nor sent to AI — it's already done.
func TestRecycleBlocks_SkipsAlreadyTranslated(t *testing.T) {
	ctx := t.Context()
	tm := sievepen.NewInMemoryTM()
	seedTMEntry(t, tm, "Hello", "Bonjour")

	done := storedBlock("b1", "Hello")
	done.Block.Targets = map[model.VariantKey]*model.Target{
		model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Salut"}}}},
	}
	res, err := recycleBlocks(ctx, tm, []*store.StoredBlock{done}, "en", "fr", 1.0)
	require.NoError(t, err)
	assert.Zero(t, res.tmCount)
	assert.Empty(t, res.filled)
	assert.Empty(t, res.remainder, "an already-translated block is excluded from both buckets")
}

// TestSeedTMFromBlocks_SeedsAndDeduplicates proves verification (b): ingesting a
// block that arrives with a target seeds the project TM, and re-seeding the same
// pair is idempotent (content-hash keyed IDs upsert, never duplicate).
func TestSeedTMFromBlocks_SeedsAndDeduplicates(t *testing.T) {
	ctx := t.Context()
	tm := sievepen.NewInMemoryTM()

	b := model.NewBlock("b1", "Hello")
	b.Targets = map[model.VariantKey]*model.Target{
		model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
	}

	n := seedTMFromBlocks(ctx, tm, []*model.Block{b}, "proj", "en", "fr", "push", "")
	assert.Equal(t, 1, n, "one target-carrying block should seed one entry")

	count, err := tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// The seeded pair must now recycle for free.
	matches, err := tm.Lookup(ctx, model.NewBlock("q", "Hello"), "en", "fr", sievepen.LookupOptions{MinScore: 1.0, MaxResults: 1})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, "Bonjour", matches[0].Entry.VariantText("fr"))

	// Re-seeding the identical pair must not duplicate (idempotent upsert).
	n2 := seedTMFromBlocks(ctx, tm, []*model.Block{b}, "proj", "en", "fr", "push", "")
	assert.Equal(t, 1, n2)
	count, err = tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "re-ingesting the same translation must not duplicate the TM entry")
}

// TestSeedTMFromBlocks_SkipsUntranslated confirms a source-only block seeds
// nothing (there is no target to learn from).
func TestSeedTMFromBlocks_SkipsUntranslated(t *testing.T) {
	ctx := t.Context()
	tm := sievepen.NewInMemoryTM()
	n := seedTMFromBlocks(ctx, tm, []*model.Block{model.NewBlock("b1", "Hello")}, "proj", "en", "fr", "push", "")
	assert.Zero(t, n)
	count, err := tm.Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

// TestSeedTMFromBlockTargets_MultiLocaleFanOut proves the ingest seeding path
// (verification (b), ingest variant): a pushed block carrying targets in several
// locales seeds one TM entry per (source, target) pair, and a source-only block
// contributes nothing.
func TestSeedTMFromBlockTargets_MultiLocaleFanOut(t *testing.T) {
	ctx := t.Context()
	tm := sievepen.NewInMemoryTM()

	withTargets := model.NewBlock("b1", "Hello")
	withTargets.Targets = map[model.VariantKey]*model.Target{
		model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
		model.Variant("de"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Hallo"}}}},
	}
	sourceOnly := model.NewBlock("b2", "Untranslated")

	seedTMFromBlockTargets(ctx, tm, []*model.Block{withTargets, sourceOnly}, "proj", "en")

	count, err := tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "one entry per (en→fr) and (en→de) pair; source-only block seeds nothing")

	frMatches, err := tm.Lookup(ctx, model.NewBlock("q", "Hello"), "en", "fr", sievepen.LookupOptions{MinScore: 1.0, MaxResults: 1})
	require.NoError(t, err)
	require.NotEmpty(t, frMatches)
	assert.Equal(t, "Bonjour", frMatches[0].Entry.VariantText("fr"))
}

// TestSeedTMFromBlockTargets_NilTMNoPanic guards the disabled path.
func TestSeedTMFromBlockTargets_NilTMNoPanic(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.Targets = map[model.VariantKey]*model.Target{
		model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
	}
	// nil TM and empty source locale must both be no-ops.
	seedTMFromBlockTargets(t.Context(), nil, []*model.Block{b}, "proj", "en")
	seedTMFromBlockTargets(t.Context(), sievepen.NewInMemoryTM(), []*model.Block{b}, "proj", "")
}

// TestProjectTMMinScore checks the recipe threshold mapping used to gate fuzzy
// leverage: default exact-only, opt-in fuzzy via the project property.
func TestProjectTMMinScore(t *testing.T) {
	assert.Equal(t, 1.0, projectTMMinScore(nil))
	assert.Equal(t, 1.0, projectTMMinScore(&store.Project{}))
	assert.Equal(t, 1.0, projectTMMinScore(&store.Project{Properties: map[string]string{"tm_fuzzy_threshold": "bad"}}))
	assert.InDelta(t, 0.85, projectTMMinScore(&store.Project{Properties: map[string]string{"tm_fuzzy_threshold": "85"}}), 1e-9)
	assert.Equal(t, 1.0, projectTMMinScore(&store.Project{Properties: map[string]string{"tm_fuzzy_threshold": "150"}}))
}
