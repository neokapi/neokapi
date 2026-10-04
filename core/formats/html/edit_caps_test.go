package html

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
)

func linkRuns(data string) []model.Run {
	return []model.Run{
		model.TextR("See "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink", SubType: "html:a", Data: data, Attrs: tagAttrsFor("link:hyperlink", data)}),
		model.TextR("the guide"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link:hyperlink", SubType: "html:a", Data: "</a>"}),
	}
}

// WriteAttr changes one attribute inside the start tag and keeps every other
// byte, escaping the value so it reads back as given whatever quotes hold it.
func TestWriteAttr(t *testing.T) {
	cases := []struct {
		name, data, attr, value, want string
	}{
		{"double quotes", `<a href="old" class="x">`, "href", `new?a=1&b="2"`, `<a href="new?a=1&amp;b=&#34;2&#34;" class="x">`},
		{"single quotes", `<a href='old'>`, "href", "it's", `<a href='it&#39;s'>`},
		{"unquoted", `<a href=old>`, "href", "a b", `<a href="a b">`},
		{"markup in the value", `<a href="old">`, "href", "x>y<z", `<a href="x&gt;y&lt;z">`},
		{"an attribute the tag lacks", `<a class="x">`, "href", "new", `<a class="x" href="new">`},
		{"an attribute with no value", `<a href>`, "href", "new", `<a href="new">`},
		{"upper case", `<A HREF="old">`, "href", "new", `<A HREF="new">`},
	}
	w := NewWriter()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seq := linkRuns(tc.data)
			out, err := w.WriteAttr(seq, 1, tc.attr, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.want, out[1].PcOpen.Data)
			assert.Equal(t, tc.value, out[1].PcOpen.Attrs["href"], "the attributes read back as the reader reads the new tag")
			assert.Equal(t, tc.data, seq[1].PcOpen.Data, "the sequence given is left as it was")
			assert.Len(t, out, len(seq))
		})
	}

	img := []model.Run{model.PhR(model.PlaceholderRun{ID: "1", Type: "media:image", Data: `<img src="a.png" alt="` + blockRefSentinelStart + "tu2\x00" + `">`})}
	out, err := w.WriteAttr(img, 0, "src", "b c.png")
	require.NoError(t, err)
	assert.Equal(t, `<img src="b c.png" alt="`+blockRefSentinelStart+"tu2\x00"+`">`, out[0].Ph.Data)
	assert.Equal(t, "b c.png", out[0].Ph.Attrs["src"])

	_, err = w.WriteAttr(img, 0, "alt", "x")
	require.ErrorContains(t, err, "block of its own")
	_, err = w.WriteAttr(linkRuns(`<a href="x">`), 1, "href", "a\x00b")
	require.ErrorContains(t, err, "NUL")
	_, err = w.WriteAttr(linkRuns(`<a href="x">trailing`), 1, "href", "y")
	require.ErrorContains(t, err, "single start tag")
	_, err = w.WriteAttr(linkRuns(`<a href="x">`), 0, "href", "y")
	require.ErrorContains(t, err, "not an inline code")
}

