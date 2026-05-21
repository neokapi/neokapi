package ttx_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ttx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// translatingRoundtrip reads snippet with a skeleton store, pseudo-translates
// every Block (target = "ZZ" + source), and writes it back. Returns the
// reconstructed TTX. Source EN-US, target FR. Used to exercise the unsegmented
// (no-<Tu>) path that wraps free text in fresh <Tu> elements.
func translatingRoundtrip(t *testing.T, snippet string, mode ttx.SegmentMode) string {
	t.Helper()
	ctx := context.Background()

	reader := ttx.NewReader()
	reader.Config().(*ttx.Config).SegmentMode = mode
	writer := ttx.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := testutil.RawDocFromString(snippet, "EN-US")
	doc.TargetLocale = "FR"
	require.NoError(t, reader.Open(ctx, doc))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			b.SetTargetText("FR", "ZZ"+b.SourceText())
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale("FR")
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// assertWellFormedXML fails the test if s is not parseable XML. Guards against
// regressions like an unescaped "]]>" in CharData (XML 1.0 §2.4).
func assertWellFormedXML(t *testing.T, s string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(s))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		require.NoError(t, err, "output is not well-formed XML:\n%s", s)
	}
}

// frontMatterTTX is an unsegmented (no-<Tu>) TTX with a full FrontMatter block
// and a single translatable paragraph wrapped in a <df>+external-<ut> pair —
// the shape of Issue_164.ttx and Test02_noseg.html.ttx.
const frontMatterTTX = `<?xml version='1.0'?>` + "\n" +
	`<TRADOStag Version="2.0"><FrontMatter>` +
	`<ToolSettings CreationDate="20110212T171817Z" CreationTool="SDL TRADOS TagEditor" CreationToolVersion="8.3.0.863"/>` +
	`<UserSettings DataType="HTML" O-Encoding="windows-1252" SettingsName="" SourceLanguage="EN-US" TargetLanguage="" TargetDefaultFont=""/>` +
	`</FrontMatter><Body><Raw>` +
	`<df Font="Candara" Size="12">` +
	`<ut Type="start" Style="external" DisplayText="paragraph">&lt;paragraph&gt;</ut>` +
	"\n" + `This is a sentence.` + "\n" +
	`</df>` +
	`<ut Type="end" Style="external" DisplayText="paragraph">&lt;/paragraph&gt;</ut>` +
	`</Raw></Body></TRADOStag>`

// TestUnsegmented_PreservesFrontMatter checks that a no-<Tu> file keeps its
// entire <FrontMatter> (ToolSettings/UserSettings) on a translating round-trip.
// Regression for the bug where an empty skeleton made the writer fall back to a
// bare <TRADOStag> wrapper, dropping FrontMatter (Issue_164 / Test02_noseg).
func TestUnsegmented_PreservesFrontMatter(t *testing.T) {
	out := translatingRoundtrip(t, frontMatterTTX, ttx.SegmentModeAuto)

	assert.Contains(t, out, "<FrontMatter>", "FrontMatter opening must survive")
	assert.Contains(t, out, "</FrontMatter>", "FrontMatter closing must survive")
	assert.Contains(t, out, `CreationTool="SDL TRADOS TagEditor"`, "ToolSettings must survive verbatim")
	assert.Contains(t, out, `SourceLanguage="EN-US"`, "UserSettings must survive verbatim")
	// External <ut> markup stays in the skeleton (not translated).
	assert.Contains(t, out, `DisplayText="paragraph">&lt;paragraph&gt;</ut>`)
	// The free-text paragraph is wrapped in a fresh <Tu> and translated.
	assert.Contains(t, out, `<Tu MatchPercent="0"><Tuv Lang="EN-US">This is a sentence.`)
	assert.Contains(t, out, `<Tuv Lang="FR">ZZThis is a sentence.`)
	assertWellFormedXML(t, out)
}

// TestUnsegmented_FillsEmptyTargetLanguage checks that an empty
// <UserSettings TargetLanguage=""> placeholder is filled with the requested
// target language code (uppercased), mirroring Okapi's TARGETLANGUAGE_ATTR
// placeholder handling (TTXFilter.buildStartElement).
func TestUnsegmented_FillsEmptyTargetLanguage(t *testing.T) {
	out := translatingRoundtrip(t, frontMatterTTX, ttx.SegmentModeAuto)
	assert.Contains(t, out, `TargetLanguage="FR"`, "empty TargetLanguage placeholder must be filled with target code")
	assert.NotContains(t, out, `TargetLanguage=""`)
}

