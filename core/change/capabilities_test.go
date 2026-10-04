package change_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/model"
)

// htmlCaps is what the HTML writer declares, with the writer spelling it: the
// capabilities the change service gives ApplyBlock for an HTML document.
func htmlCaps() change.Capabilities { return change.WriterCapabilities("html", html.NewWriter()) }

// withFormat returns env with the format's capabilities.
func withFormat(env change.BlockEnv, caps change.Capabilities) change.BlockEnv {
	env.Format = caps
	return env
}

// growingAttrWriter writes an attribute by appending a run, which the
// AttrWriter contract does not allow: it moves every run after the code.
type growingAttrWriter struct{}

func (growingAttrWriter) WritableAttrs() map[string][]string {
	return map[string][]string{"link:hyperlink": {"href"}}
}

func (growingAttrWriter) WriteAttr(seq []model.Run, _ int, _, _ string) ([]model.Run, error) {
	return append(slices.Clone(seq), model.TextR("!")), nil
}

func setAttr(edition, ifMatch, code, name, value string) change.Op {
	return change.Op{Kind: change.KindSetAttribute, At: ref(edition), IfMatch: ifMatch,
		Body: &change.SetAttribute{Code: code, Name: name, Value: value}}
}

func mark(edition, ifMatch string, sel change.Selection, typ string, attrs map[string]string) change.Op {
	return change.Op{Kind: change.KindMark, At: ref(edition), IfMatch: ifMatch,
		Body: &change.Mark{Range: sel, Type: typ, Attrs: attrs}}
}

func findSel(s string) change.Selection { return change.Selection{Find: new(s)} }

// set_attribute changes an attribute the format writes, and the writer spells
// it into the code's native data, escaped for the tag, so the block holds the
// bytes it is written with. Every other request is refused with the reason.
func TestApplyBlock_SetAttribute(t *testing.T) {
	env := withFormat(person, htmlCaps())

	t.Run("a link target", func(t *testing.T) {
		b := guideBlock()
		before := sourceRev(b)
		res := apply(t, b, env, setAttr("", before, "1", "href", "https://new.example/handbook?a=1&b=\"2\""))
		requireApplied(t, res)
		assert.Equal(t, change.OpApplied, res[0].Status)
		open := b.SourceRuns()[1].PcOpen
		assert.Equal(t, `<a href="https://new.example/handbook?a=1&amp;b=&#34;2&#34;">`, open.Data)
		assert.Equal(t, "https://new.example/handbook?a=1&b=\"2\"", open.Attrs["href"])
		assert.Equal(t, "Read the <1>shop guide</1> before you <2>order</2>.", shape(b.SourceRuns()), "text and codes stay")
		assert.Equal(t, before, res[0].Before)
		assert.Equal(t, sourceRev(b), res[0].After)
		assert.NotEqual(t, before, res[0].After, "the revision covers attributes")
		assert.Equal(t, []change.Invalidation{{Edition: "nb", Reason: change.ReasonBasisMoved}}, res[0].Invalidates)
		assert.Equal(t, `<a href="https://old.example/guide">`, b.TargetRuns("nb")[1].PcOpen.Data, "the translation keeps its own link")
	})

	t.Run("the same value is unchanged", func(t *testing.T) {
		b := guideBlock()
		res := apply(t, b, env, setAttr("", sourceRev(b), "1", "href", "https://old.example/guide"))
		assert.Equal(t, change.OpUnchanged, res[0].Status)
	})

	t.Run("a derived edition's own link", func(t *testing.T) {
		b := guideBlock()
		res := apply(t, b, env, setAttr("nb", editionRev(b, "nb"), "1", "href", "https://example.no/handbok"))
		requireApplied(t, res)
		assert.Equal(t, `<a href="https://example.no/handbok">`, b.TargetRuns("nb")[1].PcOpen.Data)
		assert.Equal(t, `<a href="https://old.example/guide">`, b.SourceRuns()[1].PcOpen.Data)
		assert.Equal(t, model.TargetStatusTranslated, model.TargetStatus(translation(t, b, "nb").Status), "a person's edit to a translation")
	})

	refusals := []struct {
		name string
		op   func(b *model.Block) change.Op
		env  change.BlockEnv
		code change.Code
		want string
	}{
		{"an attribute the format does not write", func(b *model.Block) change.Op { return setAttr("", sourceRev(b), "1", "title", "Guide") },
			env, change.CodeUnsupported, "it writes href"},
		{"a code with no writable attribute", func(b *model.Block) change.Op { return setAttr("", sourceRev(b), "2", "class", "x") },
			env, change.CodeUnsupported, "writes no attribute of a fmt:bold code"},
		{"a code the edition does not hold", func(b *model.Block) change.Op { return setAttr("", sourceRev(b), "9", "href", "x") },
			env, change.CodeNotFound, `no code "9"`},
		{"a format that declares nothing", func(b *model.Block) change.Op { return setAttr("", sourceRev(b), "1", "href", "x") },
			person, change.CodeUnsupported, "writes no attribute"},
		{"a stale read", func(*model.Block) change.Op { return setAttr("", "r:0000000000000000", "1", "href", "x") },
			env, change.CodeStale, "changed after you read it"},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			b := guideBlock()
			was := model.RunsEditText(b.SourceRuns())
			err := requireRefused(t, apply(t, b, tc.env, tc.op(b))[0], tc.code)
			assert.Contains(t, err.Message, tc.want)
			assert.Equal(t, was, model.RunsEditText(b.SourceRuns()), "nothing is written")
		})
	}

	t.Run("a writer that changes the number of runs", func(t *testing.T) {
		b := guideBlock()
		was := model.RunsEditText(b.SourceRuns())
		caps := change.Capabilities{Format: "grows", Attrs: growingAttrWriter{},
			Declared: format.EditCapabilities{WritableAttrs: map[string][]string{"link:hyperlink": {"href"}}}}
		err := requireRefused(t, apply(t, b, withFormat(person, caps), setAttr("", sourceRev(b), "1", "href", "x"))[0], change.CodeUnsupported)
		assert.Contains(t, err.Message, "number of runs")
		assert.Equal(t, was, model.RunsEditText(b.SourceRuns()), "nothing is written")
	})

	t.Run("a writer outside the process spells it when it writes", func(t *testing.T) {
		b := guideBlock()
		plugin := withFormat(person, change.DeclaredCapabilities("okf_html", format.EditCapabilities{
			WritableAttrs: map[string][]string{"link:hyperlink": {"href"}},
		}))
		requireApplied(t, apply(t, b, plugin, setAttr("", sourceRev(b), "1", "href", "https://new.example/")))
		assert.Equal(t, "https://new.example/", b.SourceRuns()[1].PcOpen.Attrs["href"])
		assert.Equal(t, `<a href="https://old.example/guide">`, b.SourceRuns()[1].PcOpen.Data, "the native data is the plugin writer's to spell")
	})
}

