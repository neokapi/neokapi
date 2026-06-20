package llmproto

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// echoHandler answers ping/info/generate deterministically so the client/server
// round trip can be exercised without a real model.
func echoHandler(req Request) Response {
	switch req.Op {
	case OpPing:
		return Response{OK: true, Version: "test"}
	case OpInfo:
		return Response{
			Version:    "test",
			Models:     []ModelInfo{{Name: "gemma-4-e2b", Loaded: true, Default: true}},
			Modalities: []string{"image", "audio"},
		}
	case OpGenerate:
		var b strings.Builder
		for _, m := range req.Messages {
			b.WriteString(m.Text)
		}
		return Response{Text: "echo:" + b.String(), InputTokens: 7, OutputTokens: 3}
	default:
		return Response{Error: "unknown op"}
	}
}

// serverPipe wires a Client to a Serve loop over in-memory pipes running on a
// goroutine, returning the client and a cleanup func.
func serverPipe(t *testing.T) (*Client, func()) {
	t.Helper()
	hostToPlugin := newSyncPipe()
	pluginToHost := newSyncPipe()
	done := make(chan struct{})
	go func() {
		_ = Serve(hostToPlugin, pluginToHost, echoHandler)
		close(done)
	}()
	c := NewClient(hostToPlugin, pluginToHost)
	return c, func() {
		hostToPlugin.Close() // EOF → Serve returns
		<-done
	}
}

func TestClientServerRoundTrip(t *testing.T) {
	c, cleanup := serverPipe(t)
	defer cleanup()

	v, err := c.Ping()
	require.NoError(t, err)
	assert.Equal(t, "test", v)

	models, mods, ver, err := c.Info()
	require.NoError(t, err)
	assert.Equal(t, "test", ver)
	require.Len(t, models, 1)
	assert.Equal(t, "gemma-4-e2b", models[0].Name)
	assert.True(t, models[0].Default)
	assert.Equal(t, []string{"image", "audio"}, mods)

	resp, err := c.Generate(Request{
		Messages:  []Message{{Role: "user", Text: "Bonjour"}},
		MaxTokens: 16,
	})
	require.NoError(t, err)
	assert.Equal(t, "echo:Bonjour", resp.Text)
	assert.Equal(t, 7, resp.InputTokens)
	assert.Equal(t, 3, resp.OutputTokens)
}

func TestGenerateSurfacesPluginError(t *testing.T) {
	errHandler := func(req Request) Response { return Response{Error: "engine unavailable"} }
	hostToPlugin := newSyncPipe()
	pluginToHost := newSyncPipe()
	go func() { _ = Serve(hostToPlugin, pluginToHost, errHandler) }()
	defer hostToPlugin.Close()

	c := NewClient(hostToPlugin, pluginToHost)
	_, err := c.Generate(Request{Messages: []Message{{Role: "user", Text: "hi"}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "engine unavailable")
}

func TestServeIDEcho(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader(`{"id":42,"op":"ping"}` + "\n")
	require.NoError(t, Serve(in, &out, echoHandler))

	var resp Response
	require.NoError(t, decodeOne(out.Bytes(), &resp))
	assert.Equal(t, int64(42), resp.ID)
	assert.True(t, resp.OK)
}

func TestServeMalformedLineContinues(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader("not json\n" + `{"op":"ping"}` + "\n")
	require.NoError(t, Serve(in, &out, echoHandler))

	lines := splitLines(out.Bytes())
	require.Len(t, lines, 2)
	var bad Response
	require.NoError(t, decodeOne(lines[0], &bad))
	assert.Contains(t, bad.Error, "malformed request")
	var ok Response
	require.NoError(t, decodeOne(lines[1], &ok))
	assert.True(t, ok.OK)
}

func TestReadLineOversizedRejected(t *testing.T) {
	// A line longer than MaxLineBytes must be drained and reported, leaving the
	// reader positioned at the following valid line.
	big := strings.Repeat("x", MaxLineBytes+10)
	var out bytes.Buffer
	in := strings.NewReader(big + "\n" + `{"op":"ping"}` + "\n")
	require.NoError(t, Serve(in, &out, echoHandler))

	lines := splitLines(out.Bytes())
	require.Len(t, lines, 2)
	var bad Response
	require.NoError(t, decodeOne(lines[0], &bad))
	assert.Contains(t, bad.Error, "maximum size")
	var ok Response
	require.NoError(t, decodeOne(lines[1], &ok))
	assert.True(t, ok.OK)
}

func TestWriteMessageSingleLine(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, WriteMessage(&out, Response{Text: "a\nb"}))
	// Exactly one trailing newline; the embedded newline is JSON-escaped.
	assert.Equal(t, 1, bytes.Count(out.Bytes(), []byte("\n")))
}

// --- small test helpers -----------------------------------------------------

func decodeOne(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	for _, l := range bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n")) {
		if len(l) > 0 {
			out = append(out, l)
		}
	}
	return out
}

// syncPipe is a tiny blocking in-memory pipe (io.Reader + io.Writer + Close)
// usable by both the client and the Serve loop. io.Pipe gives us exactly this.
type syncPipe struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func newSyncPipe() *syncPipe {
	r, w := io.Pipe()
	return &syncPipe{r: r, w: w}
}

func (p *syncPipe) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *syncPipe) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *syncPipe) Close() error                { _ = p.w.Close(); return p.r.Close() }
