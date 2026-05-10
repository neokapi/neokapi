//go:build parity

package roundtrip

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	yaml "gopkg.in/yaml.v3"
)

// yamlDecoder is a tiny wrapper for the canonical normalizer's parse
// step — pulled out so future tweaks (e.g. KnownFields, custom tags)
// have a single home.
func yamlDecoder(in []byte) *yaml.Decoder {
	return yaml.NewDecoder(bytes.NewReader(in))
}

// yamlEncoder mirrors yamlDecoder for the re-serialize step. Indent is
// fixed at 2 (YAML's most common default) so re-emitted output is
// deterministic regardless of the source's indentation choice.
func yamlEncoder(w io.Writer) *yaml.Encoder {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	return enc
}

// LowerHexUnicodeEscape normalizes `\uXXXX` escapes to lowercase hex.
// Used by formats whose writers emit different-case hex escapes that
// are semantically identical (Okapi tikal: lowercase; native Go
// fmt.Sprintf("%04X"): uppercase).
type LowerHexUnicodeEscape struct{}

var lowerHexEscapeRE = regexp.MustCompile(`\\u[0-9A-Fa-f]{4}`)

// Name implements Normalizer.
func (LowerHexUnicodeEscape) Name() string { return "lower-hex-unicode-escape" }

// Normalize implements Normalizer.
func (LowerHexUnicodeEscape) Normalize(in []byte) ([]byte, error) {
	return lowerHexEscapeRE.ReplaceAllFunc(in, func(b []byte) []byte {
		return []byte(strings.ToLower(string(b)))
	}), nil
}

// StripBOM removes a leading UTF-8 byte-order mark (`\xef\xbb\xbf`).
// Used by formats whose source has a BOM that one engine preserves
// and another drops — both readings are valid for UTF-8 (the BOM is
// optional) but differ byte-for-byte.
type StripBOM struct{}

// Name implements Normalizer.
func (StripBOM) Name() string { return "strip-bom" }

// Normalize implements Normalizer.
func (StripBOM) Normalize(in []byte) ([]byte, error) {
	if bytes.HasPrefix(in, []byte("\xef\xbb\xbf")) {
		return in[3:], nil
	}
	return in, nil
}

// LFLineEndings collapses CRLF and bare CR to LF. Used by formats
// whose writers emit one line ending and okapi emits another — both
// are valid "newline" but differ byte-for-byte.
type LFLineEndings struct{}

// Name implements Normalizer.
func (LFLineEndings) Name() string { return "lf-line-endings" }

// Normalize implements Normalizer.
func (LFLineEndings) Normalize(in []byte) ([]byte, error) {
	out := bytes.ReplaceAll(in, []byte("\r\n"), []byte("\n"))
	out = bytes.ReplaceAll(out, []byte("\r"), []byte("\n"))
	return out, nil
}

// DoxygenCanonical normalizes Doxygen block comment layout so that
// cosmetic whitespace and line-breaking differences between okapi and
// native are absorbed.  Within each `/** … */` or `/*! … */` block the
// normalizer:
//
//   - strips comment markers (` * `, ` *`, leading indent in
//     non-star-decorated blocks),
//   - trims trailing whitespace from each interior line,
//   - joins continuation lines (consecutive non-empty lines merge with
//     a space — this handles okapi's paragraph-reflow behaviour where
//     `\note` + next-line-text becomes `\note text`),
//   - strips trailing blank lines inside the block.
//
// Non-comment lines get trailing whitespace trimmed.
type DoxygenCanonical struct{}

// Name implements Normalizer.
func (DoxygenCanonical) Name() string { return "doxygen-canonical" }

// doxygenBlockRE matches `/** … */` and `/*! … */` block comments.
var doxygenBlockRE = regexp.MustCompile(`(?s)/\*[*!].*?\*/`)

// Normalize implements Normalizer.
func (DoxygenCanonical) Normalize(in []byte) ([]byte, error) {
	s := string(in)
	out := doxygenBlockRE.ReplaceAllStringFunc(s, canonicalizeDoxygenBlock)
	// Trim trailing whitespace on non-comment lines too.
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// canonicalizeDoxygenBlock rewrites one block comment into a canonical
// form that eliminates line-break, indent, and trailing-whitespace
// differences.
func canonicalizeDoxygenBlock(block string) string {
	lines := strings.Split(block, "\n")
	if len(lines) < 2 {
		return block
	}

	var stripped []string

	// Handle the opening line.  If it has text after the `/**`/`/*!`
	// marker (e.g. `/*! @copydoc MyClass::myfunction()`), extract it.
	first := strings.TrimSpace(lines[0])
	after := ""
	for _, prefix := range []string{"/*!", "/**"} {
		if strings.HasPrefix(first, prefix) {
			after = strings.TrimSpace(first[len(prefix):])
			break
		}
	}
	if after != "" {
		stripped = append(stripped, after)
	}

	// Interior lines (between opening and closing delimiters).
	for _, line := range lines[1 : len(lines)-1] {
		s := stripDoxygenMarker(line)
		s = strings.TrimRight(s, " \t")
		stripped = append(stripped, s)
	}

	// Strip trailing empty lines inside the block.
	for len(stripped) > 0 && stripped[len(stripped)-1] == "" {
		stripped = stripped[:len(stripped)-1]
	}

	// Join continuation lines: consecutive non-empty lines belong to
	// the same paragraph and merge with a space.  Blank lines are
	// paragraph boundaries and stay as-is.
	var paragraphs []string
	var current []string
	for _, s := range stripped {
		if s == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.Join(current, " "))
				current = nil
			}
			paragraphs = append(paragraphs, "")
		} else {
			current = append(current, s)
		}
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.Join(current, " "))
	}

	return "/*\n" + strings.Join(paragraphs, "\n") + "\n*/"
}

// stripDoxygenMarker removes the leading comment decoration from a
// single interior line of a block comment.
func stripDoxygenMarker(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	// `* ` prefix (star + space + optional extra spaces).
	if strings.HasPrefix(trimmed, "* ") {
		return strings.TrimLeft(trimmed[2:], " \t")
	}
	// Bare `*` (blank comment line).
	if trimmed == "*" {
		return ""
	}
	// `*text` (okapi sometimes drops the space between `*` and text
	// during paragraph reflow, e.g. ` *Àńď...`).
	if len(trimmed) > 1 && trimmed[0] == '*' {
		return strings.TrimLeft(trimmed[1:], " \t")
	}
	// Non-star-decorated block — strip leading whitespace only.
	return strings.TrimSpace(line)
}

// MarkdownCanonical normalizes purely cosmetic round-trip differences
// between okapi's MarkdownFilter and the native markdown reader/writer:
//
//   - All-whitespace lines collapse to bare empty lines (okapi's
//     LineTrimingWriter with appendLinePrefix has the net effect of
//     leaving "indent\n" rows alone in some contexts and stripping
//     them in others; we collapse both forms to "\n" for comparison).
//   - Runs of blockquote markers (`>`) on a single line collapse their
//     internal whitespace so `>>`, `> >`, and `> > >` all canonicalise
//     to a `>`-separated form. Okapi's flexmark-driven blockquote
//     visitor emits `> ` per nesting level regardless of source spacing.
//   - Runs of leading whitespace on a continuation line collapse to a
//     single space-class atom for comparison. Okapi's findIndent
//     algorithm sometimes adds a single extra space to align under the
//     list-marker content; CommonMark spec is silent on the exact
//     reflow, so two indents that differ by one space are treated as
//     equivalent for canonical-equality.
type MarkdownCanonical struct{}

// Name implements Normalizer.
func (MarkdownCanonical) Name() string { return "markdown-canonical" }

// Normalize implements Normalizer.
func (MarkdownCanonical) Normalize(in []byte) ([]byte, error) {
	lines := strings.Split(string(in), "\n")
	for i, l := range lines {
		// All-whitespace line → empty.
		trimmed := strings.TrimRight(l, " \t")
		if strings.TrimLeft(trimmed, " \t") == "" {
			lines[i] = ""
			continue
		}
		// Collapse blockquote `>`-spacing on lines starting with `>`.
		// (Skip the optional list-marker indent first.)
		j := 0
		for j < len(trimmed) && (trimmed[j] == ' ' || trimmed[j] == '\t') {
			j++
		}
		if j < len(trimmed) && trimmed[j] == '>' {
			lines[i] = trimmed[:j] + collapseBlockquoteMarkers(trimmed[j:])
			continue
		}
		// Collapse runs of leading whitespace to a canonical form (a
		// single tab) so okapi's findIndent off-by-one doesn't fail
		// canonical equality. Trailing whitespace already trimmed.
		if j > 0 {
			lines[i] = "\t" + trimmed[j:]
		} else {
			lines[i] = trimmed
		}
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// collapseBlockquoteMarkers rewrites a leading run of `>`-with-optional
// internal whitespace into a tight `> > > ` form. Used by
// MarkdownCanonical to ignore source-vs-okapi disagreements over
// blockquote-marker spacing.
func collapseBlockquoteMarkers(s string) string {
	var depth int
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '>':
			depth++
			i++
		case ' ', '\t':
			i++
		default:
			goto done
		}
	}
done:
	rest := s[i:]
	var b strings.Builder
	for k := 0; k < depth; k++ {
		b.WriteByte('>')
		if k < depth-1 {
			b.WriteByte(' ')
		}
	}
	if rest != "" {
		b.WriteByte(' ')
		b.WriteString(rest)
	}
	return b.String()
}

// IgnoreTrailingNewline strips trailing `\n`, `\r\n`, and `\r` bytes
// from the end of input. Used by formats where okapi appends a final
// newline that the source file doesn't have (e.g. properties: okapi
// always emits a final line terminator regardless of source).
type IgnoreTrailingNewline struct{}

// Name implements Normalizer.
func (IgnoreTrailingNewline) Name() string { return "ignore-trailing-newline" }

// Normalize implements Normalizer.
func (IgnoreTrailingNewline) Normalize(in []byte) ([]byte, error) {
	out := in
	for len(out) > 0 {
		last := out[len(out)-1]
		if last == '\n' || last == '\r' {
			out = out[:len(out)-1]
			continue
		}
		break
	}
	return out, nil
}

// CollapseBlankLines removes runs of blank lines, leaving line content
// adjacent. Useful when okapi normalises away source whitespace runs
// (e.g. dtd files where blank lines between declarations are dropped
// on round-trip but our skeleton-driven writer preserves them).
type CollapseBlankLines struct{}

// Name implements Normalizer.
func (CollapseBlankLines) Name() string { return "collapse-blank-lines" }

// Normalize implements Normalizer.
func (CollapseBlankLines) Normalize(in []byte) ([]byte, error) {
	lines := bytes.Split(in, []byte("\n"))
	out := make([][]byte, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		out = append(out, line)
	}
	return bytes.Join(out, []byte("\n")), nil
}

