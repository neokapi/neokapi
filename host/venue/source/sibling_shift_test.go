package source_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readMarkdown drives the real reader, so the names under test are the ones a
// push actually sends rather than ones a test invented.
func readMarkdown(t *testing.T, content string) []*model.Block {
	t.Helper()
	ctx := context.Background()
	r := markdown.NewReader()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI:          "test://guide.md",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(strings.NewReader(content)),
	}))

	var out []*model.Block
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil || pr.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok && b != nil && b.Translatable {
			out = append(out, b)
		}
	}
	return out
}

func keysByText(blocks []*model.Block) map[string]string {
	out := map[string]string{}
	for _, b := range blocks {
		out[b.SourceText()] = convergence.BlockKey(b)
	}
	return out
}

func namesByText(blocks []*model.Block) map[string]string {
	out := map[string]string{}
	for _, b := range blocks {
		out[b.SourceText()] = b.Name
	}
	return out
}

const threeParagraphs = `## Install

Alpha

Bravo

Charlie
`

const middleDeleted = `## Install

Alpha

Charlie
`

// Deleting a paragraph must not hand its neighbour's translation to the one
// below it.
//
// This is the defect the whole identity path exists to close, and the reader's
// own tests pin the half that causes it: markdown addresses a paragraph by its
// heading trail and its ordinal, so deleting `Bravo` re-addresses `Charlie`
// from `install/p#3` to `install/p#2`
// (TestMarkdownNames_ShiftWithinTheSameSection). Keyed on that name, a venue
// prunes Charlie's row — with its translation and its approval — and lands
// Charlie's text on the row that held Bravo's.
//
// The names still shift. What changes is that they are no longer the key.
func TestADeletedParagraphDoesNotReAddressItsSiblings(t *testing.T) {
	before := readMarkdown(t, threeParagraphs)
	after := readMarkdown(t, middleDeleted)

	// The reader's names shift, exactly as its own tests pin.
	require.Equal(t, "install/p#3", namesByText(before)["Charlie"])
	require.Equal(t, "install/p#2", namesByText(after)["Charlie"])

	// Resolve the first read, then carry its identity forward as a venue's tree
	// would and resolve the second against it.
	first := host.ResolveIdentity(map[string][]*model.Block{"docs/guide.md": before}, host.Priors{})
	tree := treeFrom(t, "docs/guide.md", first)
	host.ResolveIdentity(map[string][]*model.Block{"docs/guide.md": after}, host.Priors{
		Documents: tree.Units(),
		Units:     tree.Priors(),
	})

	wasKeyed, nowKeyed := keysByText(before), keysByText(after)

	assert.Equal(t, wasKeyed["Charlie"], nowKeyed["Charlie"],
		"Charlie's words never changed, so its translation and its approval must follow it")
	assert.Equal(t, wasKeyed["Alpha"], nowKeyed["Alpha"],
		"and the paragraph above the deletion is untouched")
	assert.NotEqual(t, wasKeyed["Bravo"], nowKeyed["Charlie"],
		"Charlie must not inherit the key of the paragraph that was deleted — that is how one paragraph's translation ends up under another's words")

	// The declared tree is what a venue prunes against, so the shift has to be
	// absent from THAT, not merely from the blocks.
	declared := venue.TreeFromBlocks(map[string][]*model.Block{"docs/guide.md": after})
	assert.Contains(t, declared["docs/guide.md"].Keys, wasKeyed["Charlie"],
		"the declaration keeps Charlie's key, so a venue holding it prunes nothing")
	assert.NotContains(t, declared["docs/guide.md"].Keys, wasKeyed["Bravo"],
		"and drops the deleted paragraph's, so the venue removes exactly that one")
}

// treeFrom renders a resolution as the venue's tree: keys, content hashes and
// context hashes per item, with the document's identity as its id — which is
// the shape a producer fetches and resolves against.
func treeFrom(t *testing.T, path string, docs []host.ResolvedDocument) venue.Tree {
	t.Helper()
	require.Len(t, docs, 1)
	d := docs[0]

	ti := venue.TreeItem{Path: path, ID: d.Scope}
	for _, b := range d.Blocks {
		id := reconcile.Identify(d.Scope, b.Block)
		ti.Keys = append(ti.Keys, b.Unit)
		ti.Content = append(ti.Content, id.ContentHash)
		ti.Context = append(ti.Context, id.ContextHash)
	}
	return venue.Tree{path: ti}
}

// Without priors there is nothing to match against, and the keys are the
// reader's names — which is the behaviour that loses the translation. Pinned so
// the fix above is measured against the thing it fixes rather than asserted in
// isolation.
func TestWithoutPriorsTheNamesAreStillTheKeys(t *testing.T) {
	before := readMarkdown(t, threeParagraphs)
	after := readMarkdown(t, middleDeleted)

	host.ResolveIdentity(map[string][]*model.Block{"docs/guide.md": before}, host.Priors{})
	// No priors carried forward: a venue that cannot be reached, or one too old
	// to serve context hashes.
	host.ResolveIdentity(map[string][]*model.Block{"docs/guide.md": after}, host.Priors{})

	wasKeyed, nowKeyed := keysByText(before), keysByText(after)
	assert.NotEqual(t, wasKeyed["Charlie"], nowKeyed["Charlie"],
		"with nothing to match against, Charlie is a new unit — the loss this change exists to prevent")
}
