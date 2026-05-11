package openxml

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindParagraphs_simple(t *testing.T) {
	src := []byte(`<w:body><w:p><w:r><w:t>hello</w:t></w:r></w:p></w:body>`)
	got := findParagraphs(src)
	assert.Len(t, got, 1)
	assert.Equal(t, "<w:p><w:r><w:t>hello</w:t></w:r></w:p>", string(src[got[0].start:got[0].end]))
}

func TestFindRuns_basic(t *testing.T) {
	src := []byte(`<w:p><w:r><w:t>a</w:t></w:r><w:r><w:t>b</w:t></w:r></w:p>`)
	got := findRuns(src)
	assert.Len(t, got, 2)
}

func TestParseRunPropElements_basic(t *testing.T) {
	src := []byte(`<w:rPr><w:rFonts w:ascii="Arial"/><w:b/></w:rPr>`)
	got := parseRunPropElements(src)
	assert.Len(t, got, 2)
	assert.Equal(t, "rFonts", got[0].name)
	assert.Equal(t, "b", got[1].name)
}

func TestOptimizeWMLPart_MultipleRunsCommonProps(t *testing.T) {
	// Two runs with the same rFonts — common prop should be extracted
	// into a synthesised style. Mirrors the 1437-color-exclusion fixture
	// shape (multi-run paragraphs where Okapi factors out a common rPr
	// shape into a paragraph style).
	src := []byte(`<w:body><w:p><w:r><w:rPr><w:rFonts w:ascii="Arial"/></w:rPr><w:t>a</w:t></w:r><w:r><w:rPr><w:rFonts w:ascii="Arial"/></w:rPr><w:t>b</w:t></w:r></w:p></w:body>`)
	existing := map[string]bool{}
	var counter int
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, "", &counter, syn, &ids)
	assert.Contains(t, string(got), "NF974E24F-Normal1")
	assert.Len(t, ids, 1)
}

func TestOptimizeWMLPart_SingleRun_Optimised(t *testing.T) {
	// Post-#592, 1-run paragraphs are also optimised, mirroring upstream
	// Okapi StyleOptimisation.Default.applyTo (StyleOptimisation.java
	// line 98 bypasses only when chunks.size() <= 2 == 0 runs). The
	// writer now preserves per-source-run rPr on every emitted <w:r>
	// (#592 — see source_rpr.go), so the run carries the same rPr
	// payload Okapi sees and the optimisation premise — common props
	// across rendered runs — holds for 1-run paragraphs too.
	src := []byte(`<w:body><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>a</w:t></w:r></w:p></w:body>`)
	existing := map[string]bool{}
	var counter int
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, "", &counter, syn, &ids)
	assert.Contains(t, string(got), "NF974E24F-Normal1")
	assert.Len(t, ids, 1)
}

func TestOptimizeWMLPart_SingleRun_RStyle_Bypassed(t *testing.T) {
	// rStyle (character style reference) is in the WSO exclusion list
	// (mirrors upstream WordDocument.java construction of
	// StyleOptimisation.Default with Collections.singletonList(rStyle)).
	// A 1-run paragraph whose only rPr child is rStyle must NOT be
	// optimised — the rStyle stays on the run.
	src := []byte(`<w:body><w:p><w:r><w:rPr><w:rStyle w:val="Emphasis"/></w:rPr><w:t>a</w:t></w:r></w:p></w:body>`)
	existing := map[string]bool{}
	var counter int
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, "", &counter, syn, &ids)
	assert.NotContains(t, string(got), "NF974E24F")
	assert.Len(t, ids, 0)
	// Bypass keeps rStyle on the run.
	assert.Contains(t, string(got), `<w:rStyle w:val="Emphasis"/>`)
}

func TestParseRunProps_StripsDefaultValuedRtl(t *testing.T) {
	// rtl is a WpmlToggleRunProperty (RunPropertyFactory.java:219).
	// Toggle properties default to "true" per ECMA-376-1 §17.3.2, so
	// `<w:rtl w:val="0"/>` is a no-op that upstream Okapi strips at
	// parse time via RunProperties.minified() (RunParser.java:280-294 +
	// RunProperties.java:497-540). minifyRPrChildren mirrors that
	// pre-WSO step in native, so the rtl child never reaches
	// rPrChildren and the writer never re-emits it.
	//
	// Without this, redundant `<w:rtl w:val="0"/>` rPrs round-trip
	// into synthesised pStyles via WSO (reordered-zip.docx fixture).
	src := `<w:rPr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:rtl w:val="0"/></w:rPr>`
	dec := xml.NewDecoder(strings.NewReader(src))
	_, err := dec.Token()
	require.NoError(t, err)
	props, err := parseRunProps(dec, false)
	require.NoError(t, err)
	for _, c := range props.rPrChildren {
		if c.name == "rtl" {
			t.Fatalf("rtl should be stripped by minified(), got %q", c.xml)
		}
	}
}

