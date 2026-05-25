package satproto

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestEncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{
			name: "segment full",
			req:  Request{ID: 1, Op: OpSegment, Text: "Hello world. How are you?", Lang: "en", Model: "sat-3l-sm", Threshold: 0.25},
			want: `{"id":1,"op":"segment","text":"Hello world. How are you?","lang":"en","model":"sat-3l-sm","threshold":0.25}`,
		},
		{
			name: "ping omits empties",
			req:  Request{Op: OpPing},
			want: `{"op":"ping"}`,
		},
		{
			name: "info omits empties",
			req:  Request{Op: OpInfo},
			want: `{"op":"info"}`,
		},
		{
			name: "segment minimal",
			req:  Request{ID: 7, Op: OpSegment, Text: "x"},
			want: `{"id":7,"op":"segment","text":"x"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.req)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(b))

			var got Request
			require.NoError(t, json.Unmarshal(b, &got))
			assert.Equal(t, tt.req, got)
		})
	}
}

func TestResponseEncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		resp Response
		want string
	}{
		{
			name: "segment boundaries",
			resp: Response{ID: 1, Boundaries: []int{13}},
			want: `{"id":1,"boundaries":[13]}`,
		},
		{
			name: "ping ok",
			resp: Response{OK: true, Version: "0.1.0"},
			want: `{"ok":true,"version":"0.1.0"}`,
		},
		{
			name: "info models",
			resp: Response{Version: "0.1.0", Models: []ModelInfo{{Name: "sat-3l-sm", Loaded: true, Default: true}, {Name: "sat-12l-sm"}}},
			want: `{"version":"0.1.0","models":[{"name":"sat-3l-sm","loaded":true,"default":true},{"name":"sat-12l-sm","loaded":false}]}`,
		},
		{
			name: "error",
			resp: Response{ID: 9, Error: "boom"},
			want: `{"id":9,"error":"boom"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.resp)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(b))

			var got Response
			require.NoError(t, json.Unmarshal(b, &got))
			assert.Equal(t, tt.resp, got)
		})
	}
}

func TestWriteMessageOneLine(t *testing.T) {
	var buf bytes.Buffer
	// Text contains a newline; JSON must escape it so the message stays on
	// a single physical line.
	require.NoError(t, WriteMessage(&buf, Request{ID: 1, Op: OpSegment, Text: "a\nb"}))
	out := buf.String()
	require.True(t, strings.HasSuffix(out, "\n"), "must end with a newline terminator")
	assert.Equal(t, 1, strings.Count(out, "\n"), "must be exactly one physical line")
	assert.Contains(t, out, `\n`, "embedded newline must be JSON-escaped")
}

// echoHandler reflects requests back so we can exercise Serve + Client end to
// end over an in-memory pipe.
func echoHandler(req Request) Response {
	switch req.Op {
	case OpPing:
		return Response{OK: true, Version: "test"}
	case OpInfo:
		return Response{Version: "test", Models: []ModelInfo{{Name: "sat-3l-sm", Loaded: false, Default: true}}}
	case OpSegment:
		if req.Text == "fail" {
			return Response{Error: "deliberate failure"}
		}
		// Deterministic fake: boundary at each space.
		var bounds []int
		for i, r := range []rune(req.Text) {
			if r == ' ' {
				bounds = append(bounds, i+1)
			}
		}
		if bounds == nil {
			bounds = []int{}
		}
		return Response{Boundaries: bounds}
	default:
		return Response{Error: "unknown op"}
	}
}

func TestServeClientRoundTrip(t *testing.T) {
	// Wire: client.w -> serverIn ; serverOut -> client.r
	serverIn, clientW := io.Pipe()
	clientR, serverOut := io.Pipe()

	go func() {
		_ = Serve(serverIn, serverOut, echoHandler)
		_ = serverOut.Close()
	}()

	client := NewClient(clientW, clientR)

	ver, err := client.Ping()
	require.NoError(t, err)
	assert.Equal(t, "test", ver)

	models, ver, err := client.Info()
	require.NoError(t, err)
	assert.Equal(t, "test", ver)
	require.Len(t, models, 1)
	assert.Equal(t, "sat-3l-sm", models[0].Name)
	assert.True(t, models[0].Default)

	bounds, err := client.Segment("a b c", "sat-3l-sm", "en", 0.25)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 4}, bounds)

	bounds, err = client.Segment("nospaces", "", "", 0)
	require.NoError(t, err)
	assert.Equal(t, []int{}, bounds)

	_, err = client.Segment("fail", "", "", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deliberate failure")

	// Closing the client's write side ends the server loop cleanly.
	require.NoError(t, clientW.Close())
}

func TestServeMalformedLineContinues(t *testing.T) {
	in := strings.NewReader("not json\n" + `{"op":"ping"}` + "\n")
	var out bytes.Buffer
	require.NoError(t, Serve(in, &out, echoHandler))

	dec := json.NewDecoder(&out)
	var first Response
	require.NoError(t, dec.Decode(&first))
	assert.Contains(t, first.Error, "malformed request")

	var second Response
	require.NoError(t, dec.Decode(&second))
	assert.True(t, second.OK)
	assert.Equal(t, "test", second.Version)
}

func TestServeEOFCleanShutdown(t *testing.T) {
	in := strings.NewReader("") // immediate EOF
	var out bytes.Buffer
	require.NoError(t, Serve(in, &out, echoHandler))
	assert.Empty(t, out.String())
}

func TestClientPoisonedAfterPluginClose(t *testing.T) {
	serverIn, clientW := io.Pipe()
	clientR, serverOut := io.Pipe()

	go func() {
		// Read one request then close stdout to simulate a crashed plugin.
		sc := newScanner(serverIn)
		sc.Scan()
		_ = serverOut.Close()
	}()

	client := NewClient(clientW, clientR)
	_, err := client.Ping()
	require.Error(t, err)

	// Subsequent calls fail fast with the recorded error.
	_, err2 := client.Ping()
	require.Error(t, err2)
	assert.Equal(t, err.Error(), err2.Error())
}