// A format that declares every attribute writable, such as a plugin's HTML,
// is refused what would run code where the page is read: an event handler
// whatever its value, and a script URL in any attribute a browser follows,
// loads or submits to, however it is spelled. Another attribute, or a safe
// URL, applies.
func TestApplyBlock_SetAttributeRefusesScript(t *testing.T) {
	plugin := withFormat(person, change.DeclaredCapabilities("okf_html", format.EditCapabilities{
		WritableAttrs: map[string][]string{format.AnyCodeType: {format.AnyAttr}},
	}))
	for _, tc := range []struct{ name, value, field, want string }{
		{"onclick", "alert(1)", "name", "script"},
		{"onmouseover", "", "name", "script"},
		{"srcdoc", "<script>alert(1)</script>", "name", "script"},
		{"href", "&#106;avascript:alert(1)", "value", "javascript:"},
		{"href", `javascript\:alert(1)`, "value", "javascript:"},
		{"formaction", "javascript:alert(1)", "value", "javascript:"},
		{"action", "vbscript:msgbox(1)", "value", "vbscript:"},
		{"data", "data:text/html,<script>alert(1)</script>", "value", "data:"},
		{"poster", "javascript:alert(1)", "value", "javascript:"},
		{"background", "javascript:alert(1)", "value", "javascript:"},
		{"ping", "/p javascript:alert(1)", "value", "javascript:"},
		{"srcset", "a.png 1x, data:image/svg+xml,<svg/> 2x", "value", "image/svg+xml"},
	} {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			b := guideBlock()
			was := model.RunsEditText(b.SourceRuns())
			err := requireRefused(t, apply(t, b, plugin, setAttr("", sourceRev(b), "1", tc.name, tc.value))[0], change.CodeInvalid)
			assert.Equal(t, tc.field, err.Field)
			assert.Contains(t, err.Message, tc.want)
			assert.Equal(t, was, model.RunsEditText(b.SourceRuns()), "nothing is written")
		})
	}
	for _, tc := range []struct{ name, value string }{
		{"title", "javascript:alert(1)"},
		{"formaction", "https://example.com/send"},
		{"href", "https://example.com/a?b=1&amp;c=2"},
	} {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			b := guideBlock()
			requireApplied(t, apply(t, b, plugin, setAttr("", sourceRev(b), "1", tc.name, tc.value)))
			assert.Equal(t, tc.value, b.SourceRuns()[1].PcOpen.Attrs[tc.name])
		})
	}
	t.Run("a new link", func(t *testing.T) {
		caps := change.DeclaredCapabilities("okf_html", format.EditCapabilities{
			WritableAttrs: map[string][]string{format.AnyCodeType: {format.AnyAttr}},
			Synthesizes:   []string{"link:hyperlink"},
		})
		b := guideBlock()
		err := requireRefused(t, apply(t, b, withFormat(person, caps),
			mark("", sourceRev(b), findSel("Read"), "link:hyperlink", map[string]string{"href": "https://example.com/", "onclick": "alert(1)"}))[0], change.CodeInvalid)
		assert.Equal(t, "attrs", err.Field)
		assert.Contains(t, err.Message, "onclick")
	})
}