func TestParseRunProps_KeepsExplicitRtlTrue(t *testing.T) {
	// A bare `<w:rtl/>` (or explicit `w:val="1"` / `"true"`) is the
	// actual on-toggle and must travel through to the writer. Only the
	// no-op default (false-equivalent values) gets minified out.
	src := `<w:rPr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:rtl/></w:rPr>`
	dec := xml.NewDecoder(strings.NewReader(src))
	_, err := dec.Token()
	require.NoError(t, err)
	props, err := parseRunProps(dec, false)
	require.NoError(t, err)
	found := false
	for _, c := range props.rPrChildren {
		if c.name == "rtl" {
			found = true
			break
		}
	}
	assert.True(t, found, "bare <w:rtl/> must be preserved (toggle defaults to true)")
}

func TestOptimizeWMLPart_SingleRun_Vanish_Bypassed(t *testing.T) {
	// vanish (hidden text) is conservatively excluded pending
	// paragraph-style→run inheritance support in the native reader
	// (TestRoundtripFormatted relies on hidden runs remaining hidden
	// after roundtrip; promoting vanish into a synthesised pStyle
	// would expose them as translatable on the second read). See
	// runPropExclusions godoc. Upstream Okapi DOES lift vanish; this
	// is a temporary native-only over-exclusion.
	src := []byte(`<w:body><w:p><w:r><w:rPr><w:vanish/></w:rPr><w:t>a</w:t></w:r></w:p></w:body>`)
	existing := map[string]bool{}
	var counter int
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, "", &counter, syn, &ids)
	assert.NotContains(t, string(got), "NF974E24F")
	assert.Len(t, ids, 0)
	assert.Contains(t, string(got), `<w:vanish/>`)
}

func TestInsertPStyle_OpenCloseFormStripped(t *testing.T) {
	// captureRawElement re-emits self-closing pStyle elements in
	// open/close form ("<w:pStyle ...></w:pStyle>"). insertPStyle must
	// strip the existing element regardless of which form the source
	// uses — otherwise a paragraph that already has <w:pStyle> ends up
	// with TWO pStyle children (the new synthesised one, then the
	// preserved original). Pre-#592 this never bit because the writer
	// dropped per-source rPr and WSO never fired on these paragraphs.
	// See gettysburg_en.docx for the original repro fixture.
	src := []byte(`<w:pPr><w:pStyle w:val="style0"></w:pStyle><w:jc w:val="center"/></w:pPr>`)
	got := insertPStyle(src, "NF974E24F-style01")
	assert.Equal(t,
		`<w:pPr><w:pStyle w:val="NF974E24F-style01"/><w:jc w:val="center"/></w:pPr>`,
		string(got),
		"open/close <w:pStyle> form must be stripped before inserting the synthesised id",
	)
}

func TestParseRunProps_PreservesNonToggleChildren(t *testing.T) {
	// #592: parseRunProps must capture every non-toggle <w:rPr> child as
	// a writer-friendly serialisation in props.rPrChildren so the writer
	// can re-emit it on every <w:r>. Toggle children (b, i, u, strike,
	// vertAlign, vanish) are intentionally excluded because the writer
	// reconstructs them from PcOpen/PcClose runs.
	src := `<w:rPr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:rStyle w:val="Emphasis"/><w:rFonts w:ascii="Arial"/><w:color w:val="FF0000"/><w:sz w:val="24"/><w:b/><w:i/></w:rPr>`
	dec := xml.NewDecoder(strings.NewReader(src))
	// Skip past the opening <w:rPr> token.
	_, err := dec.Token()
	require.NoError(t, err)
	props, err := parseRunProps(dec, false)
	require.NoError(t, err)
	// Toggles parsed into struct fields.
	assert.True(t, props.bold)
	assert.True(t, props.italic)
	// Non-toggles kept verbatim in source order.
	names := make([]string, 0, len(props.rPrChildren))
	for _, c := range props.rPrChildren {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"rStyle", "rFonts", "color", "sz"}, names)
	// Each child uses the "w:" element prefix the writer needs.
	assert.Equal(t, `<w:rStyle w:val="Emphasis"/>`, props.rPrChildren[0].xml)
	assert.Equal(t, `<w:color w:val="FF0000"/>`, props.rPrChildren[2].xml)
}

