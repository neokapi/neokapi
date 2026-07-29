package ts

import (
	"bufio"
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundtripPath drives the reader through a specific path (streaming vs
// buffered) by calling the unexported entry point directly, then reconstructs
// the document via the writer + skeleton store. It is in-package so it can
// bypass readContent's gate and exercise both paths on the same input.
func roundtripPath(t *testing.T, input string, streaming bool) string {
	t.Helper()
	ctx := context.Background()

	r := NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	r.SetSkeletonStore(store)
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		br := bufio.NewReaderSize(r.Doc.Reader, 1<<16)
		if streaming {
			r.readStreaming(ctx, ch, locale, br)
		} else {
			r.readBuffered(ctx, ch, locale, br)
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

// TestStreamingBufferedParity asserts the streaming path reproduces the buffered
// path's output byte-for-byte across the tricky cases: numerus forms (exercises
// the within-message backward slices — the `<numerusform` LastIndex, per-form
// prefixes, and trailing whitespace), a synthesized <translation> section
// (source with no translation, where output != input), CDATA in the skeleton,
// and unicode. Both paths share walkTokens, so any divergence is in the raw-byte
// sourcing (buffer vs bounded window) — exactly what this pins down.
func TestStreamingBufferedParity(t *testing.T) {
	cases := map[string]string{
		"numerus": `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr">
<context>
    <name>Plurals</name>
    <message numerus="yes">
        <source>%n file(s)</source>
        <translation>
            <numerusform>%n fichier</numerusform>
            <numerusform>%n fichiers</numerusform>
        </translation>
    </message>
</context>
</TS>
`,
		"synthesized_no_translation": `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr">
<context>
    <name>Synth</name>
    <message>
        <source>Untranslated text</source>
    </message>
</context>
</TS>
`,
		"cdata_and_comments": `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr">
<context>
    <name>Mix</name>
    <message>
        <source><![CDATA[a & b < c]]></source>
        <comment>note here</comment>
        <translation>a et b</translation>
    </message>
</context>
</TS>
`,
		"unicode": `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="ja">
<context>
    <name>U</name>
    <message>
        <source>Hello</source>
        <translation>こんにちは 世界</translation>
    </message>
</context>
</TS>
`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			buffered := roundtripPath(t, in, false)
			streaming := roundtripPath(t, in, true)
			assert.Equal(t, buffered, streaming, "streaming output must match buffered output")
		})
	}
}

// TestExtractTSPrologue_QuotedAngleBracket pins the extent of the captured
// `<TS …>` tag against attribute values containing a '>'. A '>' needs no
// escaping in an XML attribute value, so a conforming producer may emit
// language="a>b"; scanning for the first '>' byte truncates the tag
// mid-attribute and the writer re-emits the truncation (#1606).
func TestExtractTSPrologue_QuotedAngleBracket(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		raw      string
		prologue string
		tsTag    string
	}{
		"plain": {
			raw:      `<?xml version="1.0"?><TS version="2.1" language="fr">`,
			prologue: `<?xml version="1.0"?>`,
			tsTag:    `<TS version="2.1" language="fr">`,
		},
		"gt in double-quoted value": {
			raw:      `<?xml version="1.0"?><TS version="2.1" language="a>b" sourcelanguage="en">`,
			prologue: `<?xml version="1.0"?>`,
			tsTag:    `<TS version="2.1" language="a>b" sourcelanguage="en">`,
		},
		"gt in single-quoted value": {
			raw:      `<?xml version="1.0"?><TS version='2.1' language='a>b'>`,
			prologue: `<?xml version="1.0"?>`,
			tsTag:    `<TS version='2.1' language='a>b'>`,
		},
		"quote of the other kind inside a value": {
			raw:   `<TS version="2.1" language="it's > here" sourcelanguage='say "hi" > there'>`,
			tsTag: `<TS version="2.1" language="it's > here" sourcelanguage='say "hi" > there'>`,
		},
		"bare tag": {
			raw:   `<TS>`,
			tsTag: `<TS>`,
		},
		"doctype internal subset before the tag": {
			raw:      `<?xml version="1.0"?><!DOCTYPE TS []><TS version="2.1" language="a>b">`,
			prologue: `<?xml version="1.0"?><!DOCTYPE TS []>`,
			tsTag:    `<TS version="2.1" language="a>b">`,
		},
		"never terminated": {
			raw: `<TS version="2.1" language="a`,
		},
		"unterminated quote swallows the rest": {
			raw: `<TS version="2.1" language="a>b>`,
		},
		"no TS element": {
			raw: `<?xml version="1.0"?><other/>`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			prologue, tsTag := extractTSPrologue(tc.raw)
			assert.Equal(t, tc.prologue, prologue, "prologue")
			assert.Equal(t, tc.tsTag, tsTag, "tsTag")
		})
	}
}
