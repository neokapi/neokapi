package formats_test

import (
	"bytes"
	"context"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// The capability cells of the operations matrix. A writer declares the
// attributes of a code it can write (format.AttrWriter) and the vocabulary
// types it can write as a new code (format.CodeSynthesizer); the change
// service applies set_attribute, mark and a new code in a runs payload only
// where a format declares them. Each cell here reads a fixture, applies one
// operation through change.ApplyBlock with the capabilities the format's
// writer declares (change.WriterCapabilities, as the change service does),
// writes the document through the same writer, and asserts
//
//   - the bytes: the document is the input with the cell's one change, so
//     every byte the operation did not touch is unchanged, and the value or
//     the new code is spelled and escaped as the cell says;
//   - the read back: the edited block reads back with the content the
//     operation produced, its new or changed code included, and every other
//     block as it was read.
//
// A refusal cell asserts the operation is refused with its reason and the
// document is written unchanged. TestCapabilityMatrixCoversEveryDeclaration
// fails when a built-in writer declares a capability no cell here proves.

// capCell is one capability cell.
type capCell struct {
	// name tells the cells of a fixture apart.
	name string
	// block picks the block the operation addresses: the one block whose edit
	// text contains it.
	block string
	// attrType is the code type an attribute cell's declaration names: the
	// code's type, or format.AnyCodeType where the format declares every type.
	attrType string
	// attr is a set_attribute cell's operation; mark a mark cell's. A mark
	// cell is also driven as a set_content whose runs name the new code by
	// type and attributes.
	attr *change.SetAttribute
	mark *capMark
	// from and to are the cell's one change to the document's bytes: from
	// occurs once in the input and the output holds to in its place.
	from, to string
	// refused, when set, is the error the operation is refused with, and
	// reason a word of its message.
	refused change.Code
	reason  string
}

// capMark is a mark cell's operation.
type capMark struct {
	find  string
	typ   string
	attrs map[string]string
}

// claims are the declarations a cell proves.
func (c capCell) claims() []string {
	switch {
	case c.refused != "":
		return nil
	case c.attr != nil && c.attrType == format.AnyCodeType:
		return []string{"set_attribute " + format.AnyCodeType + "." + format.AnyAttr}
	case c.attr != nil:
		return []string{"set_attribute " + c.attrType + "." + c.attr.Name}
	case c.mark != nil:
		return []string{"mark " + c.mark.typ, "new code " + c.mark.typ}
	}
	return nil
}

// capFixture is a fixture and its capability cells.
type capFixture struct {
	fx    opsFixture
	cells []capCell
}

// capabilityMatrix is the table. A cell's attrType is the declaration key it
// proves: the code type, or format.AnyCodeType where the format declares
// every type.
func capabilityMatrix() []capFixture {
	attr := func(code, name, value string) *change.SetAttribute {
		return &change.SetAttribute{Code: code, Name: name, Value: value}
	}
	return []capFixture{
		{
			fx: opsFixture{format: "html", template: "capabilities/html.html.tmpl"},
			cells: []capCell{
				{name: "href", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", `https://example.com/new?a=1&b="2"`),
					from: `href="https://example.com/guide"`, to: `href="https://example.com/new?a=1&amp;b=&#34;2&#34;"`},
				{name: "href in single quotes", block: "Read the", attrType: "link:hyperlink",
					attr: attr("1", "href", "https://example.com/o'k"),
					from: `href='https://example.com/order'`, to: `href='https://example.com/o&#39;k'`},
				{name: "src", block: "See ", attrType: "media:image",
					attr: attr("1", "src", "images/fresh herbs.png"),
					from: `src="herbs.png"`, to: `src="images/fresh herbs.png"`},
				{name: "title is a block of its own", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "title", "Guide"), refused: change.CodeUnsupported, reason: "it writes href"},
				{name: "bold", block: "We use the", mark: &capMark{find: "spices", typ: "fmt:bold"},
					from: `&amp; spices.`, to: `&amp; <strong>spices</strong>.`},
				{name: "italic", block: "Herbs we", mark: &capMark{find: "daily", typ: "fmt:italic"},
					from: `</strong> daily</li>`, to: `</strong> <em>daily</em></li>`},
				{name: "link", block: "See ", mark: &capMark{find: "the herbs we grow", typ: "link:hyperlink", attrs: map[string]string{"href": "https://example.com/herbs?x=1&y=2"}},
					from: `for the herbs we grow.`, to: `for <a href="https://example.com/herbs?x=1&amp;y=2">the herbs we grow</a>.`},
				{name: "a link inside a link", block: "We use the", mark: &capMark{find: "ingredients", typ: "link:hyperlink", attrs: map[string]string{"href": "https://x.example/"}},
					refused: change.CodeUnsupported, reason: "inside another link"},
				{name: "the document title holds no markup", block: "Fresh ingredients", mark: &capMark{find: "Fresh", typ: "fmt:bold"},
					refused: change.CodeUnsupported, reason: "holds no markup"},
				// A paragraph wrapped over lines, with a quote and a bare
				// ampersand: a change to its codes keeps the line breaks,
				// the indentation and the characters as the document
				// spells them.
				{name: "href in a wrapped paragraph", block: "Order the", attrType: "link:hyperlink",
					attr: attr("1", "href", "https://example.com/box?week=2"),
					from: `href="https://example.com/box"`, to: `href="https://example.com/box?week=2"`},
				{name: "bold in a wrapped paragraph", block: "Order the", mark: &capMark{find: "Friday", typ: "fmt:bold"},
					from: `before Friday,`, to: `before <strong>Friday</strong>,`},
				{name: "a link inside a link the markup holds", block: "Big title", mark: &capMark{find: "Big", typ: "link:hyperlink", attrs: map[string]string{"href": "https://x.example/"}},
					refused: change.CodeUnsupported, reason: "sits inside an <a> element"},
				// A link target that runs code where the page is read is
				// refused, however it is written or added. A link the
				// document already holds is read, kept and replaced.
				{name: "a javascript: href", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", "javascript:alert(1)"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "a vbscript: href", block: "Read the", attrType: "link:hyperlink",
					attr: attr("1", "href", " VBScript:msgbox(1)"), refused: change.CodeInvalid, reason: "vbscript:"},
				{name: "a data: src that is not an image", block: "See ", attrType: "media:image",
					attr: attr("1", "src", "data:text/html,<script>alert(1)</script>"), refused: change.CodeInvalid, reason: "data:"},
				{name: "an image as a data: src", block: "See ", attrType: "media:image",
					attr: attr("1", "src", "data:image/png;base64,iVBORw0KGgo="),
					from: `src="herbs.png"`, to: `src="data:image/png;base64,iVBORw0KGgo="`},
				{name: "a new link to a javascript: URL", block: "Herbs we", mark: &capMark{find: "daily", typ: "link:hyperlink", attrs: map[string]string{"href": "java\tscript:alert(1)"}},
					refused: change.CodeInvalid, reason: "javascript:"},
				{name: "bold beside a javascript: link the document holds", block: "opens here", mark: &capMark{find: "opens", typ: "fmt:bold"},
					from: `opens here`, to: `<strong>opens</strong> here`},
				{name: "an href in place of a javascript: one", block: "opens here", attrType: "link:hyperlink",
					attr: attr("1", "href", "https://example.com/menu"),
					from: `href="javascript:openMenu()"`, to: `href="https://example.com/menu"`},
				// A value is judged as a reader of the document decodes it,
				// and an SVG image can carry script.
				{name: "a javascript: href spelled with a character reference", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", "&#106;avascript:alert(1)"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "an SVG image as a data: href", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", "data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+"), refused: change.CodeInvalid, reason: "image/svg+xml"},
				{name: "a new link to a javascript: URL spelled with a character reference", block: "Herbs we", mark: &capMark{find: "daily", typ: "link:hyperlink", attrs: map[string]string{"href": "javascript&colon;alert(1)"}},
					refused: change.CodeInvalid, reason: "javascript:"},
			},
		},
		{
			fx: opsFixture{format: "markdown", template: "capabilities/markdown.md.tmpl"},
			cells: []capCell{
				{name: "href", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", "https://example.com/new path?a=(1)"),
					from: `](https://example.com/guide)`, to: `](<https://example.com/new path?a=(1)>)`},
				{name: "href of a titled link", block: "Read the", attrType: "link:hyperlink",
					attr: attr("1", "href", "https://example.com/checkout"),
					from: `(<https://example.com/order page> "Ordering")`, to: `(<https://example.com/checkout> "Ordering")`},
				{name: "src", block: "Fresh herbs", attrType: "media:image",
					attr: attr("1", "src", "img/herbs&co.png"),
					from: `](herbs.png)`, to: `](img/herbs&co.png)`},
				{name: "a reference link", block: "See the", attrType: "link:hyperlink",
					attr: attr("1", "href", "https://example.com/x"), refused: change.CodeUnsupported, reason: "definition"},
				{name: "an autolink", block: "See the", attrType: "link:hyperlink",
					attr: attr("2", "href", "https://example.com/x"), refused: change.CodeUnsupported, reason: "autolink"},
				{name: "bold", block: "Herbs we pick", mark: &capMark{find: "pick", typ: "fmt:bold"},
					from: `Herbs we pick daily.`, to: `Herbs we **pick** daily.`},
				{name: "italic", block: "We use the", mark: &capMark{find: "daily", typ: "fmt:italic"},
					from: `) daily.`, to: `) *daily*.`},
				{name: "link", block: "Herbs we pick", mark: &capMark{find: "Herbs", typ: "link:hyperlink", attrs: map[string]string{"href": "https://example.com/herbs"}},
					from: `Herbs we pick daily.`, to: `[Herbs](https://example.com/herbs) we pick daily.`},
				{name: "emphasis that starts with a space", block: "Herbs we pick", mark: &capMark{find: " pick", typ: "fmt:bold"},
					refused: change.CodeUnsupported, reason: "space"},
				{name: "a link inside a link", block: "We use the", mark: &capMark{find: "ingredients", typ: "link:hyperlink", attrs: map[string]string{"href": "https://x.example/"}},
					refused: change.CodeUnsupported, reason: "another link"},
				// Characters beside the new markup that change how it
				// reads: a '!' makes a link an image, a backslash escapes
				// a delimiter, and a '*' the text holds pairs with a new
				// one before the new pair can.
				{name: "a link after a '!'", block: "Wow!great", mark: &capMark{find: "great", typ: "link:hyperlink", attrs: map[string]string{"href": "https://x.example/"}},
					refused: change.CodeUnsupported, reason: "image"},
				{name: "a backslash before the range", block: "Wow!great", mark: &capMark{find: "spices", typ: "fmt:bold"},
					refused: change.CodeUnsupported, reason: "backslash"},
				{name: "a range ending in a backslash", block: "Wow!great", mark: &capMark{find: `C:\spices\`, typ: "fmt:italic"},
					refused: change.CodeUnsupported, reason: "backslash"},
				{name: "a delimiter the text holds pairs first", block: "Wow!great", mark: &capMark{find: "stars", typ: "fmt:italic"},
					refused: change.CodeUnsupported, reason: "would not read"},
				{name: "a javascript: href", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", "javascript:alert(1)"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "a data: src that is not an image", block: "Fresh herbs", attrType: "media:image",
					attr: attr("1", "src", "data:text/html,x"), refused: change.CodeInvalid, reason: "data:"},
				{name: "a new link to a vbscript: URL", block: "Herbs we pick", mark: &capMark{find: "Herbs", typ: "link:hyperlink", attrs: map[string]string{"href": "vbscript:msgbox(1)"}},
					refused: change.CodeInvalid, reason: "vbscript:"},
				{name: "bold beside a javascript: link the document holds", block: "opens here", mark: &capMark{find: "opens", typ: "fmt:bold"},
					from: `opens here.`, to: `**opens** here.`},
				{name: "an href in place of a javascript: one", block: "opens here", attrType: "link:hyperlink",
					attr: attr("1", "href", "https://example.com/menu"),
					from: `](javascript:openMenu())`, to: `](https://example.com/menu)`},
				// CommonMark resolves backslash escapes and character
				// references in a destination, and a renderer passes the
				// references to the browser, which decodes them: each
				// spelling below is a javascript: link once rendered.
				{name: "a javascript: href with a backslash escape", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", `javascript\:alert(1)`), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "a javascript: href spelled with a decimal reference", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", "&#106;avascript:alert(1)"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "a javascript: href spelled with a hex and a named reference", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", "&#x6A;avascript&colon;alert(1)"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "a javascript: href with an escaped reference", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", `\&#106;avascript:alert(1)`), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "an SVG image as a data: href", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", "data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+"), refused: change.CodeInvalid, reason: "image/svg+xml"},
				{name: "a new link to a javascript: URL spelled with a reference", block: "Herbs we pick", mark: &capMark{find: "Herbs", typ: "link:hyperlink", attrs: map[string]string{"href": "javascript&#58;alert(1)"}},
					refused: change.CodeInvalid, reason: "javascript:"},
				{name: "an href with an escape and a reference that stays a URL", block: "We use the", attrType: "link:hyperlink",
					attr: attr("2", "href", `https://example.com/a\_b?x=1&amp;y=2`),
					from: `](https://example.com/guide)`, to: `](https://example.com/a\_b?x=1&amp;y=2)`},
			},
		},
		{
			fx: opsFixture{format: "xml", template: "capabilities/xml.xml.tmpl"},
			cells: []capCell{
				{name: "an attribute", block: "We use the", attrType: format.AnyCodeType,
					attr: attr("2", "href", "https://example.com/new?a=1&b=<2>"),
					from: `href="https://example.com/guide"`, to: `href="https://example.com/new?a=1&amp;b=&lt;2>"`},
				{name: "an attribute in single quotes", block: "We use the", attrType: format.AnyCodeType,
					attr: attr("2", "rel", "it's"),
					from: `rel='help'`, to: `rel='it&apos;s'`},
				{name: "an attribute the element does not spell", block: "We use the", attrType: format.AnyCodeType,
					attr: attr("2", "title", "Guide"), refused: change.CodeUnsupported, reason: "spells no title"},
				{name: "a namespace declaration", block: "See ", attrType: format.AnyCodeType,
					attr: attr("3", "xmlns:x", "urn:y"), refused: change.CodeUnsupported, reason: "namespace declaration"},
				// Attributes the reader reads as instructions: its:translate
				// decides whether the text is translatable, xml:lang its
				// language.
				{name: "an ITS attribute", block: "Pick ", attrType: format.AnyCodeType,
					attr: attr("4", "its:translate", "no"), refused: change.CodeUnsupported, reason: "instruction"},
				{name: "an xml: attribute", block: "Pick ", attrType: format.AnyCodeType,
					attr: attr("4", "xml:lang", "fr"), refused: change.CodeUnsupported, reason: "instruction"},
				{name: "a javascript: href", block: "We use the", attrType: format.AnyCodeType,
					attr: attr("2", "href", "javascript:alert(1)"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "another attribute of a javascript: link the document holds", block: "opens here", attrType: format.AnyCodeType,
					attr: attr("5", "rel", "menu"),
					from: `rel='nav'`, to: `rel='menu'`},
				// Every attribute the element spells is writable, so every
				// attribute that holds script, or a URL a reader follows,
				// loads or submits to, is checked.
				{name: "an event handler", block: "Tap ", attrType: format.AnyCodeType,
					attr: attr("6", "onclick", "alert(document.cookie)"), refused: change.CodeInvalid, reason: "script"},
				{name: "a javascript: href spelled with a character reference", block: "Tap ", attrType: format.AnyCodeType,
					attr: attr("6", "href", "&#106;avascript:alert(1)"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "a javascript: formaction", block: "Send ", attrType: format.AnyCodeType,
					attr: attr("7", "formaction", "javascript:alert(1)"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "a formaction", block: "Send ", attrType: format.AnyCodeType,
					attr: attr("7", "formaction", "/send"),
					from: `formaction="https://example.com/send"`, to: `formaction="/send"`},
				{name: "a javascript: object data", block: "Watch ", attrType: format.AnyCodeType,
					attr: attr("8", "data", "javascript:alert(1)"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "a javascript: cite", block: "Quote ", attrType: format.AnyCodeType,
					attr: attr("9", "cite", " JavaScript:alert(1)"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "a javascript: srcset candidate", block: "Shown ", attrType: format.AnyCodeType,
					attr: attr("10", "srcset", "small.png 1x, javascript:alert(1) 2x"), refused: change.CodeInvalid, reason: "javascript:"},
				{name: "an SVG image as a data: xlink:href", block: "Draw ", attrType: format.AnyCodeType,
					attr: attr("11", "xlink:href", "data:image/svg+xml,<svg/>"), refused: change.CodeInvalid, reason: "image/svg+xml"},
				{name: "an SVG animation pointed at an href", block: "Fade ", attrType: format.AnyCodeType,
					attr: attr("12", "attributeName", "href"), refused: change.CodeInvalid, reason: "to attribute"},
				{name: "a javascript: SVG animation value", block: "Fade ", attrType: format.AnyCodeType,
					attr: attr("12", "to", "vbscript:msgbox(1)"), refused: change.CodeInvalid, reason: "vbscript:"},
			},
		},
	}
}

// capOutcome is what driving one cell produced.
type capOutcome struct {
	out []byte
	// result is the operation's result on the addressed block.
	result change.OpResult
	// edited is the addressed block's source after the operation, and at its
	// place among the blocks read; read is the edit text of every block as
	// read, in order.
	edited []model.Run
	at     int
	read   []string
}

// drive reads input, applies ops to the cell's block with the capabilities
// the format's writer declares, and writes the document through that writer.
func (fx opsFixture) drive(t *testing.T, input []byte, pick string, ops func(b *model.Block) []change.Op) capOutcome {
	t.Helper()
	ctx := context.Background()
	reader, writer := fx.newPair(t)
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	require.NotNil(t, store, "%s has no skeleton pair", fx.format)
	defer store.Close()
	env := change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}, Format: change.WriterCapabilities(string(fx.format), writer)}

	require.NoError(t, reader.Open(ctx, &model.RawDocument{
		URI: "capabilities." + string(fx.format), SourceLocale: model.LocaleEnglish,
		Reader: io.NopCloser(bytes.NewReader(input)),
	}))
	res := capOutcome{at: -1}
	picked := 0
	var parts []*model.Part
	for r := range reader.Read(ctx) {
		require.NoError(t, r.Error)
		if r.Part == nil {
			continue
		}
		if b, ok := r.Part.Resource.(*model.Block); ok {
			res.read = append(res.read, model.RunsEditText(b.Source))
			if strings.Contains(model.RunsEditText(b.Source), pick) {
				picked++
				res.at = len(res.read) - 1
				results := change.ApplyBlock(b, ops(b), env)
				res.result = results[len(results)-1]
				res.edited = slices.Clone(b.Source)
			}
		}
		parts = append(parts, r.Part)
	}
	require.NoError(t, reader.Close())
	require.Equal(t, 1, picked, "%q picks %d blocks, not one", pick, picked)

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent(input)
	}
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	require.NoError(t, writer.Write(ctx, ch))
	require.NoError(t, writer.Close())
	res.out = buf.Bytes()
	return res
}

// capDriver is one way a cell's operation reaches the applier.
type capDriver struct {
	name string
	ops  func(t *testing.T, c capCell, b *model.Block, caps change.Capabilities) []change.Op
}

// capDrivers drives an attribute cell with set_attribute, and a mark cell with
// mark and with set_content naming the new code in a runs payload.
func capDrivers(c capCell) []capDriver {
	at := change.Ref{Doc: "capabilities", Block: "b"}
	if c.attr != nil {
		return []capDriver{{name: "set_attribute", ops: func(_ *testing.T, c capCell, b *model.Block, _ change.Capabilities) []change.Op {
			return []change.Op{{Kind: change.KindSetAttribute, At: at, IfMatch: model.EditionRevision(b, model.EditionKey{}), Body: c.attr}}
		}}}
	}
	markOp := func(b *model.Block) change.Op {
		return change.Op{Kind: change.KindMark, At: at, IfMatch: model.EditionRevision(b, model.EditionKey{}),
			Body: &change.Mark{Range: change.Selection{Find: &c.mark.find}, Type: c.mark.typ, Attrs: c.mark.attrs}}
	}
	drivers := []capDriver{{name: "mark", ops: func(_ *testing.T, _ capCell, b *model.Block, _ change.Capabilities) []change.Op {
		return []change.Op{markOp(b)}
	}}}
	if c.refused != "" {
		return drivers
	}
	return append(drivers,
		capDriver{name: "set_content", ops: func(t *testing.T, c capCell, b *model.Block, caps change.Capabilities) []change.Op {
			// The payload is the block's runs with the new code where mark
			// puts it, every code named by id, type and attributes, and
			// none carrying native data.
			probe := model.NewRunsBlock(b.ID, slices.Clone(b.Source))
			probe.Type, probe.IsReferent = b.Type, b.IsReferent
			res := change.ApplyBlock(probe, []change.Op{markOp(probe)}, change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}, Format: caps})
			require.Equal(t, change.OpApplied, res[0].Status, "%+v", res[0].Error)
			runs := make([]model.Run, len(probe.Source))
			for i, r := range probe.Source {
				runs[i] = wirePayload(r)
			}
			return []change.Op{{Kind: change.KindSetContent, At: at, IfMatch: model.EditionRevision(b, model.EditionKey{}),
				Body: &change.SetContent{Runs: runs}}}
		}},
	)
}

// wirePayload is a run as a wire payload carries it: a code with its id, type
// and attributes and no native data.
func wirePayload(r model.Run) model.Run {
	switch {
	case r.PcOpen != nil:
		return model.PcOpenR(model.PcOpenRun{ID: r.PcOpen.ID, Type: r.PcOpen.Type, Attrs: r.PcOpen.Attrs})
	case r.PcClose != nil:
		return model.PcCloseR(model.PcCloseRun{ID: r.PcClose.ID, Type: r.PcClose.Type})
	case r.Ph != nil:
		return model.PhR(model.PlaceholderRun{ID: r.Ph.ID, Type: r.Ph.Type, Attrs: r.Ph.Attrs})
	}
	return r
}

// TestCapabilityMatrix drives every capability cell.
func TestCapabilityMatrix(t *testing.T) {
	for _, cf := range capabilityMatrix() {
		fx := cf.fx
		for _, c := range cf.cells {
			for _, d := range capDrivers(c) {
				t.Run(fx.id()+"/"+c.name+"/"+d.name, func(t *testing.T) {
					doc := fx.render(t, nil)
					input := string(doc.input)
					_, writer := fx.newPair(t)
					caps := change.WriterCapabilities(string(fx.format), writer)
					res := fx.drive(t, doc.input, c.block, func(b *model.Block) []change.Op { return d.ops(t, c, b, caps) })

					if c.refused != "" {
						require.Equal(t, change.OpRefused, res.result.Status, "%s is declared refused (%s)", c.name, c.reason)
						require.NotNil(t, res.result.Error)
						assert.Equal(t, c.refused, res.result.Error.Code, res.result.Error.Message)
						assert.Contains(t, res.result.Error.Message, c.reason)
						assert.Equal(t, input, string(res.out), "a refused operation leaves the document as it was")
						return
					}
					require.Equal(t, change.OpApplied, res.result.Status, "%+v", res.result.Error)

					// Bytes: the one change, and nothing else.
					require.Equal(t, 1, strings.Count(input, c.from), "the cell's from occurs once in the input: %q", c.from)
					assert.Equal(t, strings.Replace(input, c.from, c.to, 1), string(res.out),
						"the written bytes differ from the input outside the operation")

					// Read back: the edited block as the operation left it,
					// every other block as it was read.
					back := fx.readEditable(t, res.out, "")
					require.Len(t, back, len(res.read), "the written document holds the blocks it was read with")
					for i, b := range back {
						if i == res.at {
							assert.Equal(t, string(model.CanonicalRunsJSON(renumberCodes(res.edited))), string(model.CanonicalRunsJSON(renumberCodes(b.Source))),
								"the edited block reads back with the content the operation produced")
							continue
						}
						assert.Equal(t, res.read[i], model.RunsEditText(b.Source), "block %s reads back as read", b.ID)
					}
				})
			}
		}
	}
}

// renumberCodes gives the codes of runs ids in the order they first appear,
// as a reader numbers them, so content an operation produced compares with
// the same content read back.
func renumberCodes(runs []model.Run) []model.Run {
	ids := map[string]string{}
	id := func(old string) string {
		if n, ok := ids[old]; ok {
			return n
		}
		ids[old] = strconv.Itoa(len(ids) + 1)
		return ids[old]
	}
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		switch {
		case r.PcOpen != nil:
			c := *r.PcOpen
			c.ID = id(c.ID)
			out[i] = model.Run{PcOpen: &c}
		case r.PcClose != nil:
			c := *r.PcClose
			c.ID = id(c.ID)
			out[i] = model.Run{PcClose: &c}
		case r.Ph != nil:
			c := *r.Ph
			c.ID = id(c.ID)
			out[i] = model.Run{Ph: &c}
		default:
			out[i] = r
		}
	}
	return out
}

// wildcardClaim is the declaration of every attribute of every code type.
var wildcardClaim = "set_attribute " + format.AnyCodeType + "." + format.AnyAttr

// TestCapabilityMatrixCoversEveryDeclaration keeps the declarations honest:
// every attribute and vocabulary type a built-in writer declares has a cell
// that proves it, every structural or native operation a writer declares has
// one too, and every cell proves something its format declares. A wildcard
// attribute declaration claims more than one cell can show, so it counts as
// proven only with passing cells on two attributes and a cell refusing one.
func TestCapabilityMatrixCoversEveryDeclaration(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	proven := map[registry.FormatID]map[string]bool{}
	for _, cf := range capabilityMatrix() {
		info := reg.FormatInfo(cf.fx.format)
		require.NotNil(t, info, "%s is not registered", cf.fx.format)
		assert.True(t, strings.HasSuffix(cf.fx.template, ".tmpl"), "%s: a template path ends in .tmpl", cf.fx.format)
		if proven[cf.fx.format] == nil {
			proven[cf.fx.format] = map[string]bool{}
		}
		wildcardAttrs, attrRefusals := map[string]bool{}, 0
		for _, c := range cf.cells {
			assert.NotEqual(t, c.attr == nil, c.mark == nil, "%s/%s: a cell has one operation", cf.fx.format, c.name)
			if c.refused != "" {
				assert.NotEmpty(t, c.reason, "%s/%s: a refusal names its reason", cf.fx.format, c.name)
				if c.attr != nil {
					attrRefusals++
				}
				continue
			}
			assert.NotEmpty(t, c.from, "%s/%s: a cell names the bytes it changes", cf.fx.format, c.name)
			for _, claim := range c.claims() {
				proven[cf.fx.format][claim] = true
			}
			if c.attr != nil && c.attrType == format.AnyCodeType {
				wildcardAttrs[c.attr.Name] = true
			}
		}
		if proven[cf.fx.format][wildcardClaim] {
			assert.True(t, len(wildcardAttrs) >= 2 && attrRefusals > 0,
				"%s: a wildcard attribute declaration needs passing cells on two attributes and a refusal cell", cf.fx.format)
		}
		declared := declaredClaims(info.EditCapabilities)
		for claim := range proven[cf.fx.format] {
			assert.True(t, declared[claim], "%s: a cell proves %q, which the format does not declare", cf.fx.format, claim)
		}
	}

	for _, info := range reg.FormatInfos() {
		var claims []string
		for claim := range declaredClaims(info.EditCapabilities) {
			claims = append(claims, claim)
		}
		sort.Strings(claims)
		for _, claim := range claims {
			assert.True(t, proven[info.Name][claim], "%s declares %q and no capability cell proves it", info.Name, claim)
		}
	}
}

// declaredClaims lists what a writer declares, in the claims cells make.
func declaredClaims(c format.EditCapabilities) map[string]bool {
	out := map[string]bool{}
	for typ, attrs := range c.WritableAttrs {
		for _, a := range attrs {
			out["set_attribute "+typ+"."+a] = true
		}
	}
	for _, typ := range c.Synthesizes {
		out["mark "+typ] = true
		out["new code "+typ] = true
	}
	for _, op := range c.Structural {
		out[op] = true
	}
	for _, op := range c.NativeOps {
		out["native "+op.Name] = true
	}
	return out
}