func TestParseRunProps_SkipsLangNoProof(t *testing.T) {
	// lang and noProof are stripped by upstream RunSkippableElements
	// (RunSkippableElements.java lines 50-62). The native reader must
	// match — otherwise these elements leak into the per-paragraph
	// common rPr and get lifted into a synthesised pStyle that Okapi
	// did not generate. See 1080-1.docx for the original repro
	// (paragraph whose only run rPr is <w:lang w:val="en-US"/>).
	src := `<w:rPr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:lang w:val="en-US"/><w:noProof/><w:rStyle w:val="X"/></w:rPr>`
	dec := xml.NewDecoder(strings.NewReader(src))
	_, err := dec.Token()
	require.NoError(t, err)
	props, err := parseRunProps(dec, false)
	require.NoError(t, err)
	names := make([]string, 0, len(props.rPrChildren))
	for _, c := range props.rPrChildren {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"rStyle"}, names,
		"lang and noProof must be skipped from rPrChildren capture")
}

func TestCommonRPrChildren_IntersectionAcrossRuns(t *testing.T) {
	// #592: commonRPrChildren computes the per-paragraph intersection
	// of source-run rPr children, mirroring upstream Okapi
	// StyleOptimisation.commonRunPropertiesOf
	// (StyleOptimisation.java lines 204-237). Children present and
	// equal across every text-bearing source run survive; the rest
	// drop out.
	runs := []textRun{
		{
			text: "A",
			props: runProps{rPrChildren: []rPrChild{
				{name: "rFonts", xml: `<w:rFonts w:ascii="Arial"/>`},
				{name: "color", xml: `<w:color w:val="FF0000"/>`},
				{name: "sz", xml: `<w:sz w:val="24"/>`},
			}},
		},
		{
			text: "B",
			props: runProps{rPrChildren: []rPrChild{
				{name: "rFonts", xml: `<w:rFonts w:ascii="Arial"/>`},
				{name: "color", xml: `<w:color w:val="00FF00"/>`}, // differs
				{name: "sz", xml: `<w:sz w:val="24"/>`},
			}},
		},
	}
	common := commonRPrChildren(runs)
	names := make([]string, 0, len(common))
	for _, c := range common {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"rFonts", "sz"}, names)
}

func TestCommonRPrChildren_RunWithoutRPrClearsCommon(t *testing.T) {
	// Per upstream Okapi behaviour (StyleOptimisation.java lines 224-228):
	// if any run carries an empty rPr, the common-property set is cleared.
	// This guards against lifting rPr into a synthesised paragraph style
	// when at least one run has direct heterogeneous formatting.
	runs := []textRun{
		{text: "A", props: runProps{rPrChildren: []rPrChild{
			{name: "rFonts", xml: `<w:rFonts w:ascii="Arial"/>`},
		}}},
		{text: "B", props: runProps{}}, // no rPr at all
	}
	common := commonRPrChildren(runs)
	assert.Empty(t, common)
}

