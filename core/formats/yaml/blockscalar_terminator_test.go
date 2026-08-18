package yaml_test

import (
	"testing"

	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A block scalar's terminating line break is part of its value: clip chomping
// (`|`, `>`) keeps exactly one, keep chomping (`|+`) keeps every one, and strip
// chomping (`|-`, `>-`) keeps none. The reader surfaces the value YAML defines,
// so the break arrives in the block's text — which is why a hygiene rule reading
// that text has to know the difference between a terminator and stray
// whitespace, and why the reader is not where that knowledge belongs.
//
// This pins both halves of the reason: the text and name of every block, which
// are the block's address and would move every committed target keyed against
// them, and the byte-exact round-trip that address is in service of.

// blockScalarProbe holds one scalar of each style, the four-scalar reproduction
// plus the two chomping modifiers that complete the set.
const blockScalarProbe = `literal: |
  A sentence in a literal block.
folded: >
  A sentence in a folded block
  wrapped over two lines.
stripped: |-
  A sentence with strip chomping.
foldedStripped: >-
  A sentence folded and stripped.
kept: |+
  A sentence with keep chomping.

plain: A plain scalar sentence.
`

func TestBlockScalar_TerminatorIsTheValueTheFormatDefines(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"literal":        "A sentence in a literal block.\n",
		"folded":         "A sentence in a folded block wrapped over two lines.\n",
		"stripped":       "A sentence with strip chomping.",
		"foldedStripped": "A sentence folded and stripped.",
		"kept":           "A sentence with keep chomping.\n\n",
		"plain":          "A plain scalar sentence.",
	}

	reader := yamlfmt.NewReader()
	require.NoError(t, reader.Open(t.Context(), testutil.RawDocFromString(blockScalarProbe, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	got := map[string]string{}
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		require.True(t, ok)
		got[b.Name] = b.SourceText()
	}
	assert.Equal(t, want, got)
}

func TestBlockScalar_EveryStyleRoundTripsByteExact(t *testing.T) {
	t.Parallel()
	assert.Equal(t, blockScalarProbe, snippetRoundtripWithSkeleton(t, blockScalarProbe))
}
