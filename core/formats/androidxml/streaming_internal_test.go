package androidxml

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

// roundtripPathAndroid drives the reader through a specific path (streaming vs
// buffered) by calling the unexported entry point directly, then reconstructs
// via the writer + skeleton store. In-package so it can bypass readContent's
// gate and exercise both paths (and both tokenizers) on the same input.
func roundtripPathAndroid(t *testing.T, input string, streaming bool) string {
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

// TestStreamingBufferedParityAndroid asserts the streaming pull-walk reproduces
// the buffered index-walk's output byte-for-byte across the tricky shapes:
// a comment attached to the following entry, printf/<xliff:g> inline codes,
// CDATA, a resource-reference alias, a translatable="false" entry, a
// <string-array>, <plurals>, and a non-translatable structural element. Both
// paths share the emit* handlers, so any divergence is in the streaming walk or
// tokenizer — exactly what this pins down.
func TestStreamingBufferedParityAndroid(t *testing.T) {
	cases := map[string]string{
		"plain_and_comment": `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <!-- a greeting -->
    <string name="hello">Hello %1$s</string>
</resources>
`,
		"inline_and_cdata": `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="markup">Tap <xliff:g id="btn">OK</xliff:g> now</string>
    <string name="cdata"><![CDATA[<b>bold</b> & more]]></string>
</resources>
`,
		"reference_and_nontranslatable": `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="alias">@string/hello</string>
    <string name="fixed" translatable="false">DO NOT TRANSLATE</string>
    <string name="real">Real text</string>
</resources>
`,
		"array_and_plurals": `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string-array name="days">
        <item>Monday</item>
        <item>Tuesday</item>
    </string-array>
    <plurals name="count">
        <item quantity="one">%d item</item>
        <item quantity="other">%d items</item>
    </plurals>
</resources>
`,
		"structural_noise": `<?xml version="1.0" encoding="utf-8"?>
<resources xmlns:xliff="urn:oasis:names:tc:xliff:document:1.2">
    <color name="brand">#FF0000</color>
    <string name="hi">Hi</string>
    <declare-styleable name="X"><attr name="y" format="string"/></declare-styleable>
</resources>
`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			buffered := roundtripPathAndroid(t, in, false)
			streaming := roundtripPathAndroid(t, in, true)
			assert.Equal(t, buffered, streaming, "streaming output must match buffered output")
			if name != "inline_and_cdata" { // CDATA is a documented non-byte-exact exception
				assert.Equal(t, in, streaming, "streaming round-trip should be byte-exact")
			}
		})
	}
}
