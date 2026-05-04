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
	if len(parts) == 0 {
		return "xml-canonical"
	}
	return "xml-canonical(" + strings.Join(parts, ",") + ")"
}

// Normalize implements Normalizer.
func (n XMLCanonical) Normalize(in []byte) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(in))
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
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
		// Drop CharData runs that consist of nothing but ASCII
		// whitespace — different writers indent differently and the
		// inter-element whitespace isn't significant to XML semantics
		// (excepting xml:space="preserve", which encoding/xml's
		// tokenizer doesn't propagate to us so we conservatively
		// always strip).
		if cd, ok := tok.(xml.CharData); ok {
			trimmed := bytes.TrimRight(bytes.TrimLeft(cd, " \t\r\n"), " \t\r\n")
			if len(trimmed) == 0 {
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
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		canonicalizeHTMLNode(c)
		if shouldDropHTMLNode(n, c) {
			toRemove = append(toRemove, c)
		} else if c.Type == html.TextNode && !isElementWithPreservedWhitespace(n) {
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
	for _, c := range toRemove {
		n.RemoveChild(c)
	}
	if n.Type == html.ElementNode && len(n.Attr) > 1 {
		sort.SliceStable(n.Attr, func(i, j int) bool {
			if n.Attr[i].Namespace != n.Attr[j].Namespace {
				return n.Attr[i].Namespace < n.Attr[j].Namespace
			}
			return n.Attr[i].Key < n.Attr[j].Key
		})
	}
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
	}
	return false
}

func isElementWithPreservedWhitespace(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Pre, atom.Textarea:
		return true
	}
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
