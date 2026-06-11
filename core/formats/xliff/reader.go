package xliff

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"golang.org/x/text/encoding/ianaindex"
)

// xmlCharsetReader is wired into every encoding/xml.Decoder we create
// so XLIFF files declared with non-UTF-8 encodings (windows-1252,
// ISO-8859-1, etc.) parse without rejecting the prolog. The reader's
// raw-byte skeleton path requires offsets that line up with the input
// string, so we transcode the file to UTF-8 *before* feeding it to the
// decoder (see transcodeToUTF8) — at that point the declared charset
// is already UTF-8, but the decoder still re-reads the prolog and
// invokes this hook. Returning the input verbatim keeps the byte
// offsets stable.
func xmlCharsetReader(charset string, input io.Reader) (io.Reader, error) {
	return input, nil
}

// transcodeToUTF8 inspects the XML declaration's encoding attribute on
// the leading bytes of the file and, if it names a non-UTF-8 charset
// (windows-1252, ISO-8859-1, …), decodes the entire input to UTF-8.
// The XML declaration is rewritten to encoding="UTF-8" so the parser's
// own prolog read agrees with the bytes that follow. Returns the
// (possibly transcoded) UTF-8 text and the original detected charset
// (empty if the file was already UTF-8 or had no declaration).
//
// When no declaration is present and the bytes are not valid UTF-8,
// fall back to Windows-1252 (a Latin-1 superset) — this matches okapi's
// tolerant behavior for legacy XLIFF files like
// integration-tests/.../xliff/generalstructure.xlf.
//
// After charset resolution, C0 control characters (other than the
// XML-allowed \t \n \r) are replaced with U+FFFD. This keeps the
// encoding/xml decoder happy on fixtures like invalid_xml_entity.xlf
// that smuggle a literal \x03 into character data.
func transcodeToUTF8(raw []byte) (string, string, error) {
	decl := xmlEncodingFromProlog(raw)
	if decl == "" || strings.EqualFold(decl, "UTF-8") || strings.EqualFold(decl, "UTF8") {
		if utf8Valid(raw) {
			return sanitizeXMLControlChars(string(raw)), decl, nil
		}
		// Fallback: undeclared file with non-UTF-8 bytes. Decode as
		// Windows-1252 — covers ISO-8859-1 too since 0x80-0x9F differ
		// only in unprintable region. Marks the result as if the file
		// declared "windows-1252" so SimulateBrokenWindows1252Read can
		// pick it up downstream.
		enc, _ := ianaindex.IANA.Encoding("windows-1252")
		if enc == nil {
			return string(raw), decl, errors.New("xliff: file lacks XML declaration and bytes are not valid UTF-8")
		}
		decoded, err := io.ReadAll(enc.NewDecoder().Reader(bytes.NewReader(raw)))
		if err != nil {
			return string(raw), decl, fmt.Errorf("xliff: undeclared-charset fallback decode: %w", err)
		}
		return sanitizeXMLControlChars(string(decoded)), "windows-1252", nil
	}
	enc, err := ianaindex.IANA.Encoding(decl)
	if err != nil || enc == nil {
		return string(raw), decl, fmt.Errorf("xliff: unsupported declared charset %q", decl)
	}
	decoded, err := io.ReadAll(enc.NewDecoder().Reader(bytes.NewReader(raw)))
	if err != nil {
		return string(raw), decl, fmt.Errorf("xliff: transcoding %q to UTF-8: %w", decl, err)
	}
	rewritten := rewriteXMLEncodingToUTF8(string(decoded))
	return sanitizeXMLControlChars(rewritten), decl, nil
}

// sanitizeXMLControlChars replaces C0 control characters (U+0000-U+001F
// excluding U+0009, U+000A, U+000D) with U+FFFD, both in literal byte
// form and as numeric character references (`&#x03;`, `&#3;`). XML 1.0
// §2.2 forbids these in well-formed documents; the encoding/xml decoder
// rejects them even with Strict=false. Some real-world fixtures (e.g.
// okapi's invalid_xml_entity.xlf with literal U+0003 AND `&#x03;`
// references) expect tolerant handling — replacing keeps the rest of
// the document parseable, matching okapi's `XLIFFFilter` behavior.
func sanitizeXMLControlChars(s string) string {
	out := s
	// Step 1: replace literal C0 control bytes.
	if strings.ContainsAny(out, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0B\x0C\x0E\x0F\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1A\x1B\x1C\x1D\x1E\x1F") {
		var b strings.Builder
		b.Grow(len(out))
		for _, r := range out {
			if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
				b.WriteRune('�')
				continue
			}
			b.WriteRune(r)
		}
		out = b.String()
	}
	// Step 2: replace `&#x..;` and `&#..;` references that resolve to a
	// disallowed C0 control char. Done by regex on the source bytes
	// before the XML decoder resolves them.
	out = c0NumericRefRE.ReplaceAllStringFunc(out, func(m string) string {
		val, err := parseNumericRef(m)
		if err != nil {
			return m
		}
		if val < 0x20 && val != 0x09 && val != 0x0A && val != 0x0D {
			return "&#xFFFD;"
		}
		return m
	})
	return out
}

var c0NumericRefRE = regexp.MustCompile(`&#(?:x[0-9A-Fa-f]+|[0-9]+);`)