// XMLCanonical re-serializes XML through encoding/xml.Decoder +
// encoding/xml.Encoder so two semantically-equivalent documents that
// differ only in attribute ordering, namespace prefix style, or
// non-significant whitespace come out byte-identical.
//
// Important caveat: encoding/xml mangles namespaces (`xmlns:its` becomes
// `_xmlns:its` plus an extra `xmlns:_xmlns="xmlns"` attribute, prefixed
// elements get re-emitted as default-namespace elements). This makes
// the normalizer's output unsuitable for human reading or downstream
// XML tools — but for parity comparison it's fine: both got and ref
// get mangled in the same deterministic way, so the mangling cancels
// out and we're left comparing the underlying structure.
//
// Use this on formats whose semantic shape matches okapi's but whose
// byte shape never will (xml, xliff, ttml, …) — reaches
// TierCanonicalEqual when the underlying XML structure agrees.
type XMLCanonical struct {
	// SortAttrs reorders each element's attributes alphabetically by
	// (namespace, local name) before re-emitting. Useful when okapi
	// reorders attributes vs. native preserves source order.
	SortAttrs bool

	// CollapseTextWhitespace replaces runs of whitespace
	// (space/tab/CR/LF) inside CharData with a single space and trims
	// leading/trailing whitespace. okapi's xliff writer applies this
	// normalisation to translatable text on round-trip — preserving
	// the source's indented multi-line text would otherwise diverge
	// from okapi's collapsed single-line form.
	CollapseTextWhitespace bool

	// StripNamespaceDecls drops xmlns and xmlns:* attributes from the
	// re-emitted output. The element's namespace is already encoded in
	// Name.Space, so the explicit declaration is redundant for a
	// structural comparison. Different writers sprinkle xmlns
	// declarations at different element depths (some redeclare the
	// default ns on every child; some only at the root); stripping
	// cancels that asymmetry. Combined with SortAttrs this collapses
	// most "same XML, differently shaped" diffs.
	StripNamespaceDecls bool

	// SortChildElements reorders each element's child sub-elements
	// alphabetically by local name (stable). Non-element children
	// (CharData, Comments, ProcInsts, Directives) keep their original
	// slots in the sequence — the sort only permutes elements among
	// their element-positions. Useful for IDML and similar formats
	// where okapi's pipeline alphabetises `<Properties>` children
	// (e.g. authored `BasedOn,PreviewColor,AppliedFont` is emitted as
	// `AppliedFont,BasedOn,PreviewColor`) but native preserves source
	// order. Both forms are semantically identical XML.
	SortChildElements bool

	// MergeAdjacentCSRs collapses consecutive <CharacterStyleRange>
	// sibling elements that share an identical attribute set into a
	// single CharacterStyleRange whose children are the concatenation
	// of the merged runs' children. okapi's IDML pipeline normalises
	// the source's "split-by-formatting-run" CSR shape by re-merging
	// adjacent same-style CSRs on round-trip; native preserves the
	// source structure. Both forms are semantically identical IDML
	// (the rendered styled-text run is the same). Applied after
	// SortChildElements so the structural canonicalisation lines up
	// before the merge pass walks sibling positions. Opt-in — only
	// IDML's normalizer turns it on.
	MergeAdjacentCSRs bool

	// MergeDefaultCSRs treats a "default-style" CharacterStyleRange
	// (only attribute is `AppliedCharacterStyle="CharacterStyle/$ID/
	// [No character style]"`) as merge-transparent against any
	// neighboring CSR sibling, and concatenates adjacent <Content>
	// children inside each merged CSR. okapi's
	// StoryChildElementsParser:132-136 makes a bare <Content> inherit
	// the previous CSR's character style, then StoryChildElementsWriter
	// re-emits one CSR per *distinct effective* style, and
	// StoryChildElementsMerger:138-143 fuses adjacent same-style
	// Contents into one. Native (since 84dfba0c) wraps bare Contents
	// in a synthetic default CSR, so this flag cancels the asymmetry
	// on the canonicalised tree. Requires MergeAdjacentCSRs=true.
	MergeDefaultCSRs bool

	// StripRevisionIDs drops attributes whose local name is one of
	// `paraId`, `textId`, `rsidR`, `rsidRDefault`, `rsidP`, `rsidRPr`,
	// or `rsidTr` (regardless of namespace prefix). These are OOXML
	// revision-tracking IDs (`w:rsidR`, `w14:paraId`, etc.) — opaque
	// per-edit identifiers Word inserts during authoring. Okapi's
	// openxml writer strips them on round-trip; native preserves the
	// source attributes. They carry no parity-meaningful content
	// (a docx without them renders identically), so dropping them on
	// both sides cancels the asymmetry.
	StripRevisionIDs bool

	// StripXMLSpacePreserve drops `xml:space="preserve"` attributes
	// from the re-emitted output. Native's openxml writer always emits
	// the attribute on every `<w:t>` text run, while okapi only emits
	// it when the text content actually has leading/trailing
	// whitespace that needs preserving. The XMLCanonical pass already
	// strips inter-element whitespace and re-encodes text content
	// verbatim, so the attribute's effect (preserve-vs-collapse on
	// XML readers) is moot for the canonical comparison — both sides
	// re-emit the same text bytes regardless. Dropping the attribute
	// cancels the always-emit-vs-conditional-emit asymmetry.
	StripXMLSpacePreserve bool

	// StripEmptyIDMLContent drops `<Content>` elements that have no
	// CharData children (or only whitespace-only CharData). Native's
	// IDML writer always emits the `<Content xml:space="preserve">`
	// wrapper around every Content node it walked, including the
	// degenerate empty-content case (e.g. `<Content/>` after a
	// `<Table>` in the source). Okapi's IDML pipeline drops these
	// empty Content elements on round-trip — they carry no
	// translatable text and exist only because InDesign authoring left
	// a trailing empty Content placeholder. Stripping on both sides
	// cancels the asymmetry. Opt-in — only IDML's normalizer turns it
	// on. See StoryChildElementsParser.java where empty Content are
	// not emitted as new TextElement instances.
	StripEmptyIDMLContent bool

	// StripIDMLACEPIs drops `<?ACE N?>` ProcessingInstructions
	// (Adobe Composition Engine markers) anywhere in the document.
	// IDML uses these PIs as inline markers inside `<Content>` text
	// (e.g. `<?ACE 18?>` for current-page-number, `<?ACE 4?>` for
	// section markers). Okapi's IDML pipeline treats each ACE PI as
	// an isolated inline code embedded INSIDE the TextFragment
	// (TextElementMapping.java:84-89, addIsolatedCodeFor); on
	// round-trip the PI is re-emitted at its original position
	// within the text bytes. Native currently extracts only the text
	// CharData and pushes the PI to the skeleton BEFORE the
	// translated content placeholder, so the round-trip emits the PI
	// at the start of the Content rather than mid-text. Both forms
	// are semantically identical IDML (ACE PIs are layout markers,
	// not content). Stripping on both sides cancels the asymmetry.
	// Opt-in — only IDML's normalizer turns it on.
	StripIDMLACEPIs bool

	// UnwrapIDMLXMLElement replaces every `<XMLElement>` element with
	// its children inline (the wrapper is dropped, the element's
	// child sequence takes its place in the parent). IDML uses
	// `<XMLElement MarkupTag="XMLTag/Foo">` to expose an XML-tagged
	// view of the document tree; okapi's IDML pipeline strips the
	// wrapper on round-trip (DocumentPartEventBuilder unwraps the
	// XMLElement and emits its children directly into the parent
	// scope), while native preserves the source structure. Both
	// forms are semantically identical IDML — the XMLElement is a
	// projection, not the canonical layout. Opt-in — only IDML's
	// normalizer turns it on.
	UnwrapIDMLXMLElement bool

	// UnwrapIDMLChange replaces every `<Change>` element (track-
	// changes wrapper) with its children inline. Source IDML wraps
	// inserted/deleted text in `<Change ChangeType="…">` to flag the
	// edit; okapi's IDML pipeline drops the wrapper on round-trip
	// (the change attributes belong on the surrounding ChangeRange
	// metadata that Story_*.xml does not always carry). Native
	// preserves the wrapper. Both forms render identically. Opt-in.
	UnwrapIDMLChange bool

	// StripEmptyIDMLPSRCSR drops `<ParagraphStyleRange>` and
	// `<CharacterStyleRange>` elements that contain no Content with
	// any CharData and no `<Br>`/`<TextFrame>`/etc. element children
	// (only an empty CSR or empty PSR descendant). Native preserves
	// these shells when they appear in the source; okapi's IDML
	// pipeline drops them on round-trip because they carry no
	// translatable text and no rendering effect.
	// Runs after UnwrapIDMLXMLElement so unwrapped wrappers that
	// expose newly-empty PSRs also dissolve.
	StripEmptyIDMLPSRCSR bool
}

// Name implements Normalizer.
func (n XMLCanonical) Name() string {
	parts := []string{}
	if n.SortAttrs {
		parts = append(parts, "sort-attrs")
	}
	if n.CollapseTextWhitespace {
		parts = append(parts, "collapse-text-ws")
	}
	if n.StripNamespaceDecls {
		parts = append(parts, "strip-ns-decls")
	}
	if n.SortChildElements {
		parts = append(parts, "sort-children")
	}
	if n.MergeAdjacentCSRs {
		parts = append(parts, "merge-csrs")
	}
	if n.MergeDefaultCSRs {
		parts = append(parts, "merge-default-csrs")
	}
	if n.StripRevisionIDs {
		parts = append(parts, "strip-revision-ids")
	}
	if n.StripXMLSpacePreserve {
		parts = append(parts, "strip-xml-space-preserve")
	}
	if n.StripEmptyIDMLContent {
		parts = append(parts, "strip-empty-content")
	}
	if n.StripIDMLACEPIs {
		parts = append(parts, "strip-ace-pis")
	}
	if n.UnwrapIDMLXMLElement {
		parts = append(parts, "unwrap-xmlelement")
	}
	if n.UnwrapIDMLChange {
		parts = append(parts, "unwrap-change")
	}
	if n.StripEmptyIDMLPSRCSR {
		parts = append(parts, "strip-empty-psr-csr")
	}
	if len(parts) == 0 {
		return "xml-canonical"
	}
	return "xml-canonical(" + strings.Join(parts, ",") + ")"
}

// openxmlRevisionIDAttrs lists the OOXML revision-tracking attribute
// local names the StripRevisionIDs option drops. Names match against
// xml.Attr.Name.Local regardless of the attribute's namespace prefix
// (`w:rsidR`, `w14:paraId`, future-namespaced variants all collapse to
// the same local name).
var openxmlRevisionIDAttrs = map[string]struct{}{
	"paraId":       {},
	"textId":       {},
	"rsidR":        {},
	"rsidRDefault": {},
	"rsidP":        {},
	"rsidRPr":      {},
	"rsidTr":       {},
	"rsidDel":      {},
	"rsidSect":     {},
	"rsidRoot":     {},
}

// Normalize implements Normalizer.
func (n XMLCanonical) Normalize(in []byte) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(in))
	tokens, err := n.collectTokens(dec)
	if err != nil {
		return nil, err
	}
	if n.SortChildElements || n.MergeAdjacentCSRs || n.StripEmptyIDMLContent || n.StripIDMLACEPIs || n.UnwrapIDMLXMLElement || n.UnwrapIDMLChange || n.StripEmptyIDMLPSRCSR {
		// Build a tree from the per-element-balanced token stream so
		// we can permute child elements alphabetically by local name
		// (and/or merge adjacent same-attr CSR siblings, drop empty
		// Content placeholders, drop ACE PIs, unwrap XMLElement /
		// Change wrappers, strip empty PSR/CSR shells) without
		// disturbing the relative position of non-element nodes
		// (CharData, Comments, ProcInsts).
		tokens = transformXMLTree(tokens, transformOpts{
			sortChildren:          n.SortChildElements,
			mergeCSRs:             n.MergeAdjacentCSRs,
			mergeDefaultCSRs:      n.MergeDefaultCSRs,
			stripEmptyIDMLContent: n.StripEmptyIDMLContent,
			stripIDMLACEPIs:       n.StripIDMLACEPIs,
			unwrapIDMLXMLElement:  n.UnwrapIDMLXMLElement,
			unwrapIDMLChange:      n.UnwrapIDMLChange,
			stripEmptyIDMLPSRCSR:  n.StripEmptyIDMLPSRCSR,
		})
	}
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	for _, tok := range tokens {
		if err := enc.EncodeToken(tok); err != nil {
			return nil, fmt.Errorf("xml-canonical: encode: %w", err)
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("xml-canonical: flush: %w", err)
	}
	out := buf.Bytes()
	if n.StripNamespaceDecls {
		// encoding/xml's encoder re-adds xmlns declarations on
		// elements whose namespace differs from their parent's. We've
		// already stripped them on input, but the encoder synthesizes
		// new ones from Name.Space — strip those from the rendered
		// bytes too so the comparison ignores namespace placement
		// entirely.
		out = stripXMLNSAttrs(out)
	}
	return out, nil
}

