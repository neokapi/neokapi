package xml

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// elementRuns is a link code holding data, recording each attribute the tag
// spells except a namespace declaration and those named in interpreted, as
// the reader records them.
func elementRuns(data string, interpreted ...string) []model.Run {
	var attrs map[string]string
	if tag, ok := format.ParseStartTag(data); ok {
		for _, a := range tag.Attrs {
			if !a.HasValue || isNamespaceDecl(a.Name) || slices.Contains(interpreted, a.Name) {
				continue
			}
			if attrs == nil {
				attrs = map[string]string{}
			}
			attrs[a.Name] = data[a.ValueStart:a.ValueEnd]
		}
	}
	return []model.Run{
		model.TextR("See "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:link", Data: data, Attrs: attrs}),
		model.TextR("the guide"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:link", Data: "</link>"}),
	}
}

// WriteAttr changes the value of an attribute the element spells, escaped for
// its quotes, keeping every other byte; it adds no attribute, and changes no
// attribute the reader reads as an instruction.
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
			assert.Equal(t, tc.value, out[1].PcOpen.Attrs[tc.attr], "the code records the value a parser reads from the new tag")
			assert.Equal(t, tc.data, seq[1].PcOpen.Data, "the sequence given is left as it was")
		})
	}

	for _, tc := range []struct {
		name, data, attr, value, want string
		interpreted                   []string
	}{
		{"an attribute the element does not spell", `<link href="x">`, "title", "t", "spells no title", nil},
		{"a namespace declaration", `<link xmlns:x="urn:x">`, "xmlns:x", "urn:y", "namespace declaration", nil},
		{"a translatable attribute", "<link title=\"\x01REF:tu2\x02\">", "title", "t", "block of its own", nil},
		{"an attribute the reader reads", `<link its:translate="yes" href="x">`, "its:translate", "no", "instruction", []string{"its:translate"}},
		{"a character XML cannot carry", `<link href="x">`, "href", "a\x01b", "U+0001", nil},
		{"markup that is not one start tag", `<link href="x">y</link>`, "href", "z", "single start tag", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := w.WriteAttr(elementRuns(tc.data, tc.interpreted...), 1, tc.attr, tc.value)
			assert.ErrorContains(t, err, tc.want)
		})
	}
	ph := []model.Run{model.PhR(model.PlaceholderRun{ID: "1", Data: `<x translate="no">y</x>`})}
	_, err := w.WriteAttr(ph, 0, "translate", "yes")
	assert.ErrorContains(t, err, "start tag")
}

// inlineCodes reads src with cfg (the default when nil) and returns the
// opening halves of the inline codes of its blocks.
func inlineCodes(t *testing.T, src string, cfg *Config) []*model.PcOpenRun {
	t.Helper()
	r := NewReader()
	if cfg != nil {
		require.NoError(t, r.SetConfig(cfg))
	}
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		URI: "a.xml", SourceLocale: model.LocaleEnglish, Reader: io.NopCloser(strings.NewReader(src)),
	}))
	var out []*model.PcOpenRun
	for res := range r.Read(context.Background()) {
		require.NoError(t, res.Error)
		if b, ok := res.Part.Resource.(*model.Block); ok {
			for _, run := range b.SourceRuns() {
				if run.PcOpen != nil {
					out = append(out, run.PcOpen)
				}
			}
		}
	}
	require.NoError(t, r.Close())
	return out
}

// The reader records an inline element's attributes on its code, with the
// values an XML parser reads, leaving out namespace declarations and the
// attributes it reads as blocks of their own.
func TestReaderRecordsInlineElementAttributes(t *testing.T) {
	codes := inlineCodes(t, `<?xml version="1.0"?>
<doc><p>See <link href="a&amp;b&#x41;" rel='x y' title="a&#10;b
c" xmlns:x="urn:x">the guide</link> now.</p></doc>`, nil)
	require.Len(t, codes, 1)
	assert.Equal(t, map[string]string{"href": "a&bA", "rel": "x y", "title": "a\nb c"}, codes[0].Attrs,
		"a reference keeps its character and a literal line break reads as a space")
}

// An attribute whose value decides how the document reads is left out of the
// code's attributes, so set_attribute refuses it: changing it would change
// what the next read extracts.
func TestReaderLeavesOutAttributesItInterprets(t *testing.T) {
	t.Run("xml and ITS attributes, and one the document's ITS rules test", func(t *testing.T) {
		codes := inlineCodes(t, `<?xml version="1.0"?>
<doc xmlns:its="http://www.w3.org/2005/11/its" its:version="2.0">
<its:rules version="2.0"><its:translateRule selector="//b[@role='skip']" translate="no"/></its:rules>
<p>See <b its:translate="yes" xml:lang="en" xml:space="preserve" role="x" class="c">bold words</b> now.</p></doc>`, nil)
		require.Len(t, codes, 1)
		assert.Equal(t, map[string]string{"class": "c"}, codes[0].Attrs)
	})
	t.Run("an attribute the configuration names", func(t *testing.T) {
		cfg := &Config{
			IDAttributeNames: []string{"key"},
			ElementRules:     []*ElementRule{{Name: "b", RuleTypes: []RuleType{RuleInline}, Condition: &Condition{Attribute: "kind", Op: ConditionEquals, Value: "x"}}},
		}
		codes := inlineCodes(t, `<?xml version="1.0"?>
<doc><p>See <b key="k1" kind="y" class="c">bold words</b> now.</p></doc>`, cfg)
		require.Len(t, codes, 1)
		assert.Equal(t, map[string]string{"class": "c"}, codes[0].Attrs)
	})
}

// An ITS attribute on an inline element decides whether its text is
// translatable, so a change to it is refused before it reaches the bytes.
func TestWriteAttrRefusesAnAttributeTheReaderReads(t *testing.T) {
	codes := inlineCodes(t, `<?xml version="1.0"?>
<doc xmlns:its="http://www.w3.org/2005/11/its"><p>A <b its:translate="yes">bold words</b> here.</p></doc>`, nil)
	require.Len(t, codes, 1)
	seq := []model.Run{model.TextR("A "), model.PcOpenR(*codes[0]), model.TextR("bold words"),
		model.PcCloseR(model.PcCloseRun{ID: codes[0].ID, Data: "</b>"})}
	_, err := NewWriter().WriteAttr(seq, 1, "its:translate", "no")
	assert.ErrorContains(t, err, "instruction")
}
