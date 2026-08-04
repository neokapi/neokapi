package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
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

// TestPromoteDecisionsToMemory pins the corpus's one door in: an approval
// promotes its (source, target) pair, a rejection evicts it, and a decision
// blessing a translation the store no longer holds moves nothing in either
// direction. Nil memory and empty source locale are no-ops.
func TestPromoteDecisionsToMemory(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()
	tm := memory.NewInMemoryStore()

	projectID := "promote-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Promote"}))

	b := model.NewBlock("greeting", "Hello")
	b.Targets = map[model.VariantKey]*model.Target{
		model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
	}
	require.NoError(t, deps.ContentStore.StoreBlocksForItem(ctx, projectID, "main", "en.json", []*model.Block{b}))

	approval := store.UnitDecision{
		ItemName: "en.json", Unit: "greeting", Variant: "fr",
		ReviewState: "approved", Status: "reviewed",
		TargetHash: state.TargetHash("Bonjour"),
		DecidedBy:  "reviewer@example.com",
	}
	promoted, evicted := PromoteDecisionsToMemory(ctx, deps.ContentStore, tm, projectID, "main", "en", []store.UnitDecision{approval})
	assert.Equal(t, 1, promoted)
	assert.Zero(t, evicted)
	count, err := tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Idempotent: the entry ID is content-derived.
	promoted, _ = PromoteDecisionsToMemory(ctx, deps.ContentStore, tm, projectID, "main", "en", []store.UnitDecision{approval})
	assert.Equal(t, 1, promoted, "re-promotion upserts the same row")
	count, err = tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// A stale decision (blesses a different translation) moves nothing.
	stale := approval
	stale.TargetHash = state.TargetHash("Salut")
	promoted, evicted = PromoteDecisionsToMemory(ctx, deps.ContentStore, tm, projectID, "main", "en", []store.UnitDecision{stale})
	assert.Zero(t, promoted)
	assert.Zero(t, evicted)

	// A rejection evicts the pair it condemns.
	rejection := approval
	rejection.ReviewState = "rejected"
	rejection.Status = "draft"
	_, evicted = PromoteDecisionsToMemory(ctx, deps.ContentStore, tm, projectID, "main", "en", []store.UnitDecision{rejection})
	assert.Equal(t, 1, evicted)
	count, err = tm.Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count, "rejected wording leaves the corpus")

	// Disabled paths are no-ops, never panics.
	p, e := PromoteDecisionsToMemory(ctx, deps.ContentStore, nil, projectID, "main", "en", []store.UnitDecision{approval})
	assert.Zero(t, p+e)
	p, e = PromoteDecisionsToMemory(ctx, deps.ContentStore, tm, projectID, "main", "", []store.UnitDecision{approval})
	assert.Zero(t, p+e)
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