// mark wraps text in a new paired code of a vocabulary type the format
// writes. The code nests inside or around the codes at its edges, takes an
// id no edition of the block uses, and carries the native data the writer
// spells for it.
func TestApplyBlock_Mark(t *testing.T) {
	env := withFormat(person, htmlCaps())

	cases := []struct {
		name    string
		edition string
		sel     change.Selection
		typ     string
		attrs   map[string]string
		shape   string
		data    [2]string
	}{
		{"bold over plain text", "", findSel("before you"), "fmt:bold", nil,
			"Read the <1>shop guide</1> <3>before you</3> <2>order</2>.", [2]string{"<strong>", "</strong>"}},
		{"inside the link it fills", "", findSel("shop guide"), "fmt:italic", nil,
			"Read the <1><3>shop guide</3></1> before you <2>order</2>.", [2]string{"<em>", "</em>"}},
		{"around a code it covers", "", findSel("before you order"), "fmt:italic", nil,
			"Read the <1>shop guide</1> <3>before you <2>order</2></3>.", [2]string{"<em>", "</em>"}},
		{"a link with its target", "", findSel("Read"), "link:hyperlink", map[string]string{"href": "https://example.com/a&b"},
			"<3>Read</3> the <1>shop guide</1> before you <2>order</2>.", [2]string{`<a href="https://example.com/a&amp;b">`, "</a>"}},
		{"by code points", "", change.Selection{Start: new(9), End: new(13)}, "fmt:bold", nil,
			"Read the <1><3>shop</3> guide</1> before you <2>order</2>.", [2]string{"<strong>", "</strong>"}},
		{"a derived edition", "nb", findSel("før du"), "fmt:bold", nil,
			"Les <1>butikkguiden</1> <3>før du</3> <2>bestiller</2>.", [2]string{"<strong>", "</strong>"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := guideBlock()
			res := apply(t, b, env, mark(tc.edition, editionRev(b, tc.edition), tc.sel, tc.typ, tc.attrs))
			requireApplied(t, res)
			runs := b.SourceRuns()
			if tc.edition != "" {
				runs = b.TargetRuns(model.LocaleID(tc.edition))
			}
			assert.Equal(t, tc.shape, shape(runs))
			var open *model.PcOpenRun
			var closing *model.PcCloseRun
			for _, r := range runs {
				if r.PcOpen != nil && r.PcOpen.ID == "3" {
					open = r.PcOpen
				}
				if r.PcClose != nil && r.PcClose.ID == "3" {
					closing = r.PcClose
				}
			}
			require.NotNil(t, open)
			require.NotNil(t, closing)
			assert.Equal(t, tc.typ, open.Type)
			assert.Equal(t, tc.data[0], open.Data)
			assert.Equal(t, tc.data[1], closing.Data)
			assert.Equal(t, tc.attrs["href"], open.Attrs["href"])
			require.Len(t, res[0].Resolved, 1)
		})
	}

	refusals := []struct {
		name string
		sel  change.Selection
		typ  string
		env  change.BlockEnv
		code change.Code
		want string
	}{
		{"a range that crosses a code", findSel("guide before"), "fmt:bold", env, change.CodeGuard, "crosses the boundary"},
		{"a type the format does not write", findSel("order"), "fmt:underline", env, change.CodeUnsupported, "it writes fmt:bold, fmt:italic, link:hyperlink"},
		{"a link inside a link", findSel("shop"), "link:hyperlink", env, change.CodeUnsupported, "inside another link"},
		{"an empty range", change.Selection{Start: new(3), End: new(3)}, "fmt:bold", env, change.CodeInvalid, "selects no text"},
		{"a format that declares nothing", findSel("order"), "fmt:bold", person, change.CodeUnsupported, "writes no new inline code"},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			b := guideBlock()
			attrs := map[string]string(nil)
			if tc.typ == "link:hyperlink" {
				attrs = map[string]string{"href": "https://x.example/"}
			}
			was := model.RunsEditText(b.SourceRuns())
			err := requireRefused(t, apply(t, b, tc.env, mark("", sourceRev(b), tc.sel, tc.typ, attrs))[0], tc.code)
			assert.Contains(t, err.Message, tc.want)
			assert.Equal(t, was, model.RunsEditText(b.SourceRuns()))
		})
	}

	t.Run("an attribute the new code may not carry", func(t *testing.T) {
		b := guideBlock()
		err := requireRefused(t, apply(t, b, env, mark("", sourceRev(b), findSel("order"), "link:hyperlink",
			map[string]string{"href": "https://x.example/", "title": "x"}))[0], change.CodeUnsupported)
		assert.Equal(t, "attrs", err.Field)
	})

	t.Run("overlays keep their text", func(t *testing.T) {
		b := guideBlock()
		b.Overlays = []model.Overlay{{Type: model.OverlayTerm, Spans: []model.Span{{ID: "t1", Range: model.RangeAnchor(b.SourceRuns(), 9, 19)}}}}
		requireApplied(t, apply(t, b, env, mark("", sourceRev(b), findSel("Read the"), "fmt:bold", nil)))
		require.Len(t, b.Overlays, 1)
		s, e := b.Overlays[0].Spans[0].Range.TextSpan(b.SourceRuns())
		assert.Equal(t, "shop guide", string([]rune(model.RunsText(b.SourceRuns()))[s:e]))
	})

	t.Run("a writer outside the process spells it when it writes", func(t *testing.T) {
		b := guideBlock()
		plugin := withFormat(person, change.DeclaredCapabilities("okf_html", format.EditCapabilities{Synthesizes: []string{"fmt:bold"}}))
		requireApplied(t, apply(t, b, plugin, mark("", sourceRev(b), findSel("Read"), "fmt:bold", nil)))
		assert.Equal(t, "<3>Read</3> the <1>shop guide</1> before you <2>order</2>.", shape(b.SourceRuns()))
		assert.Empty(t, b.SourceRuns()[0].PcOpen.Data)
		assert.Equal(t, "[B]", b.SourceRuns()[0].PcOpen.Disp, "the vocabulary gives the code its display")
	})
}