func TestCommonRPrChildren_RFontsAttributeSubset(t *testing.T) {
	// Heterogeneous rFonts — same value but different attribute subsets
	// across runs (gettysburg_en.docx pattern). Mirrors upstream Okapi
	// RunFonts.canBeMerged + RunFonts.merge (RunFonts.java lines 190-247
	// and 267-315): RunMerger fuses adjacent runs whose rFonts are
	// mergeable BEFORE WSO sees them. Native does not run RunMerger, so
	// commonRPrChildren approximates the upstream merge as the per-
	// attribute intersection of every run's rFonts. Per ECMA-376-1
	// §17.3.2.26 the rFonts attributes (ascii, hAnsi, cs, eastAsia, *Theme,
	// hint) are independent and an rFonts may carry any subset.
	runs := []textRun{
		{text: "A", props: runProps{rPrChildren: []rPrChild{
			{name: "rFonts", xml: `<w:rFonts w:ascii="DejaVu Serif" w:cs="DejaVu Serif" w:hAnsi="DejaVu Serif"/>`},
			{name: "b", xml: `<w:b/>`},
		}}},
		{text: "B", props: runProps{rPrChildren: []rPrChild{
			{name: "rFonts", xml: `<w:rFonts w:ascii="DejaVu Serif" w:cs="DejaVu Serif" w:eastAsia="DejaVu Serif" w:hAnsi="DejaVu Serif"/>`},
			{name: "b", xml: `<w:b/>`},
		}}},
	}
	common := commonRPrChildren(runs)
	require.Len(t, common, 2)
	// Pick out by name; order is implementation-defined.
	byName := map[string]rPrChild{}
	for _, c := range common {
		byName[c.name] = c
	}
	require.Contains(t, byName, "b")
	require.Contains(t, byName, "rFonts")
	// b survives unchanged.
	assert.Equal(t, `<w:b/>`, byName["b"].xml)
	// rFonts is the per-attribute intersection: ascii + cs + hAnsi
	// (eastAsia present in only one run is dropped).
	assert.Contains(t, byName["rFonts"].xml, `w:ascii="DejaVu Serif"`)
	assert.Contains(t, byName["rFonts"].xml, `w:cs="DejaVu Serif"`)
	assert.Contains(t, byName["rFonts"].xml, `w:hAnsi="DejaVu Serif"`)
	assert.NotContains(t, byName["rFonts"].xml, "eastAsia")
}

func TestCommonRPrChildren_RFontsValueDisagreementDrops(t *testing.T) {
	// When runs disagree on a shared rFonts attribute value, that
	// attribute drops out of the intersection. If nothing remains,
	// rFonts is excluded from the common set entirely (and stays on
	// each run).
	runs := []textRun{
		{text: "A", props: runProps{rPrChildren: []rPrChild{
			{name: "rFonts", xml: `<w:rFonts w:ascii="Arial"/>`},
		}}},
		{text: "B", props: runProps{rPrChildren: []rPrChild{
			{name: "rFonts", xml: `<w:rFonts w:ascii="Times"/>`},
		}}},
	}
	common := commonRPrChildren(runs)
	assert.Empty(t, common, "rFonts with disagreeing ascii drops; nothing else common")
}

func TestOptimizeWMLPart_HeterogeneousRFontsLiftedToStyle(t *testing.T) {
	// End-to-end: post-write WSO sees runs with identical (already-merged)
	// rFonts content prepended by renderWMLBlock from the source-rPr
	// annotation, and lifts the rFonts plus other shared rPr children
	// into the synthesised paragraph style. The reader-side merge happens
	// in commonRPrChildren (source_rpr.go); the post-write merge in
	// commonProps (style_optimization.go) handles the unusual case where
	// the source-rPr annotation was bypassed and runs reach the post-pass
	// with heterogeneous rFonts.
	src := []byte(`<w:body><w:p>` +
		`<w:r><w:rPr><w:rFonts w:ascii="DejaVu Serif" w:cs="DejaVu Serif" w:hAnsi="DejaVu Serif"/><w:b/></w:rPr><w:t>a</w:t></w:r>` +
		`<w:r><w:rPr><w:rFonts w:ascii="DejaVu Serif" w:cs="DejaVu Serif" w:eastAsia="DejaVu Serif" w:hAnsi="DejaVu Serif"/><w:b/></w:rPr><w:t>b</w:t></w:r>` +
		`</w:p></w:body>`)
	existing := map[string]bool{}
	var counter int
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, "", &counter, syn, &ids)
	require.Len(t, ids, 1, "one synthesised style expected")
	s := syn[ids[0]]
	assert.Contains(t, s.rPrXML, "rFonts", "synthesised style must include common rFonts")
	assert.Contains(t, s.rPrXML, `w:ascii="DejaVu Serif"`)
	assert.Contains(t, s.rPrXML, `w:cs="DejaVu Serif"`)
	assert.Contains(t, s.rPrXML, `w:hAnsi="DejaVu Serif"`)
	assert.NotContains(t, s.rPrXML, "eastAsia", "eastAsia present in only one run must NOT be in common")
	assert.Contains(t, string(got), "NF974E24F-Normal1")
	// Both runs should have rFonts stripped (full strip by name, mirroring
	// upstream Run.refineRunProperties).
	assert.NotContains(t, string(got), "rFonts")
}