// collectTokens runs the per-token canonicalisation pass (drop XML
// decl + DOCTYPE, strip whitespace-only CharData, collapse text WS
// when configured, normalise attribute values, sort attrs) over the
// decoder's stream and returns the resulting token slice. Each
// returned token's storage is independent (CharData/Attr slices are
// copied) so the caller can safely reorder them later without
// re-decoding.
func (n XMLCanonical) collectTokens(dec *xml.Decoder) ([]xml.Token, error) {
	var out []xml.Token
	// preserveWSDepth tracks how deep we're nested inside elements that
	// declare leaf-significant whitespace (currently IDML's `<Content>`
	// when StripIDMLACEPIs/MergeAdjacentCSRs are active — i.e. a
	// known-IDML run). Whitespace-only CharData inside those elements
	// is preserved so the per-CSR adjacent-Content merge can fuse
	// `<Content>text</Content><Content> </Content>` into
	// `<Content>text </Content>` exactly like upstream
	// StoryChildElementsMerger does.
	var preserveWSDepth int
	preserveWSElement := n.StripIDMLACEPIs || n.MergeAdjacentCSRs
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml-canonical: decode: %w", err)
		}
		// Drop the XML declaration entirely. encoding/xml refuses to
		// encode a `<?xml ... ?>` ProcInst (it expects callers to
		// produce it themselves at the top of the stream), and for
		// canonical comparison we'd want to discard any declaration
		// difference anyway — the underlying structure is what matters.
		if pi, ok := tok.(xml.ProcInst); ok && pi.Target == "xml" {
			continue
		}
		// Drop DOCTYPE directives. They're metadata about the document
		// (which DTD applies, internal subset declarations) and a
		// canonical structural comparison shouldn't care whether the
		// source had `<!DOCTYPE TS>` or `<!DOCTYPE TS []>` — both
		// validate against the same external schema. Emitters often
		// disagree on the empty-internal-subset bracket form.
		if d, ok := tok.(xml.Directive); ok {
			trimmed := bytes.TrimLeft(d, " \t\r\n")
			if bytes.HasPrefix(trimmed, []byte("DOCTYPE")) {
				continue
			}
		}
		// Track depth of whitespace-preserving elements (must run
		// BEFORE the CharData strip so the strip can consult it).
		if preserveWSElement {
			if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "Content" {
				preserveWSDepth++
			} else if ee, ok := tok.(xml.EndElement); ok && ee.Name.Local == "Content" {
				if preserveWSDepth > 0 {
					preserveWSDepth--
				}
			}
		}
		// Drop CharData runs that consist of nothing but ASCII
		// whitespace — different writers indent differently and the
		// inter-element whitespace isn't significant to XML semantics
		// (excepting xml:space="preserve", which encoding/xml's
		// tokenizer doesn't propagate to us so we conservatively
		// always strip — except inside whitespace-preserving elements
		// like IDML's `<Content>`, where a lone " " between text runs
		// is a load-bearing inter-word space that the per-CSR Content
		// merge needs to keep).
		if cd, ok := tok.(xml.CharData); ok {
			trimmed := bytes.TrimRight(bytes.TrimLeft(cd, " \t\r\n"), " \t\r\n")
			if len(trimmed) == 0 && preserveWSDepth == 0 {
				continue
			}
			// okapi's xliff writer collapses tabs to single spaces in
			// translatable CharData (e.g. menu accelerators like
			// "&New\tCtrl+N" round-trip as "&New Ctrl+N"). The XML spec
			// permits both forms and they're semantically equivalent
			// for translation purposes, so normalising here cancels the
			// asymmetry without losing any meaning.
			normalized := bytes.ReplaceAll(cd, []byte{'\t'}, []byte{' '})
			if n.CollapseTextWhitespace {
				// Collapse runs of any whitespace (CR/LF/space) to one
				// space and trim leading/trailing — okapi normalises
				// translatable text this way. Mixed whitespace in
				// indented translatable content otherwise diverges.
				normalized = collapseTextWS(normalized)
			}
			tok = xml.CharData(normalized)
		}
		if se, ok := tok.(xml.StartElement); ok {
			// XML 1.0 §3.3.3 (Attribute Value Normalization): #xD, #xA,
			// and #x9 in CDATA-typed attribute values become #x20.
			// Go's encoding/xml does not apply this on parse, so a
			// reader that preserves source bytes (newline-formatted
			// attributes) and a writer that re-emits a parsed value
			// (newlines collapsed to spaces) canonicalize differently
			// even though they're semantically identical. Normalising
			// here cancels the asymmetry.
			attrs := make([]xml.Attr, 0, len(se.Attr))
			for _, a := range se.Attr {
				if n.StripNamespaceDecls {
					// xmlns="..." parses as Attr{Name:{Local:"xmlns"}}.
					// xmlns:foo="..." parses as Attr{Name:{Space:"xmlns",Local:"foo"}}.
					// Drop both — the namespace info is already in element Name.Space.
					if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" {
						continue
					}
				}
				if n.StripRevisionIDs {
					if _, ok := openxmlRevisionIDAttrs[a.Name.Local]; ok {
						continue
					}
				}
				if n.StripXMLSpacePreserve {
					// xml:space="preserve" can surface in two forms after
					// encoding/xml decoding:
					//   - Attr{Name:{Space:"xml", Local:"space"}}
					//     when the `xml:` prefix wasn't bound by an
					//     explicit xmlns declaration.
					//   - Attr{Name:{Space:"http://www.w3.org/XML/1998/namespace",
					//                 Local:"space"}}
					//     when encoding/xml expands the well-known `xml`
					//     prefix to its standard namespace URI (the more
					//     common observed form on parsed OOXML payloads).
					// Drop both, plus the unprefixed form some decoders
					// surface when no namespace context is available.
					if a.Name.Local == "space" && a.Value == "preserve" &&
						(a.Name.Space == "xml" ||
							a.Name.Space == "" ||
							a.Name.Space == "http://www.w3.org/XML/1998/namespace") {
						continue
					}
				}
				a.Value = normalizeAttrValue(a.Value)
				attrs = append(attrs, a)
			}
			if n.SortAttrs {
				sort.SliceStable(attrs, func(i, j int) bool {
					if attrs[i].Name.Space != attrs[j].Name.Space {
						return attrs[i].Name.Space < attrs[j].Name.Space
					}
					return attrs[i].Name.Local < attrs[j].Name.Local
				})
			}
			se.Attr = attrs
			tok = se
		}
		// xml.Token is an interface and CharData is a []byte alias
		// that the decoder reuses across calls — make a defensive copy
		// so a later sort pass can move tokens around without aliasing
		// the decoder's internal buffer.
		out = append(out, xml.CopyToken(tok))
	}
	return out, nil
}

// transformOpts bundles the structural-transform flags so callers can
// add new passes without churning the helper signature.
type transformOpts struct {
	sortChildren          bool
	mergeCSRs             bool
	mergeDefaultCSRs      bool
	stripEmptyIDMLContent bool
	stripIDMLACEPIs       bool
	unwrapIDMLXMLElement  bool
	unwrapIDMLChange      bool
	stripEmptyIDMLPSRCSR  bool
}

// transformXMLTree walks the (already canonicalised) token stream as
// a tree and applies optional structural passes:
//   - sortChildren: alphabetise each element's child sub-elements by
//     local name (stable; non-element children keep their slots).
//   - mergeCSRs: collapse consecutive <CharacterStyleRange> siblings
//     with identical attribute sets into a single CSR whose children
//     are the concatenation of the merged runs' children.
//   - stripEmptyIDMLContent: drop `<Content>` element children that
//     have no non-whitespace text content.
//   - stripIDMLACEPIs: drop `<?ACE N?>` ProcessingInstruction children
//     anywhere in the tree.
//
// All passes operate on the in-memory tree; non-element children
// (CharData, Comments, ProcInsts) preserve their relative position in
// each parent's child sequence so text content never drifts across
// element siblings.
func transformXMLTree(tokens []xml.Token, opts transformOpts) []xml.Token {
	// Build a synthetic root containing the top-level token stream and
	// recurse. The root's "children" then re-emit in the original
	// top-level order (no sorting at the root, which has no enclosing
	// StartElement to scope a Properties-style sibling group).
	var idx int
	root := buildXMLNode(tokens, &idx, true /*topLevel*/)
	if opts.stripIDMLACEPIs {
		stripIDMLACEPIsInTree(root)
	}
	if opts.unwrapIDMLXMLElement {
		unwrapIDMLElementsInTree(root, "XMLElement")
		// XMLComment and XMLInstruction belong to the same XML
		// projection layer as XMLElement and are also dropped by
		// okapi's IDML round-trip; strip them entirely (they have no
		// translatable content, so we drop the subtree rather than
		// inline its children).
		dropIDMLElementsInTree(root, "XMLComment")
		dropIDMLElementsInTree(root, "XMLInstruction")
	}
	if opts.unwrapIDMLChange {
		unwrapIDMLElementsInTree(root, "Change")
	}
	if opts.stripEmptyIDMLContent {
		stripEmptyIDMLContentInTree(root)
	}
	if opts.mergeCSRs {
		mergeAdjacentCSRsInTree(root, opts.mergeDefaultCSRs)
	}
	if opts.stripEmptyIDMLPSRCSR {
		// Run after merge so any CSR that became empty by losing its
		// children to a sibling-merge also dissolves. Two passes
		// (CSR first, then PSR) so a PSR whose only child is the
		// just-dropped empty CSR clears in the second pass.
		stripEmptyIDMLPSRCSRInTree(root, "CharacterStyleRange")
		stripEmptyIDMLPSRCSRInTree(root, "ParagraphStyleRange")
	}
	var out []xml.Token
	emitXMLNode(root, &out, true /*topLevel*/, opts.sortChildren)
	return out
}

// stripIDMLACEPIsInTree drops `<?ACE N?>` ProcessingInstruction
// children from every node in the tree. Adobe Composition Engine
// PIs are layout markers, not content; native and okapi disagree on
// where exactly inside the surrounding text they re-emit, so the
// bytes diverge even though the IDML semantics are identical. See
// the StripIDMLACEPIs doc on XMLCanonical for the full rationale.
func stripIDMLACEPIsInTree(node *xmlNode) {
	if node == nil {
		return
	}
	kept := node.children[:0]
	for _, c := range node.children {
		if c.sub == nil {
			if pi, ok := c.raw.(xml.ProcInst); ok && pi.Target == "ACE" {
				continue
			}
			kept = append(kept, c)
			continue
		}
		stripIDMLACEPIsInTree(c.sub)
		kept = append(kept, c)
	}
	node.children = kept
}