// parseNumericRef parses an `&#NNN;` or `&#xHHH;` reference into its
// integer value. Returns an error on malformed input (the regex
// shouldn't let any through, but stay defensive).
func parseNumericRef(ref string) (int, error) {
	if !strings.HasPrefix(ref, "&#") || !strings.HasSuffix(ref, ";") {
		return 0, errors.New("not a numeric ref")
	}
	body := ref[2 : len(ref)-1]
	base := 10
	if len(body) > 0 && (body[0] == 'x' || body[0] == 'X') {
		base = 16
		body = body[1:]
	}
	v, err := strconv.ParseInt(body, base, 32)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

// xmlEncodingFromProlog extracts the encoding name from a `<?xml … ?>`
// declaration if present in the first ~256 bytes. Returns "" when the
// file has no declaration or no encoding attribute.
func xmlEncodingFromProlog(raw []byte) string {
	limit := min(len(raw), 256)
	head := string(raw[:limit])
	start := strings.Index(head, "<?xml")
	if start < 0 {
		return ""
	}
	end := strings.Index(head[start:], "?>")
	if end < 0 {
		return ""
	}
	prolog := head[start : start+end]
	_, after, ok := strings.Cut(prolog, "encoding")
	if !ok {
		return ""
	}
	rest := after
	rest = strings.TrimLeft(rest, " \t=")
	if len(rest) == 0 {
		return ""
	}
	quote := rest[0]
	if quote != '"' && quote != '\'' {
		return ""
	}
	rest = rest[1:]
	before, _, ok := strings.Cut(rest, string(quote))
	if !ok {
		return ""
	}
	return before
}

// rewriteXMLEncodingToUTF8 normalizes the encoding attribute in the
// `<?xml … ?>` declaration to UTF-8 after a transcode. Leaves the rest
// of the prolog (version, standalone, whitespace) untouched.
func rewriteXMLEncodingToUTF8(s string) string {
	end := strings.Index(s, "?>")
	if end < 0 {
		return s
	}
	prolog := s[:end]
	rest := s[end:]
	before, after, ok := strings.Cut(prolog, "encoding")
	if !ok {
		return s
	}
	tail := after
	trimmed := strings.TrimLeft(tail, " \t=")
	if len(trimmed) == 0 {
		return s
	}
	quote := trimmed[0]
	if quote != '"' && quote != '\'' {
		return s
	}
	q := strings.IndexByte(trimmed[1:], quote)
	if q < 0 {
		return s
	}
	consumed := len(tail) - len(trimmed) + 1 + q + 1
	return before + `encoding="UTF-8"` + tail[consumed:] + rest
}

// Reader implements DataFormatReader for XLIFF 1.2 files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new XLIFF 1.2 reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xliff",
			FormatDisplayName: "XLIFF 1.2",
			FormatMimeType:    "application/xliff+xml",
			FormatExtensions:  []string{".xlf", ".xliff"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/xliff+xml", "application/x-xliff+xml"},
		Extensions: []string{".xlf", ".xliff"},
		Sniff: func(data []byte) bool {
			s := string(data)
			return strings.Contains(s, "<xliff") && strings.Contains(s, "urn:oasis:names:tc:xliff:document:1")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("xliff: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

// fileInfo holds metadata from a <file> element.
type fileInfo struct {
	original   string
	sourceLang string
	targetLang string
	datatype   string
}

// elemPos tracks the byte position of a source or target element's inner content.
type elemPos struct {
	startOffset int    // byte offset after opening tag
	endOffset   int    // byte offset before closing tag
	blockIdx    int    // 0-based block index
	elemType    string // "source" or "target"
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	raw, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xliff: reading: %w", err)}
		return
	}
	rawText, srcCharset, err := transcodeToUTF8(raw)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xliff: parsing: %w", err)}
		return
	}
	// OkapiCompatConfig.SimulateBrokenWindows1252Read: when the source
	// declared a non-UTF-8 charset, replace every non-ASCII rune in
	// the transcoded text with U+FFFD. This mimics okapi's xliff filter
	// bug where windows-1252 single-byte chars for accented Latin
	// letters end up as replacement chars in output. See OkapiCompatConfig.
	if r.cfg != nil && r.cfg.OkapiCompat.SimulateBrokenWindows1252Read &&
		srcCharset != "" && !strings.EqualFold(srcCharset, "UTF-8") && !strings.EqualFold(srcCharset, "UTF8") {
		rawText = simulateBrokenWindows1252(rawText)
	}
	// content holds the same bytes the decoder is reading from, so byte
	// offsets reported by decoder.InputOffset() can index into it
	// directly. After a non-UTF-8 transcode this differs from the
	// on-disk bytes. We keep a single full copy of the UTF-8 text here:
	// the decoder reads from a bytes.Reader over it, per-unit bodies are
	// sliced out of it, and the skeleton text is sliced from it too — so
	// there is no separate `[]byte(rawText)` duplicate.
	content := []byte(rawText)

	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = false
	decoder.CharsetReader = xmlCharsetReader

	var currentFile *fileInfo
	var inBody bool
	var groupStack []string // stack of group IDs
	var blockCount int
	// preserveWSStack tracks xml:space inheritance. When we see xml:space="preserve"
	// on any ancestor, we push true. Default is false.
	var preserveWSStack []bool
	var elemPositions []elemPos

	// inheritPreserveWS returns true if any ancestor has xml:space="preserve"
	// or if the config sets preserveSpaceByDefault.
	inheritPreserveWS := func() bool {
		if r.cfg.PreserveSpaceByDefault {
			return true
		}
		for i := len(preserveWSStack) - 1; i >= 0; i-- {
			if preserveWSStack[i] {
				return true
			}
		}
		return false
	}

	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xliff: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local

			switch local {
			case "xliff":
				// Track xml:space at the document root so the
				// preserveWSStack inherits when <file>, <group>, or
				// <trans-unit> don't redeclare it. about_the.htm.xlf
				// declares `<xliff xml:space="preserve">` and relies on
				// inheritance for every nested element.
				ws := xmlSpaceAttr(t.Attr)
				preserveWSStack = append(preserveWSStack, ws == "preserve")
			case "file":
				fi := &fileInfo{}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "original":
						fi.original = a.Value
					case "source-language":
						fi.sourceLang = a.Value
					case "target-language":
						fi.targetLang = a.Value
					case "datatype":
						fi.datatype = a.Value
					}
				}
				currentFile = fi

				// Check for xml:space on file
				ws := xmlSpaceAttr(t.Attr)
				preserveWSStack = append(preserveWSStack, ws == "preserve")

				sourceLang := model.LocaleID(fi.sourceLang)
				targetLang := model.LocaleID(fi.targetLang)

				layer := &model.Layer{
					ID:             "file-" + fi.original,
					Name:           fi.original,
					Format:         "xliff",
					Locale:         sourceLang,
					Encoding:       "UTF-8",
					IsMultilingual: true,
					Properties: map[string]string{
						"datatype":        fi.datatype,
						"target-language": string(targetLang),
					},
				}
				// Record the source XML declaration's encoding so the
				// writer can replicate okapi's encoding-conditional
				// entity escaping (XMLEncoder skips entity escaping when
				// encoding is UTF-8/16, otherwise escapes anything not
				// representable in the declared charset). Empty when the
				// source had no declaration or was already UTF-8.
				if srcCharset != "" && !strings.EqualFold(srcCharset, "UTF-8") && !strings.EqualFold(srcCharset, "UTF8") {
					layer.Properties["xliff:source-encoding"] = srcCharset
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
					return
				}

			case "body":
				inBody = true

			case "group":
				if !inBody {
					continue
				}
				groupID := ""
				translateAttr := ""
				ws := xmlSpaceAttr(t.Attr)
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "id":
						groupID = a.Value
					case "translate":
						translateAttr = a.Value
					}
				}
				preserveWSStack = append(preserveWSStack, ws == "preserve")
				groupStack = append(groupStack, translateAttr)

				gs := &model.GroupStart{
					ID:   groupID,
					Name: groupID,
					Properties: map[string]string{
						"translate": translateAttr,
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: gs}) {
					return
				}

			case "trans-unit":
				if !inBody || currentFile == nil {
					continue
				}
				tu, tuPositions := r.parseTransUnit(decoder, t, currentFile, blockCount, content)
				if tu == nil {
					continue
				}
				if r.skeletonStore != nil {
					elemPositions = append(elemPositions, tuPositions...)
				}

				// Check xml:space on this trans-unit
				ws := xmlSpaceAttr(t.Attr)
				preserveWS := ws == "preserve" || inheritPreserveWS()

				sourceLang := model.LocaleID(currentFile.sourceLang)
				targetLang := model.LocaleID(currentFile.targetLang)

				// Check translate attribute on trans-unit and group ancestors
				translatable := tu.translatable
				if translatable {
					// Check group stack for translate="no"
					if slices.Contains(groupStack, "no") {
						translatable = false
					}
				}

				block := r.buildBlock(tu, sourceLang, targetLang, translatable, preserveWS)
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
				blockCount++
			}

		case xml.EndElement:
			local := t.Name.Local
			switch local {
			case "xliff":
				if len(preserveWSStack) > 0 {
					preserveWSStack = preserveWSStack[:len(preserveWSStack)-1]
				}
			case "file":
				if currentFile != nil {
					if len(preserveWSStack) > 0 {
						preserveWSStack = preserveWSStack[:len(preserveWSStack)-1]
					}
					layer := &model.Layer{
						ID:   "file-" + currentFile.original,
						Name: currentFile.original,
					}
					r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
					currentFile = nil
				}

			case "body":
				inBody = false

			case "group":
				if len(groupStack) > 0 {
					if len(preserveWSStack) > 0 {
						preserveWSStack = preserveWSStack[:len(preserveWSStack)-1]
					}
					groupStack = groupStack[:len(groupStack)-1]
				}
				r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{}})
			}
		}
	}

	// Build skeleton from collected element positions
	if r.skeletonStore != nil && len(elemPositions) > 0 {
		skelPos := 0
		for _, ep := range elemPositions {
			if ep.startOffset > skelPos {
				r.skelText(string(content[skelPos:ep.startOffset]))
			}
			refID := fmt.Sprintf("%d:%s", ep.blockIdx, ep.elemType)
			r.skelRef(refID)
			skelPos = ep.endOffset
		}
		if skelPos < len(content) {
			r.skelText(string(content[skelPos:]))
		}
		r.skelFlush()
	}
}

