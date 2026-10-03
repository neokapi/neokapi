package change_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// A read shows a plural by one of its branches, so a find taken from the
// text it shows can lie in a branch. Without a path, such a find is refused
// as not_found with the path of each branch that holds it, a candidate per
// match, and the edit then lands when sent with one of those paths.
func TestApplyBlock_FindInABranchNamesItsPath(t *testing.T) {
	b := model.NewRunsBlock("p", pluralRuns())
	err := requireRefused(t, apply(t, b, agent, replace("", sourceRev(b), find("item", "thing")))[0], change.CodeNotFound)
	assert.Equal(t, "edits/0/find", err.Field)
	assert.Contains(t, err.Message, `it is in the branches at paths [1,{"plural":"one"}], [1,{"plural":"other"}]`)
	require.Len(t, err.Candidates, 2)
	var paths []string
	for _, c := range err.Candidates {
		require.NotNil(t, c.At)
		path, jerr := json.Marshal(c.At.Path)
		require.NoError(t, jerr)
		paths = append(paths, string(path))
		assert.Equal(t, 0, c.Occurrence, "one match per branch needs no occurrence")
	}
	assert.Equal(t, []string{`[1,{"plural":"one"}]`, `[1,{"plural":"other"}]`}, paths)

	// One branch holds the text: the refusal names it alone.
	err = requireRefused(t, apply(t, b, agent, replace("", sourceRev(b), find("items", "things")))[0], change.CodeNotFound)
	assert.Contains(t, err.Message, `it is in the branch at path [1,{"plural":"other"}]: send the edit with that path`)

	// Sent with the path the refusal named, the edit lands in that branch.
	edit := find("item", "thing")
	edit.Path = err.Candidates[0].At.Path
	requireApplied(t, apply(t, b, agent, replace("", sourceRev(b), edit)))
	assert.Equal(t, " things", b.Source[1].Plural.Forms[model.PluralOther][1].Text.Text)
	assert.Equal(t, " item", b.Source[1].Plural.Forms[model.PluralOne][1].Text.Text)

	// Text in no branch either is refused naming the text it searched.
	err = requireRefused(t, apply(t, b, agent, replace("", sourceRev(b), find("cart", "bag")))[0], change.CodeNotFound)
	assert.Equal(t, `"cart" is not in the text, which is "You have <x id=\"n/\"/> things in your basket."`, err.Message)
	require.NotNil(t, err.Searched)
	assert.Empty(t, err.Searched.Path)
	assert.Equal(t, `You have <x id="n/"/> things in your basket.`, err.Searched.Text)
	assert.Empty(t, err.Candidates)
}

// A find that matches nothing in a branch says which branch it searched and
// what its text is, and offers up to three matches that differ only in case.
func TestApplyBlock_NotFoundNamesWhatItSearched(t *testing.T) {
	b := model.NewRunsBlock("p", pluralRuns())
	one := model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}
	edit := find("article", "thing")
	edit.Path = one
	err := requireRefused(t, apply(t, b, agent, replace("", sourceRev(b), edit))[0], change.CodeNotFound)
	assert.Equal(t, `"article" is not in [1,{"plural":"one"}], whose text is "<x id=\"n/\"/> item"`, err.Message)
	require.NotNil(t, err.Searched)
	assert.Equal(t, one, err.Searched.Path)

	c := model.NewRunsBlock("c", []model.Run{model.TextR("Book a Book now. BOOK it.")})
	err = requireRefused(t, apply(t, c, agent, replace("", sourceRev(c), find("book", "order")))[0], change.CodeNotFound)
	require.Len(t, err.Candidates, 3, "the matches that differ only in case, at most three")
	assert.Equal(t, model.RunPos{Run: 0, Offset: 7}, err.Candidates[1].At.Start)
}