// stripEmptyIDMLContentInTree drops `<Content>` element children
// that have no CharData with non-whitespace bytes. Native's IDML
// writer emits the `<Content xml:space="preserve">` wrapper around
// every Content node it walked (including the degenerate empty case
// e.g. `<Content/>` after a `<Table>`); okapi's IDML pipeline drops
// these on round-trip. Stripping on both sides cancels the
// asymmetry without affecting any translatable content.
func stripEmptyIDMLContentInTree(node *xmlNode) {
	if node == nil {
		return
	}
	kept := node.children[:0]
	for _, c := range node.children {
		if c.sub == nil {
			kept = append(kept, c)
			continue
		}
		if c.sub.start.Name.Local == "Content" && isEmptyContentNode(c.sub) {
			continue
		}
		stripEmptyIDMLContentInTree(c.sub)
		kept = append(kept, c)
	}
	node.children = kept
}

// stripEmptyIDMLPSRCSRInTree drops every element of the given local
// name whose subtree is "content-empty" — has no `<Content>` with
// CharData, no `<Br>`, no `<TextFrame>`, no `<HyperlinkTextSource>`,
// no `<Footnote>`/`<Endnote>` and no other meaningful text-bearing
// element. Used to dissolve `<ParagraphStyleRange>` and
// `<CharacterStyleRange>` shells that wrap nothing visible. Okapi's
// IDML pipeline drops these on round-trip; native preserves them.
func stripEmptyIDMLPSRCSRInTree(node *xmlNode, name string) {
	if node == nil {
		return
	}
	out := node.children[:0]
	for _, c := range node.children {
		if c.sub != nil {
			stripEmptyIDMLPSRCSRInTree(c.sub, name)
			if c.sub.start.Name.Local == name && isContentEmptySubtree(c.sub) {
				continue
			}
		}
		out = append(out, c)
	}
	node.children = out
}

// isContentEmptySubtree reports whether a node's subtree carries no
// visible text-bearing element. Recursively checks all element
// descendants for any text-meaningful element name; returns false on
// the first hit.
func isContentEmptySubtree(node *xmlNode) bool {
	for _, c := range node.children {
		if c.sub == nil {
			continue
		}
		switch c.sub.start.Name.Local {
		case "Content":
			// A Content with any CharData (even whitespace) or a
			// non-empty subtree counts as visible.
			if !isEmptyContentNode(c.sub) {
				return false
			}
			// CharData-empty Content within a wrapper still doesn't
			// resurrect the wrapper — fall through.
		case "Br", "TextFrame", "HyperlinkTextSource", "Footnote", "Endnote", "Note", "Table", "Cell":
			return false
		default:
			if !isContentEmptySubtree(c.sub) {
				return false
			}
		}
	}
	return true
}

// dropIDMLElementsInTree walks the tree and removes every element
// (and its subtree) whose local name matches `name`. Like
// unwrapIDMLElementsInTree but the children disappear too — used
// for XML projection elements (XMLComment, XMLInstruction) that
// okapi strips wholesale on round-trip.
func dropIDMLElementsInTree(node *xmlNode, name string) {
	if node == nil {
		return
	}
	out := node.children[:0]
	for _, c := range node.children {
		if c.sub != nil && c.sub.start.Name.Local == name {
			continue
		}
		if c.sub != nil {
			dropIDMLElementsInTree(c.sub, name)
		}
		out = append(out, c)
	}
	node.children = out
}

// unwrapIDMLElementsInTree walks the tree and replaces every
// element whose local name matches `name` with its children inline.
// Used to drop IDML wrappers like `<XMLElement>` and `<Change>`
// that okapi unwraps on round-trip but native preserves. The
// wrapper's start/end tokens disappear; its children are spliced
// into the parent's children at the wrapper's slot. Recurses
// into the unwrapped children too in case wrappers nest.
func unwrapIDMLElementsInTree(node *xmlNode, name string) {
	if node == nil {
		return
	}
	out := node.children[:0]
	for _, c := range node.children {
		if c.sub == nil {
			out = append(out, c)
			continue
		}
		if c.sub.start.Name.Local == name {
			// Recurse into the unwrapped subtree first so any nested
			// wrappers also dissolve.
			unwrapIDMLElementsInTree(c.sub, name)
			out = append(out, c.sub.children...)
			continue
		}
		unwrapIDMLElementsInTree(c.sub, name)
		out = append(out, c)
	}
	node.children = out
}

// isEmptyContentNode reports whether a `<Content>` node has zero
// content of any kind: no element children, no CharData (not even
// whitespace), no ProcessingInstructions. The strict "truly empty"
// check matches okapi's drop behavior for the `<Content/>` placeholder
// that InDesign sometimes leaves after a `<Table>`. A
// whitespace-only `<Content xml:space="preserve"> </Content>` is
// LEFT IN PLACE: it carries a load-bearing inter-word space that the
// per-CSR adjacent-Content merge will fuse into the surrounding text
// runs (e.g. CSR-with-`text` + CSR-with-` ` merge to "text ").
func isEmptyContentNode(node *xmlNode) bool {
	for _, c := range node.children {
		if c.sub != nil {
			return false
		}
		if cd, ok := c.raw.(xml.CharData); ok {
			if len(cd) > 0 {
				return false
			}
		}
	}
	return true
}

// xmlNode is a tree representation of a buffered token slice. For
// element nodes, Children holds the in-order child token slots:
// xmlNode entries for sub-elements (recursive) and rawToken entries
// for non-element children (CharData / Comment / ProcInst / …).
type xmlNode struct {
	// start is the StartElement token for this node. For the synthetic
	// root, start.Name.Local is empty and the End/Children apply to
	// the top-level stream.
	start    xml.StartElement
	end      xml.EndElement
	children []xmlChild
}

// xmlChild is a discriminated union: either a child element (sub) or
// a raw passthrough token (raw, e.g. xml.CharData).
type xmlChild struct {
	sub *xmlNode
	raw xml.Token // nil when sub != nil
}

// buildXMLNode reads tokens starting at *idx and builds the children
// of the current element until it sees the matching EndElement (or
// end of stream when topLevel). The decoder/canonical pass guarantees
// the token slice is well-balanced.
func buildXMLNode(tokens []xml.Token, idx *int, topLevel bool) *xmlNode {
	node := &xmlNode{}
	for *idx < len(tokens) {
		tok := tokens[*idx]
		switch t := tok.(type) {
		case xml.StartElement:
			*idx++
			child := buildXMLNode(tokens, idx, false)
			child.start = t
			node.children = append(node.children, xmlChild{sub: child})
		case xml.EndElement:
			node.end = t
			*idx++
			return node
		default:
			node.children = append(node.children, xmlChild{raw: tok})
			*idx++
		}
	}
	if !topLevel {
		// Stream ended mid-element — return what we have. The encoder
		// will fail on an unbalanced End anyway, so signalling via the
		// returned node is enough.
	}
	return node
}

// emitXMLNode appends the token stream representing node (and its
// recursively-sorted children) to out. When topLevel is true, the
// node is the synthetic root: skip emitting its StartElement /
// EndElement and don't sort its top-level children (the document's
// root element sits there as a sole child anyway).
func emitXMLNode(node *xmlNode, out *[]xml.Token, topLevel, sortChildren bool) {
	if !topLevel {
		*out = append(*out, node.start)
	}
	children := node.children
	if sortChildren && !topLevel {
		children = sortXMLChildElements(children)
	}
	for _, c := range children {
		if c.sub != nil {
			emitXMLNode(c.sub, out, false, sortChildren)
			continue
		}
		*out = append(*out, c.raw)
	}
	if !topLevel {
		*out = append(*out, node.end)
	}
}

// mergeAdjacentCSRsInTree walks the tree depth-first, finding runs of
// consecutive <CharacterStyleRange> sibling elements whose attribute
// sets are identical and merging each run into a single CSR. Non-CSR
// children act as "barriers" — a CharData or non-CSR element between
// two CSRs ends the current run. The merged CSR retains the first
// run member's start/end tokens; its children are the concatenation
// of all run members' children (in source order). Recurses into the
// merged children, then into the remaining (non-CSR) children.
//
// Why this matters for IDML: the source format splits runs of styled
// text into multiple CSRs each time a formatting attribute toggles,
// even across boundaries that round-trip back to the same effective
// style. okapi's IDML pipeline rewrites these by emitting one CSR
// per *style*, not one per source range; native preserves the source
// shape. After this merge pass both forms canonicalise to the same
// "one CSR per distinct style" tree.
func mergeAdjacentCSRsInTree(node *xmlNode, mergeDefaultCSRs bool) {
	if node == nil || len(node.children) == 0 {
		return
	}
	merged := make([]xmlChild, 0, len(node.children))
	i := 0
	for i < len(node.children) {
		c := node.children[i]
		if c.sub == nil || !isCSR(c.sub) {
			merged = append(merged, c)
			i++
			continue
		}
		// Start of a potential CSR run. Walk forward while the next
		// sibling is also a CSR whose attrs are equal to the run's
		// current effective attrs (or, when mergeDefaultCSRs is set,
		// either side is a default-only CSR — see isDefaultOnlyCSR).
		// Mirrors upstream StoryChildElementsMerger.canStyleRangesBeMerged
		// (StoryChildElementsMerger.java:215-218) which gates the merge
		// on BOTH attribute equality AND `<Properties>` equality. CSRs
		// with same attrs but distinct Properties are NOT merged
		// (their visual style genuinely differs).
		runStart := i
		runEnd := i + 1 // exclusive
		runAttrs := c.sub.start.Attr
		runPropsRef := csrPropertiesChildren(c.sub)
		for runEnd < len(node.children) {
			next := node.children[runEnd]
			if next.sub == nil || !isCSR(next.sub) {
				break
			}
			nextProps := csrPropertiesChildren(next.sub)
			propsCompatible := samePropertiesList(runPropsRef, nextProps)
			if sameXMLAttrs(runAttrs, next.sub.start.Attr) && propsCompatible {
				runEnd++
				continue
			}
			// Default-only CSR is merge-transparent (okapi's
			// StoryChildElementsParser:132-136 makes bare contents
			// inherit the previous CSR's style). Surviving attrs come
			// from the non-default neighbor. Properties still gate
			// the merge: a default-only CSR without Properties is
			// transparent (gets the neighbor's), but if both sides
			// carry distinct Properties the merge is unsafe.
			if mergeDefaultCSRs && isDefaultOnlyCSRAttrs(runAttrs) && propsCompatible {
				runAttrs = next.sub.start.Attr
				if len(runPropsRef) == 0 {
					runPropsRef = nextProps
				}
				runEnd++
				continue
			}
			if mergeDefaultCSRs && isDefaultOnlyCSRAttrs(next.sub.start.Attr) && propsCompatible {
				runEnd++
				continue
			}
			break
		}
		if runEnd-runStart == 1 {
			// Singleton run — nothing to merge across CSR siblings.
			// Still apply the per-CSR content merge so adjacent
			// `<Content>` children of this single CSR collapse: okapi's
			// StoryChildElementsMerger:138-143 always fuses them
			// regardless of whether neighboring CSRs were merged.
			if mergeDefaultCSRs {
				c.sub.children = mergeAdjacentContentsInCSR(c.sub.children)
			}
			merged = append(merged, c)
			i++
			continue
		}
		// Merge: keep the first CSR's start/end; concatenate all
		// children. head.start.Attr becomes runAttrs (the surviving
		// non-default attribute set, or the original). Properties
		// children are deduplicated — every merged CSR carried the
		// same `<Properties>` (the merge gate above proved it), so
		// keeping one copy reproduces upstream's per-CSR Properties
		// emission (StoryChildElementsWriter writes one CSR with one
		// Properties even when N source CSRs were merged).
		head := node.children[runStart].sub
		head.start.Attr = runAttrs
		var combined []xmlChild
		var savedProps []xmlChild
		appendNonProperties := func(children []xmlChild) {
			for _, ch := range children {
				if ch.sub != nil && ch.sub.start.Name.Local == "Properties" {
					if savedProps == nil {
						savedProps = []xmlChild{ch}
					}
					continue
				}
				combined = append(combined, ch)
			}
		}
		appendNonProperties(head.children)
		for k := runStart + 1; k < runEnd; k++ {
			appendNonProperties(node.children[k].sub.children)
		}
		// Re-attach the single Properties at the end so adjacent
		// Contents become genuinely adjacent for
		// mergeAdjacentContentsInCSR. SortChildElements (sort-children)
		// later re-orders alphabetically anyway, so trailing position
		// is canonicalisation-safe.
		combined = append(combined, savedProps...)
		head.children = combined
		if mergeDefaultCSRs {
			// Same-style adjacent Contents are fused into one by okapi's
			// StoryChildElementsMerger:138-143; mirror that here.
			head.children = mergeAdjacentContentsInCSR(head.children)
		}
		merged = append(merged, xmlChild{sub: head})
		i = runEnd
	}
	node.children = merged
	// Recurse into all element children (post-merge so merged CSRs are
	// processed too — useful if a merged CSR's combined children have
	// their own nested CSR runs that now line up).
	for _, c := range node.children {
		if c.sub != nil {
			mergeAdjacentCSRsInTree(c.sub, mergeDefaultCSRs)
		}
	}
}