// xmlSpaceAttr returns the value of xml:space attribute from attrs, or "".
func xmlSpaceAttr(attrs []xml.Attr) string {
	for _, a := range attrs {
		if a.Name.Local == "space" && (a.Name.Space == "xml" || a.Name.Space == "http://www.w3.org/XML/1998/namespace") {
			return a.Value
		}
	}
	return ""
}

// segSourceMatchesSource reports whether the text projection of a
// `<seg-source>` body matches the corresponding `<source>` body.
// Mirrors okapi XLIFFFilter.java:2278's CODE_DATA_ONLY compare on the
// joined-segments form: both bodies are stripped of element tags so
// only the character-data content (including any &lt;/&gt; entity
// escapes that survived) and any inter-mrk text remain, then the
// result is compared.
//
// The preserveWS flag corresponds to okapi's `preserveSpaces.peek()`
// gate at XLIFFFilter.java:2309: when xml:space="preserve", okapi
// skips the `unwrap()` pre-pass and treats every inner space as
// significant; otherwise it collapses runs of ASCII whitespace into
// single spaces. Working from the raw seg-source bytes (segSourceRaw)
// rather than the parsed segment list preserves the inter-mrk text
// that participates in okapi's joinAll comparison — important for
// fixtures whose mrks have no inter-mrk whitespace and whose source
// has no joining whitespace either (e.g. about_the.htm.xlf id=3,
// where the parsed segments alone would falsely match).
//
// Used to gate the reader's "fall back to un-segmented source"
// behavior under okapi-compat (matches okapi's "log error and use
// source" decision when seg-source is inconsistent with source).
func segSourceMatchesSource(segSourceRaw, source string, preserveWS bool) bool {
	left := decodeBasicXMLEntities(stripInlineTags(segSourceRaw))
	right := decodeBasicXMLEntities(stripInlineTags(source))
	if preserveWS {
		// xml:space="preserve" — every inner space is significant.
		return strings.TrimSpace(left) == strings.TrimSpace(right)
	}
	return collapseWS(left) == collapseWS(right)
}

// stripInlineTags removes all element tags (start, end, self-closing)
// from a fragment of XML, preserving text content. Used for text-only
// comparison of source vs seg-source bodies.
var inlineTagStripRE = regexp.MustCompile(`<[^>]+>`)

func stripInlineTags(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	return inlineTagStripRE.ReplaceAllString(s, "")
}

// collapseWS replaces every run of ASCII whitespace (space, tab, CR,
// LF) with a single space and trims the result. Mirrors okapi's
// TextContainer.unwrap whitespace collapse.
func collapseWS(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	wasWS := false
	for i := range len(s) {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			if !wasWS {
				b.WriteByte(' ')
				wasWS = true
			}
			continue
		}
		wasWS = false
		b.WriteByte(c)
	}
	return strings.TrimSpace(b.String())
}

// decodeBasicXMLEntities expands the five XML predefined entities so
// the comparison ignores byte-level differences in how source vs
// seg-source happened to encode `>` etc. Matches the helper in
// okapi_compat_helpers.go used by the writer post-process pass.
func decodeBasicXMLEntities(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	out := strings.ReplaceAll(s, "&gt;", ">")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&quot;", "\"")
	out = strings.ReplaceAll(out, "&apos;", "'")
	out = strings.ReplaceAll(out, "&amp;", "&")
	return out
}

// parsedTransUnit holds parsed trans-unit data.
type parsedTransUnit struct {
	id           string
	resname      string
	translatable bool
	approved     bool
	state        string
	maxWidth     string
	sizeUnit     string

	source       string     // raw inner XML of <source>
	target       string     // raw inner XML of <target>
	targetLang   string     // xml:lang attribute on <target> (when <file> has no target-language)
	hasTarget    bool       // true when the source had a <target> element (even empty/self-closing)
	targetAttrs  []xml.Attr // <target>'s own attributes (state, xml:lang, etc.) for round-trip
	segSource    []segment  // parsed <seg-source> segments
	segSourceRaw string     // raw inner XML of <seg-source> (with mrk wrappers and inter-mrk text)

	notes    []parsedNote
	altTrans []parsedAltTrans

	preserveWS bool // xml:space="preserve" on this TU
}

type segment struct {
	mid  string
	text string // inner XML
}

type parsedNote struct {
	text      string
	from      string
	priority  int
	annotates string
}

type parsedAltTrans struct {
	matchQuality float64
	origin       string
	source       string
	target       string
}

