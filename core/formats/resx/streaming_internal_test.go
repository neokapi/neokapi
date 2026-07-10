package resx

import (
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundtripPathRESX drives the reader through a specific path (streaming vs
// buffered) by calling the unexported entry point directly, then reconstructs
// via the writer + skeleton store. In-package so it can bypass readContent's
// gate and exercise both paths (and both tokenizers) on the same input.
func roundtripPathRESX(t *testing.T, input string, streaming bool) string {
	t.Helper()
	ctx := context.Background()

	r := NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	r.SetSkeletonStore(store)
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))

	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		if streaming {
			r.readStreaming(ctx, ch)
		} else {
			r.readBuffered(ctx, ch)
		}
	}()
	var parts []*model.Part
	for res := range ch {
		require.NoError(t, res.Error)
		if res.Part != nil {
			parts = append(parts, res.Part)
		}
	}
	r.Close()

	w := NewWriter()
	w.SetSkeletonStore(store)
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	require.NoError(t, w.Write(ctx, testutil.PartsToChannel(parts)))
	w.Close()
	return buf.String()
}

// TestStreamingBufferedParityRESX asserts the streaming tokenizer + incremental
// walk reproduce the buffered path's output byte-for-byte across the tricky
// shapes: entity-escaped values, CDATA, a self-closing <value/>, a
// non-translatable typed <data>, a designer name-reference (@name ">"), a
// sibling <comment>, and multi-line content. Both paths share the walk/skeleton
// helpers, so any divergence is in the tokenizer — exactly what this pins down.
func TestStreamingBufferedParityRESX(t *testing.T) {
	const header = `<?xml version="1.0" encoding="utf-8"?>
<root>
  <resheader name="resmimetype">
    <value>text/microsoft-resx</value>
  </resheader>
`
	cases := map[string]string{
		"plain": header + `  <data name="Hello" xml:space="preserve">
    <value>Hello world</value>
  </data>
</root>
`,
		"entities_and_comment": header + `  <data name="Greeting" xml:space="preserve">
    <value>A &amp; B &lt;tag&gt; "q" 'a'</value>
    <comment>a developer note</comment>
  </data>
</root>
`,
		"cdata": header + `  <data name="Markup" xml:space="preserve">
    <value><![CDATA[<b>bold</b> & more]]></value>
  </data>
</root>
`,
		"self_closing_value_and_typed": header + `  <data name="Empty" xml:space="preserve">
    <value/>
  </data>
  <data name="Icon" type="System.Drawing.Bitmap, System.Drawing">
    <value>binary-goes-here</value>
  </data>
</root>
`,
		"designer_reference": header + `  <data name="&gt;&gt;btn.Name" xml:space="preserve">
    <value>btn</value>
  </data>
  <data name="Real" xml:space="preserve">
    <value>Real text</value>
  </data>
</root>
`,
		"multiline": header + `  <data name="Multi" xml:space="preserve">
    <value>line one
line two
line three</value>
  </data>
</root>
`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			buffered := roundtripPathRESX(t, in, false)
			streaming := roundtripPathRESX(t, in, true)
			assert.Equal(t, buffered, streaming, "streaming output must match buffered output")
			// The buffered path is the established byte-exact behavior, so this
			// also asserts the streaming path round-trips the input byte-exact
			// (for these non-CDATA cases; CDATA is a documented exception).
			if name != "cdata" {
				assert.Equal(t, in, streaming, "streaming round-trip should be byte-exact")
			}
		})
	}
}