// csrPropertiesChildren returns the `<Properties>` element children of
// a CSR node. A CSR carries at most one `<Properties>` per upstream's
// data model (one per StyleRange), but native may emit zero or more
// depending on whether the source IDML had a Properties block; we
// return them all so the equality check is faithful.
func csrPropertiesChildren(n *xmlNode) []*xmlNode {
	if n == nil {
		return nil
	}
	var out []*xmlNode
	for _, c := range n.children {
		if c.sub != nil && c.sub.start.Name.Local == "Properties" {
			out = append(out, c.sub)
		}
	}
	return out
}

// samePropertiesList reports whether two `<Properties>` lists are
// structurally equal (same number of Properties, each pairwise
// structurally equal). Order-sensitive — Okapi emits a stable order
// and never reorders within a single StyleRange's Properties.
func samePropertiesList(a, b []*xmlNode) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !sameXMLSubtree(a[k], b[k]) {
			return false
		}
	}
	return true
}

// sameXMLSubtree reports whether two element subtrees are structurally
// equal: same name, same attribute set (order-insensitive), and
// recursively equal children. Raw children (CharData / Comments /
// ProcInsts) compare by exact byte content.
func sameXMLSubtree(a, b *xmlNode) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.start.Name.Space != b.start.Name.Space ||
		a.start.Name.Local != b.start.Name.Local {
		return false
	}
	if !sameXMLAttrs(a.start.Attr, b.start.Attr) {
		return false
	}
	if len(a.children) != len(b.children) {
		return false
	}
	for k := range a.children {
		ac, bc := a.children[k], b.children[k]
		if (ac.sub == nil) != (bc.sub == nil) {
			return false
		}
		if ac.sub != nil {
			if !sameXMLSubtree(ac.sub, bc.sub) {
				return false
			}
			continue
		}
		// raw token comparison
		switch av := ac.raw.(type) {
		case xml.CharData:
			bv, ok := bc.raw.(xml.CharData)
			if !ok || string(av) != string(bv) {
				return false
			}
		case xml.Comment:
			bv, ok := bc.raw.(xml.Comment)
			if !ok || string(av) != string(bv) {
				return false
			}
		case xml.ProcInst:
			bv, ok := bc.raw.(xml.ProcInst)
			if !ok || av.Target != bv.Target || string(av.Inst) != string(bv.Inst) {
				return false
			}
		default:
			// Unknown raw token type — be conservative and reject.
			return false
		}
	}
	return true
}

// idmlDefaultCharacterStyle matches okapi's StyleRange.java:44
// `CHARACTER_STYLE_DEFAULT_VALUE` — the placeholder for "no character
// style applied". A CSR with only this attribute carries no styling.
const idmlDefaultCharacterStyle = "CharacterStyle/$ID/[No character style]"

// isDefaultOnlyCSRAttrs reports whether attrs is exactly the single
// `AppliedCharacterStyle="CharacterStyle/$ID/[No character style]"`
// attribute and nothing else.
func isDefaultOnlyCSRAttrs(attrs []xml.Attr) bool {
	if len(attrs) != 1 {
		return false
	}
	a := attrs[0]
	return a.Name.Local == "AppliedCharacterStyle" && a.Value == idmlDefaultCharacterStyle
}

// mergeAdjacentContentsInCSR collapses runs of consecutive `<Content>`
// element siblings into a single `<Content>` whose CharData is the
// concatenation of the merged Contents' text. Non-Content children act
// as barriers. Mirrors StoryChildElementsMerger.java:138-143.
func mergeAdjacentContentsInCSR(children []xmlChild) []xmlChild {
	if len(children) < 2 {
		return children
	}
	out := make([]xmlChild, 0, len(children))
	i := 0
	for i < len(children) {
		c := children[i]
		if c.sub == nil || c.sub.start.Name.Local != "Content" {
			out = append(out, c)
			i++
			continue
		}
		runEnd := i + 1
		for runEnd < len(children) {
			n := children[runEnd]
			if n.sub == nil || n.sub.start.Name.Local != "Content" {
				break
			}
			runEnd++
		}
		if runEnd-i == 1 {
			out = append(out, c)
			i++
			continue
		}
		head := c.sub
		var combinedText []byte
		for k := i; k < runEnd; k++ {
			for _, gc := range children[k].sub.children {
				if gc.sub == nil {
					if cd, ok := gc.raw.(xml.CharData); ok {
						combinedText = append(combinedText, cd...)
					}
				}
			}
		}
		head.children = []xmlChild{{raw: xml.CharData(combinedText)}}
		out = append(out, xmlChild{sub: head})
		i = runEnd
	}
	return out
}

// isCSR reports whether the given node is a <CharacterStyleRange>
// element (matched by local name only; namespace is ignored to avoid
// false negatives on documents that decorate the IDML namespace
// differently).
func isCSR(n *xmlNode) bool {
	return n != nil && n.start.Name.Local == "CharacterStyleRange"
}

// sameXMLAttrs reports whether two attribute slices represent the
// same set of (name, value) pairs. Order is ignored — we sort
// canonical copies before comparing — so attribute reordering
// elsewhere in the pipeline doesn't break the equality test.
func sameXMLAttrs(a, b []xml.Attr) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	ax := make([]xml.Attr, len(a))
	copy(ax, a)
	bx := make([]xml.Attr, len(b))
	copy(bx, b)
	sort.SliceStable(ax, func(i, j int) bool {
		if ax[i].Name.Space != ax[j].Name.Space {
			return ax[i].Name.Space < ax[j].Name.Space
		}
		return ax[i].Name.Local < ax[j].Name.Local
	})
	sort.SliceStable(bx, func(i, j int) bool {
		if bx[i].Name.Space != bx[j].Name.Space {
			return bx[i].Name.Space < bx[j].Name.Space
		}
		return bx[i].Name.Local < bx[j].Name.Local
	})
	for k := range ax {
		if ax[k].Name.Space != bx[k].Name.Space ||
			ax[k].Name.Local != bx[k].Name.Local ||
			ax[k].Value != bx[k].Value {
			return false
		}
	}
	return true
}

// sortXMLChildElements returns a copy of children with the element
// slots permuted so that the sequence of element names is
// alphabetical (by local name, stable). Non-element slots keep their
// position in the child sequence — i.e. if children is `[E1, raw,
// E2]`, the output is `[min(E1,E2), raw, max(E1,E2)]`. This preserves
// the relative position of CharData / Comments / ProcInsts to text
// content while letting elements rearrange among themselves.
func sortXMLChildElements(children []xmlChild) []xmlChild {
	if len(children) < 2 {
		return children
	}
	// Pull out element children in order; remember the slot indices.
	var elemSlots []int
	var elems []*xmlNode
	for i, c := range children {
		if c.sub != nil {
			elemSlots = append(elemSlots, i)
			elems = append(elems, c.sub)
		}
	}
	if len(elems) < 2 {
		return children
	}
	sort.SliceStable(elems, func(i, j int) bool {
		return elems[i].start.Name.Local < elems[j].start.Name.Local
	})
	out := make([]xmlChild, len(children))
	copy(out, children)
	for k, slot := range elemSlots {
		out[slot] = xmlChild{sub: elems[k]}
	}
	return out
}

// stripXMLNSAttrs removes xmlns="..." and xmlns:foo="..." attribute
// occurrences from rendered XML bytes. Cheap regexp-based pass — the
// encoder we use double-quotes attribute values, so the pattern is
// stable. Used by XMLCanonical{StripNamespaceDecls:true}.
var xmlnsAttrRE = regexp.MustCompile(` xmlns(:[A-Za-z_][A-Za-z0-9._-]*)?="[^"]*"`)

func stripXMLNSAttrs(b []byte) []byte {
	return xmlnsAttrRE.ReplaceAll(b, nil)
}

// collapseTextWS collapses runs of whitespace (space/tab/CR/LF) into a
// single space and trims leading/trailing whitespace. Used to mirror
// okapi's xliff-writer translatable-text normalisation.
func collapseTextWS(in []byte) []byte {
	if len(in) == 0 {
		return in
	}
	out := make([]byte, 0, len(in))
	prevWS := false
	for _, c := range in {
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			if !prevWS {
				out = append(out, ' ')
				prevWS = true
			}
			continue
		}
		out = append(out, c)
		prevWS = false
	}
	// Trim trailing space.
	if len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
	}
	// Trim leading space.
	if len(out) > 0 && out[0] == ' ' {
		out = out[1:]
	}
	return out
}

// normalizeAttrValue collapses XML 1.0 attribute-value whitespace per
// §3.3.3: literal CR, LF, TAB are replaced with a single space (they
// remain individual spaces, not collapsed). For CDATA-typed attributes
// (the default when no DTD declares otherwise) further collapsing of
// runs is not required by the spec, so we leave non-CR/LF/TAB
// whitespace alone.
func normalizeAttrValue(v string) string {
	if !strings.ContainsAny(v, "\r\n\t") {
		return v
	}
	out := make([]byte, len(v))
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\r' || c == '\n' || c == '\t' {
			out[i] = ' '
		} else {
			out[i] = c
		}
	}
	return string(out)
}

// JSONCanonical re-serializes JSON through encoding/json so two
// semantically-identical documents that differ in whitespace (e.g.
// `"key" : "value"` vs `"key": "value"`) come out byte-equal. The
// re-serialization uses encoding/json's compact form (no extra
// whitespace) and HTML-escapes `<`, `>`, `&` — both got and ref get
// the same treatment, so the difference cancels.
//
// Use on json fixtures whose source has unusual spacing around colons
// or whose pretty-printer differs from okapi's.
type JSONCanonical struct{}

// Name implements Normalizer.
func (JSONCanonical) Name() string { return "json-canonical" }

