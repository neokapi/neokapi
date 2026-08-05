package openxml

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A drawing's destination is not in the element that names it: <a:blip r:embed>
// points at a relationship, and only that lookup yields the image part. Until
// the reader resolved it, an embedded image exported as four lines of graphic
// metadata — a name, a filename, and the alt text once per non-visual-properties
// element — with no image anywhere in the output.

const imageFixture = "../../../web/static/samples/embedded-image.docx"

func readDocxFileBlocks(t *testing.T, path string) []*model.Block {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := NewReader()
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })

	var blocks []*model.Block
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// pictureBlock returns the detached RolePicture block, failing if there is none.
func pictureBlock(t *testing.T, blocks []*model.Block) *model.Block {
	t.Helper()
	for _, b := range blocks {
		if b.SemanticRole() == model.RolePicture {
			return b
		}
	}
	t.Fatal("no RolePicture block was emitted for the drawing")
	return nil
}

func TestDrawingResolvesSourceAndAltText(t *testing.T) {
	pic := pictureBlock(t, readDocxFileBlocks(t, imageFixture))

	require.Len(t, pic.Source, 1)
	ph := pic.Source[0].Ph
	require.NotNil(t, ph, "the picture block should carry the drawing's placeholder run")
	assert.Equal(t, TypeImage, ph.Type)

	assert.Equal(t, "media/image1.png", ph.Attrs[model.AttrSrc],
		"the blip's relationship should resolve to the image part")
	assert.Equal(t, "A quarterly results figure", ph.Attrs[model.AttrAlt],
		"descr= should resolve to alt text, through the marker that replaced it")

	// The verbatim markup the skeleton path replays is untouched.
	assert.Contains(t, ph.Data, "<w:drawing>")
	assert.Contains(t, ph.Data, `r:embed="rId1"`)
}

// The picture block must stay out of the translation unit stream: it is
// surfaced for export, not for a translator.
func TestPictureBlockIsNotTranslatable(t *testing.T) {
	pic := pictureBlock(t, readDocxFileBlocks(t, imageFixture))
	assert.False(t, pic.Translatable)
	assert.Empty(t, pic.SourceText(), "a picture carries no text of its own")
}

// The graphic metadata is still surfaced — the fix suppresses it on the
// generative path only, and an ingestion consumer and the editor still see it.
func TestDrawingPropertyBlocksAreStillSurfaced(t *testing.T) {
	blocks := readDocxFileBlocks(t, imageFixture)

	byElement := map[string][]string{}
	for _, b := range blocks {
		if el := b.Properties["element"]; el != "" {
			byElement[el] = append(byElement[el], b.SourceText())
		}
	}
	assert.Equal(t, []string{"Picture 1", "image1.png"}, byElement["drawing-name"])
	assert.Equal(t, []string{"A quarterly results figure", "A quarterly results figure"},
		byElement["drawing-descr"])
}

// With non-translatable surfacing off — the configuration the parity runner
// uses — nothing additive is emitted and the part stream is unchanged.
func TestPictureBlockIsGatedOnNonTranslatableSurfacing(t *testing.T) {
	data, err := os.ReadFile(imageFixture)
	require.NoError(t, err)

	r := NewReader()
	cfg, ok := r.Config().(*Config)
	require.True(t, ok)
	cfg.SetExtractNonTranslatableContent(false)

	ctx := context.Background()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI:          imageFixture,
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })

	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok {
			assert.NotEqual(t, model.RolePicture, b.SemanticRole(),
				"no detached picture block when non-translatable surfacing is off")
		}
	}
}
