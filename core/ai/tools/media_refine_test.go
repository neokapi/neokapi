package tools

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/imageops"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syntheticPNG returns a w×h white PNG with a black rectangle, as bytes.
func syntheticPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestImageOpsCrop(t *testing.T) {
	src := syntheticPNG(t, 200, 100)
	out, err := imageops.Crop(src, 10, 20, 50, 30)
	require.NoError(t, err)
	img, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, 50, img.Bounds().Dx())
	assert.Equal(t, 30, img.Bounds().Dy())

	// Region clamped to bounds.
	out, err = imageops.Crop(src, 180, 90, 100, 100)
	require.NoError(t, err)
	img, _, err = image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, 20, img.Bounds().Dx())
	assert.Equal(t, 10, img.Bounds().Dy())

	// No intersection → error.
	_, err = imageops.Crop(src, 500, 500, 10, 10)
	require.Error(t, err)
}

func TestImageSlicer(t *testing.T) {
	src := MediaRef{Data: syntheticPNG(t, 200, 100), MimeType: "image/png"}
	b := model.NewBlock("tu1", "txt")
	b.SetGeometry(&model.GeometryAnnotation{BBox: model.Rect{X: 5, Y: 5, W: 40, H: 12}})

	part, err := ImageSlicer{}.Slice(context.Background(), src, b)
	require.NoError(t, err)
	assert.Equal(t, aiprovider.ContentImage, part.Kind)
	require.NotNil(t, part.Media)
	assert.Equal(t, "image/png", part.Media.MimeType)
	assert.NotEmpty(t, part.Media.Data)

	// No geometry → error.
	_, err = ImageSlicer{}.Slice(context.Background(), src, model.NewBlock("tu2", "x"))
	require.Error(t, err)
}

// runRefine drives the tool's Process over the given blocks and returns them.
func runRefine(t *testing.T, tool *MediaRefineTool, blocks []*model.Block) {
	t.Helper()
	in := make(chan *model.Part, len(blocks))
	out := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		in <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(in)
	require.NoError(t, tool.Process(context.Background(), in, out))
	close(out)
	n := 0
	for range out {
		n++
	}
	assert.Equal(t, len(blocks), n, "all parts forwarded")
}

func ocrBlock(id, text string, conf float64) *model.Block {
	b := model.NewBlock(id, text)
	b.SetGeometry(&model.GeometryAnnotation{BBox: model.Rect{X: 1, Y: 1, W: 40, H: 10}})
	b.SetSourceOrigin(&model.Origin{Kind: model.OriginOCR, Confidence: conf})
	return b
}

func newToolWithRaster(t *testing.T, mock *aiprovider.MockProvider) *MediaRefineTool {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "page.png")
	require.NoError(t, os.WriteFile(p, syntheticPNG(t, 200, 100), 0o600))
	return NewMediaRefineTool(mock, MediaRefineConfig{Source: p, Threshold: 0.85})
}

func TestMediaRefine_RewritesLowConfidence(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(_ context.Context, _ []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: "corrected text"}, nil
	}
	tool := newToolWithRaster(t, mock)

	low := ocrBlock("tu1", "corrupted txt", 0.40) // gated → rewritten
	high := ocrBlock("tu2", "clean text", 0.98)   // not gated → untouched
	runRefine(t, tool, []*model.Block{low, high})

	assert.Equal(t, "corrected text", low.SourceText())
	assert.Equal(t, "clean text", high.SourceText())

	o, ok := low.SourceOrigin()
	require.True(t, ok)
	assert.Equal(t, "llm:mock", o.Engine)
	assert.Equal(t, "llm-rewrite", low.Properties[PropNeedsReview])

	// untouched block keeps its original (non-llm) provenance and no review flag
	ho, _ := high.SourceOrigin()
	assert.Empty(t, ho.Engine)
	assert.Empty(t, high.Properties[PropNeedsReview])
}

func TestMediaRefine_RefusalKeepsOriginal(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(_ context.Context, _ []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: RefuseToken}, nil
	}
	tool := newToolWithRaster(t, mock)

	b := ocrBlock("tu1", "original guess", 0.30)
	runRefine(t, tool, []*model.Block{b})

	assert.Equal(t, "original guess", b.SourceText(), "refusal must not fabricate source")
	assert.Equal(t, "illegible", b.Properties[PropNeedsReview])
}

func TestMediaRefine_CapabilityError(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.InputModalitiesValue = []aiprovider.Modality{} // text-only
	mock.ChatFunc = func(_ context.Context, _ []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		t.Fatal("provider must not be called when it cannot accept the modality")
		return nil, nil
	}
	tool := newToolWithRaster(t, mock)

	b := ocrBlock("tu1", "guess", 0.30)
	runRefine(t, tool, []*model.Block{b})

	assert.Equal(t, "guess", b.SourceText())
	assert.Contains(t, b.Properties[PropNeedsReview], "refine-error")
}

func TestMediaRefine_SkipsNonOCR(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	called := false
	mock.ChatFunc = func(_ context.Context, _ []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		called = true
		return &aiprovider.ChatResponse{Content: "x"}, nil
	}
	tool := newToolWithRaster(t, mock)

	b := model.NewBlock("tu1", "human-authored")
	b.SetGeometry(&model.GeometryAnnotation{BBox: model.Rect{X: 1, Y: 1, W: 40, H: 10}})
	// no source Origin → not an OCR block
	runRefine(t, tool, []*model.Block{b})

	assert.False(t, called, "non-OCR blocks must not be refined")
	assert.Equal(t, "human-authored", b.SourceText())
}
