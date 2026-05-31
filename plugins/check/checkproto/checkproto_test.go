package checkproto

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
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

func TestServeMalformedLineContinues(t *testing.T) {
	in := strings.NewReader("not json\n" + `{"op":"ping"}` + "\n")
	var out bytes.Buffer
	require.NoError(t, Serve(in, &out, fakeHandler))

	dec := json.NewDecoder(&out)
	var first Response
	require.NoError(t, dec.Decode(&first))
	assert.Contains(t, first.Error, "malformed request")

	var second Response
	require.NoError(t, dec.Decode(&second))
	assert.True(t, second.OK)
}

func TestServeOversizedLineRejectedAndSurvives(t *testing.T) {
	huge := strings.Repeat("x", MaxLineBytes+10)
	in := strings.NewReader(huge + "\n" + `{"op":"ping"}` + "\n")
	var out bytes.Buffer
	require.NoError(t, Serve(in, &out, fakeHandler), "oversized line must not abort Serve")

	dec := json.NewDecoder(&out)
	var first Response
	require.NoError(t, dec.Decode(&first))
	assert.Contains(t, first.Error, "exceeds maximum size")

	var second Response
	require.NoError(t, dec.Decode(&second))
	assert.True(t, second.OK, "the request after an oversized line must still be served")
}

func TestClientRejectsMismatchedResponseID(t *testing.T) {
	serverIn, clientW := io.Pipe()
	clientR, serverOut := io.Pipe()

	go func() {
		sc := newScanner(serverIn)
		for sc.Scan() {
			var req Request
			if json.Unmarshal(sc.Bytes(), &req) != nil {
				continue
			}
			_ = WriteMessage(serverOut, Response{ID: req.ID + 999, OK: true, Version: "test"})
		}
		_ = serverOut.Close()
	}()

	client := NewClient(clientW, clientR)
	_, err := client.Ping()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match request id")

	_, err2 := client.Ping()
	require.Error(t, err2)
	assert.Equal(t, err.Error(), err2.Error())

	require.NoError(t, clientW.Close())
}
