package redaction

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

func egressPolicy(t *testing.T, rules ...Rule) *Policy {
	t.Helper()
	det, err := NewRuleDetector(rules)
	require.NoError(t, err)
	return &Policy{Detector: det, Locale: "en"}
}

// entryFor redacts runs under policy, as the redact step does, and returns
// the redacted runs with the entry that holds what they withheld.
func entryFor(t *testing.T, p *Policy, text string) ([]model.Run, *SecretAnnotation) {
	t.Helper()
	runs := []model.Run{model.TextR(text)}
	ms, err := p.Detector.Detect(context.Background(), text, p.Locale)
	require.NoError(t, err)
	out, recs, _ := Redact(runs, ms, p.Options)
	entry := &SecretAnnotation{Policy: p, Values: map[string]RedactedValue{}}
	for _, r := range recs {
		entry.Values[r.Token] = RedactedValue{Token: r.Token, Category: r.Category, Disp: r.Disp, Original: r.Original, Locale: p.Locale}
	}
	return out, entry
}

func TestEgress_AValueTheBlockWithholdsTakesTheBlocksToken(t *testing.T) {
	p := egressPolicy(t, Rule{Term: "Ada Lovelace", Category: CategoryPerson})
	runs, entry := entryFor(t, p, "Write to Ada Lovelace today.")
	e := NewEgress(p, entry)

	t.Run("beside the visible token", func(t *testing.T) {
		block, _ := VisibleText(runs)
		got, err := e.Redact(context.Background(), []string{block, "Ada Lovelace → Ada Lovelace"})
		require.NoError(t, err)
		assert.Equal(t, "Write to [REDACTED:Person] today.", got[0])
		assert.Equal(t, "[REDACTED:Person] → [REDACTED:Person]", got[1])
	})
	t.Run("beside the placeholder tag", func(t *testing.T) {
		got, err := e.Redact(context.Background(), []string{model.RunsPlaceholderText(runs), "was: Write to Ada Lovelace yesterday."})
		require.NoError(t, err)
		assert.Equal(t, `was: Write to <x id="rdx1/"/> yesterday.`, got[1],
			"the request shows the value as the block does")
	})
	assert.Len(t, entry.Values, 1, "a value the block holds adds nothing to its entry")
}

func TestEgress_AValueMetOnlyBesideTheBlockGetsTheNextToken(t *testing.T) {
	p := egressPolicy(t,
		Rule{Term: "Ada Lovelace", Category: CategoryPerson},
		Rule{Term: "Charles Babbage", Category: CategoryPerson})
	runs, entry := entryFor(t, p, "Write to Ada Lovelace today.")
	e := NewEgress(p, entry)

	got, err := e.Redact(context.Background(), []string{"Charles Babbage → Charles Babbage", "Copy Charles Babbage."})
	require.NoError(t, err)
	assert.Equal(t, "[REDACTED:Person]#1 → [REDACTED:Person]#1", got[0],
		"a second person is told apart from the block's")
	assert.Equal(t, "Copy [REDACTED:Person]#1.", got[1], "one value is one token across the request")

	v, ok := entry.Get("rdx2")
	require.True(t, ok, "the value is kept in the block's entry under the next token")
	assert.Equal(t, "Charles Babbage", v.Original)

	// A model that copies the token into its answer gets the value back.
	answer := append(runs, model.TextR(" Cc [REDACTED:Person]#1."))
	restored, n, _ := RestoreTree(answer, func(tok string) (string, bool) {
		v, ok := entry.Get(tok)
		return v.Original, ok
	}, valuesOf(entry))
	assert.Equal(t, 2, n)
	assert.Equal(t, "Write to Ada Lovelace today. Cc Charles Babbage.", model.RunsText(restored))
}

