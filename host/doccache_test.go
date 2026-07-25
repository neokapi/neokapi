package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/jsx"
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

	c, err := OpenDocCache(filepath.Join(dir, "cache"))
	require.NoError(t, err)
	defer c.Close()

	// Miss → record a document streaming: two blocks + a skeleton entry.
	require.Nil(t, c.OpenDocument(src, "k"), "cold cache is a miss")
	rec := c.RecordDocument(src, "k", "json")
	require.NotNil(t, rec)
	rec.SkeletonStore().WriteText([]byte("{"))
	for _, b := range []*model.Block{mkBlock("a", "Apple"), mkBlock("b", "Banana")} {
		require.NoError(t, rec.Add(&model.Part{Type: model.PartBlock, Resource: b}))
		rec.SkeletonStore().WriteRef(b.ID)
	}
	rec.SkeletonStore().WriteText([]byte("}"))
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
	skel, serr := doc.OpenSkeleton()
	require.NoError(t, serr)
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

// #1449 (same family). A recorded-but-missing skeleton is an ERROR, not "this
// document has no skeleton". Conflating the two made a run reconstruct the file
// from the content model alone — losing the source's exact formatting — and
// report success.
func TestDocCache_RecordedSkeletonMissing_IsAnError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "m.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"a":"Apple"}`), 0o644))

	c, err := OpenDocCache(filepath.Join(dir, "cache"))
	require.NoError(t, err)
	defer c.Close()

	rec := c.RecordDocument(src, "k", "json")
	require.NotNil(t, rec)
	rec.SkeletonStore().WriteText([]byte("{"))
	require.NoError(t, rec.Add(&model.Part{Type: model.PartBlock, Resource: mkBlock("a", "Apple")}))
	rec.SkeletonStore().WriteRef("a")
	require.NoError(t, rec.Commit())

	doc := c.OpenDocument(src, "k")
	require.NotNil(t, doc)
	defer doc.Close()

	// Delete the skeleton the index promised.
	cd, ok := doc.(*cachedDocument)
	require.True(t, ok)
	require.NotEmpty(t, cd.skeletonPath, "the entry must have recorded a skeleton for this to be meaningful")
	require.NoError(t, os.Remove(cd.skeletonPath))

	skel, serr := doc.OpenSkeleton()
	require.Error(t, serr, "a promised-but-absent skeleton must not read as 'no skeleton'")
	assert.Nil(t, skel)
	assert.Contains(t, serr.Error(), cd.skeletonPath)
}

// The other half: a document that recorded NO skeleton (a generative format)
// still reports (nil, nil) — the guard refuses faults, not skeleton-less formats.
func TestDocCache_NoSkeletonRecorded_IsNotAnError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "m.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"a":"Apple"}`), 0o644))

	c, err := OpenDocCache(filepath.Join(dir, "cache"))
	require.NoError(t, err)
	defer c.Close()

	rec := c.RecordDocument(src, "k", "json")
	require.NotNil(t, rec)
	// No SkeletonStore() calls at all — a lean, skeleton-less recording.
	require.NoError(t, rec.Add(&model.Part{Type: model.PartBlock, Resource: mkBlock("a", "Apple")}))
	require.NoError(t, rec.Commit())

	doc := c.OpenDocument(src, "k")
	require.NotNil(t, doc)
	defer doc.Close()

	skel, serr := doc.OpenSkeleton()
	require.NoError(t, serr)
	assert.Nil(t, skel)
}

// TestDocCache_PreservesBlockAnnotations guards a real corruption bug: the
// streaming cache serializes each block's stand-off annotations and, on replay,
// drops any whose payload type is not registered (see fromCachedAnnotations). A
// block served from cache must come back with its annotations intact. The
// jsx/.kbf reader stashes the block's content hash (plus placeholders and
// preview) in a KBFAnnotation; if that type isn't registered, cache replay
// silently strips it and reconstructed .kbf targets lose their hashes — which
// broke `kapi up` non-deterministically, since its concurrent per-locale
// converge workers share the cache and race on the same cached source document.
func TestDocCache_PreservesBlockAnnotations(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "App.kbf")
	require.NoError(t, os.WriteFile(src, []byte(`{"schemaVersion":"1.0"}`), 0o644))

	c, err := OpenDocCache(filepath.Join(dir, "cache"))
	require.NoError(t, err)
	defer c.Close()

	block := mkBlock("src/App.tsx:38:0", "Home")
	block.SetAnno(jsx.AnnotationType, &jsx.KBFAnnotation{
		Hash: "kRS4Hxwnmv9", DocumentID: "src/App.tsx", DocumentPath: "src/App.tsx",
	})

	require.Nil(t, c.OpenDocument(src, "k"), "cold cache is a miss")
	rec := c.RecordDocument(src, "k", "kbf")
	require.NotNil(t, rec)
	require.NoError(t, rec.Add(&model.Part{Type: model.PartBlock, Resource: block}))
	require.NoError(t, rec.Commit())

	doc := c.OpenDocument(src, "k")
	require.NotNil(t, doc, "after record, the document is a hit")
	defer doc.Close()
	ch := make(chan *model.Part, 1)
	go func() { _ = doc.Feed(context.Background(), ch) }()
	var got *model.Block
	for p := range ch {
		if b, ok := p.Resource.(*model.Block); ok {
			got = b
		}
	}
	require.NotNil(t, got, "the block replays from cache")

	raw, ok := got.Anno(jsx.AnnotationType)
	require.True(t, ok, "the KBFAnnotation must survive the cache round-trip (payload type must be registered)")
	ann, ok := raw.(*jsx.KBFAnnotation)
	require.True(t, ok, "the rehydrated annotation is a *KBFAnnotation")
	assert.Equal(t, "kRS4Hxwnmv9", ann.Hash, "the block content hash must survive cache replay")
}

func mkBlock(id, text string) *model.Block {
	b := model.NewBlock(id, text)
	b.Translatable = true
	return b
}