// parseTransUnit parses a <trans-unit> element and all its children.
// It returns the parsed trans-unit and skeleton element positions (if skeleton tracking is active).
//
// content is the full input file's bytes — used to slice raw inner XML
// for <source>/<target>/<seg-source> bodies. Slicing the raw input
// preserves namespace prefixes (encoding/xml resolves them to URIs,
// which loses the source's prefix mapping), original entity escaping,
// and source whitespace formatting that re-serialization would mangle.
func (r *Reader) parseTransUnit(decoder *xml.Decoder, start xml.StartElement, fi *fileInfo, blockIdx int, content []byte) (*parsedTransUnit, []elemPos) {
	tu := &parsedTransUnit{
		translatable: true,
	}

	for _, a := range start.Attr {
		switch a.Name.Local {
		case "id":
			tu.id = a.Value
		case "resname":
			tu.resname = a.Value
		case "translate":
			tu.translatable = a.Value != "no"
		case "approved":
			tu.approved = a.Value == "yes"
		case "maxwidth":
			tu.maxWidth = a.Value
		case "size-unit":
			tu.sizeUnit = a.Value
		}
		if a.Name.Local == "space" && (a.Name.Space == "xml" || a.Name.Space == "http://www.w3.org/XML/1998/namespace") {
			tu.preserveWS = a.Value == "preserve"
		}
	}

	var positions []elemPos
	hasTarget := false
	sourceAfterClose := -1  // byte offset right after </source>
	transUnitCloseOff := -1 // byte offset of `<` in `</trans-unit>`
	nextSiblingStart := -1  // offset of `<` for first start-tag sibling after </source>
	awaitingNextSibling := false

	depth := 1
	for depth > 0 {
		preOff := int(decoder.InputOffset())
		tok, err := decoder.Token()
		if err != nil {
			return nil, nil
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if awaitingNextSibling && depth == 1 {
				// First sibling-start tag at trans-unit depth after
				// </source>. Capture the offset of its `<` before we
				// step into it; this is where okapi inserts the
				// synthesised <target> when one was missing.
				nextSiblingStart = preOff
				awaitingNextSibling = false
			}
			depth++
			switch t.Name.Local {
			case "source":
				startOff := int(decoder.InputOffset())
				_, closeOff := readInnerXML(decoder)
				// Prefer the raw byte slice over readInnerXML's
				// reserialized string: encoding/xml resolves namespace
				// prefixes to URIs (so cms:id becomes
				// urn:ixiasoft:dita-cms:xliff:id, which is then invalid
				// as XML), and reserialization changes attribute order /
				// whitespace. Raw slicing keeps every byte the source
				// had between the opening `>` and the closing `<`.
				if content != nil && startOff >= 0 && closeOff >= startOff && closeOff <= len(content) {
					tu.source = string(content[startOff:closeOff])
				}
				depth-- // readInnerXML consumed the end element
				sourceAfterClose = int(decoder.InputOffset())
				awaitingNextSibling = true
				if r.skeletonStore != nil {
					positions = append(positions, elemPos{
						startOffset: startOff,
						endOffset:   closeOff,
						blockIdx:    blockIdx,
						elemType:    "source",
					})
				}
			case "target":
				// preOff was captured at the top of the loop before
				// reading the StartElement; it points at the `<` of
				// `<target`. We use that as the elemPos startOffset so
				// the writer can replace the entire <target ...>...</target>
				// element (including the open tag, attrs, and close
				// tag). Replacing inner-only doesn't work for empty/
				// self-closing targets like <target state="…" />, where
				// inserted content lands outside the element.
				targetTagStart := preOff
				startOff := int(decoder.InputOffset())
				for _, a := range t.Attr {
					if a.Name.Local == "lang" && (a.Name.Space == "xml" || a.Name.Space == "http://www.w3.org/XML/1998/namespace") {
						tu.targetLang = a.Value
						break
					}
				}
				tu.targetAttrs = copyTargetAttrs(t.Attr)
				_, closeOff := readInnerXML(decoder)
				if content != nil && startOff >= 0 && closeOff >= startOff && closeOff <= len(content) {
					tu.target = string(content[startOff:closeOff])
				}
				depth--
				hasTarget = true
				tu.hasTarget = true
				targetTagEnd := int(decoder.InputOffset())
				if r.skeletonStore != nil {
					positions = append(positions, elemPos{
						startOffset: targetTagStart,
						endOffset:   targetTagEnd,
						blockIdx:    blockIdx,
						elemType:    "target",
					})
				}
			case "seg-source":
				segSrcStart := int(decoder.InputOffset())
				tu.segSource = parseSegSource(decoder)
				segSrcEnd := int(decoder.InputOffset())
				// Capture the raw inner-XML bytes so the okapi-compat
				// "drop divergent seg-source" rule can compare source
				// vs seg-source body-by-body — the parsed segment list
				// loses inter-mrk whitespace that participates in
				// okapi's joined-content compare. The slice runs from
				// the byte just past the opening `>` of <seg-source> to
				// the start of the closing `</seg-source>`.
				if content != nil && segSrcStart >= 0 && segSrcEnd > segSrcStart && segSrcEnd <= len(content) {
					// segSrcEnd is just past `</seg-source>`; trim the
					// closing tag back off the slice.
					closeTag := []byte("</seg-source>")
					if end := bytes.Index(content[segSrcStart:segSrcEnd], closeTag); end >= 0 {
						tu.segSourceRaw = string(content[segSrcStart : segSrcStart+end])
					}
				}
				depth--
				// When the trans-unit ends up needing a synthesized
				// target (no <target> at trans-unit depth), okapi
				// inserts it right after </seg-source> rather than
				// right after </source>. Reset the next-sibling probe
				// so nextSiblingStart picks up the first element AFTER
				// seg-source instead of seg-source itself.
				nextSiblingStart = -1
				awaitingNextSibling = true
			case "note":
				n := parseNote(decoder, t)
				tu.notes = append(tu.notes, n)
				depth--
			case "alt-trans":
				at := parseAltTrans(decoder, t)
				tu.altTrans = append(tu.altTrans, at)
				depth--
			default:
				// Skip unknown elements
			}
		case xml.EndElement:
			depth--
			if depth == 0 {
				// `</trans-unit>` close: preOff is the offset of the
				// `<` in the close tag, which is exactly where okapi
				// inserts a synthesised target on round-trip (after any
				// prior whitespace, before the close tag).
				transUnitCloseOff = preOff
			}
		}
	}

	// When the trans-unit has a <source> but no <target>, emit a
	// synthetic position so the writer can inject a complete <target>.
	// okapi places the new target right before the next sibling
	// element after </source> (alt-trans, note, or </trans-unit> when
	// nothing else follows), preserving any inter-element whitespace
	// as the target's leading indent. Mirror that placement so the
	// surrounding skeleton bytes match byte-for-byte.
	//
	// Two flavors based on segmentation state:
	//   - target-inject:        unsegmented body (the typical case).
	//   - target-inject-seg:    seg-source is present, so the body
	//     wraps each segment in <mrk mtype="seg" mid="…"> matching
	//     the source segmentation (what okapi emits in segmented
	//     trans-units that still need a synthesized target).
	if r.skeletonStore != nil && !hasTarget && sourceAfterClose >= 0 {
		injectAt := nextSiblingStart
		if injectAt < 0 {
			injectAt = transUnitCloseOff
		}
		if injectAt < 0 {
			injectAt = sourceAfterClose
		}
		elemType := "target-inject"
		if len(tu.segSource) > 0 {
			elemType = "target-inject-seg"
		}
		positions = append(positions, elemPos{
			startOffset: injectAt,
			endOffset:   injectAt,
			blockIdx:    blockIdx,
			elemType:    elemType,
		})
	}

	return tu, positions
}

