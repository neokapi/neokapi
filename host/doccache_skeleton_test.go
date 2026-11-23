package host

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/model"
)

// replayParts records parts as one document in a fresh cache, opens it again
// and returns what the cache replays. It fails the test when the recording
// does not come back as a hit.
func replayParts(t *testing.T, parts []*model.Part) []*model.Part {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "page.html")
	require.NoError(t, os.WriteFile(src, []byte("<p>x</p>"), 0o644))
	c, err := OpenDocCache(filepath.Join(dir, "cache"))
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	rec := c.RecordDocument(src, "k", "html")
	require.NotNil(t, rec)
	for _, p := range parts {
		require.NoError(t, rec.Add(p))
	}
	require.NoError(t, rec.Commit())

	doc := c.OpenDocument(src, "k")
	require.NotNil(t, doc, "a recording whose blocks carry an inline skeleton is a hit, not discarded")
	t.Cleanup(func() { doc.Close() })
	ch := make(chan *model.Part, 1)
	go func() { _ = doc.Feed(context.Background(), ch) }()
	var got []*model.Part
	for p := range ch {
		got = append(got, p)
	}
	return got
}

// A block's and a Data's inline skeleton, text fragments around a reference,
// replays from the part log as it was recorded.
func TestDocCache_InlineSkeletonRoundTrips(t *testing.T) {
	block := mkBlock("b1", "Hello")
	block.Skeleton = &model.Skeleton{
		Strategy: model.SkeletonFragmentBased,
		Parts: []model.SkeletonPart{
			&model.SkeletonText{Text: `<p class="lead">`},
			&model.SkeletonRef{ResourceID: "b1", Property: "target", Locale: "fr"},
			&model.SkeletonText{Text: "</p>"},
		},
	}
	data := &model.Data{ID: "d1", Skeleton: &model.Skeleton{
		Strategy: model.SkeletonFragmentBased,
		Parts:    []model.SkeletonPart{&model.SkeletonText{Text: "<hr/>"}},
	}}
	plain := mkBlock("b2", "No skeleton")

	got := replayParts(t, []*model.Part{
		{Type: model.PartBlock, Resource: block},
		{Type: model.PartData, Resource: data},
		{Type: model.PartBlock, Resource: plain},
	})
	require.Len(t, got, 3)
	gb, ok := got[0].Resource.(*model.Block)
	require.True(t, ok)
	assert.Equal(t, block.Skeleton, gb.Skeleton)
	gd, ok := got[1].Resource.(*model.Data)
	require.True(t, ok)
	assert.Equal(t, data.Skeleton, gd.Skeleton)
	gp, ok := got[2].Resource.(*model.Block)
	require.True(t, ok)
	assert.Nil(t, gp.Skeleton, "a block with no skeleton replays with none")
}

// The blocks the HTML reader yields, the reader a subfilter hands HTML to,
// each carry an inline skeleton; a recording of the parts replays them as the
// reader produced them.
func TestDocCache_HTMLReaderPartsReplayAsRead(t *testing.T) {
	r := html.NewReader()
	src := `<html><head><title>T</title></head><body><h1 id="t">Title</h1><p>First <b>bold</b> line.</p>` +
		`<ul><li>a</li></ul><img src="x.png" alt="pic"></body></html>`
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		URI: "page.html", Reader: io.NopCloser(strings.NewReader(src)), SourceLocale: model.LocaleEnglish,
	}))
	var parts []*model.Part
	withSkeleton := 0
	for res := range r.Read(context.Background()) {
		require.NoError(t, res.Error)
		parts = append(parts, res.Part)
		if b, ok := res.Part.Resource.(*model.Block); ok && b.Skeleton != nil {
			withSkeleton++
		}
	}
	require.Positive(t, withSkeleton, "the HTML reader gives its blocks an inline skeleton")

	got := replayParts(t, parts)
	require.Len(t, got, len(parts))
	for i := range parts {
		assert.Equal(t, parts[i], got[i], "part %d replays as read", i)
	}
}