func TestOptimizeWMLPart_MixedSelfClosingAndOpenForms_NormalisedToCommon(t *testing.T) {
	// encoding/xml's Decoder/Encoder cycle re-emits captureRawElement
	// payloads in mixed forms — some runs come back with `<w:sz w:val="14"/>`
	// (self-closing), others with `<w:sz w:val="14"></w:sz>` (open/close).
	// Without normalisation, exact-xml comparison treats them as distinct
	// and commonProps spuriously returns empty — which is what caused WSO
	// to silently bypass headers/footers in fixtures like 956.docx and
	// 992.docx (where every footer run carries an identical sz=14 rPr).
	src := []byte(`<w:body><w:p>` +
		`<w:r><w:rPr><w:sz w:val="14"/></w:rPr><w:t>a</w:t></w:r>` +
		`<w:r><w:rPr><w:sz w:val="14"></w:sz></w:rPr><w:t>b</w:t></w:r>` +
		`<w:r><w:rPr><w:sz w:val="14"/></w:rPr><w:t>c</w:t></w:r>` +
		`</w:p></w:body>`)
	existing := map[string]bool{}
	var counter int
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, "", &counter, syn, &ids)
	require.Len(t, ids, 1, "the common sz=14 must lift into a synthesised style")
	s := syn[ids[0]]
	assert.Equal(t, `<w:sz w:val="14"/>`, s.rPrXML)
	assert.Contains(t, string(got), "NF974E24F-Normal1")
}

func TestOptimizeWMLPart_UnknownPStyleFallsBackToNormal(t *testing.T) {
	// Mirrors WordStyleDefinitions.Ids.basedOn (lines 453-460): a
	// paragraph pStyle that isn't defined in styles.xml falls through
	// to defaultBased() / "Normal". Fixtures like 992.docx carry
	// "Corpodeltesto" / "Pidipagina" pStyle vals that aren't actually
	// defined as styleIds; Okapi's reference resolves them to
	// Normal-based, so native must too.
	src := []byte(`<w:body><w:p><w:pPr><w:pStyle w:val="Corpodeltesto"/></w:pPr>` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>a</w:t></w:r>` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>b</w:t></w:r>` +
		`</w:p></w:body>`)
	existing := map[string]bool{} // Corpodeltesto NOT defined
	var counter int
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, "", &counter, syn, &ids)
	require.Len(t, ids, 1)
	s := syn[ids[0]]
	assert.Equal(t, "Normal", s.parentID, "unknown parent must collapse to Normal")
	assert.Contains(t, string(got), "NF974E24F-Normal1")
}

func TestOptimizeWMLPart_SharedCounter_AcrossParts(t *testing.T) {
	// idCounter is a single shared sequence per IdGenerator scope —
	// see IdGenerator.createId at okapi/core/.../IdGenerator.java:124-138.
	// Two consecutive optimizeWMLPart calls (e.g. document.xml then
	// footer1.xml) must consume sequential numbers from the shared
	// counter, so the second part's first synthesised id is N+1 (NOT
	// 1, which would collide with the first part's first id).
	docPart := []byte(`<w:body><w:p>` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>a</w:t></w:r>` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>b</w:t></w:r>` +
		`</w:p></w:body>`)
	footerPart := []byte(`<w:ftr><w:p><w:pPr><w:pStyle w:val="Footer"/></w:pPr>` +
		`<w:r><w:rPr><w:sz w:val="14"/></w:rPr><w:t>x</w:t></w:r>` +
		`<w:r><w:rPr><w:sz w:val="14"/></w:rPr><w:t>y</w:t></w:r>` +
		`</w:p></w:ftr>`)
	existing := map[string]bool{"Footer": true, "Normal": true}
	var counter int
	syn := map[string]synthesisedStyle{}
	var ids []string
	_ = optimizeWMLPart(docPart, existing, "", &counter, syn, &ids)
	_ = optimizeWMLPart(footerPart, existing, "", &counter, syn, &ids)
	require.Len(t, ids, 2, "one synthesised style per part")
	assert.Equal(t, "NF974E24F-Normal1", ids[0])
	assert.Equal(t, "NF974E24F-Footer2", ids[1], "shared counter must increment across parts")
}