func TestEgress_LeavesATokenItShowsAlone(t *testing.T) {
	// A rule that matches the category name would otherwise carve the
	// visible token apart.
	p := egressPolicy(t,
		Rule{Term: "Ada Lovelace", Category: CategoryPerson},
		Rule{Term: "Person", Category: CategoryOther})
	runs, entry := entryFor(t, p, "Write to Ada Lovelace today.")
	block, _ := VisibleText(runs)
	got, err := NewEgress(p, entry).Redact(context.Background(), []string{block})
	require.NoError(t, err)
	assert.Equal(t, "Write to [REDACTED:Person] today.", got[0])
}

func TestEgress_RedactsWhatTheBlockWithholdsWithNoDetector(t *testing.T) {
	// An entity step finds values no rule names; the entry holds them.
	entry := &SecretAnnotation{Values: map[string]RedactedValue{
		"rdx1": {Token: "rdx1", Category: CategoryOrg, Disp: "[REDACTED:Org]", Original: "Initech"},
	}}
	entry.Policy = &Policy{Locale: "en"}
	got, err := NewEgress(entry.Policy, entry).Redact(context.Background(), []string{"Initech terms: Initech → Initech"})
	require.NoError(t, err)
	assert.Equal(t, "[REDACTED:Org] terms: [REDACTED:Org] → [REDACTED:Org]", got[0])
}

func TestBindBlocks_KeepsAValueInEveryBlockOfTheRequest(t *testing.T) {
	p := egressPolicy(t, Rule{Term: "Ada Lovelace", Category: CategoryPerson})
	_, a := entryFor(t, p, "Write to Ada Lovelace today.")
	_, b := entryFor(t, p, "Nothing to withhold here.")
	blocks := []*model.Block{model.NewBlock("a", "x"), model.NewBlock("b", "y")}
	blocks[0].SetAnno(SecretAnnotationKey, a)
	blocks[1].SetAnno(SecretAnnotationKey, b)

	ctx := BindBlocks(context.Background(), blocks...)
	got, err := EgressFrom(ctx).Redact(ctx, []string{"Ada Lovelace → Ada Lovelace"})
	require.NoError(t, err)
	assert.Equal(t, "[REDACTED:Person] → [REDACTED:Person]", got[0])
	require.Len(t, b.Values, 1, "the second block can restore the token too")
	for _, v := range b.Values {
		assert.Equal(t, "Ada Lovelace", v.Original)
		assert.Equal(t, "[REDACTED:Person]", v.Disp)
	}
}

func TestBindCorpus_KeepsANewValueOnce(t *testing.T) {
	p := egressPolicy(t, Rule{Term: "Ada Lovelace", Category: CategoryPerson})
	_, a := entryFor(t, p, "Nothing here.")
	_, b := entryFor(t, p, "Nor here.")
	ctx := BindCorpus(context.Background(), a, b)
	got, err := EgressFrom(ctx).Redact(ctx, []string{"domain: Ada Lovelace"})
	require.NoError(t, err)
	assert.Equal(t, "domain: [REDACTED:Person]", got[0])
	assert.Len(t, a.Values, 1)
	assert.Empty(t, b.Values, "a corpus writes no answer back, so one entry keeps the value")
}

func TestBindContext_NeedsAPolicy(t *testing.T) {
	ctx := context.Background()
	assert.Nil(t, EgressFrom((&SecretAnnotation{}).BindContext(ctx)), "an entry no redact step left binds nothing")
	assert.Nil(t, EgressFrom(BindBlocks(ctx, model.NewBlock("a", "x"))))
	p := egressPolicy(t, Rule{Term: "Ada Lovelace", Category: CategoryPerson})
	assert.NotNil(t, EgressFrom((&SecretAnnotation{Policy: p}).BindContext(ctx)))
	assert.Nil(t, EgressFrom(WithEgress((&SecretAnnotation{Policy: p}).BindContext(ctx), nil)), "a nil Egress clears it")
}

func valuesOf(a *SecretAnnotation) []RedactedValue {
	out := make([]RedactedValue, 0, len(a.Values))
	for _, v := range a.Values {
		out = append(out, v)
	}
	return out
}