// TestUnsegmented_EscapesCDATAClose verifies the writer escapes a literal
// "]]>" sequence carried in translatable text. XML 1.0 §2.4 forbids "]]>" in
// CharData outside a CDATA section; emitting it raw produces invalid XML (the
// Test02_noseg fixture carries a JavaScript "//]]>" CDATA-close marker as
// content). Regression for "unescaped ]]> not in CDATA section".
func TestUnsegmented_EscapesCDATAClose(t *testing.T) {
	snippet := `<?xml version="1.0" encoding="utf-8"?>` +
		`<TRADOStag Version="2.0"><FrontMatter>` +
		`<UserSettings SourceLanguage="EN-US" TargetLanguage=""/>` +
		`</FrontMatter><Body><Raw>` +
		`script marker ]]&gt; here` +
		`</Raw></Body></TRADOStag>`
	out := translatingRoundtrip(t, snippet, ttx.SegmentModeAll)

	assert.NotContains(t, out, "]]>", "the literal ]]> must not appear in CharData")
	assert.Contains(t, out, "]]&gt;", "]]> must be escaped as ]]&gt;")
	assertWellFormedXML(t, out)
}

// TestXMLEscapeCDATAClose_DirectOnly verifies the ]]>-escaping rule fires even
// when EscapeGT is off: a lone '>' stays raw, but the '>' that closes a "]]>"
// run is escaped.
func TestUnsegmented_LoneGTNotEscaped(t *testing.T) {
	snippet := `<?xml version="1.0" encoding="utf-8"?>` +
		`<TRADOStag Version="2.0"><FrontMatter>` +
		`<UserSettings SourceLanguage="EN-US" TargetLanguage=""/>` +
		`</FrontMatter><Body><Raw>` +
		`a &gt; b and c ]]&gt; d` +
		`</Raw></Body></TRADOStag>`
	out := translatingRoundtrip(t, snippet, ttx.SegmentModeAll)

	// The standalone '>' (decoded from &gt;) round-trips raw (EscapeGT off),
	// while the ]]> closer is escaped — both inside the same translated run.
	assert.Contains(t, out, "ZZa > b and c ]]&gt; d")
	assertWellFormedXML(t, out)
}

// TestUnsegmented_ExternalUTIsBoundary verifies an external <ut> splits the
// surrounding free text into separate <Tu> segments and stays verbatim in the
// skeleton — mirroring Okapi's Style="external" boundary handling. Without it
// the whole <Raw> would fold into one block and external markup would be
// pseudo-translated as text.
func TestUnsegmented_ExternalUTIsBoundary(t *testing.T) {
	snippet := `<?xml version="1.0" encoding="utf-8"?>` +
		`<TRADOStag Version="2.0"><FrontMatter>` +
		`<UserSettings SourceLanguage="EN-US" TargetLanguage=""/>` +
		`</FrontMatter><Body><Raw>` +
		`First.` +
		`<ut Style="external" DisplayText="hr">&lt;hr&gt;</ut>` +
		`Second.` +
		`</Raw></Body></TRADOStag>`
	out := translatingRoundtrip(t, snippet, ttx.SegmentModeAll)

	// Two separate translated segments, external <ut> verbatim between them.
	assert.Contains(t, out, `<Tuv Lang="EN-US">First.</Tuv><Tuv Lang="FR">ZZFirst.</Tuv></Tu>`)
	assert.Contains(t, out, `<ut Style="external" DisplayText="hr">&lt;hr&gt;</ut>`)
	assert.Contains(t, out, `<Tuv Lang="EN-US">Second.</Tuv><Tuv Lang="FR">ZZSecond.</Tuv></Tu>`)
	assertWellFormedXML(t, out)
}

// TestUnsegmented_InlineUTFolded verifies an inline <ut> (no Style="external")
// has its placeholder text folded into the surrounding source run rather than
// splitting it — matching Okapi's isInline default-internal behavior.
func TestUnsegmented_InlineUTFolded(t *testing.T) {
	snippet := `<?xml version="1.0" encoding="utf-8"?>` +
		`<TRADOStag Version="2.0"><FrontMatter>` +
		`<UserSettings SourceLanguage="EN-US" TargetLanguage=""/>` +
		`</FrontMatter><Body><Raw>` +
		`before <ut Type="start">[</ut>in<ut Type="end">]</ut> after` +
		`</Raw></Body></TRADOStag>`
	reader := ttx.NewReader()
	reader.Config().(*ttx.Config).SegmentMode = ttx.SegmentModeAll
	require.NoError(t, reader.Open(context.Background(), testutil.RawDocFromString(snippet, "EN-US")))
	defer reader.Close()
	blocks := testutil.CollectBlocks(t, reader.Read(context.Background()))
	require.Len(t, blocks, 1, "inline <ut> must not split the run")
	assert.Equal(t, "before [in] after", blocks[0].SourceText())
}
