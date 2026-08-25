package memory

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// versionStores runs a case against every backend that can answer a chain, so
// the two cannot drift on the semantics — which is the whole risk of an
// optional capability implemented twice.
func versionStores(t *testing.T) map[string]func(t *testing.T) interface {
	Store
	VersionReader
} {
	t.Helper()
	return map[string]func(t *testing.T) interface {
		Store
		VersionReader
	}{
		"sqlite": func(t *testing.T) interface {
			Store
			VersionReader
		} {
			tm, err := NewSQLiteStore(filepath.Join(t.TempDir(), "memory.db"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = tm.Close() })
			return tm
		},
		"in-memory": func(t *testing.T) interface {
			Store
			VersionReader
		} {
			return NewInMemoryStore()
		},
	}
}

// answer builds one approved answer for a block: a version in the chain.
func answer(id, unit, point, source, target, fingerprint string, at time.Time) Entry {
	return Entry{
		ID:          id,
		Unit:        unit,
		Point:       point,
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: source}}},
			"nb": {{Text: &model.TextRun{Text: target}}},
		},
		Origins: []Origin{{
			Source:             "tool",
			AddedAt:            at,
			ContextFingerprint: fingerprint,
		}},
		CreatedAt: at,
		UpdatedAt: at,
	}
}

func TestVersions(t *testing.T) {
	t0 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	acme := NewPoint("acme", "support", "acme-help")
	other := NewPoint("other", "email", "other-mail")

	for name, newStore := range versionStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			tm := newStore(t)

			// One block, three successive answers — the corpus accumulating
			// versions, which is what it has always done.
			require.NoError(t, tm.Add(ctx, answer("v1", "u-1", acme, "Get started", "Kom i gang", "fp-old", t0)))
			require.NoError(t, tm.Add(ctx, answer("v2", "u-1", acme, "Get started now", "Kom i gang nå", "fp-now", t0.Add(time.Hour))))
			require.NoError(t, tm.Add(ctx, answer("v3", "u-1", acme, "Get started today", "Kom i gang i dag", "fp-now", t0.Add(2*time.Hour))))
			// A different block at the same point, and the same block at
			// another point. Neither belongs to u-1's chain at acme.
			require.NoError(t, tm.Add(ctx, answer("o1", "u-2", acme, "Sign in", "Logg inn", "fp-now", t0)))
			require.NoError(t, tm.Add(ctx, answer("m1", "u-1", other, "Get started", "Sett i gang", "fp-now", t0)))

			t.Run("the chain is the block's prior answers, newest first", func(t *testing.T) {
				got, err := tm.Versions(ctx, VersionQuery{Unit: "u-1", Point: acme}, "v3")
				require.NoError(t, err)
				require.Len(t, got, 2)
				assert.Equal(t, "v2", got[0].Entry.ID)
				assert.Equal(t, "v1", got[1].Entry.ID)
			})

			t.Run("the answer in force is excluded", func(t *testing.T) {
				got, err := tm.Versions(ctx, VersionQuery{Unit: "u-1", Point: acme}, "v3")
				require.NoError(t, err)
				for _, v := range got {
					assert.NotEqual(t, "v3", v.Entry.ID, "the caller already has this one")
				}
			})

			t.Run("another block's answers are not in the chain", func(t *testing.T) {
				got, err := tm.Versions(ctx, VersionQuery{Unit: "u-1", Point: acme}, "")
				require.NoError(t, err)
				for _, v := range got {
					assert.NotEqual(t, "o1", v.Entry.ID)
				}
			})

			t.Run("a point narrows the chain to answers approved there", func(t *testing.T) {
				got, err := tm.Versions(ctx, VersionQuery{Unit: "u-1", Point: acme}, "")
				require.NoError(t, err)
				for _, v := range got {
					assert.Equal(t, acme, v.Entry.Point)
				}

				// No point returns the block's whole history, across every place
				// it has sat — what a report on a moved block wants.
				all, err := tm.Versions(ctx, VersionQuery{Unit: "u-1"}, "")
				require.NoError(t, err)
				assert.Len(t, all, 4)
			})

			t.Run("the governing context travels with the answer", func(t *testing.T) {
				got, err := tm.Versions(ctx, VersionQuery{Unit: "u-1", Point: acme}, "v3")
				require.NoError(t, err)
				require.Len(t, got, 2)
				assert.Equal(t, "fp-now", got[0].ContextFingerprint)
				assert.Equal(t, "fp-old", got[1].ContextFingerprint)
			})

			t.Run("a limit trims from the newest end", func(t *testing.T) {
				got, err := tm.Versions(ctx, VersionQuery{Unit: "u-1", Point: acme, Limit: 1}, "")
				require.NoError(t, err)
				require.Len(t, got, 1)
				assert.Equal(t, "v3", got[0].Entry.ID)
			})

			t.Run("a chain needs a block", func(t *testing.T) {
				_, err := tm.Versions(ctx, VersionQuery{Point: acme}, "")
				assert.ErrorIs(t, err, ErrVersionQueryNeedsUnit)
			})
		})
	}
}

func TestVersionsIgnoresEntriesApprovedBeforeTheChain(t *testing.T) {
	for name, newStore := range versionStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			tm := newStore(t)
			t0 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

			// Nothing backfills the unit, so a corpus seeded before it existed
			// carries answers bound to no block. They must not surface as
			// anyone's history — an entry with no unit is not "every block".
			require.NoError(t, tm.Add(ctx, answer("pre", "", "", "Get started", "Kom i gang", "", t0)))
			require.NoError(t, tm.Add(ctx, answer("v1", "u-1", "", "Get started now", "Kom i gang nå", "fp", t0.Add(time.Hour))))

			got, err := tm.Versions(ctx, VersionQuery{Unit: "u-1"}, "")
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, "v1", got[0].Entry.ID)
		})
	}
}

func TestVersionGovernedBy(t *testing.T) {
	t.Parallel()

	v := Version{ContextFingerprint: "fp-now"}
	assert.True(t, v.GovernedBy("fp-now"))
	assert.False(t, v.GovernedBy("fp-moved"), "governance moved since this was approved")

	// Neither side may be empty. An ungoverned answer cannot be asserted to
	// satisfy governance, and a caller with no fingerprint has nothing to
	// compare against — treating either as a match is how stale wording gets
	// laundered into a target stamped with today's context.
	assert.False(t, v.GovernedBy(""))
	assert.False(t, Version{}.GovernedBy("fp-now"))
	assert.False(t, Version{}.GovernedBy(""))
}
