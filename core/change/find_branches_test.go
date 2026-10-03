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

	// Text in no branch either is refused as before.
	err = requireRefused(t, apply(t, b, agent, replace("", sourceRev(b), find("cart", "bag")))[0], change.CodeNotFound)
	assert.Equal(t, `"cart" is not in the text`, err.Message)
	assert.Empty(t, err.Candidates)
}
