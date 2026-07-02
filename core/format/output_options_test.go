package format_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreencoding "github.com/neokapi/neokapi/core/encoding"
	"github.com/neokapi/neokapi/core/format"
)

// wrapAndWrite pushes input through the option chain in the given chunk
// sizes (0 = one single write) and returns the resulting bytes.
func wrapAndWrite(t *testing.T, opts format.OutputOptions, input []byte, chunk int) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := opts.Wrap(&buf)
	require.NoError(t, err)
	if chunk <= 0 {
		chunk = len(input)
	}
	for i := 0; i < len(input); i += chunk {
		end := i + chunk
		if end > len(input) {
			end = len(input)
		}
		n, werr := w.Write(input[i:end])
		require.NoError(t, werr)
		require.Equal(t, end-i, n)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestOutputOptionsBOM(t *testing.T) {
	bom := "\xEF\xBB\xBF"
	tests := []struct {
		name  string
		mode  format.BOMMode
		in    string
		want  string
		chunk int
	}{
		{name: "add to bare stream", mode: format.BOMAdd, in: "hello", want: bom + "hello"},
		{name: "add is idempotent", mode: format.BOMAdd, in: bom + "hello", want: bom + "hello"},
		{name: "add byte-at-a-time", mode: format.BOMAdd, in: "hello", want: bom + "hello", chunk: 1},
		{name: "add to empty stream", mode: format.BOMAdd, in: "", want: bom},
		{name: "add to short stream", mode: format.BOMAdd, in: "hi", want: bom + "hi"},
		{name: "remove leading BOM", mode: format.BOMRemove, in: bom + "hello", want: "hello"},
		{name: "remove without BOM", mode: format.BOMRemove, in: "hello", want: "hello"},
		{name: "remove byte-at-a-time", mode: format.BOMRemove, in: bom + "hello", want: "hello", chunk: 1},
		{name: "remove keeps interior BOM", mode: format.BOMRemove, in: bom + "a" + bom, want: "a" + bom},
		{name: "remove short non-BOM stream", mode: format.BOMRemove, in: "\xEF\xBB", want: "\xEF\xBB"},
		{name: "keep is passthrough", mode: format.BOMKeep, in: bom + "hello", want: bom + "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := format.OutputOptions{BOM: tt.mode}
			got := wrapAndWrite(t, opts, []byte(tt.in), tt.chunk)
			assert.Equal(t, []byte(tt.want), got)
		})
	}
}

func TestOutputOptionsNewline(t *testing.T) {
	tests := []struct {
		name  string
		mode  format.NewlineMode
		in    string
		want  string
		chunk int
	}{
		{name: "lf from crlf", mode: format.NewlineLF, in: "a\r\nb\r\n", want: "a\nb\n"},
		{name: "lf from mixed", mode: format.NewlineLF, in: "a\r\nb\nc\rd", want: "a\nb\nc\nd"},
		{name: "lf split crlf across writes", mode: format.NewlineLF, in: "a\r\nb", want: "a\nb", chunk: 2},
		{name: "lf trailing lone cr", mode: format.NewlineLF, in: "a\r", want: "a\n"},
		{name: "crlf from lf", mode: format.NewlineCRLF, in: "a\nb\n", want: "a\r\nb\r\n"},
		{name: "crlf from mixed", mode: format.NewlineCRLF, in: "a\r\nb\nc\rd", want: "a\r\nb\r\nc\r\nd"},
		{name: "crlf idempotent", mode: format.NewlineCRLF, in: "a\r\nb\r\n", want: "a\r\nb\r\n"},
		{name: "crlf byte-at-a-time", mode: format.NewlineCRLF, in: "a\r\nb\nc", want: "a\r\nb\r\nc", chunk: 1},
		{name: "crlf trailing lone cr", mode: format.NewlineCRLF, in: "a\r", want: "a\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := format.OutputOptions{Newline: tt.mode}
			got := wrapAndWrite(t, opts, []byte(tt.in), tt.chunk)
			assert.Equal(t, []byte(tt.want), got)
		})
	}
}

