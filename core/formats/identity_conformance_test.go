package formats_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/formats/plaintext"
	"github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// Block identity has to survive an edit to the file the block lives in.
//
// Everything downstream matches on it: a review decision is recorded against a
// unit (AD-033), the content memory recycles against it, and an iterative
// workflow re-reads the same file after every change. If identity moves when an
// unrelated block is deleted, a decision made about one string silently becomes
// a decision about another, and recycling misses work that has not changed.
//
// AD-003 already names the hazard — format readers "assign IDs from the source
// format (XLIFF tu1, tu2, etc.), but these are not unique across files" — and
// pushes reconciliation to the persistence layer. This test asks the narrower
// question that layer depends on: within ONE file, across an edit, does a block
// that did not change keep the identity the rest of the system matches it by?
//
// The identity under test is convergence.BlockKey — Name when the reader sets
// one, the reader's ID otherwise — because that is what host/convergereport and
// host/coverage actually key on.
func TestBlockIdentity_SurvivesAnUnrelatedDeletion(t *testing.T) {
	cases := []struct {
		name   string
		reader func() format.DataFormatReader
		v1, v2 string
		// stable records the measured state. Keyed formats derive identity from
		// the key, so it survives an edit. Prose formats number positionally and
		// do not — see TestBlockIdentity_ProseFormatsAreStillPositional.
		stable bool
	}{
		{
			name:   "json",
			stable: true,
			reader: func() format.DataFormatReader { return json.NewReader() },
			v1:     `{"a":"Alpha","b":"Bravo","c":"Charlie"}`,
			v2:     `{"a":"Alpha","c":"Charlie"}`,
		},
		{
			name:   "yaml",
			stable: true,
			reader: func() format.DataFormatReader { return yaml.NewReader() },
			v1:     "a: Alpha\nb: Bravo\nc: Charlie\n",
			v2:     "a: Alpha\nc: Charlie\n",
		},
		{
			name:   "markdown",
			reader: func() format.DataFormatReader { return markdown.NewReader() },
			v1:     "Alpha\n\nBravo\n\nCharlie\n",
			v2:     "Alpha\n\nCharlie\n",
		},
		{
			name:   "plaintext",
			reader: func() format.DataFormatReader { return plaintext.NewReader() },
			v1:     "Alpha\n\nBravo\n\nCharlie\n",
			v2:     "Alpha\n\nCharlie\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.stable {
				t.Skip("known gap — see TestBlockIdentity_ProseFormatsAreStillPositional")
			}
			before := identitiesByText(t, tc.reader(), tc.v1)
			after := identitiesByText(t, tc.reader(), tc.v2)

			require.Contains(t, before, "Alpha")
			require.Contains(t, after, "Alpha")
			require.Contains(t, before, "Charlie", "Charlie must be read from v1")
			require.Contains(t, after, "Charlie", "Charlie survives the edit — only Bravo was deleted")

			// Alpha precedes the deletion, so a positional scheme gets it right
			// by luck. It is here to prove the test is not simply broken.
			require.Equal(t, before["Alpha"], after["Alpha"],
				"a block before the deletion must keep its identity")

			// Charlie follows the deletion. This is the one that matters: its
			// text did not change, so nothing about it was re-decided.
			require.Equal(t, before["Charlie"], after["Charlie"],
				"a block AFTER an unrelated deletion must keep its identity — "+
					"otherwise a decision recorded about Charlie now names whatever "+
					"took its slot, and content memory stops matching text that never changed")
		})
	}
}

// identitiesByText reads content and maps each block's source text to the
// identity the rest of the system matches it by.
func identitiesByText(t *testing.T, r format.DataFormatReader, content string) map[string]string {
	t.Helper()

	ctx := context.Background()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))

	out := map[string]string{}
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil || pr.Part.Type != model.PartBlock {
			continue
		}
		b, ok := pr.Part.Resource.(*model.Block)
		if !ok || b == nil {
			continue
		}
		text := b.SourceText()
		if text == "" {
			continue
		}
		out[text] = convergence.BlockKey(b)
	}
	return out
}

// Prose formats name blocks by position, and that is left in place.
//
// Keyed formats above are stable because their name IS the key. Prose formats
// have no key, so markdown names by "paraN", plaintext by "lineN", and the same
// shape appears in html, xml, mdx and resx. Delete a paragraph and every block
// below it is renamed.
//
// Making those names content-derived was tried and rejected. It fixes deletion
// and breaks editing — a reworded paragraph becomes a different block and loses
// its history — and worse, model.ComputeIdentity builds ContextHash from the
// name, so deriving the name from content collapses the two hashes into one and
// destroys the independent signal that identity matching depends on.
//
// So the positional name stays, and is read as what it honestly is: a CONTEXT
// signal. Identity is resolved against it in core/reconcile, which matches a
// read against the previous one on content and context together. See
// TestReaders_IdentitySurvivesAnUnrelatedDeletion for the end-to-end claim.
//
// This test pins the reader behaviour that reconcile is built on: if these names
// ever stop moving, the context signal has changed and the matching rules need
// re-examining.
func TestBlockIdentity_ProseFormatsNamePositionally(t *testing.T) {
	for _, tc := range []struct {
		name       string
		reader     func() format.DataFormatReader
		v1, v2     string
		wasCharlie string
		nowCharlie string
	}{
		{
			name:       "markdown",
			reader:     func() format.DataFormatReader { return markdown.NewReader() },
			v1:         "Alpha\n\nBravo\n\nCharlie\n",
			v2:         "Alpha\n\nCharlie\n",
			wasCharlie: "para3", nowCharlie: "para2",
		},
		{
			name:       "plaintext",
			reader:     func() format.DataFormatReader { return plaintext.NewReader() },
			v1:         "Alpha\n\nBravo\n\nCharlie\n",
			v2:         "Alpha\n\nCharlie\n",
			wasCharlie: "line5", nowCharlie: "line3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := identitiesByText(t, tc.reader(), tc.v1)
			after := identitiesByText(t, tc.reader(), tc.v2)

			require.Equal(t, tc.wasCharlie, before["Charlie"])
			require.Equal(t, tc.nowCharlie, after["Charlie"],
				"a prose block's name follows position — this is the context signal reconcile reads")
			require.NotEqual(t, before["Charlie"], after["Charlie"],
				"names moving is the premise core/reconcile is built on")
		})
	}
}