// SynthesizeCode writes the semantic element for a type, as the reader reads
// it, and refuses a place HTML cannot carry it.
func TestSynthesizeCode(t *testing.T) {
	w := NewWriter()
	para := &model.Block{Type: "paragraph"}
	for _, tc := range []struct {
		typ, open, close string
		attrs            map[string]string
	}{
		{typ: "fmt:bold", open: "<strong>", close: "</strong>"},
		{typ: "fmt:italic", open: "<em>", close: "</em>"},
		{typ: "link:hyperlink", open: `<a href="https://x.example/?a=1&amp;b=2">`, close: "</a>", attrs: map[string]string{"href": "https://x.example/?a=1&b=2"}},
	} {
		open, closing, err := w.SynthesizeCode(format.CodeSite{Type: tc.typ, Attrs: tc.attrs, Block: para})
		require.NoError(t, err, tc.typ)
		assert.Equal(t, tc.open, open.PcOpen.Data)
		assert.Equal(t, tc.close, closing.PcClose.Data)
		assert.Equal(t, tc.typ, open.PcOpen.Type)
		assert.Equal(t, tc.attrs, open.PcOpen.Attrs)
		require.NotNil(t, open.PcOpen.Constraints)
		assert.True(t, open.PcOpen.Constraints.Deletable)
	}

	link := model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink"})
	button := model.PcOpenR(model.PcOpenRun{ID: "1", Type: "x:button", SubType: "html:button"})
	inAnchor := &model.Block{Type: "h3", Properties: map[string]string{PropInteractiveAncestor: "a"}}
	href := map[string]string{"href": "x"}
	for _, tc := range []struct {
		name string
		site format.CodeSite
		want string
	}{
		{"inside a link", format.CodeSite{Type: "link:hyperlink", Attrs: href, Block: para, Enclosing: []model.Run{link}}, "inside another link"},
		{"inside a button", format.CodeSite{Type: "link:hyperlink", Attrs: href, Block: para, Enclosing: []model.Run{button}}, "or a button"},
		{"inside a link the markup holds", format.CodeSite{Type: "link:hyperlink", Attrs: href, Block: inAnchor}, "sits inside an <a> element"},
		{"around a link", format.CodeSite{Type: "link:hyperlink", Attrs: href, Block: para, Inner: []model.Run{link}}, "hold another link"},
		{"around a button", format.CodeSite{Type: "link:hyperlink", Attrs: href, Block: para, Inner: []model.Run{button}}, "or a button"},
		{"no href", format.CodeSite{Type: "link:hyperlink", Block: para}, "needs an href"},
		{"attributes on bold", format.CodeSite{Type: "fmt:bold", Attrs: href, Block: para}, "no attributes"},
		{"an attribute value", format.CodeSite{Type: "fmt:bold", Block: &model.Block{Type: "alt", IsReferent: true}}, "holds no markup"},
		{"the document title", format.CodeSite{Type: "fmt:bold", Block: &model.Block{Type: "title"}}, "holds no markup"},
		{"an unknown type", format.CodeSite{Type: "fmt:underline", Block: para}, "no element"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := w.SynthesizeCode(tc.site)
			assert.ErrorContains(t, err, tc.want)
		})
	}

	open, _, err := w.SynthesizeCode(format.CodeSite{Type: "fmt:bold", Block: inAnchor})
	require.NoError(t, err, "bold may go inside a link")
	assert.Equal(t, "html:strong", open.PcOpen.SubType)
}

// Both readers record the a or button element a block sits inside, outside its
// own runs, so the writer can refuse a new link there.
func TestReadersRecordInteractiveAncestor(t *testing.T) {
	const doc = `<html><body>
<a href="/cards"><h3>Big title</h3> more words</a>
<button><div>Press here</div></button>
<p>Plain words</p>
</body></html>`
	// The two readers split this markup into blocks differently, so each
	// block is checked by the words it holds.
	want := []struct{ words, tag string }{{"Big title", "a"}, {"more words", "a"}, {"Press here", "button"}, {"Plain words", ""}}
	for _, skeleton := range []bool{true, false} {
		r := NewReader()
		if skeleton {
			store, err := format.NewSkeletonStore()
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			r.SetSkeletonStore(store)
		}
		parts, err := spec.ReadParts(r, []byte(doc))
		require.NoError(t, err)
		seen := 0
		for _, p := range parts {
			b, ok := p.Resource.(*model.Block)
			if !ok {
				continue
			}
			for _, w := range want {
				if strings.Contains(model.RunsText(b.SourceRuns()), w.words) {
					seen++
					assert.Equal(t, w.tag, b.Properties[PropInteractiveAncestor], "skeleton=%v: the block holding %q", skeleton, w.words)
				}
			}
		}
		if skeleton {
			assert.Equal(t, len(want), seen, "every phrase is read")
		} else {
			assert.GreaterOrEqual(t, seen, 2, "the DOM reader reads the text after the heading and the paragraph")
		}
	}
}
