package reconcile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// The context signal is what separates two blocks that say the same thing, and
// what recognises one block whose words have changed. Everything reconcile
// decides rests on it, so what it does and does not distinguish is pinned here.

func typed(name, typ, text string) *model.Block {
	b := model.NewBlock("tu-"+name, text)
	b.Name, b.Type = name, typ
	return b
}

func withProp(name, text, k, v string) *model.Block {
	b := blk(name, text)
	b.Properties[k] = v
	return b
}

// Identical words in different places are different units. This is the whole
// reason context exists: "Open" on a button and "Open" in a sentence are the
// same string and not the same unit, and must never share a decision.
func TestContext_SeparatesIdenticalText(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b *model.Block
	}{
		{"different name", blk("button.open", "Open"), blk("body.open", "Open")},
		{"different type", typed("x", "heading", "Open"), typed("x", "paragraph", "Open")},
		{"different property", withProp("x", "Open", "level", "2"), withProp("x", "Open", "level", "3")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ia, ib := reconcile.Identify(doc, tc.a), reconcile.Identify(doc, tc.b)
			assert.Equal(t, ia.ContentHash, ib.ContentHash, "precondition: the words are identical")
			assert.NotEqual(t, ia.ContextHash, ib.ContextHash, "but the context must tell them apart")

			got := reconcile.Blocks(doc, []*model.Block{tc.a, tc.b}, nil)
			require.Len(t, got, 2)
			assert.NotEqual(t, got[0].Key, got[1].Key, "so they resolve to two units")
		})
	}
}

// A block's name is only unique within its own document. Every markdown file in
// a project has a "para1"; every plaintext file has a "line1". Reconciling
// across documents without saying which document a block came from would let a
// paragraph in one file claim the history of an unrelated paragraph in another.
func TestContext_SeparatesDocuments(t *testing.T) {
	// Same position, same type, same everything the block itself knows.
	intro := reconcile.Identify("docs/intro.md", blk("para1", "Alpha"))
	intro.Key = "u-intro-para1"

	// Bravo is a different paragraph in a different file that happens to sit in
	// the same slot. It must not inherit intro.md's unit.
	got := reconcile.Blocks("docs/guide.md",
		[]*model.Block{blk("para1", "Bravo")}, []reconcile.Unit{intro})
	require.Len(t, got, 1)
	assert.Equal(t, reconcile.New, got[0].Kind,
		"a same-named block in another document is not an edit of this one")
	assert.NotEqual(t, "u-intro-para1", got[0].Key)
}

// The corollary: within one document the scope changes nothing, and a file that
// is renamed does not orphan its contents wholesale... which it does today.
// Recorded so the cost of scoping by path is explicit rather than discovered.
func TestContext_ScopeIsPartOfIdentity(t *testing.T) {
	before := reconcile.Identify("docs/intro.md", blk("para1", "Alpha"))
	before.Key = "u-alpha"

	got := reconcile.Blocks("docs/getting-started.md",
		[]*model.Block{blk("para1", "Alpha")}, []reconcile.Unit{before})

	// The words are untouched, so content still finds it: a file rename reads as
	// every block moving, not as the whole file being replaced.
	assert.Equal(t, "u-alpha", got[0].Key, "content carries identity across a file rename")
	assert.Equal(t, reconcile.Moved, got[0].Kind)
}

// Position shifting is the ordinary case and must not cost identity.
func TestContext_InsertionAtTheTopShiftsEveryName(t *testing.T) {
	v1 := []*model.Block{blk("para1", "Alpha"), blk("para2", "Bravo")}
	prior := []reconcile.Unit{priorOf("u-alpha", v1[0]), priorOf("u-bravo", v1[1])}

	// A new opening paragraph renames both existing blocks.
	got := byText(reconcile.Blocks(doc, []*model.Block{blk("para1", "Intro"), blk("para2", "Alpha"), blk("para3", "Bravo")}, prior))

	assert.Equal(t, "u-alpha", got["Alpha"].Key)
	assert.Equal(t, "u-bravo", got["Bravo"].Key)
	assert.Equal(t, reconcile.Moved, got["Alpha"].Kind)
	assert.Equal(t, reconcile.New, got["Intro"].Kind)
}

// Swapping two blocks keeps both identities: their words went with them.
func TestContext_ReorderKeepsBothUnits(t *testing.T) {
	v1 := []*model.Block{blk("para1", "Alpha"), blk("para2", "Bravo")}
	prior := []reconcile.Unit{priorOf("u-alpha", v1[0]), priorOf("u-bravo", v1[1])}

	got := byText(reconcile.Blocks(doc, []*model.Block{blk("para1", "Bravo"), blk("para2", "Alpha")}, prior))

	assert.Equal(t, "u-alpha", got["Alpha"].Key)
	assert.Equal(t, "u-bravo", got["Bravo"].Key)
}

