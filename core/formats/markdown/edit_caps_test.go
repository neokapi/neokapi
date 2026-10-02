package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// A destination is spelled so the reader reads it back as the value given:
// bare where it can be, in angle brackets where it must or the document's
// spelling had them.
func TestSpellLinkDestination(t *testing.T) {
	cases := []struct {
		value string
		angle bool
		want  string
	}{
		{value: "https://example.com/a?b=1&c=2", want: "https://example.com/a?b=1&c=2"},
		{value: "a b.png", want: "<a b.png>"},
		{value: "https://example.com/new", angle: true, want: "<https://example.com/new>"},
		{value: "x(1)", want: "x(1)"},
		{value: "x)y", want: "<x)y>"},
		{value: "a&amp;b", want: "a&amp;b"},
		{value: `c:\*dir`, want: `c:\*dir`},
		{value: "", want: "<>"},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, err := spellLinkDestination(tc.value, tc.angle)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			dest, ok := parsedDestination(got)
			require.True(t, ok)
			assert.Equal(t, tc.value, dest, "a CommonMark parser reads the spelling back as the value")
		})
	}
	_, err := spellLinkDestination("a\nb", false)
	require.ErrorContains(t, err, "line break")
	_, err = spellLinkDestination("<x> y", false)
	assert.ErrorContains(t, err, "no spelling")
}

func link(id, sub, close string, attrs map[string]string) []model.Run {
	return []model.Run{
		model.TextR("See "),
		model.PcOpenR(model.PcOpenRun{ID: id, Type: "link:hyperlink", SubType: sub, Data: "[", Attrs: attrs}),
		model.TextR("the guide"),
		model.PcCloseR(model.PcCloseRun{ID: id, Type: "link:hyperlink", SubType: sub, Data: close}),
	}
}

// WriteAttr changes the destination wherever the reader put it and keeps the
// bytes around it; a link whose destination lives elsewhere is refused.
func TestWriteAttr(t *testing.T) {
	w := NewWriter()

	out, err := w.WriteAttr(link("1", "md:link", "](old 'T')", map[string]string{"href": "old"}), 1, "href", "new place")
	require.NoError(t, err)
	assert.Equal(t, "](<new place> 'T')", out[3].PcClose.Data)
	assert.Equal(t, "new place", out[1].PcOpen.Attrs["href"])

	titled := append(link("1", "md:link", "]", map[string]string{"href": "<old>", "title": "T"}),
		model.PcOpenR(model.PcOpenRun{ID: "2", Type: "link:hyperlink", SubType: subTypeLinkTitle, Data: "(\n  <old> \""}),
		model.TextR("T"),
		model.PcCloseR(model.PcCloseRun{ID: "2", Type: "link:hyperlink", SubType: subTypeLinkTitle, Data: "\")"}))
	out, err = w.WriteAttr(titled, 1, "href", "new")
	require.NoError(t, err)
	assert.Equal(t, "(\n  <new> \"", out[4].PcOpen.Data, "the destination keeps the document's angle brackets and the whitespace around it")
	assert.Equal(t, "]", out[3].PcClose.Data)
	assert.Equal(t, "(\n  <old> \"", titled[4].PcOpen.Data, "the sequence given is left as it was")

	image := []model.Run{
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "media:image", SubType: "md:image", Data: "![", Attrs: map[string]string{"src": "a.png"}}),
		model.TextR("alt"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "media:image", SubType: "md:image", Data: "](a.png)"}),
	}
	out, err = w.WriteAttr(image, 0, "src", "b.png")
	require.NoError(t, err)
	assert.Equal(t, "](b.png)", out[2].PcClose.Data)
	assert.Equal(t, "b.png", out[0].PcOpen.Attrs["src"])
	_, err = w.WriteAttr(image, 0, "href", "b.png")
	require.ErrorContains(t, err, "src of this code")

	_, err = w.WriteAttr(link("1", "md:link-ref", "][guide]", nil), 1, "href", "x")
	require.ErrorContains(t, err, "definition")
	auto := []model.Run{model.PhR(model.PlaceholderRun{ID: "1", Type: "link:hyperlink", SubType: "md:autolink", Data: "<https://x>"})}
	_, err = w.WriteAttr(auto, 0, "href", "x")
	assert.ErrorContains(t, err, "autolink")
}

