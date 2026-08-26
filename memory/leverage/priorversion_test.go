package leverage_test

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acmePoint  = "acme\x1fsupport\x1facme-help"
	inForce    = "fp-in-force"
	superseded = "fp-superseded"
)

func corpusWith(t *testing.T, entries ...memory.Entry) *memory.InMemoryStore {
	t.Helper()
	tm := memory.NewInMemoryStore()
	for _, e := range entries {
		require.NoError(t, tm.Add(t.Context(), e))
	}
	return tm
}

func approved(id, unit, source, target, fingerprint string, at time.Time) memory.Entry {
	return memory.Entry{
		ID:          id,
		Unit:        unit,
		Point:       acmePoint,
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: source}}},
			"nb": {{Text: &model.TextRun{Text: target}}},
		},
		Origins:   []memory.Origin{{Source: "record", AddedAt: at, ContextFingerprint: fingerprint}},
		CreatedAt: at,
		UpdatedAt: at,
	}
}

func TestPriorVersionFor(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	t.Run("a governed prior answer is offered with its source", func(t *testing.T) {
		tm := corpusWith(t, approved("v1", "u-1", "Get started", "Kom i gang", inForce, at))

		src, tgt, ok := leverage.PriorVersionFor(t.Context(), tm, "u-1", acmePoint, "en", "nb", inForce)
		require.True(t, ok)
		assert.Equal(t, "Get started", src, "the diff needs both halves")
		assert.Equal(t, "Kom i gang", tgt)
	})

	t.Run("an answer approved under moved governance is withheld", func(t *testing.T) {
		// The whole reason the gate is inside this function. This answer is
		// retrievable; offering it would anchor the model to wording the rules
		// in force may now reject, and the result would be stamped with today's
		// fingerprint and look fresh.
		tm := corpusWith(t, approved("v1", "u-1", "Get started", "Kom i gang", superseded, at))

		_, _, ok := leverage.PriorVersionFor(t.Context(), tm, "u-1", acmePoint, "en", "nb", inForce)
		assert.False(t, ok)
	})

	t.Run("an ungoverned run gets no reference", func(t *testing.T) {
		// Nothing to compare against is not the same as agreement. A run with no
		// fingerprint of its own cannot assert that anything satisfies it.
		tm := corpusWith(t, approved("v1", "u-1", "Get started", "Kom i gang", inForce, at))

		_, _, ok := leverage.PriorVersionFor(t.Context(), tm, "u-1", acmePoint, "en", "nb", "")
		assert.False(t, ok)
	})

	t.Run("an answer with no recorded governance is withheld", func(t *testing.T) {
		tm := corpusWith(t, approved("v1", "u-1", "Get started", "Kom i gang", "", at))

		_, _, ok := leverage.PriorVersionFor(t.Context(), tm, "u-1", acmePoint, "en", "nb", inForce)
		assert.False(t, ok)
	})

	t.Run("a block with no chain gets nothing", func(t *testing.T) {
		tm := corpusWith(t, approved("v1", "u-1", "Get started", "Kom i gang", inForce, at))

		_, _, ok := leverage.PriorVersionFor(t.Context(), tm, "u-other", acmePoint, "en", "nb", inForce)
		assert.False(t, ok)
	})

	t.Run("a block approved before the chain existed gets nothing", func(t *testing.T) {
		// Nothing backfills the unit, so a pre-chain corpus carries answers
		// bound to no block. They are nobody's history.
		tm := corpusWith(t, approved("v1", "", "Get started", "Kom i gang", inForce, at))

		_, _, ok := leverage.PriorVersionFor(t.Context(), tm, "", acmePoint, "en", "nb", inForce)
		assert.False(t, ok)
	})

	t.Run("the newest governed answer wins", func(t *testing.T) {
		tm := corpusWith(t,
			approved("v1", "u-1", "Get started", "Kom i gang", inForce, at),
			approved("v2", "u-1", "Get started now", "Kom i gang nå", inForce, at.Add(time.Hour)),
		)

		src, tgt, ok := leverage.PriorVersionFor(t.Context(), tm, "u-1", acmePoint, "en", "nb", inForce)
		require.True(t, ok)
		assert.Equal(t, "Get started now", src)
		assert.Equal(t, "Kom i gang nå", tgt)
	})

	t.Run("an answer approved elsewhere is not this point's history", func(t *testing.T) {
		tm := corpusWith(t, approved("v1", "u-1", "Get started", "Kom i gang", inForce, at))

		_, _, ok := leverage.PriorVersionFor(t.Context(), tm, "u-1",
			"other\x1femail\x1fother-mail", "en", "nb", inForce)
		assert.False(t, ok)
	})
}