// A new paired code in a runs payload is written where the format declares
// its type, as mark writes it; any other new code is refused as before.
func TestApplyBlock_NewCodesInRuns(t *testing.T) {
	env := withFormat(person, htmlCaps())
	payload := func(open model.PcOpenRun) []model.Run {
		r := guideRuns()
		for i := range r {
			r[i] = stripData(r[i])
		}
		return append(r[:len(r)-1:len(r)-1], model.PcOpenR(open), model.TextR("."), model.PcCloseR(model.PcCloseRun{ID: open.ID, Type: open.Type}))
	}

	b := guideBlock()
	requireApplied(t, apply(t, b, env, setRuns("", sourceRev(b), payload(model.PcOpenRun{ID: "7", Type: "fmt:bold"}))))
	assert.Equal(t, "Read the <1>shop guide</1> before you <2>order</2><7>.</7>", shape(b.SourceRuns()))
	assert.Equal(t, "<strong>", b.SourceRuns()[8].PcOpen.Data)
	assert.Equal(t, "</strong>", b.SourceRuns()[10].PcClose.Data)
	assert.Equal(t, `<a href="https://old.example/guide">`, b.SourceRuns()[1].PcOpen.Data, "held codes keep their native form")

	b = guideBlock()
	err := requireRefused(t, apply(t, b, env, setRuns("", sourceRev(b), payload(model.PcOpenRun{ID: "7", Type: "fmt:underline"})))[0], change.CodeUnsupported)
	assert.Equal(t, "synthesize:fmt:underline", err.Capability)

	b = guideBlock()
	unclosed := append(guideRuns(), model.PcOpenR(model.PcOpenRun{ID: "7", Type: "fmt:bold"}), model.TextR("!"))
	requireRefused(t, apply(t, b, env, setRuns("", sourceRev(b), unclosed))[0], change.CodeInvalid)

	b = guideBlock()
	err = requireRefused(t, apply(t, b, env, setRuns("", sourceRev(b),
		payload(model.PcOpenRun{ID: "7", Type: "link:hyperlink", Attrs: map[string]string{"href": "x", "title": "y"}})))[0], change.CodeUnsupported)
	assert.Contains(t, err.Message, "title")

	// Each plural form is a sequence of its own, so a new code opens once
	// in each; twice in one sequence is refused.
	bold := func(text string) []model.Run {
		return []model.Run{model.PcOpenR(model.PcOpenRun{ID: "9", Type: "fmt:bold"}), model.TextR(text), model.PcCloseR(model.PcCloseRun{ID: "9", Type: "fmt:bold"})}
	}
	plural := []model.Run{model.TextR("You have "), {Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne: bold("one item"), model.PluralOther: bold("many items"),
	}}}, model.TextR(".")}
	b = model.NewBlock("b", "You have items.")
	requireApplied(t, apply(t, b, env, setRuns("", sourceRev(b), plural)))
	for _, form := range []model.PluralForm{model.PluralOne, model.PluralOther} {
		runs := b.SourceRuns()[1].Plural.Forms[form]
		assert.Equal(t, "<strong>", runs[0].PcOpen.Data, "the %s form holds the new code", form)
		assert.Equal(t, "</strong>", runs[2].PcClose.Data)
	}

	b = model.NewBlock("b", "Twice.")
	err = requireRefused(t, apply(t, b, env, setRuns("", sourceRev(b), append(bold("a"), bold("b")...)))[0], change.CodeInvalid)
	assert.Contains(t, err.Message, "opens twice")
}