// Normalize implements Normalizer.
func (JSONCanonical) Normalize(in []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, fmt.Errorf("json-canonical: parse: %w", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json-canonical: re-serialize: %w", err)
	}
	return out, nil
}

// YAMLCanonical re-serializes YAML through gopkg.in/yaml.v3 so two
// documents that differ in flow vs block style, indentation,
// quote-character choice, or boolean/null spelling come out identical
// (or close enough that surviving differences are real).
//
// Important caveat: YAML's "what is a string vs keyword" decisions are
// driven by the source spelling — `true` is a boolean, `"true"` is a
// string. If native preserves `true` while okapi pseudo-translates it
// as `ţŕũē` (treating it as a translatable string), those are real
// semantic differences the normalizer can't bridge — both sides will
// re-serialize them differently. The normalizer only collapses
// stylistic noise.
type YAMLCanonical struct{}

// Name implements Normalizer.
func (YAMLCanonical) Name() string { return "yaml-canonical" }

// Normalize implements Normalizer.
func (YAMLCanonical) Normalize(in []byte) ([]byte, error) {
	// Parse via yaml.Decoder so multi-document streams (separated by
	// `---`) round-trip correctly — each doc gets its own re-emit.
	dec := yamlDecoder(in)
	var buf bytes.Buffer
	enc := yamlEncoder(&buf)
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("yaml-canonical: parse: %w", err)
		}
		if err := enc.Encode(v); err != nil {
			return nil, fmt.Errorf("yaml-canonical: re-serialize: %w", err)
		}
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("yaml-canonical: flush: %w", err)
	}
	return buf.Bytes(), nil
}

// HTMLCanonical re-serializes HTML through golang.org/x/net/html so two
// documents whose semantic structure agrees but whose byte form differs
// in attribute quoting (`width=300` vs `width="300"`), attribute order,
// inter-tag whitespace, void-element close style (`<br>` vs `<br/>`),
// or boilerplate injection (e.g. okapi's `<meta http-equiv="Content-Type">`)
// come out byte-identical.
//
// Important caveats:
//   - html.Parse normalizes the document tree: missing `<html>`, `<head>`,
//     `<body>` elements get auto-inserted; misplaced elements get reflowed.
//     Both engines' outputs go through the same parser, so the same
//     reflowing happens to each — for parity it cancels.
//   - Attributes are sorted alphabetically by namespace+name. Order
//     differences are not semantic, so this is safe.
//   - Inter-tag text nodes that are wholly ASCII whitespace are dropped.
//     This matches the okapi reformatter that emits one element per
//     line vs native preserving source's tab indentation.
//   - `<meta http-equiv="Content-Type">` and `<meta charset>` injected by
//     okapi's writer are dropped, since they're transport hints not
//     content. Authored meta tags with name= or property= are kept.
//
// Use on html fixtures whose semantic structure matches okapi's but
// whose byte form differs in formatting/quoting/injection.
type HTMLCanonical struct{}

// Name implements Normalizer.
func (HTMLCanonical) Name() string { return "html-canonical" }

// Normalize implements Normalizer.
func (HTMLCanonical) Normalize(in []byte) ([]byte, error) {
	doc, err := html.Parse(bytes.NewReader(in))
	if err != nil {
		return nil, fmt.Errorf("html-canonical: parse: %w", err)
	}
	canonicalizeHTMLNode(doc)
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return nil, fmt.Errorf("html-canonical: render: %w", err)
	}
	return buf.Bytes(), nil
}

// canonicalizeHTMLNode walks the parsed tree in place, applying:
//   - drop inter-element whitespace-only TextNode (html.Render preserves
//     them otherwise; different writers indent differently)
//   - drop transport-hint <meta> tags (charset / Content-Type) that some
//     writers inject and others omit
//   - collapse runs of whitespace inside text nodes to a single space
//     (HTML §13.5 already collapses these on render; native preserves
//     source CRLFs that okapi normalises away)
//   - sort each element's attributes alphabetically (case-insensitive,
//     by namespace+key) so order differences cancel
func canonicalizeHTMLNode(n *html.Node) {
	// First, recurse + collect children to drop. We can't drop while
	// iterating with the linked-list traversal that html uses.
	var toRemove []*html.Node
	var toUnwrap []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		canonicalizeHTMLNode(c)
		if c.Type == html.ElementNode && c.DataAtom == atom.Center {
			toUnwrap = append(toUnwrap, c)
			continue
		}
		if shouldDropHTMLNode(n, c) {
			toRemove = append(toRemove, c)
		} else if c.Type == html.TextNode {
			if isElementWithPreservedWhitespace(n) {
				// Inside script/style we still collapse whitespace
				// runs — script source whitespace is not visible
				// content, and okapi's reformatter routinely
				// rewrites the indentation around comment markers
				// (`<!--` / `//-->`) inside `<script>`.
				if isScriptLike(n) {
					c.Data = collapseHTMLTextWhitespace(c.Data)
					if c.Data == " " {
						c.Data = ""
					}
				}
				if c.Data == "" {
					toRemove = append(toRemove, c)
				}
			} else {
				c.Data = stripEscapedMetaText(c.Data)
				c.Data = collapseHTMLTextWhitespace(c.Data)
				// Trim leading whitespace when this text is the first
				// child of its parent (element-boundary whitespace
				// doesn't render). Same for trailing when it's the last
				// child. Avoids native preserving a source `\r\n` after
				// `<body>` while okapi strips it.
				if c.PrevSibling == nil {
					c.Data = strings.TrimLeft(c.Data, " ")
				}
				if c.NextSibling == nil {
					c.Data = strings.TrimRight(c.Data, " ")
				}
				if c.Data == "" {
					toRemove = append(toRemove, c)
				}
			}
		}
	}
	for _, c := range toRemove {
		n.RemoveChild(c)
	}
	// Unwrap purely presentational `<center>` elements: their
	// auto-close behavior under html.Parse is fragile around
	// `<P>` siblings, and they convey no translatable content.
	for _, c := range toUnwrap {
		unwrapHTMLElement(c)
	}
	// Drop empty inline elements `<b></b>`, `<i></i>`, `<u></u>`,
	// `<font></font>`, `<span></span>`, `<em></em>`, `<strong></strong>` —
	// they're noise from html.Parse auto-closing on malformed input
	// (e.g. `<B>` at top-level then a block element auto-closes the B).
	if n.Type == html.ElementNode {
		var emptyKids []*html.Node
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || c.FirstChild != nil {
				continue
			}
			if !isEmptyDroppable(c.DataAtom) {
				continue
			}
			// Drop unconditionally for purely-presentational inline
			// elements regardless of attrs — `<font size="5"></font>`,
			// `<span class="x"></span>`, `<i id="y"></i>` are all noise.
			// okapi's HTML reformatter sometimes emits these on
			// implicit-close interactions while native folds them away
			// (or vice versa); both are equally meaningless.
			emptyKids = append(emptyKids, c)
		}
		for _, c := range emptyKids {
			n.RemoveChild(c)
		}
	}
	if n.Type == html.ElementNode && len(n.Attr) > 0 {
		// Trim whitespace from attribute values — okapi strips
		// surrounding whitespace, native preserves source bytes.
		// Also drop attrs whose key is empty / pure punctuation —
		// e.g. malformed input `border=0; padding=0;` parses as
		// `border="0"`+`;=""`, and the duplicate `;=""` count
		// differs between native (preserves all source artifacts)
		// and okapi (collapses).
		filtered := n.Attr[:0]
		seen := map[string]struct{}{}
		for _, a := range n.Attr {
			a.Val = strings.TrimSpace(a.Val)
			if a.Key == "" || isPunctOnly(a.Key) {
				if _, ok := seen[a.Key]; ok {
					continue
				}
				seen[a.Key] = struct{}{}
			}
			filtered = append(filtered, a)
		}
		n.Attr = filtered
		if len(n.Attr) > 1 {
			sort.SliceStable(n.Attr, func(i, j int) bool {
				if n.Attr[i].Namespace != n.Attr[j].Namespace {
					return n.Attr[i].Namespace < n.Attr[j].Namespace
				}
				return n.Attr[i].Key < n.Attr[j].Key
			})
		}
	}
}

// isScriptLike reports whether n is a `<script>` or `<style>` element.
// `<pre>` and `<textarea>` content IS visible to the user, so we don't
// collapse whitespace inside them.
func isScriptLike(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.Script, atom.Style:
		return true
	}
	return false
}

// isPunctOnly reports whether s consists entirely of ASCII punctuation
// characters (no letters / digits). Used to detect malformed-attribute
// remnants like `;=""` that some parsers keep and others collapse.
func isPunctOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= 'A' && r <= 'Z':
			return false
		case r >= '0' && r <= '9':
			return false
		}
	}
	return true
}

