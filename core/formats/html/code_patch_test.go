package html

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

// An edit to a block's codes is written as a patch of the block's own bytes:
// the whitespace the reader normalized and the characters the encoding pass
// would respell keep the document's spelling, and only the codes change.
func TestPatchCodes(t *testing.T) {
	// The block as the document holds it, and as the reader reads it once its
	// whitespace is normalized: a line break and indentation become a space,
	// and the leading line break is peeled off.
	const original = "\n    Order the <a href=\"/box\">weekly box</a>\n    before Friday, \"rain\"\n    & shine."
	link := func(data string) (model.Run, model.Run) {
		return model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: data}),
			model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"})
	}
	open, closing := link(`<a href="/box">`)
	read := []model.Run{model.TextR("Order the "), open, model.TextR("weekly box"), closing, model.TextR(" before Friday, \"rain\" & shine.")}
	strong := func(id string) (model.Run, model.Run) {
		return model.PcOpenR(model.PcOpenRun{ID: id, Type: "fmt:bold", Data: "<strong>"}),
			model.PcCloseR(model.PcCloseRun{ID: id, Type: "fmt:bold", Data: "</strong>"})
	}
	s, e := strong("2")
	newOpen, _ := link(`<a href="/box?week=2">`)

	cases := []struct {
		name   string
		edited []model.Run
		want   string
	}{
		{"a code's data", []model.Run{model.TextR("Order the "), newOpen, model.TextR("weekly box"), closing, model.TextR(" before Friday, \"rain\" & shine.")},
			"\n    Order the <a href=\"/box?week=2\">weekly box</a>\n    before Friday, \"rain\"\n    & shine."},
		{"a new code inside the text", []model.Run{model.TextR("Order the "), open, model.TextR("weekly box"), closing, model.TextR(" before "), s, model.TextR("Friday"), e, model.TextR(", \"rain\" & shine.")},
			"\n    Order the <a href=\"/box\">weekly box</a>\n    before <strong>Friday</strong>, \"rain\"\n    & shine."},
		{"a new code over text a collapsed line break joins", []model.Run{model.TextR("Order the "), open, model.TextR("weekly box"), closing, model.TextR(" before Friday, "), s, model.TextR("\"rain\" & shine."), e},
			"\n    Order the <a href=\"/box\">weekly box</a>\n    before Friday, <strong>\"rain\"\n    & shine.</strong>"},
		{"a new code at the start", []model.Run{s, model.TextR("Order"), e, model.TextR(" the "), open, model.TextR("weekly box"), closing, model.TextR(" before Friday, \"rain\" & shine.")},
			"\n    <strong>Order</strong> the <a href=\"/box\">weekly box</a>\n    before Friday, \"rain\"\n    & shine."},
		{"a new code around a held one", []model.Run{model.TextR("Order the "), s, open, model.TextR("weekly box"), closing, e, model.TextR(" before Friday, \"rain\" & shine.")},
			"\n    Order the <strong><a href=\"/box\">weekly box</a></strong>\n    before Friday, \"rain\"\n    & shine."},
		{"a word replaced", []model.Run{model.TextR("Order the "), open, model.TextR("weekly box"), closing, model.TextR(" before Monday, \"rain\" & shine.")},
			"\n    Order the <a href=\"/box\">weekly box</a>\n    before Monday, \"rain\"\n    & shine."},
		{"added text that needs escaping", []model.Run{model.TextR("Order the "), open, model.TextR("weekly box"), closing, model.TextR(" before Friday & <Monday>, \"rain\" & shine.")},
			"\n    Order the <a href=\"/box\">weekly box</a>\n    before Friday &amp; &lt;Monday>, \"rain\"\n    & shine."},
		{"a code removed", []model.Run{model.TextR("Order the weekly box before Friday, \"rain\" & shine.")},
			"\n    Order the weekly box\n    before Friday, \"rain\"\n    & shine."},
		{"a code moved", []model.Run{open, model.TextR("Order the "), closing, model.TextR("weekly box before Friday, \"rain\" & shine.")},
			"\n    <a href=\"/box\">Order the </a>weekly box\n    before Friday, \"rain\"\n    & shine."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := patchEdit(original, read, tc.edited)
			assert.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}

	// What patchEdit declines, the writer renders.
	t.Run("bytes that do not align", func(t *testing.T) {
		_, ok := patchEdit("Order the weekly box", read, read)
		assert.False(t, ok)
	})
}

// Runs of whitespace the reader collapsed keep their bytes beside an edit,
// and the whitespace the reader peeled off the edges stays where it was.
func TestPatchEditKeepsCollapsedWhitespace(t *testing.T) {
	const original = "  We utilize   the\n   widget.  "
	read := []model.Run{model.TextR("We utilize the widget.")}
	got, ok := patchEdit(original, read, []model.Run{model.TextR("We use the widget.")})
	assert.True(t, ok)
	assert.Equal(t, "  We use   the\n   widget.  ", got)

	got, ok = patchEdit(original, read, []model.Run{model.TextR("Today we use the widget.")})
	assert.True(t, ok)
	assert.Equal(t, "  Today we use   the\n   widget.  ", got)
}

// A bare `<` or `&` reads as text because of the byte after it. When an edit
// changes that byte, the kept character is escaped, so it opens no tag and no
// character reference with what follows; where the byte after it is kept, it
// keeps its spelling.
func TestPatchEditEscapesAKeptMarkupCharacterBesideTheEdit(t *testing.T) {
	cases := []struct{ name, original, read, edited, want string }{
		{"a < before a removed space", "a < b c", "a < b c", "a <b c", "a &lt;b c"},
		{"an & before added letters", "R & D", "R & D", "R &copy D", "R &amp;copy D"},
		{"an & at the end before added text", "R &", "R &", "R &amp", "R &amp;amp"},
		{"a < whose next byte is kept", "a < b c", "a < b c", "a < b d", "a < b d"},
		{"an & whose next byte is kept", "R & D now", "R & D now", "R & D later", "R & D later"},
		{"an & before kept letters and an added ;", "R &am now", "R &am now", "R &amp; now", "R &amp;amp; now"},
		{"an & before letters one of which changed", "R &amx now", "R &amx now", "R &amp now", "R &amp;amp now"},
		{"an & whose letters are kept", "Say &am now", "Say &am now", "Say &am later", "Say &am later"},
		{"an & whose letters end the block", "Say &am", "Say &am", "So say &am", "So say &am"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := patchEdit(tc.original, []model.Run{model.TextR(tc.read)}, []model.Run{model.TextR(tc.edited)})
			assert.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A one-word edit of a long block keeps every run of whitespace the reader
// collapsed, however long the block is.
func TestPatchEditKeepsWhitespaceInALongBlock(t *testing.T) {
	long := strings.Repeat("word   and\n   more ", 5000)
	readLong := strings.Repeat("word and more ", 5000)
	original := long + "We utilize it " + long
	read := []model.Run{model.TextR(strings.TrimSpace(readLong + "We utilize it " + readLong))}
	edited := []model.Run{model.TextR(strings.TrimSpace(readLong + "We use it " + readLong))}
	got, ok := patchEdit(original, read, edited)
	assert.True(t, ok)
	assert.Equal(t, long+"We use it "+long, got)
}