// SynthesizeCode writes Markdown delimiters only where CommonMark reads them
// as the code, and a link only where Markdown allows one.
func TestSynthesizeCode(t *testing.T) {
	w := NewWriter()
	para := &model.Block{}
	site := func(typ, before, inner, after string, attrs map[string]string) format.CodeSite {
		run := func(s string) []model.Run {
			if s == "" {
				return nil
			}
			return []model.Run{model.TextR(s)}
		}
		return format.CodeSite{Type: typ, Attrs: attrs, Block: para, Before: run(before), Inner: run(inner), After: run(after)}
	}

	open, closing, err := w.SynthesizeCode(site("fmt:bold", "We ", "pick", " daily", nil))
	require.NoError(t, err)
	assert.Equal(t, "**", open.PcOpen.Data)
	assert.Equal(t, "md:strong", closing.PcClose.SubType)
	open, closing, err = w.SynthesizeCode(site("link:hyperlink", "", "Herbs", " we", map[string]string{"href": "a b"}))
	require.NoError(t, err)
	assert.Equal(t, "[", open.PcOpen.Data)
	assert.Equal(t, "](<a b>)", closing.PcClose.Data)
	assert.Equal(t, "a b", open.PcOpen.Attrs["href"])

	code := model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:code"})
	lnk := model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink"})
	for _, tc := range []struct {
		name string
		site format.CodeSite
		want string
	}{
		{"a leading space", site("fmt:bold", "We", " pick", "", nil), "space"},
		{"a trailing space", site("fmt:italic", "We ", "pick ", "", nil), "space"},
		{"punctuation after a word", site("fmt:bold", "a", `"b"`, "", nil), "does not open"},
		{"punctuation before a word", site("fmt:bold", "", `"b"`, "c", nil), "does not close"},
		{"beside a star", site("fmt:italic", "a*", "b", "", nil), "join"},
		{"inside a code span", format.CodeSite{Type: "fmt:bold", Block: para, Enclosing: []model.Run{code}, Inner: []model.Run{model.TextR("x")}}, "code span"},
		{"a code block", format.CodeSite{Type: "fmt:bold", Block: &model.Block{Type: "code-block"}}, "literal"},
		{"inside a link", format.CodeSite{Type: "link:hyperlink", Attrs: map[string]string{"href": "x"}, Block: para, Enclosing: []model.Run{lnk}}, "another link"},
		{"an unbalanced bracket", site("link:hyperlink", "", "a ] b", "", map[string]string{"href": "x"}), "bracket"},
		{"a title", site("link:hyperlink", "", "a", "", map[string]string{"href": "x", "title": "t"}), "no title"},
		{"no href", site("link:hyperlink", "", "a", "", nil), "needs an href"},
		{"a link after a bang", site("link:hyperlink", "Wow!", "great", " stuff", map[string]string{"href": "x"}), "image"},
		{"a backslash before a link", site("link:hyperlink", `foo\`, "bar", " here", map[string]string{"href": "x"}), "backslash"},
		{"a backslash before bold", site("fmt:bold", `foo\`, "bar", " here", nil), "backslash"},
		{"a backslash ending the range", site("fmt:bold", "", `foo\`, " bar", nil), "backslash"},
		{"a backslash ending a link's text", site("link:hyperlink", "", `foo\`, " bar", map[string]string{"href": "x"}), "backslash"},
		{"a delimiter the text holds pairs first", site("fmt:italic", "Rated *five", "stars", " by cooks", nil), "would not read"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := w.SynthesizeCode(tc.site)
			assert.ErrorContains(t, err, tc.want)
		})
	}

	// Markup the text around it does not disturb is written, including
	// beside an escaped character and around codes of the text's own.
	span := []model.Run{
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:code", Data: "`"}), model.TextR("x"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:code", Data: "`"}),
	}
	for _, tc := range []struct {
		name string
		site format.CodeSite
	}{
		{"a link after an escaped bang", site("link:hyperlink", `Wow\!`, "great", "", map[string]string{"href": "x"})},
		{"bold after an escaped backslash", site("fmt:bold", `foo\\ `, "bar", "", nil)},
		{"bold around a code span", format.CodeSite{Type: "fmt:bold", Block: para, Before: []model.Run{model.TextR("Run ")}, Inner: span, After: []model.Run{model.TextR(" now")}}},
		{"a link around bold", format.CodeSite{Type: "link:hyperlink", Attrs: map[string]string{"href": "x"}, Block: para, Inner: []model.Run{
			model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "**"}), model.TextR("x"),
			model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "**"})}}},
		{"text that would open a list on its own line", site("fmt:bold", "- ", "item", "", nil)},
		{"a literal star that closes nothing", site("fmt:italic", "Rated *five ", "stars", " by cooks", nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := w.SynthesizeCode(tc.site)
			assert.NoError(t, err)
		})
	}
}

// readsBack is the proof under the named checks: parsed in place, markup the
// text around it would turn into something else is refused even with no check
// naming the cause, and markup that reads back as the code over the range
// passes.
func TestReadsBack(t *testing.T) {
	isLink := func(n ast.Node) bool { l, ok := n.(*ast.Link); return ok && string(l.Destination) == "u" }
	isBold := func(n ast.Node) bool { em, ok := n.(*ast.Emphasis); return ok && em.Level == 2 }
	for _, tc := range []struct {
		name                 string
		before, inner, after string
		open, closing        string
		isCode               func(ast.Node) bool
		ok                   bool
	}{
		{"a link after a bang reads as an image", "Wow!", "great", " stuff", "[", "](u)", isLink, false},
		{"a backslash escapes a link's bracket", `foo\`, "bar", " here", "[", "](u)", isLink, false},
		{"a backslash escapes a bold opener", `foo\`, "bar", " here", "**", "**", isBold, false},
		{"a backslash escapes a bold closer", "Path ", `foo\`, " bar", "**", "**", isBold, false},
		{"a closer that pairs with an earlier opener", "Rated **five", "stars", " by", "**", "**", isBold, false},
		{"bold over the range", "We ", "pick", " daily", "**", "**", isBold, true},
		{"a link over the range", "See ", "the guide", ".", "[", "](u)", isLink, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := format.CodeSite{Before: []model.Run{model.TextR(tc.before)}, Inner: []model.Run{model.TextR(tc.inner)}, After: []model.Run{model.TextR(tc.after)}}
			err := readsBack(site, tc.open, tc.closing, tc.isCode)
			if tc.ok {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err)
		})
	}
}
