package asrproto

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveOn wires a Client to an in-process Serve loop over two pipes, returning
// the client and a stop func. The handler runs the protocol end to end with no
// subprocess.
func serveOn(t *testing.T, h Handler) (*Client, func()) {
	t.Helper()
	reqR, reqW := io.Pipe()   // host → plugin
	respR, respW := io.Pipe() // plugin → host
	done := make(chan struct{})
	go func() {
		_ = Serve(reqR, respW, h)
		close(done)
	}()
	c := NewClient(reqW, respR)
	return c, func() {
		_ = reqW.Close() // EOF → Serve returns
		<-done
		_ = respW.Close()
	}
}

func TestTranscribeRoundTrip(t *testing.T) {
	c, stop := serveOn(t, func(req Request) Response {
		assert.Equal(t, OpTranscribe, req.Op)
		assert.Equal(t, "/tmp/track.wav", req.AudioPath)
		return Response{
			Language: "en",
			Segments: []Segment{{Text: "hello", StartMS: 0, EndMS: 900, Confidence: 0.9}},
		}
	})
	defer stop()

	resp, err := c.Transcribe(context.Background(), "/tmp/track.wav", "", "en")
	require.NoError(t, err)
	assert.Equal(t, "en", resp.Language)
	require.Len(t, resp.Segments, 1)
	assert.Equal(t, "hello", resp.Segments[0].Text)
	assert.Equal(t, int64(900), resp.Segments[0].EndMS)
}

func TestPingAndInfo(t *testing.T) {
	c, stop := serveOn(t, func(req Request) Response {
		switch req.Op {
		case OpPing:
			return Response{OK: true, Version: "0.1.0"}
		case OpInfo:
			return Response{Version: "0.1.0", Models: []ModelInfo{{Name: "whisper-base", Loaded: true, Default: true}}}
		default:
			return Response{Error: "unknown op"}
		}
	})
	defer stop()

	v, err := c.Ping()
	require.NoError(t, err)
	assert.Equal(t, "0.1.0", v)

	models, ver, err := c.Info()
	require.NoError(t, err)
	assert.Equal(t, "0.1.0", ver)
	require.Len(t, models, 1)
	assert.Equal(t, "whisper-base", models[0].Name)
}

func TestPluginErrorSurfaced(t *testing.T) {
	c, stop := serveOn(t, func(Request) Response {
		return Response{Error: "model not found"}
	})
	defer stop()

	_, err := c.Transcribe(context.Background(), "/x.wav", "missing", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model not found")
}

func TestContextCancelled(t *testing.T) {
	c, stop := serveOn(t, func(Request) Response { return Response{} })
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Transcribe(ctx, "/x.wav", "", "")
	require.ErrorIs(t, err, context.Canceled)
}