// readInnerXML reads all content until the matching end element. It
// returns the inner XML as a string plus the byte offset (in the
// decoder's input stream) where the matching close tag begins —
// captured *before* consuming the EndElement token, so the caller can
// build a skeleton-ref range that doesn't include the close tag (and
// works for prefixed namespaces like `</x:source>` where the close-tag
// length isn't `len("</source>")`).
func readInnerXML(decoder *xml.Decoder) (string, int) {
	var buf strings.Builder
	depth := 1
	closeOff := 0
	for depth > 0 {
		// Capture offset before reading the next token; if it turns out
		// to be the matching EndElement, this is the offset of `<` in
		// the close tag.
		preOff := int(decoder.InputOffset())
		tok, err := decoder.Token()
		if err != nil {
			return buf.String(), closeOff
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			buf.WriteString("<")
			buf.WriteString(t.Name.Local)
			for _, a := range t.Attr {
				buf.WriteString(" ")
				if a.Name.Space != "" {
					buf.WriteString(a.Name.Space)
					buf.WriteString(":")
				}
				buf.WriteString(a.Name.Local)
				buf.WriteString(`="`)
				buf.WriteString(xmlEscapeAttr(a.Value))
				buf.WriteString(`"`)
			}
			buf.WriteString(">")
		case xml.EndElement:
			depth--
			if depth > 0 {
				buf.WriteString("</")
				buf.WriteString(t.Name.Local)
				buf.WriteString(">")
			} else {
				closeOff = preOff
			}
		case xml.CharData:
			buf.WriteString(xmlEscapeText(string(t)))
		case xml.Comment:
			buf.WriteString("<!--")
			buf.Write(t)
			buf.WriteString("-->")
		}
	}
	return buf.String(), closeOff
}

// xmlEscapeText escapes XML special characters in text content. Per
// XML 1.0 §2.4, only `<` and `&` MUST be escaped in character data;
// `>` only needs escaping when it follows `]]` (to avoid the `]]>`
// CDATA-end sequence). Most writers (including okapi's XLIFFWriter)
// emit literal `>` in text. We follow that convention for byte-stable
// round-trips and match the spec's minimum requirement.
func xmlEscapeText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	// Only escape `>` after `]]` to avoid the CDATA-end sequence.
	if strings.Contains(s, "]]>") {
		s = strings.ReplaceAll(s, "]]>", "]]&gt;")
	}
	return s
}

// xmlEscapeAttr escapes XML special characters in attribute values.
func xmlEscapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

// parseSegSource parses <seg-source> and returns mrk segments.
func parseSegSource(decoder *xml.Decoder) []segment {
	var segs []segment
	depth := 1

	var currentSeg *segment
	var buf strings.Builder

	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "mrk" {
				mtype := ""
				mid := ""
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "mtype":
						mtype = a.Value
					case "mid":
						mid = a.Value
					}
				}
				if mtype == "seg" {
					buf.Reset()
					currentSeg = &segment{mid: mid}
				} else if currentSeg != nil {
					// Non-seg mrk — write it as inline content
					buf.WriteString("<mrk")
					for _, a := range t.Attr {
						buf.WriteString(" ")
						buf.WriteString(a.Name.Local)
						buf.WriteString(`="`)
						buf.WriteString(xmlEscapeAttr(a.Value))
						buf.WriteString(`"`)
					}
					buf.WriteString(">")
				}
			} else if currentSeg != nil {
				// Inline element within mrk: preserve as-is
				buf.WriteString("<")
				buf.WriteString(t.Name.Local)
				for _, a := range t.Attr {
					buf.WriteString(" ")
					if a.Name.Space != "" {
						buf.WriteString(a.Name.Space)
						buf.WriteString(":")
					}
					buf.WriteString(a.Name.Local)
					buf.WriteString(`="`)
					buf.WriteString(xmlEscapeAttr(a.Value))
					buf.WriteString(`"`)
				}
				buf.WriteString(">")
			} else {
				// Inline element outside mrk (e.g., bpt/ept spanning segments)
				buf.WriteString("<")
				buf.WriteString(t.Name.Local)
				for _, a := range t.Attr {
					buf.WriteString(" ")
					buf.WriteString(a.Name.Local)
					buf.WriteString(`="`)
					buf.WriteString(xmlEscapeAttr(a.Value))
					buf.WriteString(`"`)
				}
				buf.WriteString(">")
			}
		case xml.EndElement:
			depth--
			if depth > 0 {
				if t.Name.Local == "mrk" && currentSeg != nil {
					currentSeg.text = buf.String()
					segs = append(segs, *currentSeg)
					currentSeg = nil
				} else if currentSeg != nil {
					buf.WriteString("</")
					buf.WriteString(t.Name.Local)
					buf.WriteString(">")
				}
			}
		case xml.CharData:
			if currentSeg != nil {
				// CharData is already decoded (`&lt;b>` → `<b>`). The
				// buffer is re-parsed downstream as XML, so re-escape
				// here — otherwise text like `<b>` reads as a start tag
				// and the inner text disappears from the native IR.
				buf.WriteString(xmlEscapeText(string(t)))
			}
		}
	}
	return segs
}

// parseNote parses a <note> element and returns parsed data.
func parseNote(decoder *xml.Decoder, start xml.StartElement) parsedNote {
	n := parsedNote{}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "from":
			n.from = a.Value
		case "priority":
			if p, err := strconv.Atoi(a.Value); err == nil {
				n.priority = p
			}
		case "annotates":
			n.annotates = a.Value
		}
	}

	var buf strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			buf.Write(t)
		}
	}
	n.text = buf.String()
	return n
}

// parseAltTrans parses an <alt-trans> element.
func parseAltTrans(decoder *xml.Decoder, start xml.StartElement) parsedAltTrans {
	at := parsedAltTrans{}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "match-quality":
			if f, err := strconv.ParseFloat(a.Value, 64); err == nil {
				at.matchQuality = f
			}
		case "origin":
			at.origin = a.Value
		}
	}

	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "source":
				at.source = readInnerXMLCharData(decoder)
				depth--
			case "target":
				at.target = readInnerXMLCharData(decoder)
				depth--
			}
		case xml.EndElement:
			depth--
		}
	}
	return at
}

// readInnerXMLCharData reads character data until end element, ignoring child elements.
func readInnerXMLCharData(decoder *xml.Decoder) string {
	var buf strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			buf.Write(t)
		}
	}
	return buf.String()
}

