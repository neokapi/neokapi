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

// The gap, pinned as a fact rather than left as a comment.
//
// Prose formats have no natural key, so their readers number blocks by position
// — markdown "paraN", plaintext "lineN", and the same shape in html, xml, mdx
// and resx. Delete a paragraph and every block after it is renumbered, so a
// decision recorded about one paragraph names a different one, and content
// memory stops matching text that never changed.
//
// This is the dogfood's main exposure, not an edge case: neokapi-docs (277
// items) and bowrain-docs (110) are markdown — most of the corpus.
//
// The fix is identity derived from content plus an occurrence ordinal to
// disambiguate repeats, so identity moves only when the words move. It has to
// land with the skeleton refs (reader.go writes WriteRef(id), writer.go joins
// on blocksByID), which move together in one read/write pass — skeletons are
// recomputed at the edge, never stored, so there is no stored form to migrate.
//
// This test asserts the CURRENT behaviour so the change is deliberate: when the
// fix lands, this fails and gets deleted, and the formats move to the stable
// table above.
func TestBlockIdentity_ProseFormatsAreStillPositional(t *testing.T) {
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
				"identity still follows position, so an unrelated deletion renames this block")
			require.NotEqual(t, before["Charlie"], after["Charlie"],
				"delete this test when identity stops being positional")
		})
	}
}
