package mdx

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// MDX inherits markdown's translation-invariant address along with its naming
// state: the prose spans are read by markdown.Readers sharing one NamingState,
// and MDX only re-IDs the blocks they emit. This pins that the address survives
// that hand-off, because the repository's own published documentation is MDX and
// pairing it with its translations is what the address exists for.
func mdxAddresses(t *testing.T, src string) map[string]string {
	t.Helper()

	r := NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	r.SetSkeletonStore(store)
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		Reader:       io.NopCloser(bytes.NewReader([]byte(src))),
		SourceLocale: model.LocaleEnglish,
	}))

	out := map[string]string{}
	for pr := range r.Read(context.Background()) {
		require.NoError(t, pr.Error)
		if pr.Part == nil || pr.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok && b != nil {
			if text := b.SourceText(); text != "" {
				out[text] = b.StructuralAddress()
			}
		}
	}
	return out
}

func TestMDXAddress_SurvivesTranslation(t *testing.T) {
	const source = `import Note from '@site/src/components/Note';

# Tidewatch

An opening paragraph.

<Note>

A paragraph inside the component.

</Note>

## What it reads

The first paragraph of the section.
`

	const translated = `import Note from '@site/src/components/Note';

# Tidevakt

Et innledende avsnitt.

<Note>

Et avsnitt inne i komponenten.

</Note>

## Hva den leser

Det første avsnittet i seksjonen.
`

	src := mdxAddresses(t, source)
	tgt := mdxAddresses(t, translated)

	assert.Equal(t, "h/p", src["An opening paragraph."])
	assert.Equal(t, "h/p", tgt["Et innledende avsnitt."])
	assert.Equal(t, "h/h/p", src["The first paragraph of the section."])
	assert.Equal(t, "h/h/p", tgt["Det første avsnittet i seksjonen."],
		"a block under a translated heading addresses the same in both documents")

	names := mdxNames(t, source)
	assert.Equal(t, "tidewatch/what-it-reads/p", names["The first paragraph of the section."],
		"the readable name still carries its heading's words")
}
