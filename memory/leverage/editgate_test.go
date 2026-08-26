package leverage_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/edit"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gate, end to end through the real matcher and the real provider.
//
// The classifier is unit-tested next to itself; what this proves is that the
// verdict survives the round trip: a block whose meaning was inverted arrives
// at the fill decision marked substantive even though it scores 94, and a block
// that gained a full stop arrives cosmetic even though it scores 91.
//
// Those two numbers are the whole argument. Ranked by score the inversion looks
// like the better match, so any single fill floor between them fills the
// dangerous one and refuses the harmless one.

const (
	longSource = "You must accept the terms before we can activate your account"
	longTarget = "Du må godta vilkårene før vi kan aktivere kontoen din"
)

func providerWith(t *testing.T, source, target string) *leverage.Provider {
	t.Helper()
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID:          "approved",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: source}}},
			"nb": {{Text: &model.TextRun{Text: target}}},
		},
	}))
	return leverage.NewProvider(tm)
}

func blockOf(text string) *model.Block {
	return &model.Block{
		ID:           "b",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
	}
}

func TestLookupBlockClassifiesTheEdit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  string
		current string
		want    edit.Kind
	}{
		{"unchanged", longSource, longSource, edit.None},
		{
			// Scores 94, and the same edit on the /coordinate ladder's
			// sentence scores 95. Ranked by percentage these are the best
			// non-exact matches there are, and they say the opposite thing.
			name:    "a negation added",
			source:  longSource,
			current: "You must not accept the terms before we can activate your account",
			want:    edit.Substantive,
		},
		{
			// Scores 91 — lower than the negation, and completely harmless.
			name:    "a full stop on a short label",
			source:  "Get started",
			current: "Get started.",
			want:    edit.Cosmetic,
		},
		{
			name:    "a number changed",
			source:  "Cancel within 30 days",
			current: "Cancel within 3 days",
			want:    edit.Substantive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := providerWith(t, tt.source, longTarget)
			m, ok := p.Lookup(t.Context(), corememory.Request{Block: blockOf(tt.current), Source: "en", Target: "nb", MinScore: 1})
			require.True(t, ok, "the corpus holds a match to classify")
			assert.Equal(t, tt.want, m.Edit)
			assert.Equal(t, tt.want.SafeToFill(), m.Edit.SafeToFill())
		})
	}
}

// TestTheDangerousMatchOutscoresTheHarmlessOne pins the inversion the
// classifier exists to correct. If this ever stops holding, the argument for
// classifying rather than scoring has changed and should be re-made.
func TestTheDangerousMatchOutscoresTheHarmlessOne(t *testing.T) {
	t.Parallel()

	negation := providerWith(t, longSource, longTarget)
	nm, ok := negation.Lookup(t.Context(), corememory.Request{Block: blockOf("You must not accept the terms before we can activate your account"), Source: "en", Target: "nb", MinScore: 1})
	require.True(t, ok)

	fullStop := providerWith(t, "Get started", "Kom i gang")
	fm, ok := fullStop.Lookup(t.Context(), corememory.Request{Block: blockOf("Get started."), Source: "en", Target: "nb", MinScore: 1})
	require.True(t, ok)

	assert.Greater(t, nm.Score, fm.Score,
		"the percentage ranks the meaning inversion above the full stop")
	assert.False(t, nm.Edit.SafeToFill(), "and the classification does not")
	assert.True(t, fm.Edit.SafeToFill())
}
