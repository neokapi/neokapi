package venue

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func block(id, text string) *model.Block {
	b := &model.Block{ID: id, Translatable: true}
	b.SetSourceText(text)
	return b
}

// A scope decides whether an item the producer did not mention was deleted or
// simply not looked at, so every reading of a pattern is a decision about
// whether content survives.
func TestScopeCovers(t *testing.T) {
	cases := []struct {
		name  string
		scope Scope
		path  string
		want  bool
	}{
		{"a directory names everything under it", Scope{"web/docs"}, "web/docs/a/b.md", true},
		{"a directory does not name its siblings", Scope{"web/docs"}, "web/docsite/a.md", false},
		{"a directory names itself", Scope{"web/docs"}, "web/docs", true},
		{"a glob matches one segment", Scope{"i18n/*.json"}, "i18n/en.json", true},
		{"a glob does not span separators", Scope{"i18n/*.json"}, "i18n/nested/en.json", false},
		{"a double star spans depth", Scope{"src/**/*.tsx"}, "src/a/b/C.tsx", true},
		{"a double star still respects its prefix", Scope{"src/**/*.tsx"}, "lib/a/C.tsx", false},
		{"a double star matches directly beneath", Scope{"src/**/*.tsx"}, "src/C.tsx", true},
		{"a bare double star is everything", Scope{"**"}, "anything/at/all.md", true},
		{"several patterns are an or", Scope{"a", "b"}, "b/c.md", true},
		{"an empty scope covers nothing", Scope{}, "a.md", false},
		{"an empty pattern covers nothing", Scope{""}, "a.md", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.scope.Covers(tc.path))
		})
	}
}

// The tree carries what each item holds now — including an item that holds
// nothing, which is the only way a file whose last string was deleted can be
// cleaned.
func TestTreeFromBlocksDeclaresEveryItemItRead(t *testing.T) {
	tree := TreeFromBlocks(map[string][]*model.Block{
		"a.json": {block("b1", "Hello"), block("b2", "World")},
		"b.json": {},
	})
	require.Len(t, tree, 2)
	assert.Equal(t, []string{"b1", "b2"}, tree["a.json"].Keys)
	assert.Len(t, tree["a.json"].Content, 2)
	assert.Empty(t, tree["b.json"].Keys)

	keys := tree.BlockKeys()
	require.Contains(t, keys, "b.json")
	assert.Empty(t, keys["b.json"], "an item that read to nothing declares an empty set, not no set")
}

// A block that moved from one file to another is content the venue already
// holds. Asking per item would upload it again under the new name.
func TestTreeRecordsAreGlobal(t *testing.T) {
	tree := Tree{
		"a.json": {Path: "a.json", Record: []string{"r1", "r2"}},
		"b.json": {Path: "b.json", Record: []string{"r2", "r3"}},
	}
	have := tree.Records()
	assert.Len(t, have, 3)
	for _, r := range []string{"r1", "r2", "r3"} {
		assert.Contains(t, have, r)
	}
}

// Renaming a file and pushing must leave one document, at the new path, with
// the identity it had — which is what carries its approvals across.
func TestTreeUnitsResolveARename(t *testing.T) {
	prior := Tree{
		"old/name.json": {Path: "old/name.json", ID: "item-42", Content: []string{"c1", "c2", "c3", "c4"}},
	}.Units()

	current := Tree{
		// Same content at a new path, with one string edited: a rename and an
		// edit in the same revision is ordinary.
		"new/name.json": {Path: "new/name.json", Content: []string{"c1", "c2", "c3", "cX"}},
	}.Units()

	got := reconcile.DocumentUnits(current, prior)
	require.Len(t, got, 1)
	assert.Equal(t, "item-42", got[0].Key, "the renamed file keeps the identity its approvals hang from")
	assert.Equal(t, reconcile.Moved, got[0].Kind)
	assert.Equal(t, "new/name.json", got[0].Path)
}

// A file that merely changed is not a rename, and a genuinely new file is not
// an inheritance.
func TestTreeUnitsSeparateEditsFromArrivals(t *testing.T) {
	prior := Tree{
		"a.json": {Path: "a.json", ID: "item-a", Content: []string{"c1", "c2"}},
	}.Units()

	current := Tree{
		"a.json": {Path: "a.json", Content: []string{"c1", "cZ"}},
		"b.json": {Path: "b.json", Content: []string{"q1", "q2"}},
	}.Units()

	got := reconcile.DocumentUnits(current, prior)
	require.Len(t, got, 2)
	byPath := map[string]reconcile.DocResult{}
	for _, r := range got {
		byPath[r.Path] = r
	}
	assert.Equal(t, "item-a", byPath["a.json"].Key)
	assert.Equal(t, reconcile.Edited, byPath["a.json"].Kind)
	assert.Equal(t, reconcile.New, byPath["b.json"].Kind)
	assert.NotEqual(t, "item-a", byPath["b.json"].Key, "a new file must not inherit another's identity")
}
