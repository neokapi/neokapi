package checkproto

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHandler answers ping/info and returns canned scores/embeddings so the
// round-trip can be tested with no model.
func fakeHandler(req Request) Response {
	switch req.Op {
	case OpPing:
		return Response{OK: true, Version: "test"}
	case OpInfo:
		return Response{Models: []ModelInfo{{Name: "m", Loaded: true, Default: true}}, Version: "test"}
	case OpEmbed:
		return Response{Embedding: []float32{0.1, 0.2, 0.3}}
	case OpSimilarity:
		scores := make([]float64, len(req.Refs))
		for i := range req.Refs {
			scores[i] = float64(i) / 10.0
		}
		return Response{Scores: scores}
	default:
		return Response{Error: "unknown op"}
	}
}

func newRoundTrip(t *testing.T) (*Client, func()) {
	t.Helper()
	hpR, hpW := io.Pipe() // host -> plugin
	phR, phW := io.Pipe() // plugin -> host
	done := make(chan struct{})
	go func() {
		_ = Serve(hpR, phW, fakeHandler)
		close(done)
	}()
	c := NewClient(hpW, phR)
	return c, func() {
		_ = hpW.Close()
		<-done
		_ = phW.Close()
	}
}

func TestRoundTrip(t *testing.T) {
	c, stop := newRoundTrip(t)
	defer stop()

	v, err := c.Ping()
	require.NoError(t, err)
	assert.Equal(t, "test", v)

	models, ver, err := c.Info()
	require.NoError(t, err)
	assert.Equal(t, "test", ver)
	require.Len(t, models, 1)
	assert.True(t, models[0].Default)

	emb, err := c.Embed("hello", "")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, emb)

	scores, err := c.Similarity("text", []string{"a", "b", "c"}, "")
	require.NoError(t, err)
	assert.Equal(t, []float64{0.0, 0.1, 0.2}, scores)
}

func TestSimilarityNoRefs(t *testing.T) {
	c, stop := newRoundTrip(t)
	defer stop()
	scores, err := c.Similarity("text", nil, "")
	require.NoError(t, err)
	assert.Empty(t, scores)
}
