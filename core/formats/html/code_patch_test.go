package html

import (
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := patchCodes(original, read, tc.edited)
			assert.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}

	// What patchCodes declines, the writer renders.
	for _, tc := range []struct {
		name     string
		original string
		edited   []model.Run
	}{
		{"a text edit", original, []model.Run{model.TextR("Order the "), open, model.TextR("weekly box"), closing, model.TextR(" before Monday, \"rain\" & shine.")}},
		{"a code removed", original, []model.Run{model.TextR("Order the weekly box before Friday, \"rain\" & shine.")}},
		{"a code moved", original, []model.Run{open, model.TextR("Order the "), closing, model.TextR("weekly box before Friday, \"rain\" & shine.")}},
		{"bytes that do not align", "Order the weekly box", []model.Run{model.TextR("Order the "), open, model.TextR("weekly box"), closing, model.TextR(" before Friday, \"rain\" & shine.")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := patchCodes(tc.original, read, tc.edited)
			assert.False(t, ok)
		})
	}
}