// A block that changes type in place is not the same unit continuing: promoting
// a paragraph to a heading changes how it is written, read and translated.
func TestContext_TypeChangeIsNotAnEdit(t *testing.T) {
	prior := []reconcile.Unit{priorOf("u-para", typed("x", "paragraph", "Alpha"))}

	got := reconcile.Blocks(doc, []*model.Block{typed("x", "heading", "Alpha")}, prior)

	// Content still matches, so identity is carried and the change is visible as
	// a move rather than being silently absorbed.
	assert.Equal(t, "u-para", got[0].Key)
	assert.Equal(t, reconcile.Moved, got[0].Kind)
}

// Content normalization is trim-only by contract, so these must be graded on
// their words alone. Pinned because a looser normalization would silently merge
// units that are genuinely different to a translator.
func TestContext_ContentNormalizationBoundaries(t *testing.T) {
	prior := []reconcile.Unit{priorOf("u-alpha", blk("para1", "Alpha"))}

	t.Run("surrounding whitespace is not a change", func(t *testing.T) {
		got := reconcile.Blocks(doc, []*model.Block{blk("para1", "  Alpha  ")}, prior)
		assert.Equal(t, reconcile.Unchanged, got[0].Kind)
	})

	t.Run("case is a change", func(t *testing.T) {
		got := reconcile.Blocks(doc, []*model.Block{blk("para1", "alpha")}, prior)
		assert.Equal(t, reconcile.Edited, got[0].Kind)
		assert.Equal(t, "u-alpha", got[0].Key)
	})

	t.Run("interior spacing is a change", func(t *testing.T) {
		got := reconcile.Blocks(doc, []*model.Block{blk("para1", "Al  pha")}, prior)
		assert.Equal(t, reconcile.Edited, got[0].Kind)
	})
}

// Empty and near-empty context must not become a wildcard that matches anything.
func TestContext_UnnamedBlocksDoNotCollide(t *testing.T) {
	a, b := model.NewBlock("tu1", "Alpha"), model.NewBlock("tu2", "Bravo")

	got := reconcile.Blocks(doc, []*model.Block{a, b}, nil)
	require.Len(t, got, 2)
	assert.NotEqual(t, got[0].Key, got[1].Key,
		"two unnamed blocks are still two units")
}

// The corpus shape this was built for: many documents, each with a "para1",
// reconciled one document at a time against the whole project's units.
func TestContext_ManyDocumentsSharingBlockNames(t *testing.T) {
	docs := map[string]string{
		"docs/intro.md":   "Welcome",
		"docs/guide.md":   "Getting started",
		"docs/install.md": "Requirements",
	}

	// First pass over a project with no history mints one unit per document.
	var project []reconcile.Unit
	keys := map[string]string{}
	for path, text := range docs {
		got := reconcile.Blocks(path, []*model.Block{blk("para1", text)}, project)
		require.Len(t, got, 1)
		require.Equal(t, reconcile.New, got[0].Kind)

		u := reconcile.Identify(path, blk("para1", text))
		u.Key = got[0].Key
		project = append(project, u)
		keys[path] = got[0].Key
	}
	require.Len(t, unique(values(keys)), 3, "three documents, three units")

	// Second pass: every document sees the whole project's units and must still
	// recognise only its own, despite every block being named "para1".
	for path, text := range docs {
		got := reconcile.Blocks(path, []*model.Block{blk("para1", text)}, project)
		assert.Equal(t, keys[path], got[0].Key, "%s must keep its own unit", path)
		assert.Equal(t, reconcile.Unchanged, got[0].Kind)
	}
}

// Text moved between files keeps its translation. This is why scope binds the
// context hash only and leaves content document-free.
func TestContext_TextMovedBetweenDocuments(t *testing.T) {
	origin := reconcile.Identify("docs/intro.md", blk("para2", "Install the CLI"))
	origin.Key = "u-install"

	// The paragraph is cut from intro.md and pasted into install.md.
	got := reconcile.Blocks("docs/install.md",
		[]*model.Block{blk("para1", "Install the CLI")}, []reconcile.Unit{origin})

	assert.Equal(t, "u-install", got[0].Key,
		"the same sentence in a new file is not new work")
	assert.Equal(t, reconcile.Moved, got[0].Kind)
}

func values(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