// buildBlock creates a Block from parsed trans-unit data.
func (r *Reader) buildBlock(tu *parsedTransUnit, sourceLang, targetLang model.LocaleID, translatable, preserveWS bool) *model.Block {
	hasTargetElem := tu.hasTarget
	block := &model.Block{
		ID:                 tu.id,
		Name:               tu.id,
		Translatable:       translatable,
		PreserveWhitespace: preserveWS || tu.preserveWS,
		Properties:         make(map[string]string),
		Targets:            make(map[model.VariantKey]*model.Target),
	}

	if tu.resname != "" {
		block.Name = tu.resname
		block.Properties["resname"] = tu.resname
	} else if r.cfg.FallbackToID && tu.id != "" {
		block.Name = tu.id
	}

	if tu.approved {
		block.Properties["approved"] = "yes"
	}

	if tu.state != "" {
		block.Properties["state"] = tu.state
	}

	if tu.maxWidth != "" {
		block.Properties["maxwidth"] = tu.maxWidth
	}
	if tu.sizeUnit != "" {
		block.Properties["size-unit"] = tu.sizeUnit
	}

	// Build source segments
	useSegSource := len(tu.segSource) > 0
	segSourceDivergent := false
	if useSegSource && r.cfg != nil && r.cfg.OkapiCompat.UnwrapSingleSegMrk {
		// Mirror okapi XLIFFFilter.java:2278 — when seg-source content
		// disagrees with <source> content (CODE_DATA_ONLY compare with
		// the same `unwrap()` pre-pass okapi runs on both containers),
		// okapi LOGS an error and falls back to the un-segmented
		// <source>. The seg-source segmentation is treated as
		// inconsistent and dropped. The downstream writer post-pass
		// (unwrapSingleSegMrkWhenSourceDiffers) drops the seg-source
		// bytes; mirroring the same decision at read-time means the
		// source segments and any synthesized target match the
		// un-segmented source rather than the divergent segmentation.
		// Without this, RB-12-Test02.xlf's id="11withWarning" emits a
		// concatenation of the divergent seg-source segments instead
		// of the single-segment source. The preserveWS flag mirrors
		// XLIFFFilter.java:2309 — okapi only collapses whitespace via
		// unwrap when xml:space ≠ "preserve"; in preserve mode every
		// inner space is significant, so a fixture like
		// about_the.htm.xlf (declares xml:space="preserve") that has
		// "About the  Agent" (two spaces) in source vs
		// "About the Agent" (one space) in seg-source is correctly
		// flagged as divergent.
		if !segSourceMatchesSource(tu.segSourceRaw, tu.source, preserveWS) {
			useSegSource = false
			segSourceDivergent = true
		}
	}
	if useSegSource {
		// Use seg-source segments. Concatenate every segment's runs into
		// a flat block.Source and lay a source segmentation overlay over
		// the run-index boundaries (one Span per <mrk mtype="seg">, Span.ID
		// = the segment mid). Each segment's xliff-native IR is stored
		// block-level under segNativeKey(mid).
		var srcRuns []model.Run
		spans := make([]model.Span, len(tu.segSource))
		for i, seg := range tu.segSource {
			nc := parseNativeContent(seg.text)
			runs := nativeToRuns(nc)
			start := len(srcRuns)
			srcRuns = append(srcRuns, runs...)
			spans[i] = model.Span{ID: seg.mid, Range: model.RunRange{StartRun: start, EndRun: len(srcRuns)}}
			block.SetAnno(segNativeKey(seg.mid), &SegmentNativeAnnotation{Content: nc})
		}
		block.Source = srcRuns
		block.SetSegmentation(nil, spans)
		// Attach body-level native IR for <source>: parsed from the
		// raw <source> body (which is unsegmented but mirrors the
		// inline-code structure). Falls back to building it from the
		// segments + reasonable separators when <source> wasn't present.
		block.SetAnno("xliff:source-body", &SourceBodyNativeAnnotation{
			Content: parseNativeContent(tu.source),
		})
	} else {
		// Use <source> content. The single segment and the body
		// annotation cover the same bytes, so parse the native IR once
		// and share it between both rather than decoding tu.source twice.
		srcNative := parseNativeContent(tu.source)
		block.Source = nativeToRuns(srcNative)
		block.SetAnno(segNativeKey("s1"), &SegmentNativeAnnotation{Content: srcNative})
		block.SetAnno("xliff:source-body", &SourceBodyNativeAnnotation{
			Content: srcNative,
		})
		// When we dropped the seg-source segments, also clear the
		// downstream segmentation hints so the writer doesn't try to
		// re-emit a `target-inject-seg` (segmented target wrapper) for
		// what is now a single-segment trans-unit.
		if len(tu.segSource) > 0 {
			tu.segSource = nil
		}
	}
	// Mark the block when the reader dropped seg-source under okapi-
	// compat so the writer post-process knows to strip the seg-source
	// bytes (which still come through from the literal skeleton).
	// When the reader kept seg-source, no marker is set — the writer
	// preserves it untouched, matching okapi's "use the segmented
	// content" branch (XLIFFFilter.java:2281-2291).
	if segSourceDivergent {
		block.SetAnno("xliff:divergent-segsource", &DivergentSegSourceAnnotation{})
	}

	// Build target segments. Prefer the file's target-language; fall
	// back to the <target xml:lang="..."> attribute when the file
	// element didn't declare a language. okapi reads the existing
	// target regardless of the <file> attribute, so storing it here
	// lets pseudo-translate find an existing target as the base.
	targetContent := tu.target
	effectiveTargetLang := targetLang
	if effectiveTargetLang.IsEmpty() && tu.targetLang != "" {
		effectiveTargetLang = model.LocaleID(tu.targetLang)
	}
	if targetContent != "" && !effectiveTargetLang.IsEmpty() {
		// Parse the target body native IR once. It feeds the body-level
		// annotation (so the writer can reconstruct mrk wrappers and
		// between-mrk whitespace exactly) and, in the unsegmented case,
		// is shared with the single target segment to avoid re-decoding.
		targetNative := parseNativeContent(targetContent)
		// Check if target has mrk segments
		targetSegs := parseMrkSegmentsFromString(targetContent)
		if len(targetSegs) > 0 {
			var tgtRuns []model.Run
			spans := make([]model.Span, len(targetSegs))
			for i, seg := range targetSegs {
				nc := parseNativeContent(seg.text)
				runs := nativeToRuns(nc)
				start := len(tgtRuns)
				tgtRuns = append(tgtRuns, runs...)
				spans[i] = model.Span{ID: seg.mid, Range: model.RunRange{StartRun: start, EndRun: len(tgtRuns)}}
				block.SetAnno(targetSegNativeKey(effectiveTargetLang, seg.mid), &SegmentNativeAnnotation{Content: nc})
			}
			block.SetTargetRuns(effectiveTargetLang, tgtRuns)
			key := model.Variant(effectiveTargetLang)
			block.SetSegmentation(&key, spans)
		} else {
			block.SetTargetRuns(effectiveTargetLang, nativeToRuns(targetNative))
			block.SetAnno(targetSegNativeKey(effectiveTargetLang, "s1"), &SegmentNativeAnnotation{Content: targetNative})
		}
		// Attach body-level native IR for <target>. Walking this lets
		// the writer reconstruct mrk wrappers and between-mrk
		// whitespace exactly as the source file had them.
		block.SetAnno("xliff:target-body", &TargetBodyNativeAnnotation{
			Locale:  effectiveTargetLang,
			Content: targetNative,
		})
	}
	if hasTargetElem {
		// Whether target was populated, empty, or self-closing —
		// preserve its attributes (state, state-qualifier, xml:lang,
		// custom-namespace) so the writer can re-emit the complete
		// element verbatim.
		block.SetAnno("xliff:target-attrs", newTargetAttrsAnnotation(tu.targetAttrs))
	}

	// Add notes (one note collection, not numbered keys).
	for _, note := range tu.notes {
		block.AddNote(&model.NoteAnnotation{
			Text:      note.text,
			From:      note.from,
			Priority:  note.priority,
			Annotates: note.annotates,
		})
	}

	// Add alt-trans as alt-translation candidates (one collection, not numbered keys).
	for _, at := range tu.altTrans {
		var matchType model.MatchType
		if at.matchQuality >= 100 {
			matchType = model.MatchExact
		} else if at.matchQuality > 0 {
			matchType = model.MatchFuzzy
		}

		alt := &model.AltTranslation{
			Origin:        at.origin,
			CombinedScore: at.matchQuality,
			MatchType:     matchType,
			FromOriginal:  true,
		}
		if at.source != "" {
			alt.Source = []model.Run{{Text: &model.TextRun{Text: at.source}}}
		}
		if at.target != "" {
			alt.Target = []model.Run{{Text: &model.TextRun{Text: at.target}}}
		}
		block.AddAltTranslation(alt)
	}

	return block
}

