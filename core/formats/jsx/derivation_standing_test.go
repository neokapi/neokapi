package jsx_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// The derivation the change service records on a translation (the
// authoritative edition and its revision) travels in a bundle, and the block
// read back grades it from its own content: current while the source holds
// what the translation was made from, under the key the bundle's project names
// and under the project language a project read files the block by, and stale
// once the source moves.
func TestKBFDerivationIsGradedOnTheBlockReadBack(t *testing.T) {
	blocks := readBlocks(t, emptyLocaleBundle(`{}`, `{}`))
	require.Len(t, blocks, 1)
	b := blocks[0]
	b.SourceLocale = "en" // the bundle's project language
	nb := model.Variant("nb")
	from := b.Authoritative(model.AuthorityPolicy{})
	b.SetTargetEdition(nb, model.Edition{
		Runs:    []model.Run{model.TextR("Logg inn")},
		Derived: &model.Derivation{From: from, Rev: model.EditionRevision(b, from)},
	})
	require.Equal(t, model.StandingCurrent, b.DerivationStanding(nb))

	back := readBlocks(t, writeBundle(t, b))
	require.Len(t, back, 1)
	read := back[0]
	read.SourceLocale = "en"
	d, ok := read.Derivation(nb)
	require.True(t, ok, "the derivation reads back")
	assert.True(t, read.IsSourceEdition(d.From), "as one from the source")
	assert.Equal(t, model.StandingCurrent, read.DerivationStanding(nb),
		"the source holds the content the translation was made from")

	// A project whose language is en-US files the block under it and keeps
	// the bundle's language beside it, as host.fileUnderSource does.
	read.SourceLocale = "en-US"
	read.Properties = map[string]string{model.PropReadSourceLocale: "en"}
	assert.Equal(t, model.StandingCurrent, read.DerivationStanding(nb),
		"the basis is matched under the language the bundle named")

	read.SetSourceRuns([]model.Run{model.TextR("Log in")})
	assert.Equal(t, model.StandingStale, read.DerivationStanding(nb), "the source moved under the translation")
}
