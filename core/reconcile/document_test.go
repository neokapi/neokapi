package reconcile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/reconcile"
)

func doc3(path string, texts ...string) reconcile.Document {
	d := reconcile.Document{Path: path}
	for i, txt := range texts {
		d.Blocks = append(d.Blocks, blk("para"+string(rune('1'+i)), txt))
	}
	return d
}

func docPrior(key string, d reconcile.Document) reconcile.DocUnit {
	u := d.Identify()
	u.Key = key
	return u
}

func TestDocuments_Grades(t *testing.T) {
	original := doc3("docs/intro.md", "Alpha", "Bravo")
	prior := []reconcile.DocUnit{docPrior("d-intro", original)}

	t.Run("untouched", func(t *testing.T) {
		got := reconcile.Documents([]reconcile.Document{original}, prior)
		assert.Equal(t, "d-intro", got[0].Key)
		assert.Equal(t, reconcile.Unchanged, got[0].Kind)
	})

	t.Run("edited in place", func(t *testing.T) {
		got := reconcile.Documents([]reconcile.Document{doc3("docs/intro.md", "Alpha", "Revised")}, prior)
		assert.Equal(t, "d-intro", got[0].Key)
		assert.Equal(t, reconcile.Edited, got[0].Kind)
	})

	t.Run("renamed", func(t *testing.T) {
		got := reconcile.Documents([]reconcile.Document{doc3("docs/overview.md", "Alpha", "Bravo")}, prior)
		assert.Equal(t, "d-intro", got[0].Key, "a renamed file is the same document")
		assert.Equal(t, reconcile.Moved, got[0].Kind)
	})

	t.Run("renamed and partly rewritten", func(t *testing.T) {
		got := reconcile.Documents([]reconcile.Document{doc3("docs/overview.md", "Alpha", "Rewritten")}, prior)
		assert.Equal(t, "d-intro", got[0].Key, "half its content survived, so it is the same document")
	})

	t.Run("unrelated file", func(t *testing.T) {
		got := reconcile.Documents([]reconcile.Document{doc3("docs/other.md", "Xray", "Yankee")}, prior)
		assert.Equal(t, reconcile.New, got[0].Kind)
		assert.NotEqual(t, "d-intro", got[0].Key)
	})
}

// The point of all of it: renaming a file must cost nothing downstream.
func TestDocuments_RenameLeavesEveryBlockUntouched(t *testing.T) {
	before := doc3("docs/intro.md", "Alpha", "Bravo", "Charlie")

	docs := reconcile.Documents([]reconcile.Document{before}, nil)
	scope := docs[0].Key

	v1 := reconcile.Blocks(scope, before.Blocks, nil)
	var prior []reconcile.Unit
	for i, b := range before.Blocks {
		u := reconcile.Identify(scope, b)
		u.Key = v1[i].Key
		prior = append(prior, u)
	}

	// The file is renamed. Nothing inside it changed.
	after := doc3("docs/getting-started.md", "Alpha", "Bravo", "Charlie")
	renamed := reconcile.Documents([]reconcile.Document{after}, []reconcile.DocUnit{docPrior(scope, before)})
	require.Equal(t, scope, renamed[0].Key, "the document keeps its identity")

	got := reconcile.Blocks(renamed[0].Key, after.Blocks, prior)
	for i := range got {
		assert.Equal(t, v1[i].Key, got[i].Key)
		assert.Equal(t, reconcile.Unchanged, got[i].Kind,
			"renaming a file must not report its blocks as moved")
	}
}

// Two files cannot both claim one prior, or they would share a history.
func TestDocuments_APriorIsClaimedOnce(t *testing.T) {
	prior := []reconcile.DocUnit{docPrior("d-only", doc3("docs/a.md", "Alpha", "Bravo"))}

	got := reconcile.Documents([]reconcile.Document{
		doc3("docs/copy1.md", "Alpha", "Bravo"),
		doc3("docs/copy2.md", "Alpha", "Bravo"),
	}, prior)

	require.Len(t, got, 2)
	assert.Equal(t, "d-only", got[0].Key)
	assert.NotEqual(t, "d-only", got[1].Key, "a copy is a new document")
	assert.Equal(t, reconcile.New, got[1].Kind)
}

// A file staying put outranks a file being renamed, because a path is a real
// identifier — the inverse of how block names are graded, and deliberately so.
func TestDocuments_PathBeatsContent(t *testing.T) {
	prior := []reconcile.DocUnit{docPrior("d-intro", doc3("docs/intro.md", "Alpha", "Bravo"))}

	// intro.md is rewritten wholesale, and its old text reappears in a new file.
	got := reconcile.Documents([]reconcile.Document{
		doc3("docs/intro.md", "Totally", "Different"),
		doc3("docs/moved.md", "Alpha", "Bravo"),
	}, prior)

	assert.Equal(t, "d-intro", got[0].Key, "the file at that path is still that document")
	assert.Equal(t, reconcile.Edited, got[0].Kind)
	assert.Equal(t, reconcile.New, got[1].Kind)
}

// Below the threshold a renamed-and-rewritten file is honestly new, rather than
// silently inheriting decisions about text it no longer contains.
func TestDocuments_RenameWithTooLittleLeftIsNew(t *testing.T) {
	prior := []reconcile.DocUnit{docPrior("d-intro", doc3("docs/intro.md", "Alpha", "Bravo", "Charlie"))}

	got := reconcile.Documents([]reconcile.Document{doc3("docs/new.md", "Alpha", "Xray", "Yankee")}, prior)
	assert.Equal(t, reconcile.New, got[0].Kind)
}

// An empty file has nothing to recognise it by, so it must not match anything.
func TestDocuments_EmptyDocumentDoesNotMatchOnContent(t *testing.T) {
	prior := []reconcile.DocUnit{docPrior("d-intro", doc3("docs/intro.md", "Alpha"))}

	got := reconcile.Documents([]reconcile.Document{{Path: "docs/empty.md"}}, prior)
	assert.Equal(t, reconcile.New, got[0].Kind)
}