// collapseHTMLTextWhitespace collapses runs of ASCII whitespace
// (space, tab, CR, LF) to a single space — matching HTML's own
// rendering rules (§13.5.1). Pre/script/style/textarea text never
// reaches this function (caller filters via isElementWithPreservedWhitespace).
func collapseHTMLTextWhitespace(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		switch r {
		case ' ', '\t', '\r', '\n':
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return b.String()
}

// shouldDropHTMLNode reports whether c (a child of parent) should be
// removed during canonicalization. Drop targets:
//   - whitespace-only text nodes that sit between elements (their
//     placement is presentation, not content)
//   - <meta charset> and <meta http-equiv="Content-Type"> tags, which
//     some writers inject for transport correctness and others omit
func shouldDropHTMLNode(parent, c *html.Node) bool {
	if c.Type == html.TextNode {
		// Drop whitespace-only text inside element nodes that aren't
		// CDATA-style content holders (script/style/pre/textarea
		// preserve whitespace).
		if isElementWithPreservedWhitespace(parent) {
			return false
		}
		if isASCIIWhitespace(c.Data) {
			return true
		}
		return false
	}
	if c.Type == html.ElementNode && c.DataAtom == atom.Meta {
		for _, a := range c.Attr {
			lk := strings.ToLower(a.Key)
			if lk == "charset" {
				return true
			}
			if lk == "http-equiv" && strings.EqualFold(a.Val, "Content-Type") {
				return true
			}
		}
		// Detect malformed `<META ...>` whose attributes contain
		// commas/quotes/embedded HTML — these come from source like
		// `<META KEYWORDS="x","y","z">` which okapi treats as text
		// content (and entity-escapes back into the body). Both
		// representations are noise; drop the native side and rely
		// on `shouldDropMETAEscapeText` to drop the ref side.
		if hasMalformedMetaAttr(c.Attr) {
			return true
		}
	}
	// Drop `<link>` elements entirely. Their placement in the
	// document tree (head vs body) varies depending on whether the
	// preceding sibling was an element or text — html.Parse moves
	// `<link>` to body once it sees text content in head. Sources
	// with malformed `<META>` (which okapi escapes as text and
	// native preserves as element) cause the ref/native split. The
	// `<link>` itself is transport metadata, not translation
	// payload, so drop it on both sides.
	if c.Type == html.ElementNode && c.DataAtom == atom.Link {
		return true
	}
	// Drop `<hr>` elements — pure presentation, no translatable
	// content. okapi's reformatter can change their position
	// relative to surrounding `<p>` and `<center>` elements
	// (parser auto-closing rules differ between implementations
	// on `<P><HR><P>` style soup).
	if c.Type == html.ElementNode && c.DataAtom == atom.Hr {
		return true
	}
	// Drop `<noscript>` elements — content is JS-fallback markup
	// that some readers extract internal text/attrs from (okapi via
	// NekoHTML in scripting-disabled mode) while others (native)
	// treat as opaque per the HTML5 scripting-enabled tokenisation
	// rule (golang.org/x/net/html). Both behaviours are spec-defensible;
	// the divergence is a pure config knob, not a bug.
	if c.Type == html.ElementNode && c.DataAtom == atom.Noscript {
		return true
	}
	return false
}

// isEmptyDroppable reports whether an empty (no children, no attrs)
// element of this kind is safe to drop in canonical comparison.
// Inline formatting elements only — block elements like `<div>`
// have semantic meaning even when empty. `<p>` is included because
// okapi's HTML reformatter treats source `<P>` as a separator (it
// emits `<p></p>` whereas native folds the implicit closure into
// the next block element).
func isEmptyDroppable(a atom.Atom) bool {
	switch a {
	case atom.B, atom.I, atom.U, atom.Em, atom.Strong, atom.Span, atom.Font, atom.P:
		return true
	}
	return false
}

// unwrapHTMLElement removes element c from its parent but keeps c's
// children, re-parented in c's slot. Used to flatten purely
// presentational wrappers (`<center>`, `<font>`) whose only role is
// styling. After unwrapping, the surrounding text/inline runs
// canonicalise as if the wrapper was never there.
func unwrapHTMLElement(c *html.Node) {
	parent := c.Parent
	if parent == nil {
		return
	}
	for c.FirstChild != nil {
		child := c.FirstChild
		c.RemoveChild(child)
		parent.InsertBefore(child, c)
	}
	parent.RemoveChild(c)
}

// hasMalformedMetaAttr reports whether any META attr's key contains a
// character that would never appear in a real HTML attribute name —
// quotes, commas, equals, etc. html.Parse coerces a malformed
// `<META KEYWORDS="x","y">` into a meta element with attribute keys
// like `,"y"` and similar — clear signals the source META wasn't
// well-formed. okapi's tagsoup parser bails on these and emits the
// whole tag as text in the body; native preserves the malformed
// attrs. For canonical comparison we collapse both forms.
func hasMalformedMetaAttr(attrs []html.Attribute) bool {
	for _, a := range attrs {
		for _, r := range a.Key {
			switch r {
			case '"', '\'', ',', '=', '<', '>':
				return true
			}
		}
	}
	return false
}

// stripEscapedMetaText removes `<META …>` substrings (case-insensitive,
// pseudo-translated tag-name allowed) from text. okapi's HTML reader
// emits malformed `<META KEYWORDS="x","y","z">` source as escaped text
// in the body; native parses it as a meta element. The native side is
// dropped via shouldDropHTMLNode + hasMalformedMetaAttr; this handles
// the symmetric ref-side text.
//
// Also strips a single trailing space after the `>` so that
// "<META …> rest" and "rest" align byte-for-byte.
func stripEscapedMetaText(s string) string {
	if s == "" {
		return s
	}
	out := s
	for {
		idx := indexEscapedMetaTag(out)
		if idx < 0 {
			break
		}
		end := strings.IndexByte(out[idx:], '>')
		if end < 0 {
			break
		}
		end += idx + 1
		// Eat one trailing space if present.
		if end < len(out) && out[end] == ' ' {
			end++
		}
		out = out[:idx] + out[end:]
	}
	return out
}

// indexEscapedMetaTag returns the byte offset of the first `<META`
// (or pseudo-translated `<MĒŢÀ` etc.) in s. Matches case-insensitively
// for ASCII letters and accepts the common pseudo-translation
// substitutions for M/E/T/A.
func indexEscapedMetaTag(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			continue
		}
		// Need at least `<META` worth of bytes (5 ASCII bytes,
		// or up to ~12 if all four letters are 2-byte UTF-8).
		rest := s[i+1:]
		if isMETALiteral(rest) {
			return i
		}
	}
	return -1
}

// isMETALiteral reports whether s starts with the four characters
// M, E, T, A in that order — accepting either the ASCII letter or
// any single Unicode codepoint (the pseudo-translation substitutes
// each letter with one accented variant). After the four characters
// the next byte must be a space, `=`, `>`, `/`, tab, or newline.
func isMETALiteral(s string) bool {
	want := []rune{'M', 'E', 'T', 'A'}
	idx := 0
	for _, w := range want {
		if idx >= len(s) {
			return false
		}
		r, sz := utf8.DecodeRuneInString(s[idx:])
		if r == utf8.RuneError {
			return false
		}
		// Accept the ASCII letter or any single non-ASCII rune as
		// the pseudo-translated stand-in. We don't try to match a
		// specific accented letter — the pseudo map varies.
		if !(r == w || r > 127) {
			return false
		}
		idx += sz
	}
	if idx >= len(s) {
		return false
	}
	switch s[idx] {
	case ' ', '\t', '\n', '\r', '=', '>', '/':
		return true
	}
	return false
}

func isElementWithPreservedWhitespace(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Pre:
		return true
	}
	// `<textarea>` is intentionally not in this list. Per HTML5
	// §13.2.5.4 (RAWTEXT/RCDATA states) it preserves whitespace in the
	// browser, but Okapi's HtmlFilter classifies it as INLINE in
	// wellformedConfiguration.yml — so its content text is folded into
	// the surrounding TEXTUNIT and gets standard HTML whitespace
	// collapsing on extraction. Native classifies textarea as
	// preserve-whitespace per HTML5; canonicalising both sides through
	// whitespace collapse cancels the configuration divergence (parity
	// contract: "same semantic config → same results", #557).
	return false
}

func isASCIIWhitespace(s string) bool {
	for _, r := range s {
		switch r {
		case ' ', '\t', '\r', '\n':
		default:
			return false
		}
	}
	return true
}

// Chain composes multiple normalizers, applying them in sequence.
// Each normalizer's output is fed to the next.
type Chain struct {
	Steps []Normalizer
}

// Name implements Normalizer.
func (c Chain) Name() string {
	parts := make([]string, 0, len(c.Steps))
	for _, s := range c.Steps {
		parts = append(parts, s.Name())
	}
	return "chain[" + strings.Join(parts, "+") + "]"
}

// Normalize implements Normalizer.
func (c Chain) Normalize(in []byte) ([]byte, error) {
	cur := in
	for _, s := range c.Steps {
		out, err := s.Normalize(cur)
		if err != nil {
			return nil, err
		}
		cur = out
	}
	return cur, nil
}

// StripXMLDeclaration removes the leading `<?xml ... ?>` processing
// instruction from the input without otherwise touching it. Useful
// when two writers emit the same XML body but disagree on the decl
// quote style (single vs double quotes), encoding token case
// (utf-8 vs UTF-8), standalone attribute, or whether the decl is
// emitted at all. Both forms are valid XML; the decl is metadata.
type StripXMLDeclaration struct{}

// Name implements Normalizer.
func (StripXMLDeclaration) Name() string { return "strip-xml-decl" }

var xmlDeclRE = regexp.MustCompile(`(?s)\A(?:\xef\xbb\xbf)?\s*<\?xml[^?]*\?>\s*`)

// Normalize implements Normalizer.
func (StripXMLDeclaration) Normalize(in []byte) ([]byte, error) {
	return xmlDeclRE.ReplaceAll(in, nil), nil
}

// ZipEntryNormalizer applies an inner Normalizer to each entry of a
// zip archive, then re-zips with deterministic metadata. Used for
// zip-of-XML formats (idml, openxml, …) where the per-entry content
// would canonicalize cleanly under the inner normalizer but the
// outer zip metadata (mtime, central-dir order, compression level)
// differs between writers.
//
// The post-normalize comparison goes back through compareZip
// (compare.go honours isZip after normalize), so this only needs to
// produce a valid zip whose entries match the inner normalization —
// the outer byte-stream doesn't have to be byte-identical.
//
// XML-only entries (.xml/.rels) and other text-XML are passed through
// the inner normalizer; any entry that fails normalization (e.g.
// binary content like .png inside the zip) is passed through verbatim
// so we don't lose the entry.
type ZipEntryNormalizer struct {
	Inner Normalizer
}

// Name implements Normalizer.
func (z ZipEntryNormalizer) Name() string {
	if z.Inner == nil {
		return "zip-entries"
	}
	return "zip-entries[" + z.Inner.Name() + "]"
}

// Normalize implements Normalizer.
func (z ZipEntryNormalizer) Normalize(in []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(in), int64(len(in)))
	if err != nil {
		return nil, fmt.Errorf("zip-entries: open: %w", err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("zip-entries: open %q: %w", f.Name, err)
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("zip-entries: read %q: %w", f.Name, err)
		}
		body := raw
		if z.Inner != nil && isLikelyXML(f.Name, raw) {
			normed, err := z.Inner.Normalize(raw)
			if err == nil {
				body = normed
			}
			// On normalize error, fall through with raw bytes — keeps
			// the comparison meaningful for entries whose content the
			// inner normalizer can't handle.
		}
		// Write with stored compression + zero mtime so metadata is
		// stable. The compareZip ignores zip metadata anyway, so this
		// is belt-and-braces.
		entry, err := w.CreateHeader(&zip.FileHeader{
			Name:   f.Name,
			Method: zip.Store,
		})
		if err != nil {
			return nil, fmt.Errorf("zip-entries: create %q: %w", f.Name, err)
		}
		if _, err := entry.Write(body); err != nil {
			return nil, fmt.Errorf("zip-entries: write %q: %w", f.Name, err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("zip-entries: close: %w", err)
	}
	return out.Bytes(), nil
}

// isLikelyXML reports whether the given zip entry should be treated
// as XML for normalization. We check the filename suffix and a quick
// content sniff on the first non-whitespace bytes.
func isLikelyXML(name string, body []byte) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".xml"),
		strings.HasSuffix(lower, ".rels"),
		strings.HasSuffix(lower, ".xhtml"):
		return true
	}
	// Quick sniff: first non-whitespace bytes start with `<`.
	for _, b := range body {
		switch b {
		case ' ', '\t', '\r', '\n', 0xef, 0xbb, 0xbf: // skip whitespace + UTF-8 BOM
			continue
		case '<':
			return true
		default:
			return false
		}
	}
	return false
}

// POCharsetCase folds case differences in PO `charset=...` values.
// okapi normalises `charset=utf-8` → `charset=UTF-8` on round-trip;
// native preserves source case. Both are valid per RFC 2046 (§2.1
// "Character set names are case-insensitive").
type POCharsetCase struct{}

// Name implements Normalizer.
func (POCharsetCase) Name() string { return "po-charset-case" }

var poCharsetRE = regexp.MustCompile(`(?i)charset=[A-Za-z0-9_\-]+`)

// Normalize implements Normalizer.
func (POCharsetCase) Normalize(in []byte) ([]byte, error) {
	return poCharsetRE.ReplaceAllFunc(in, func(m []byte) []byte {
		return bytes.ToUpper(m)
	}), nil
}

// POJoinContinuations folds PO multi-line string continuations into
// single-line strings. PO allows splitting one msgid/msgstr value
// across multiple `"..."` chunks separated by newlines:
//
//	msgstr ""
//	"Project-Id-Version: Foo\n"
//	"Last-Translator: Bar\n"
//
// is semantically equivalent to:
//
//	msgstr "Project-Id-Version: Foo\nLast-Translator: Bar\n"
//
// Both engines emit valid PO but disagree on where to split — okapi
// rewraps at its own boundaries, native preserves source splits. This
// normalizer joins all `"\n"` (close-quote, newline, optional indent,
// open-quote) sequences into nothing, collapsing both forms to a
// single canonical join. Compatible with multi-line msgstr/msgid only;
// regular comment / instruction lines aren't touched because they
// don't carry the inter-line `"` `"` join pattern.
type POJoinContinuations struct{}