func TestOutputOptionsEncoding(t *testing.T) {
	em := coreencoding.NewEncoderManager()

	t.Run("windows-1252 round-trip", func(t *testing.T) {
		in := "café — naïve"
		got := wrapAndWrite(t, format.OutputOptions{Encoding: "windows-1252"}, []byte(in), 3)
		// The encoded bytes are not valid UTF-8 for the accented chars…
		assert.NotEqual(t, []byte(in), got)
		// …but decode back to the original text via core/encoding.
		back, err := em.Decode(got, "windows-1252")
		require.NoError(t, err)
		assert.Equal(t, in, back)
	})

	t.Run("utf-16le with BOM add", func(t *testing.T) {
		got := wrapAndWrite(t, format.OutputOptions{BOM: format.BOMAdd, Encoding: "utf-16le"}, []byte("hi"), 0)
		assert.Equal(t, []byte{0xFF, 0xFE, 'h', 0x00, 'i', 0x00}, got)
	})

	t.Run("utf-8 aliases are passthrough", func(t *testing.T) {
		for _, name := range []string{"", "utf-8", "UTF-8", "utf8"} {
			assert.True(t, format.OutputOptions{Encoding: name}.IsZero(), name)
		}
	})

	t.Run("unknown charset errors", func(t *testing.T) {
		_, err := format.OutputOptions{Encoding: "no-such-charset"}.Wrap(&bytes.Buffer{})
		assert.Error(t, err)
	})
}

func TestOutputOptionsCombined(t *testing.T) {
	// newline → BOM → charset all together.
	opts := format.OutputOptions{BOM: format.BOMAdd, Newline: format.NewlineCRLF, Encoding: "utf-16le"}
	got := wrapAndWrite(t, opts, []byte("a\nb"), 1)
	assert.Equal(t, []byte{0xFF, 0xFE, 'a', 0x00, '\r', 0x00, '\n', 0x00, 'b', 0x00}, got)
}

func TestSplitOutputConfig(t *testing.T) {
	t.Run("nested map", func(t *testing.T) {
		opts, rest, err := format.SplitOutputConfig(map[string]any{
			"output":        map[string]any{"bom": "add", "newline": "crlf", "encoding": "utf-16le"},
			"segmentByLine": true,
		})
		require.NoError(t, err)
		assert.Equal(t, format.BOMAdd, opts.BOM)
		assert.Equal(t, format.NewlineCRLF, opts.Newline)
		assert.Equal(t, "utf-16le", opts.Encoding)
		assert.Equal(t, map[string]any{"segmentByLine": true}, rest)
	})

	t.Run("dotted keys", func(t *testing.T) {
		opts, rest, err := format.SplitOutputConfig(map[string]any{
			"output.bom":     "remove",
			"output.newline": "lf",
		})
		require.NoError(t, err)
		assert.Equal(t, format.BOMRemove, opts.BOM)
		assert.Equal(t, format.NewlineLF, opts.Newline)
		assert.Empty(t, rest)
	})

	t.Run("defaults are keep", func(t *testing.T) {
		opts, rest, err := format.SplitOutputConfig(map[string]any{"other": 1})
		require.NoError(t, err)
		assert.True(t, opts.IsZero())
		assert.Equal(t, map[string]any{"other": 1}, rest)
	})

	t.Run("invalid enum", func(t *testing.T) {
		_, _, err := format.SplitOutputConfig(map[string]any{"output": map[string]any{"bom": "maybe"}})
		assert.Error(t, err)
	})

	t.Run("unknown sub-key", func(t *testing.T) {
		_, _, err := format.SplitOutputConfig(map[string]any{"output": map[string]any{"eol": "lf"}})
		assert.Error(t, err)
	})
}

// TestBaseFormatWriterOutputOptions exercises the seam itself: a writer
// embedding BaseFormatWriter gets the post-encode chain on its Output and a
// flush on Close, in either configuration order.
func TestBaseFormatWriterOutputOptions(t *testing.T) {
	t.Run("options then output", func(t *testing.T) {
		var buf bytes.Buffer
		b := &format.BaseFormatWriter{FormatName: "test"}
		require.NoError(t, b.SetOutputOptions(format.OutputOptions{Newline: format.NewlineCRLF}))
		require.NoError(t, b.SetOutputWriter(&buf))
		_, err := b.Output.Write([]byte("x\ny"))
		require.NoError(t, err)
		require.NoError(t, b.Close())
		assert.Equal(t, "x\r\ny", buf.String())
	})

	t.Run("output then options", func(t *testing.T) {
		var buf bytes.Buffer
		b := &format.BaseFormatWriter{FormatName: "test"}
		require.NoError(t, b.SetOutputWriter(&buf))
		require.NoError(t, b.SetOutputOptions(format.OutputOptions{BOM: format.BOMAdd}))
		_, err := b.Output.Write([]byte("x"))
		require.NoError(t, err)
		require.NoError(t, b.Close())
		assert.Equal(t, "\xEF\xBB\xBFx", buf.String())
	})

	t.Run("passthrough by default", func(t *testing.T) {
		var buf bytes.Buffer
		b := &format.BaseFormatWriter{FormatName: "test"}
		require.NoError(t, b.SetOutputWriter(&buf))
		assert.Equal(t, &buf, b.Output.(*bytes.Buffer))
	})
}
