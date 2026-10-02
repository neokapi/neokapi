package xml

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

func elementRuns(data string) []model.Run {
	return []model.Run{
		model.TextR("See "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:link", Data: data, Attrs: inlineTagAttrs(data)}),
		model.TextR("the guide"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:link", Data: "</link>"}),
	}
}

// WriteAttr changes the value of an attribute the element spells, escaped for
// its quotes, keeping every other byte; it adds no attribute.
func TestWriteAttr(t *testing.T) {
	cases := []struct {
		name, data, attr, value, want string
	}{
		{"double quotes", `<link href="old" rel='x'>`, "href", `a&b<c>"d"'e'`, `<link href="a&amp;b&lt;c>&quot;d&quot;'e'" rel='x'>`},
		{"single quotes", `<link href='old'>`, "href", `"d"'e'`, `<link href='"d"&apos;e&apos;'>`},
		{"whitespace a parser would normalize", `<link href="old">`, "href", "a\tb\nc", `<link href="a&#9;b&#10;c">`},
		{"a namespaced attribute", `<link xlink:href="old">`, "xlink:href", "new", `<link xlink:href="new">`},
	}
	w := NewWriter()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seq := elementRuns(tc.data)
			out, err := w.WriteAttr(seq, 1, tc.attr, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.want, out[1].PcOpen.Data)
			assert.Equal(t, tc.value, out[1].PcOpen.Attrs[tc.attr], "the attributes read back as the reader reads the new tag")
			assert.Equal(t, tc.data, seq[1].PcOpen.Data, "the sequence given is left as it was")
		})
	}

	for _, tc := range []struct {
		name, data, attr, value, want string
	}{
		{"an attribute the element does not spell", `<link href="x">`, "title", "t", "spells no title"},
		{"a namespace declaration", `<link xmlns:x="urn:x">`, "xmlns:x", "urn:y", "namespace declaration"},
		{"a translatable attribute", "<link title=\"\x01REF:tu2\x02\">", "title", "t", "block of its own"},
		{"a character XML cannot carry", `<link href="x">`, "href", "a\x01b", "U+0001"},
		{"markup that is not one start tag", `<link href="x">y</link>`, "href", "z", "single start tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := w.WriteAttr(elementRuns(tc.data), 1, tc.attr, tc.value)
			assert.ErrorContains(t, err, tc.want)
		})
	}
	ph := []model.Run{model.PhR(model.PlaceholderRun{ID: "1", Data: `<x translate="no">y</x>`})}
	_, err := w.WriteAttr(ph, 0, "translate", "yes")
	assert.ErrorContains(t, err, "start tag")
}

// The reader records an inline element's attributes on its code, decoded and
// normalized as an XML parser reads them, leaving out namespace declarations
// and the attributes it reads as blocks of their own.
func TestReaderRecordsInlineElementAttributes(t *testing.T) {
	src := `<?xml version="1.0"?>
<doc><p>See <link href="a&amp;b&#x41;" rel='x y' xmlns:x="urn:x">the guide</link> now.</p></doc>`
	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		URI: "a.xml", SourceLocale: model.LocaleEnglish, Reader: io.NopCloser(strings.NewReader(src)),
	}))
	var found bool
	for res := range r.Read(context.Background()) {
		require.NoError(t, res.Error)
		b, ok := res.Part.Resource.(*model.Block)
		if !ok {
			continue
		}
		for _, run := range b.Source {
			if run.PcOpen != nil {
				found = true
				assert.Equal(t, map[string]string{"href": "a&bA", "rel": "x y"}, run.PcOpen.Attrs)
			}
		}
	}
	require.NoError(t, r.Close())
	assert.True(t, found)

	assert.Equal(t, map[string]string{"a": "x y"}, inlineTagAttrs("<e a=\"x\ny\">"), "a literal line break reads as a space")
	assert.Nil(t, inlineTagAttrs("<e>"))
}