// Name implements Normalizer.
func (POJoinContinuations) Name() string { return "po-join-continuations" }

// poJoinRE matches a closing quote, line terminator (LF or CRLF),
// optional leading whitespace, and an opening quote — the `"` `"`
// continuation join. Replaces the whole match with nothing so the
// two `"..."` chunks are stitched together.
var poJoinRE = regexp.MustCompile(`"\r?\n[ \t]*"`)

// Normalize implements Normalizer.
func (POJoinContinuations) Normalize(in []byte) ([]byte, error) {
	return poJoinRE.ReplaceAll(in, nil), nil
}

// VTTCueFlattenWS folds in-cue word-wrapped lines back into a single
// line per cue body and collapses runs of blank lines between cues
// into a single blank line. okapi's WebVTT writer wraps cue text at
// maxCharsPerLine (defaults around 47 chars) on round-trip and pads
// empty-text cues with an extra blank line; native preserves the
// source line shape. Both render the same WebVTT semantics — collapse
// soft line breaks inside a cue body and squash inter-cue blank-line
// runs so the byte shape lines up for canonical comparison.
//
// A cue body is the run of non-blank lines that follows a timing line
// (one containing `-->`) and ends at the next blank line. Cue
// identifier lines, the `WEBVTT` header, NOTE / STYLE / REGION
// blocks, and metadata before the first timing line are left
// untouched.
type VTTCueFlattenWS struct{}

// Name implements Normalizer.
func (VTTCueFlattenWS) Name() string { return "vtt-cue-flatten-ws" }

// Normalize implements Normalizer.
func (VTTCueFlattenWS) Normalize(in []byte) ([]byte, error) {
	lines := strings.Split(string(in), "\n")
	out := make([]string, 0, len(lines))
	inCueBody := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if trimmed == "" {
			inCueBody = false
			// Skip consecutive blank lines — okapi pads empty cues with
			// a second blank line on output; collapse to one.
			if len(out) > 0 && strings.TrimRight(out[len(out)-1], "\r") == "" {
				continue
			}
			out = append(out, line)
			continue
		}
		if strings.Contains(trimmed, "-->") {
			inCueBody = true
			out = append(out, line)
			continue
		}
		if inCueBody && len(out) > 0 && strings.TrimRight(out[len(out)-1], "\r") != "" {
			// Append to previous body line with a single space separator,
			// preserving the previous line's CR if it had one.
			prev := out[len(out)-1]
			cr := ""
			if strings.HasSuffix(prev, "\r") {
				prev = strings.TrimSuffix(prev, "\r")
				cr = "\r"
			}
			out[len(out)-1] = prev + " " + trimmed + cr
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n")), nil
}

// DTDCanonical re-emits a DTD through a tiny tokeniser that yields one
// declaration (comment / `<!ENTITY ...>` / `<!ELEMENT ...>` / `<!ATTLIST
// ...>` / etc.) per line, with whitespace inside each declaration
// canonicalised. Used to bridge the byte gap between the native
// skeleton-driven writer (which preserves source bytes verbatim) and
// okapi's DTDFilter, which re-parses through `com.wutka.dtd.DTDParser`
// and re-serialises through `DTDOutput` — that round-trip rewrites
// quote style, spacing inside parens, comma/pipe spacing, leading
// per-line whitespace, and inlines parameter-entity references.
//
// Normalisations applied per declaration body:
//   - quotes around literal values folded to `"` (okapi always emits
//     double quotes; source may use single).
//   - runs of internal whitespace collapsed to a single space, then
//     space adjacent to `,` `(` `)` `|` removed (so `(a, b)` and
//     `(a,b)` and `( a | b )` all canonicalise to the same shape).
//   - parameter-entity references (`%name;`) substituted with the
//     value of the matching `<!ENTITY % name "value">` declaration
//     seen earlier in the same document. okapi's DTDOutput inlines
//     these on serialisation; native preserves the reference. Both
//     forms collapse to the substituted text.
//   - trailing whitespace before the closing `>` stripped.
//   - leading per-line whitespace stripped (outside comments — comments
//     keep their internal indentation since their text is content,
//     not declaration syntax).
//
// Comments are emitted verbatim. Blank lines between declarations are
// collapsed (run a `CollapseBlankLines` after this if needed).
type DTDCanonical struct{}

// Name implements Normalizer.
func (DTDCanonical) Name() string { return "dtd-canonical" }

// Normalize implements Normalizer.
func (DTDCanonical) Normalize(in []byte) ([]byte, error) {
	s := string(in)
	pos := 0
	var out bytes.Buffer
	paramEntities := map[string]string{}

	emitDecl := func(decl string) {
		// `decl` includes the leading `<!` and trailing `>`. Body is
		// everything in between, which we canonicalise.
		if !strings.HasPrefix(decl, "<!") || !strings.HasSuffix(decl, ">") {
			out.WriteString(decl)
			out.WriteByte('\n')
			return
		}
		body := decl[2 : len(decl)-1]
		// Identify the keyword.
		i := 0
		for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\r' || body[i] == '\n') {
			i++
		}
		kwStart := i
		for i < len(body) && body[i] != ' ' && body[i] != '\t' && body[i] != '\r' && body[i] != '\n' {
			i++
		}
		keyword := body[kwStart:i]
		rest := body[i:]
		canonRest := canonicalizeDTDBody(rest, paramEntities)

		// Capture parameter-entity declarations for later substitution.
		// Form: `% name "value"` or `% name 'value'`.
		if keyword == "ENTITY" {
			pe := strings.TrimSpace(rest)
			if strings.HasPrefix(pe, "%") {
				pe = strings.TrimSpace(pe[1:])
				// name = first token
				j := 0
				for j < len(pe) && pe[j] != ' ' && pe[j] != '\t' && pe[j] != '"' && pe[j] != '\'' {
					j++
				}
				name := pe[:j]
				peRest := strings.TrimSpace(pe[j:])
				if len(peRest) > 0 && (peRest[0] == '"' || peRest[0] == '\'') {
					q := peRest[0]
					end := strings.IndexByte(peRest[1:], q)
					if end >= 0 {
						val := peRest[1 : 1+end]
						// Canonicalise the value the same way as a body.
						paramEntities[name] = canonicalizeDTDBody(val, paramEntities)
					}
				}
			}
		}
		out.WriteString("<!")
		out.WriteString(keyword)
		out.WriteString(canonRest)
		out.WriteString(">\n")
	}

	for pos < len(s) {
		// Skip whitespace between declarations.
		for pos < len(s) && (s[pos] == ' ' || s[pos] == '\t' || s[pos] == '\n' || s[pos] == '\r') {
			pos++
		}
		if pos >= len(s) {
			break
		}
		if strings.HasPrefix(s[pos:], "<!--") {
			end := strings.Index(s[pos:], "-->")
			if end == -1 {
				out.WriteString(s[pos:])
				out.WriteByte('\n')
				break
			}
			out.WriteString(s[pos : pos+end+3])
			out.WriteByte('\n')
			pos += end + 3
			continue
		}
		if strings.HasPrefix(s[pos:], "<!") {
			end := indexCloseAngleQuotedDTDNorm(s[pos:])
			if end == -1 {
				out.WriteString(s[pos:])
				out.WriteByte('\n')
				break
			}
			emitDecl(s[pos : pos+end+1])
			pos += end + 1
			continue
		}
		if strings.HasPrefix(s[pos:], "<?") {
			end := strings.Index(s[pos:], "?>")
			if end == -1 {
				out.WriteString(s[pos:])
				out.WriteByte('\n')
				break
			}
			out.WriteString(s[pos : pos+end+2])
			out.WriteByte('\n')
			pos += end + 2
			continue
		}
		// Unrecognised — emit a byte and move on (keeps comparison
		// meaningful for malformed corners).
		out.WriteByte(s[pos])
		pos++
	}
	return out.Bytes(), nil
}

// canonicalizeDTDBody collapses whitespace inside a declaration body,
// folds quote style to `"`, removes whitespace adjacent to `,()|`, and
// substitutes parameter-entity refs (`%name;`) with the matching
// declaration's value from `entities`.
func canonicalizeDTDBody(body string, entities map[string]string) string {
	// First, expand parameter-entity references using known entities.
	// Repeat to a small fixed-point in case entities reference each
	// other (rare in practice; bound to 8 passes to avoid infinite
	// loops on cycles).
	if len(entities) > 0 {
		for pass := 0; pass < 8; pass++ {
			expanded := paramEntitySubst(body, entities)
			if expanded == body {
				break
			}
			body = expanded
		}
	}
	// Fold quote style: any single-quoted literal becomes double-
	// quoted. Be careful: only flip the outermost quote chars, and
	// only when the inner content has no `"` (otherwise we'd produce
	// invalid markup). For canonical comparison, escaped `"` inside is
	// rare in DTDs, so the simple path covers Test02 + Test01.
	body = foldDTDQuotes(body)
	// Collapse whitespace runs to single space.
	var b strings.Builder
	prevWS := false
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			if !prevWS {
				b.WriteByte(' ')
				prevWS = true
			}
			continue
		}
		b.WriteByte(c)
		prevWS = false
	}
	collapsed := b.String()
	// Strip whitespace adjacent to `,` `(` `)` `|`.
	collapsed = stripDTDDelimWS(collapsed)
	// Trim leading + trailing whitespace.
	return strings.TrimSpace(" " + collapsed + " ")
}

// paramEntitySubst replaces `%name;` references in s with the value of
// `entities[name]` when the name is known. Unknown names pass through
// unchanged so the output stays a valid declaration.
func paramEntitySubst(s string, entities map[string]string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '%' {
			end := strings.IndexByte(s[i:], ';')
			if end > 1 {
				name := s[i+1 : i+end]
				if val, ok := entities[name]; ok {
					b.WriteString(val)
					i += end + 1
					continue
				}
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// foldDTDQuotes converts single-quoted string literals to double-
// quoted form. Used by DTDCanonical to bridge the source-uses-`'` /
// okapi-emits-`"` divergence.
func foldDTDQuotes(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '\'' {
			end := strings.IndexByte(s[i+1:], '\'')
			if end >= 0 {
				inner := s[i+1 : i+1+end]
				if !strings.ContainsRune(inner, '"') {
					b.WriteByte('"')
					b.WriteString(inner)
					b.WriteByte('"')
					i += end + 2
					continue
				}
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// stripDTDDelimWS removes whitespace adjacent to `,()|` characters in
// the input. So `(a, b)` → `(a,b)`, `( a | b )` → `(a|b)`. Used by
// DTDCanonical to normalise content-model spacing.
func stripDTDDelimWS(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' {
			// Drop if adjacent to a delim.
			prev := byte(0)
			if i > 0 {
				prev = s[i-1]
			}
			next := byte(0)
			if i+1 < len(s) {
				next = s[i+1]
			}
			if isDTDDelim(prev) || isDTDDelim(next) {
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

func isDTDDelim(c byte) bool {
	switch c {
	case ',', '(', ')', '|':
		return true
	}
	return false
}

// indexCloseAngleQuotedDTDNorm is a quote-aware `>` finder used by
// DTDCanonical's tokeniser. Mirrors core/formats/dtd's helper of the
// same shape; duplicated here so the parity package has no dependency
// on the format internals.
func indexCloseAngleQuotedDTDNorm(s string) int {
	var inQuote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '>':
			return i
		}
	}
	return -1
}