// stripData returns a code as a wire payload names it: by id, with no data.
func stripData(r model.Run) model.Run {
	switch {
	case r.PcOpen != nil:
		return model.PcOpenR(model.PcOpenRun{ID: r.PcOpen.ID})
	case r.PcClose != nil:
		return model.PcCloseR(model.PcCloseRun{ID: r.PcClose.ID})
	}
	return r
}

func TestWriterCapabilities(t *testing.T) {
	caps := htmlCaps()
	assert.Equal(t, "html", caps.Format)
	assert.NotNil(t, caps.Attrs)
	assert.NotNil(t, caps.Codes)
	assert.Equal(t, []string{"href"}, caps.Declared.Writable("link:hyperlink"))
	assert.Equal(t, []string{"fmt:bold", "fmt:italic", "link:hyperlink"}, caps.Declared.Synthesizes)
}

// FormatOps builds the describe table from what the round trip carries and
// what the writer declares; what neither gives is null.
func TestFormatOps(t *testing.T) {
	base := []change.Kind{change.KindSetContent, change.KindReplaceText, change.KindRemoveEdition, change.KindAnnotate, change.KindUnannotate}
	got, err := json.Marshal(change.FormatOps(change.FormatDecl{
		Format: "html", Base: base, Edit: htmlCaps().Declared,
	}))
	require.NoError(t, err)
	assert.JSONEq(t, `{
	  "format": "html",
	  "editions": "one-per-file",
	  "ops": {
	    "set_content": {"forms": ["text", "runs"], "new_codes": ["fmt:bold", "fmt:italic", "link:hyperlink"]},
	    "replace_text": {},
	    "set_attribute": {"link:hyperlink": ["href"], "media:image": ["src"]},
	    "mark": {"types": ["fmt:bold", "fmt:italic", "link:hyperlink"]},
	    "remove_edition": {},
	    "annotate": {"inline": []},
	    "unannotate": {},
	    "insert_block": null,
	    "delete_block": null
	  },
	  "native": []
	}`, string(got))

	got, err = json.Marshal(change.FormatOps(change.FormatDecl{
		Format: "xliff2", Editions: change.EditionsInFile, Base: []change.Kind{change.KindReplaceText},
		InlineAnnotations: []string{"term"},
		Edit:              format.EditCapabilities{Structural: []string{"insert_block"}, NativeOps: []format.NativeOp{{Name: "x.op"}}},
	}))
	require.NoError(t, err)
	assert.JSONEq(t, `{
	  "format": "xliff2",
	  "editions": "in-file",
	  "ops": {
	    "set_content": null, "replace_text": {}, "set_attribute": null, "mark": null,
	    "remove_edition": null, "annotate": null, "unannotate": null,
	    "insert_block": {}, "delete_block": null
	  },
	  "native": [{"name": "x.op"}]
	}`, string(got))
}
