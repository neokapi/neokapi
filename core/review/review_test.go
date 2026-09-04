package review

import (
	"encoding/json"
	"testing"
	"time"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultWindowMatchesTheTranslateTool: a reviewer reading the neighbourhood
// reads the neighbourhood the model read, which holds only while the two
// defaults agree.
func TestDefaultWindowMatchesTheTranslateTool(t *testing.T) {
	assert.Equal(t, aitools.DefaultContextWindow, DefaultWindow)
}

func TestMatchOfRendersOnePercentScale(t *testing.T) {
	entry := memory.Entry{
		ID: "m1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR("Hello {name}!")},
			"nb": {model.TextR("Hei {name}!")},
		},
	}

	tests := []struct {
		name  string
		match memory.Match
		want  *MemoryMatch
	}{
		{
			name:  "a fraction rounds half up to a percent",
			match: memory.Match{Entry: entry, Score: 0.915, MatchType: memory.MatchFuzzy},
			want:  &MemoryMatch{Score: 92, Kind: "fuzzy", Source: "Hello {name}!", Target: "Hei {name}!"},
		},
		{
			name:  "an exact match reads as 100",
			match: memory.Match{Entry: entry, Score: 1, MatchType: memory.MatchExact},
			want:  &MemoryMatch{Score: 100, Kind: "exact", Source: "Hello {name}!", Target: "Hei {name}!"},
		},
		{
			name: "a match with no wording for the target locale is withheld",
			match: memory.Match{Entry: memory.Entry{
				ID:       "half",
				Variants: map[model.LocaleID][]model.Run{"en": {model.TextR("Only the source")}},
			}, Score: 0.9},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, MatchOf(tc.match, "en", "nb"))
		})
	}
}

func TestPriorVersionOfReadsTheChain(t *testing.T) {
	ctx := t.Context()
	tm := memory.NewInMemoryStore()

	older := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	require.NoError(t, tm.Add(ctx, chainAnswer("v1", "settings.save", "Save file", "Lagre fil", "fp-old", older)))
	require.NoError(t, tm.Add(ctx, chainAnswer("v2", "settings.save", "Save the file", "Lagre filen", "fp-now", newer)))

	block := &model.Block{
		ID:           "b1",
		Name:         "settings.save",
		Unit:         "settings.save",
		Translatable: true,
		Source:       []model.Run{model.TextR("Save this file")},
	}

	tests := []struct {
		name         string
		contextHash  string
		wantGoverned bool
	}{
		{name: "the newest answer wins, governed by the context in force", contextHash: "fp-now", wantGoverned: true},
		{name: "an answer approved under superseded rules is reported ungoverned", contextHash: "fp-moved"},
		{name: "with no recorded context the chain still answers, ungoverned", contextHash: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prior := PriorVersionOf(ctx, tm, block, "en", "nb", tc.contextHash)
			require.NotNil(t, prior)
			assert.Equal(t, "Save the file", prior.Source)
			assert.Equal(t, "Lagre filen", prior.Target)
			assert.Equal(t, "fp-now", prior.ContextFingerprint)
			assert.Equal(t, tc.wantGoverned, prior.Governed)
		})
	}
}

func TestPriorVersionOfWithoutAChain(t *testing.T) {
	ctx := t.Context()
	tm := memory.NewInMemoryStore()

	t.Run("no reader answers nothing", func(t *testing.T) {
		assert.Nil(t, PriorVersionOf(ctx, nil, &model.Block{ID: "b", Unit: "x"}, "en", "nb", ""))
	})

	t.Run("a block with no chain identity asks nothing", func(t *testing.T) {
		assert.Nil(t, PriorVersionOf(ctx, tm, &model.Block{ID: "b"}, "en", "nb", ""))
	})

	t.Run("a chain the corpus has never seen answers nothing", func(t *testing.T) {
		block := &model.Block{ID: "b", Unit: "never.written", Translatable: true}
		assert.Nil(t, PriorVersionOf(ctx, tm, block, "en", "nb", ""))
	})

	t.Run("an answer missing one locale is withheld", func(t *testing.T) {
		require.NoError(t, tm.Add(ctx, memory.Entry{
			ID:          "half",
			Unit:        "half.answer",
			HintSrcLang: "en",
			Variants: map[model.LocaleID][]model.Run{
				"en": {model.TextR("Only the source")},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}))
		block := &model.Block{ID: "b", Unit: "half.answer", Translatable: true}
		assert.Nil(t, PriorVersionOf(ctx, tm, block, "en", "nb", ""))
	})
}

// TestContextWireKeys pins the JSON spelling every client reads: the five
// layers by name, and the facts a platform used to spell differently.
func TestContextWireKeys(t *testing.T) {
	score := 74
	c := Context{
		Point:         Point{Path: "content/app.json", Voice: &Voice{Name: "Retail", Guide: "Be brief."}, TermsTotal: 1},
		Neighbourhood: Neighbourhood{Key: "greeting", After: []Neighbour{{Key: "bye", Source: []model.Run{model.TextR("Bye")}, Status: "reviewed"}}, Window: 2},
		History:       History{Match: &MemoryMatch{Score: 88, Kind: "fuzzy", Target: "Hei"}},
		Judgement:     Judgement{AIScore: &score},
		Provenance:    Provenance{ReviewState: "approved", Status: "reviewed", Stale: true},
	}
	raw, err := json.Marshal(c)
	require.NoError(t, err)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &got))
	for _, layer := range []string{"point", "neighbourhood", "history", "judgement", "provenance"} {
		assert.Contains(t, got, layer)
	}
	s := string(raw)
	assert.Contains(t, s, `"terms_total":1`)
	assert.Contains(t, s, `"guide":"Be brief."`)
	assert.Contains(t, s, `"status":"reviewed"`)
	assert.Contains(t, s, `"score":88`)
	assert.Contains(t, s, `"kind":"fuzzy"`)
	assert.Contains(t, s, `"review_state":"approved"`)
	assert.Contains(t, s, `"stale":true`)
	assert.Contains(t, s, `"window":2`)
}

// chainAnswer builds one approved answer in a block's version chain.
func chainAnswer(id, unit, source, target, fingerprint string, at time.Time) memory.Entry {
	return memory.Entry{
		ID:          id,
		Unit:        unit,
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR(source)},
			"nb": {model.TextR(target)},
		},
		Origins:   []memory.Origin{{Source: "tool", AddedAt: at, ContextFingerprint: fingerprint}},
		CreatedAt: at,
		UpdatedAt: at,
	}
}