// Annotation is an alias for any to make things cleaner.
type Annotation = any

// copyTargetAttrs copies a <target> element's attributes into the
// parsed-tu structure. Skips xmlns:* declarations (handled at the
// document level) but keeps everything else (state, state-qualifier,
// xml:lang, custom-namespace attrs) so the writer can reproduce the
// element verbatim.
func copyTargetAttrs(in []xml.Attr) []xml.Attr {
	if len(in) == 0 {
		return nil
	}
	out := make([]xml.Attr, 0, len(in))
	for _, a := range in {
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// newTargetAttrsAnnotation converts an xml.Attr slice (from the
// parser) into the wire-friendly Attr form. encoding/xml resolves
// namespace prefixes to URIs in Name.Space, so we map the well-known
// URIs back to their conventional prefixes (xml, xmlns) — otherwise
// the writer would emit `http://www.w3.org/XML/1998/namespace:lang`
// which is invalid XML.
func newTargetAttrsAnnotation(in []xml.Attr) *TargetAttrsAnnotation {
	out := make([]Attr, 0, len(in))
	for _, a := range in {
		out = append(out, Attr{Space: prefixForURI(a.Name.Space), Local: a.Name.Local, Value: a.Value})
	}
	return &TargetAttrsAnnotation{Attrs: out}
}

// prefixForURI maps known XML namespace URIs back to their
// conventional prefix. Unknown URIs are returned as-is (caller can
// decide whether to emit them, drop them, or invent a prefix).
func prefixForURI(uri string) string {
	switch uri {
	case "http://www.w3.org/XML/1998/namespace":
		return "xml"
	case "http://www.w3.org/2000/xmlns/":
		return "xmlns"
	}
	return uri
}

// parseInlineContent parses XLIFF 1.2 inline elements and returns
// a Run sequence with text interleaved with Ph / PcOpen / PcClose /
// Sub inline codes.
func parseInlineContent(innerXML string) []model.Run {
	if innerXML == "" {
		return nil
	}

	// Wrap in a root element for parsing.
	wrapped := "<root>" + innerXML + "</root>"
	decoder := xml.NewDecoder(strings.NewReader(wrapped))
	decoder.Strict = false

	var runs []model.Run
	var textBuf strings.Builder
	var gStack []string // stack of open <g> ids so the closing </g> knows which pcOpen to match
	depth := 0

	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		runs = append(runs, model.Run{Text: &model.TextRun{Text: textBuf.String()}})
		textBuf.Reset()
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				// Root element
				continue
			}

			switch t.Name.Local {
			case "bpt":
				id := attrVal(t.Attr, "id")
				data, subTexts := readInlineCodeContent(decoder)
				depth--
				flushText()
				runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{
					ID: id, Type: ctypeToSpanType(attrVal(t.Attr, "ctype")),
					Data: data, Equiv: attrVal(t.Attr, "equiv-text"),
				}})
				for _, sub := range subTexts {
					runs = append(runs, model.Run{Text: &model.TextRun{Text: sub}})
				}

			case "ept":
				id := attrVal(t.Attr, "id")
				data, subTexts := readInlineCodeContent(decoder)
				depth--
				flushText()
				runs = append(runs, model.Run{PcClose: &model.PcCloseRun{
					ID: id, Type: ctypeToSpanType(attrVal(t.Attr, "ctype")),
					Data: data, Equiv: attrVal(t.Attr, "equiv-text"),
				}})
				for _, sub := range subTexts {
					runs = append(runs, model.Run{Text: &model.TextRun{Text: sub}})
				}

			case "ph":
				id := attrVal(t.Attr, "id")
				data, subTexts := readInlineCodeContent(decoder)
				depth--
				flushText()
				runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
					ID: id, Type: ctypeToSpanType(attrVal(t.Attr, "ctype")),
					Data: data, Equiv: attrVal(t.Attr, "equiv-text"),
				}})
				// One text run per nested <sub> sub-flow, in tree order.
				// The writer's IR walk re-encounters the sub during ph
				// inner traversal and consumes these texts there — so
				// pseudo-translate / AI-translate transformations on the
				// run text propagate into <sub>.
				for _, sub := range subTexts {
					runs = append(runs, model.Run{Text: &model.TextRun{Text: sub}})
				}

			case "x":
				id := attrVal(t.Attr, "id")
				flushText()
				runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
					ID: id, Type: ctypeToSpanType(attrVal(t.Attr, "ctype")),
					Equiv: attrVal(t.Attr, "equiv-text"),
				}})

			case "bx":
				id := attrVal(t.Attr, "id")
				flushText()
				runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{
					ID: id, Type: ctypeToSpanType(attrVal(t.Attr, "ctype")),
					Equiv: attrVal(t.Attr, "equiv-text"),
				}})

			case "ex":
				id := attrVal(t.Attr, "id")
				flushText()
				runs = append(runs, model.Run{PcClose: &model.PcCloseRun{
					ID: id, Type: ctypeToSpanType(attrVal(t.Attr, "ctype")),
					Equiv: attrVal(t.Attr, "equiv-text"),
				}})

			case "g":
				id := attrVal(t.Attr, "id")
				flushText()
				runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{
					ID: id, Type: ctypeToSpanType(attrVal(t.Attr, "ctype")),
					Equiv: attrVal(t.Attr, "equiv-text"),
				}})
				gStack = append(gStack, id)

			case "it":
				id := attrVal(t.Attr, "id")
				pos := attrVal(t.Attr, "pos")
				data, subTexts := readInlineCodeContent(decoder)
				depth--
				flushText()
				typ := ctypeToSpanType(attrVal(t.Attr, "ctype"))
				equiv := attrVal(t.Attr, "equiv-text")
				switch pos {
				case "open":
					runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{
						ID: id, Type: typ, Data: data, Equiv: equiv,
					}})
				case "close":
					runs = append(runs, model.Run{PcClose: &model.PcCloseRun{
						ID: id, Type: typ, Data: data, Equiv: equiv,
					}})
				default:
					runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
						ID: id, Type: typ, Data: data, Equiv: equiv,
					}})
				}
				for _, sub := range subTexts {
					runs = append(runs, model.Run{Text: &model.TextRun{Text: sub}})
				}

			case "mrk":
				id := attrVal(t.Attr, "mid")
				mtype := attrVal(t.Attr, "mtype")
				flushText()
				runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{
					ID: id, Type: "xliff:mrk:" + mtype,
				}})
				gStack = append(gStack, id)

			case "sub":
				// Translatable sub-flow inside inline codes. Read content.
				readElementText(decoder)
				depth--

			default:
				// Unknown inline element — skip content.
			}

		case xml.EndElement:
			depth--
			if depth == 0 {
				// Root end
				continue
			}
			if t.Name.Local == "g" {
				flushText()
				id := ""
				if n := len(gStack); n > 0 {
					id = gStack[n-1]
					gStack = gStack[:n-1]
				}
				runs = append(runs, model.Run{PcClose: &model.PcCloseRun{ID: id}})
			} else if t.Name.Local == "mrk" {
				flushText()
				id := ""
				if n := len(gStack); n > 0 {
					id = gStack[n-1]
					gStack = gStack[:n-1]
				}
				runs = append(runs, model.Run{PcClose: &model.PcCloseRun{ID: id, Type: "xliff:mrk"}})
			}

		case xml.CharData:
			if depth >= 1 {
				textBuf.Write(t)
			}
		}
	}
	flushText()
	return runs
}

