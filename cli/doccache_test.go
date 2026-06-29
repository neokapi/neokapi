package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDocCache_StreamingRoundTrip records a parsed document (blocks + a skeleton)
// through the streaming sink and replays it through the streaming source, with no
// whole-document buffering: parts come back one at a time, and the skeleton is
// opened lazily from its file (a process-only consumer never opens it).
func TestDocCache_StreamingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "m.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"a":"Apple","b":"Banana"}`), 0o644))

	c, err := openDocCache(filepath.Join(dir, "cache"))
	require.NoError(t, err)
	defer c.close()

	// Miss → record a document streaming: two blocks + a skeleton entry.
	require.Nil(t, c.OpenDocument(src, "k"), "cold cache is a miss")
	rec := c.RecordDocument(src, "k", "json")
	require.NotNil(t, rec)
	require.NoError(t, rec.SkeletonStore().WriteText([]byte("{")))
	for _, b := range []*model.Block{mkBlock("a", "Apple"), mkBlock("b", "Banana")} {
		require.NoError(t, rec.Add(&model.Part{Type: model.PartBlock, Resource: b}))
		require.NoError(t, rec.SkeletonStore().WriteRef(b.ID))
	}
	require.NoError(t, rec.SkeletonStore().WriteText([]byte("}")))
	require.NoError(t, rec.Commit())

	// Hit → stream parts back one at a time.
	doc := c.OpenDocument(src, "k")
	require.NotNil(t, doc, "after record, the document is a hit")
	defer doc.Close()
	ch := make(chan *model.Part, 1)
	go func() { _ = doc.Feed(context.Background(), ch) }()
	var got []string
	for p := range ch {
		if b, ok := p.Resource.(*model.Block); ok {
			got = append(got, b.SourceText())
		}
	}
	assert.Equal(t, []string{"Apple", "Banana"}, got, "parts replay in order from the streamed log")

	// The skeleton is a real file, opened lazily and streamed entry-by-entry.
	skel := doc.OpenSkeleton()
	require.NotNil(t, skel, "a recorded skeleton is available to a writer")
	defer skel.Close()
	refs := 0
	for {
		e, err := skel.Next()
		if err != nil {
			break
		}
		if e.Type == format.SkeletonRef {
			refs++
		}
	}
	assert.Equal(t, 2, refs, "skeleton streams its block refs from the file")

	// Staleness: change the source → miss (re-parse).
	require.NoError(t, os.WriteFile(src, []byte(`{"a":"Cherry"}`), 0o644))
	assert.Nil(t, c.OpenDocument(src, "k"), "a changed source is a miss")
}

func mkBlock(id, text string) *model.Block {
	b := model.NewBlock(id, text)
	b.Translatable = true
	return b
}
