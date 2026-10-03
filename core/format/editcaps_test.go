package format_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// plainWriter declares no edit capability.
type plainWriter struct{ format.BaseFormatWriter }

func (plainWriter) Write(context.Context, <-chan *model.Part) error { return nil }

// editingWriter declares every edit capability, out of order, so the probe's
// copy and sort show.
type editingWriter struct{ plainWriter }

func (editingWriter) WritableAttrs() map[string][]string {
	return map[string][]string{"link:hyperlink": {"title", "href"}, format.AnyCodeType: {"id"}}
}

func (editingWriter) WriteAttr([]model.Run, int, string, string) ([]model.Run, error) {
	return nil, errors.New("unused")
}
func (editingWriter) Synthesizes() []string { return []string{"fmt:italic", "fmt:bold"} }
func (editingWriter) SynthesizeCode(format.CodeSite) (model.Run, model.Run, error) {
	return model.Run{}, model.Run{}, errors.New("unused")
}
func (editingWriter) Structural() []string { return []string{"insert_block"} }
func (editingWriter) EditStructure(doc []byte, _ []format.StructuralEdit) ([]byte, error) {
	return doc, nil
}

// listingWriter lists a structural operation and has no half that writes it.
type listingWriter struct{ plainWriter }

func (listingWriter) Structural() []string { return []string{"insert_block"} }
func (editingWriter) NativeOps() []format.NativeOp {
	return []format.NativeOp{{Name: "z.op"}, {Name: "a.op"}}
}

func TestProbeEditCapabilities(t *testing.T) {
	w := editingWriter{}
	c := format.ProbeEditCapabilities(&w)
	assert.Equal(t, map[string][]string{"link:hyperlink": {"href", "title"}, "*": {"id"}}, c.WritableAttrs)
	assert.Equal(t, []string{"fmt:bold", "fmt:italic"}, c.Synthesizes)
	assert.Equal(t, []string{"insert_block"}, c.Structural)
	assert.Equal(t, []format.NativeOp{{Name: "a.op"}, {Name: "z.op"}}, c.NativeOps)
	assert.False(t, c.IsZero())

	assert.Equal(t, []string{"href", "id", "title"}, c.Writable("link:hyperlink"), "a type's own attributes and every type's")
	assert.Equal(t, []string{"id"}, c.Writable("media:image"))
	assert.True(t, c.CanWrite("link:hyperlink", "href"))
	assert.True(t, c.CanWrite("fmt:bold", "id"))
	assert.False(t, c.CanWrite("fmt:bold", "href"))
	assert.True(t, c.CanSynthesize("fmt:bold"))
	assert.False(t, c.CanSynthesize("fmt:underline"))

	plain := format.ProbeEditCapabilities(&plainWriter{})
	assert.True(t, plain.IsZero(), "a writer that implements nothing declares nothing")
	listing := format.ProbeEditCapabilities(&listingWriter{})
	assert.Empty(t, listing.Structural, "a structural operation is declared only with the half that writes it")

	wild := format.EditCapabilities{WritableAttrs: map[string][]string{format.AnyCodeType: {format.AnyAttr}}}
	assert.True(t, wild.CanWrite("fmt:link", "rel"), "a wildcard declares every attribute of every code")

	clone := c.Clone()
	clone.WritableAttrs["link:hyperlink"][0] = "changed"
	assert.Equal(t, "href", c.WritableAttrs["link:hyperlink"][0], "a clone shares nothing")
}

func TestParseStartTag(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		ok     bool
		attrs  map[string][3]string // name: value, quote, hasValue
		insert int
	}{
		{name: "double and single quotes", in: `<a href="x" class='y'>`, ok: true,
			attrs: map[string][3]string{"href": {"x", `"`, "1"}, "class": {"y", "'", "1"}}, insert: 21},
		{name: "unquoted and bare", in: `<input value=v disabled>`, ok: true,
			attrs: map[string][3]string{"value": {"v", "", "1"}, "disabled": {"", "", ""}}, insert: 23},
		{name: "self-closing", in: `<img src="a.png"/>`, ok: true,
			attrs: map[string][3]string{"src": {"a.png", `"`, "1"}}, insert: 16},
		{name: "spaces around the equals sign", in: `<link  href = "x" >`, ok: true,
			attrs: map[string][3]string{"href": {"x", `"`, "1"}}, insert: 17},
		{name: "no attributes", in: `<b>`, ok: true, insert: 2},
		{name: "a namespaced attribute", in: `<text:a xlink:href="u">`, ok: true,
			attrs: map[string][3]string{"xlink:href": {"u", `"`, "1"}}, insert: 22},
		{name: "a quote that does not close", in: `<a href="x>`, ok: false},
		{name: "no closing bracket", in: `<a href="x"`, ok: false},
		{name: "not a start tag", in: `</a>`, ok: false},
		{name: "text", in: `**`, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tag, ok := format.ParseStartTag(tc.in)
			require.Equal(t, tc.ok, ok)
			if !ok {
				return
			}
			assert.Equal(t, len(tc.in), tag.End)
			assert.Equal(t, tc.insert, tag.InsertAt)
			assert.Len(t, tag.Attrs, len(tc.attrs))
			for name, want := range tc.attrs {
				a, found := tag.Attr(name, false)
				require.True(t, found, name)
				assert.Equal(t, want[0], tc.in[a.ValueStart:a.ValueEnd], name)
				assert.Equal(t, want[1], string(bytesOrEmpty(a.Quote)), name)
				assert.Equal(t, want[2] == "1", a.HasValue, name)
			}
		})
	}
	tag, _ := format.ParseStartTag(`<A HREF="x">`)
	_, found := tag.Attr("href", true)
	assert.True(t, found, "HTML names compare without case")
	_, found = tag.Attr("href", false)
	assert.False(t, found, "XML names compare exactly")
}

func bytesOrEmpty(b byte) []byte {
	if b == 0 {
		return nil
	}
	return []byte{b}
}