// readElementText reads text content of an element until its end tag.
// It handles nested elements like <sub>.
func readElementText(decoder *xml.Decoder) string {
	var buf strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			buf.Write(t)
		}
	}
	return buf.String()
}

// readInlineCodeContent reads the content of an inline code element
// (bpt/ept/ph/it) until its end tag, returning two things in parallel:
//
//   - data: the raw text content (the "native code" payload — e.g.
//     "<b>" inside `<bpt>`), for use as PlaceholderRun.Data and the
//     downconversion path consumed by tools that don't see the native
//     IR.
//   - subTexts: one entry per translatable text node inside nested
//     `<sub>` sub-flow elements, in the same tree order the writer
//     will encounter them. The IR's Sub.Children has a separate Text
//     node for each CharData chunk between nested elements (e.g.
//     `[nested<ph>x</ph>still in sub]` is three children: Text("[nested"),
//     Ph, Text("still in sub]")), so we must emit one subTexts entry
//     per such Text node — not one concatenated string per sub — for
//     the writer's run-substitution to align.
//
// Text inside nested code elements (ph/bpt/ept/it) within the sub is
// treated as opaque native code, not a translatable sub-flow text.
func readInlineCodeContent(decoder *xml.Decoder) (data string, subTexts []string) {
	var dataBuf strings.Builder
	var pending strings.Builder
	// subDepth: depth of <sub> nesting we're currently inside (0 = not in any sub).
	// codeDepth: depth of nested code elements (ph/bpt/ept/it) inside sub.
	// We only collect text into pending when subDepth >= 1 && codeDepth == 0.
	subDepth, codeDepth := 0, 0
	depth := 1
	flushSubText := func() {
		if pending.Len() > 0 {
			subTexts = append(subTexts, pending.String())
			pending.Reset()
		}
	}
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "sub":
				if subDepth >= 1 && codeDepth == 0 {
					flushSubText()
				}
				subDepth++
			case "ph", "bpt", "ept", "it":
				if subDepth >= 1 && codeDepth == 0 {
					flushSubText()
					codeDepth++
				} else if codeDepth >= 1 {
					codeDepth++
				}
			}
		case xml.EndElement:
			depth--
			switch t.Name.Local {
			case "sub":
				if subDepth == 1 && codeDepth == 0 {
					flushSubText()
				}
				subDepth--
			case "ph", "bpt", "ept", "it":
				if codeDepth >= 1 {
					codeDepth--
				}
			}
		case xml.CharData:
			dataBuf.Write(t)
			if subDepth >= 1 && codeDepth == 0 {
				pending.Write(t)
			}
		}
	}
	return dataBuf.String(), subTexts
}

// attrVal returns the value of named attribute, or "".
func attrVal(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// ctypeToSpanType converts a ctype attribute to a semantic span type.
func ctypeToSpanType(ctype string) string {
	switch ctype {
	case "bold", "x-bold":
		return "fmt:bold"
	case "italic", "x-italic":
		return "fmt:italic"
	case "underlined", "x-underlined":
		return "fmt:underline"
	case "link", "x-link":
		return "link:hyperlink"
	case "lb", "x-lb":
		return "struct:break"
	case "image", "x-image":
		return "media:image"
	case "":
		return ""
	default:
		return "xliff:" + ctype
	}
}

// parseMrkSegmentsFromString parses mrk mtype="seg" elements from a target string.
func parseMrkSegmentsFromString(targetXML string) []segment {
	var segs []segment
	wrapped := "<root>" + targetXML + "</root>"
	decoder := xml.NewDecoder(strings.NewReader(wrapped))
	decoder.Strict = false

	depth := 0
	var currentSeg *segment
	var buf strings.Builder

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "mrk" && depth == 2 {
				mtype := attrVal(t.Attr, "mtype")
				mid := attrVal(t.Attr, "mid")
				if mtype == "seg" {
					buf.Reset()
					currentSeg = &segment{mid: mid}
					continue
				}
			}
			if currentSeg != nil {
				// Inline element inside mrk
				buf.WriteString("<")
				buf.WriteString(t.Name.Local)
				for _, a := range t.Attr {
					buf.WriteString(" ")
					buf.WriteString(a.Name.Local)
					buf.WriteString(`="`)
					buf.WriteString(xmlEscapeAttr(a.Value))
					buf.WriteString(`"`)
				}
				buf.WriteString(">")
			}
		case xml.EndElement:
			depth--
			if t.Name.Local == "mrk" && currentSeg != nil {
				currentSeg.text = buf.String()
				segs = append(segs, *currentSeg)
				currentSeg = nil
			} else if currentSeg != nil {
				buf.WriteString("</")
				buf.WriteString(t.Name.Local)
				buf.WriteString(">")
			}
		case xml.CharData:
			if currentSeg != nil {
				// CharData arrives decoded; the buffer is re-parsed as
				// XML downstream, so re-escape so `<b>` doesn't read
				// back as a start tag and lose the inline text.
				buf.WriteString(xmlEscapeText(string(t)))
			}
		}
	}
	return segs
}

// skelText appends text to the skeleton buffer if active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		r.skelBuf.WriteString(s)
	}
}

// skelRef flushes buffered text and writes a block reference to the skeleton store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
