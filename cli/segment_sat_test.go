package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSatTransport struct {
	boundaries []int
	err        error
	gotText    string
	gotModel   string
	gotThresh  float64
}

func (f *fakeSatTransport) segment(text, modelName, _ string, threshold float64) ([]int, error) {
	f.gotText, f.gotModel, f.gotThresh = text, modelName, threshold
	return f.boundaries, f.err
}

func TestSatEngine_MapsBoundariesToSpans(t *testing.T) {
	ft := &fakeSatTransport{boundaries: []int{13, 26}}
	eng := &satEngine{cfg: segment.Config{SatModel: "sat-3l-sm", Threshold: 0.25}, transport: ft}

	runs := []model.Run{{Text: &model.TextRun{Text: "Hello world. How are you? I am fine."}}}
	spans, err := eng.Segment(context.Background(), runs, "en")
	require.NoError(t, err)
	require.Len(t, spans, 3)
	assert.Equal(t, "Hello world. ", model.RunsText(spans[0].Range.ExtractRuns(runs)))
	assert.Equal(t, "How are you? ", model.RunsText(spans[1].Range.ExtractRuns(runs)))
	assert.Equal(t, "I am fine.", model.RunsText(spans[2].Range.ExtractRuns(runs)))
	// Config is forwarded to the plugin.
	assert.Equal(t, "sat-3l-sm", ft.gotModel)
	assert.InDelta(t, 0.25, ft.gotThresh, 1e-9)
	assert.Equal(t, segment.LayerSentence, eng.Layer())
}

func TestSatEngine_EmptyTextSkipsTransport(t *testing.T) {
	ft := &fakeSatTransport{err: errors.New("should not be called")}
	eng := &satEngine{cfg: segment.Config{}, transport: ft}
	spans, err := eng.Segment(context.Background(), nil, "en")
	require.NoError(t, err)
	assert.Nil(t, spans)
	assert.Empty(t, ft.gotText, "transport not invoked for empty content")
}

func TestSatEngine_TransportErrorWrapped(t *testing.T) {
	ft := &fakeSatTransport{err: errors.New("model.onnx not found: build with -tags onnx")}
	eng := &satEngine{cfg: segment.Config{}, transport: ft}
	runs := []model.Run{{Text: &model.TextRun{Text: "One. Two."}}}
	_, err := eng.Segment(context.Background(), runs, "en")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sat:")
	assert.Contains(t, err.Error(), "model.onnx")
}

func TestSatEngine_RegisteredAndDiscoverable(t *testing.T) {
	assert.True(t, segment.HasEngine("sat"), "cli registers the sat engine at init")
}

func TestFindSatPlugin_NotInstalled(t *testing.T) {
	// Point discovery at an empty dir and disable system roots via the env
	// override; with no sat plugin present, the error guides installation.
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	_, err := findSatPlugin()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugins install sat")
}
