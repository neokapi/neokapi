package openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// wmlLangElementRE matches the WordprocessingML <w:lang> element in
// both self-closing (`<w:lang .../>`) and open/close (`<w:lang ...>
// </w:lang>`) forms. Source documents almost always self-close this
// element, but the encoding/xml-driven reader/skeleton path can
// re-emit it in open/close form, in which case the self-closing-only
// regex would silently fail to strip it.
//
// <w:lang> is stripped by upstream Okapi's RunSkippableElements
// (lines 50-62 of RunSkippableElements.java) keyed on the
// TRANSITIONAL WPML namespace QName — see stripWMLSkippableElements
// for the Strict-OOXML gate that mirrors upstream's QName semantics.
var wmlLangElementRE = regexp.MustCompile(
	`<w:lang\b[^>]*/>` +
		`|<w:lang\b[^>]*></w:lang>`,
)

// wmlBidiVisualElementRE matches the WordprocessingML <w:bidiVisual>
// paragraph-property element (RTL visual hint, ECMA-376-1 §17.3.1.7)
// in both forms. Stripped unconditionally by upstream Okapi's
// BlockPropertiesFactory via SkippableElements.Default
// (BLOCK_PROPERTY_BIDI_VISUAL).
var wmlBidiVisualElementRE = regexp.MustCompile(
	`<w:bidiVisual\b[^>]*/>` +
		`|<w:bidiVisual\b[^>]*></w:bidiVisual>`,
)

// wmlMoveRangeStrippableElementRE matches the cross-structure
// revision-tracking range markers that okapi unconditionally drops
// during round-trip when bPreferenceAutomaticallyAcceptRevisions=true
// (the default). Each marker is overwhelmingly emitted in self-closing
// form (`<w:moveToRangeStart .../>`) but the schema permits an empty
// open/close pair; both forms are matched. Element list:
//   - <w:moveToRangeStart>   / <w:moveToRangeEnd>
//   - <w:moveFromRangeStart> / <w:moveFromRangeEnd>
//
// See SkippableElement.RevisionCrossStructure (lines 143-173 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/SkippableElement.java)
// and the wiring in SkippableElements.RevisionCrossStructure /
// MoveFromRevisionCrossStructure (lines 336-410 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/SkippableElements.java),
// BlockSkippableElements (lines 64-78 of BlockSkippableElements.java),
// and StyledTextPart (lines 212-225 of StyledTextPart.java).
var wmlMoveRangeStrippableElementRE = regexp.MustCompile(
	`<w:(?:moveToRangeStart|moveToRangeEnd|moveFromRangeStart|moveFromRangeEnd)\b[^>]*/>` +
		`|<w:(?:moveToRangeStart|moveToRangeEnd|moveFromRangeStart|moveFromRangeEnd)\b[^>]*>\s*</w:(?:moveToRangeStart|moveToRangeEnd|moveFromRangeStart|moveFromRangeEnd)>`,
)

// wmlEmptyPropertiesContainerRE matches WordprocessingML run-property and
// paragraph-property containers that have no attributes and no element
// children. Okapi's RunProperties.Default.getEvents (line 580 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/RunProperties.java)
// returns Collections.emptyList() when properties().isEmpty(), and
// BlockProperties.Default.getEvents (line 169-180 of BlockProperties.java)
// returns Collections.emptyList() when isEmpty() (no attributes AND no
// properties; isEmpty at line 203-205). The element is therefore omitted
// entirely from the round-trip output. Both <w:rPr> and <w:pPr> are
// container-only in the WML schema and never carry attributes in
// okapi-testdata fixtures, so the empty-with-attributes case need not
// be considered. Stripping must be iterated because removing an empty
// <w:rPr/> can leave its parent <w:pPr/> empty and itself eligible.
//
// The body matcher [\s]* tolerates any whitespace between the open and
// close tags — encoding/xml may emit indented or newline-padded XML
// which would otherwise survive the strip and force the empty container
// into the output (and into the canonical comparison, where it diverges
// against the okapi reference's omitted-entirely encoding).
var wmlEmptyPropertiesContainerRE = regexp.MustCompile(
	`<w:rPr>\s*</w:rPr>` +
		`|<w:rPr\s*/>` +
		`|<w:pPr>\s*</w:pPr>` +
		`|<w:pPr\s*/>`,
)

// wmlNoProofRE matches the run-property <w:noProof> element (no
// spelling/grammar marker) in both self-closing and open/close form.
// Okapi strips this from rPr in run/style contexts via
// RunSkippableElements (line 55 of okapi/filters/openxml/src/main/java/
// net/sf/okapi/filters/openxml/RunSkippableElements.java) and the
// RunProperties parser. The element is container-free in practice
// (carries no attributes either; the schema permits w:val but no fixture
// corpus uses it), so a simple element-only regex is sufficient.
var wmlNoProofRE = regexp.MustCompile(
	`<w:noProof\b[^>]*/>` +
		`|<w:noProof\b[^>]*></w:noProof>`,
)

// wmlRevisionParagraphMarkRE matches the EMPTY-BODY forms of the
// paragraph-mark revision elements that appear INSIDE <w:rPr>:
//   - <w:ins .../>           (RUN_PROPERTY_INSERTED_PARAGRAPH_MARK)
//   - <w:ins ...></w:ins>    (same, re-emitted by encoding/xml)
//   - <w:del .../>           (RUN_PROPERTY_DELETED_PARAGRAPH_MARK)
//   - <w:del ...></w:del>    (same)
//   - <w:moveTo .../>        (RUN_PROPERTY_MOVED_PARAGRAPH_TO)
//   - <w:moveTo ...></w:moveTo>
//   - <w:moveFrom .../>      (RUN_PROPERTY_MOVED_PARAGRAPH_FROM)
//   - <w:moveFrom ...></w:moveFrom>
//
// These are skipped by okapi's SkippableElements.RevisionProperty when
// AutomaticallyAcceptRevisions=true (the default — line 819 of
// ConditionalParameters.java). Per okapi's SkippableElement.java lines
// 231-234.
//
// Only the empty-body form is matched: the content-wrapping form
// (<w:ins><w:r>...</w:r></w:ins> as inline-content marker — child
// element present) is handled differently by okapi (the wrapper is
// unwrapped, children kept). The fixture corpus uses self-closing/empty
// form for paragraph-mark revisions universally; the empty-body open/
// close form arises only when encoding/xml re-emits a previously
// self-closing tag as open/close.
var wmlRevisionParagraphMarkRE = regexp.MustCompile(
	`<w:ins\b[^>]*/>` +
		`|<w:ins\b[^>]*></w:ins>` +
		`|<w:del\b[^>]*/>` +
		`|<w:del\b[^>]*></w:del>` +
		`|<w:moveTo\b[^>]*/>` +
		`|<w:moveTo\b[^>]*></w:moveTo>` +
		`|<w:moveFrom\b[^>]*/>` +
		`|<w:moveFrom\b[^>]*></w:moveFrom>`,
)

// wmlRevisionPropertyChangeNames are the WordprocessingML
// revision-property "change tracking" elements that okapi strips when
// AutomaticallyAcceptRevisions=true (the default — see line 819 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// ConditionalParameters.java) via SkippableElements.RevisionProperty
// (lines 506-569 of SkippableElements.java). All carry nested
// <w:rPr>/<w:pPr>/etc. snapshots of pre-revision properties; stripping
// them preserves only the post-revision (current) state.
//
// Per okapi's SkippableElement.RevisionProperty enum (lines 229-245 of
// SkippableElement.java):
//   - pPrChange    (PARAGRAPH_PROPERTIES_CHANGE)
//   - rPrChange    (RUN_PROPERTIES_CHANGE)
//   - sectPrChange (SECTION_PROPERTIES_CHANGE)
//   - tblGridChange (TABLE_GRID_CHANGE)
//   - tblPrChange  (TABLE_PROPERTIES_CHANGE)
//   - tblPrExChange (TABLE_PROPERTIES_EXCEPTIONS_CHANGE)
//   - tcPrChange   (TABLE_CELL_PROPERTIES_CHANGE)
//   - trPrChange   (TABLE_ROW_PROPERTIES_CHANGE)
//
// Note: <w:ins> and <w:del> when used as paragraph-mark revision markers
// inside <w:rPr> (RUN_PROPERTY_INSERTED/DELETED_PARAGRAPH_MARK) are also
// in the same enum but require context-aware stripping (only inside
// <w:rPr>, not as content wrappers); they are intentionally NOT included
// in the unconditional regex to avoid stripping content-wrapper <w:ins>/
// <w:del> elements that have legitimate text payload. Most fixtures with
// these don't reach the canonical-equal tier for other reasons anyway.
var wmlRevisionPropertyChangeNames = []string{
	"pPrChange",
	"rPrChange",
	"sectPrChange",
	"tblGridChange",
	"tblPrChange",
	"tblPrExChange",
	"tcPrChange",
	"trPrChange",
}

// stripBalancedElement removes every occurrence of <w:NAME ...>...</w:NAME>
// (and the self-closing form <w:NAME .../>) from data, where NAME is the
// supplied local name. The matcher is non-nested — the *Change elements
// in the okapi-testdata corpus never embed themselves recursively, and
// the schema doesn't allow it either. Returns the original slice if the
// element name doesn't appear at all (cheap fast path).
func stripBalancedElement(data []byte, name string) []byte {
	startPrefix := []byte("<w:" + name)
	if !bytes.Contains(data, startPrefix) {
		return data
	}
	endTag := []byte("</w:" + name + ">")
	out := make([]byte, 0, len(data))
	for {
		i := bytes.Index(data, startPrefix)
		if i < 0 {
			out = append(out, data...)
			break
		}
		// Confirm the element-name boundary so "<w:noProofX" doesn't match
		// a longer element name. The next byte must be `>`, `/`, or
		// whitespace.
		j := i + len(startPrefix)
		if j >= len(data) {
			out = append(out, data...)
			break
		}
		b := data[j]
		if b != '>' && b != '/' && b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			out = append(out, data[:j+1]...)
			data = data[j+1:]
			continue
		}
		// Find element terminator within the start tag.
		k := bytes.IndexByte(data[j:], '>')
		if k < 0 {
			out = append(out, data...)
			break
		}
		startEnd := j + k
		out = append(out, data[:i]...)
		// Self-closing form <w:NAME .../>: skip the tag.
		if startEnd > 0 && data[startEnd-1] == '/' {
			data = data[startEnd+1:]
			continue
		}
		// Open form: find matching close tag.
		closeIdx := bytes.Index(data[startEnd+1:], endTag)
		if closeIdx < 0 {
			// Unbalanced — bail out, append remainder unchanged.
			out = append(out, data[i:]...)
			break
		}
		data = data[startEnd+1+closeIdx+len(endTag):]
	}
	return out
}

// stripWMLSkippableElements removes WordprocessingML elements from an
// XML part to mirror okapi's BlockProperties/RunProperties,
// RevisionCrossStructure, and RevisionProperty stripping. Returns the
// original slice if nothing was matched (cheap fast paths).
//
// <w:lang> stripping is gated on the document's WordprocessingML
// namespace URI. Upstream Okapi's RunSkippableElements identifies lang
// by QName — Namespaces.WordProcessingML.getQName("lang") — keyed on
// the TRANSITIONAL WPML URI ("http://schemas.openxmlformats.org/
// wordprocessingml/2006/main", Namespaces.java:26). For Strict OOXML
// documents using "http://purl.oclc.org/ooxml/wordprocessingml/main"
// (858.docx — Word's "Save As → Strict Open XML Document" output),
// the QName does NOT match — the SkippableElements.Default contains
// check at SkippableElements.java:122 returns false — so upstream
// PRESERVES <w:lang> through round-trip. The reference output for
// 858.docx keeps <w:lang> in the paragraph mark rPr (inside pPr) AND
// in the WSO-synthesised paragraph style's rPr.
//
// Native mirrors this: when the document binds the "w" prefix to the
// strict URI, the lang strip is skipped. Both prefix and URI are
// observed in the part itself — the writer doesn't track which doc
// the part came from, but every WPML XML part declares the prefix
// binding on its root element. ECMA-376 Part 1 §A.1 / ISO/IEC 29500-1
// §A.1 (the two URIs).
func stripWMLSkippableElements(data []byte) []byte {
	stripLang := !bytes.Contains(data, []byte(wmlStrictNamespace))
	if stripLang && bytes.Contains(data, []byte("<w:lang")) {
		data = wmlLangElementRE.ReplaceAll(data, nil)
	}
	if bytes.Contains(data, []byte("<w:bidiVisual")) {
		data = wmlBidiVisualElementRE.ReplaceAll(data, nil)
	}
	if bytes.Contains(data, []byte("<w:moveToRange")) || bytes.Contains(data, []byte("<w:moveFromRange")) {
		data = wmlMoveRangeStrippableElementRE.ReplaceAll(data, nil)
	}
	if bytes.Contains(data, []byte("<w:noProof")) {
		data = wmlNoProofRE.ReplaceAll(data, nil)
	}
	if bytes.Contains(data, []byte("<w:ins")) ||
		bytes.Contains(data, []byte("<w:del")) ||
		bytes.Contains(data, []byte("<w:moveTo")) ||
		bytes.Contains(data, []byte("<w:moveFrom")) {
		data = wmlRevisionParagraphMarkRE.ReplaceAll(data, nil)
	}
	for _, name := range wmlRevisionPropertyChangeNames {
		data = stripBalancedElement(data, name)
	}
	// Iterate empty <w:rPr>/<w:pPr> stripping until fixpoint: removing an
	// empty <w:rPr></w:rPr> nested inside an otherwise-empty <w:pPr>
	// leaves the parent eligible on the next pass. The fixture corpus
	// requires at most two iterations (<w:p><w:pPr><w:rPr><w:lang/></w:rPr></w:pPr>
	// becomes <w:p/> after lang+rPr+pPr strips), but the loop terminates
	// generally because each pass strictly shrinks the buffer.
	// Loop until the empty-container regex stops shrinking the buffer.
	// The fast-path bytes.Contains gate looks at "<w:rPr" and "<w:pPr"
	// substrings — matches any potentially-stripable form including
	// whitespace-padded variants — which a more specific gate would
	// miss after encoding/xml indented re-emission.
	for bytes.Contains(data, []byte("<w:rPr")) ||
		bytes.Contains(data, []byte("<w:pPr")) {
		next := wmlEmptyPropertiesContainerRE.ReplaceAll(data, nil)
		if len(next) == len(data) {
			break
		}
		data = next
	}
	return data
}

// shouldStripWMLLang reports whether the given ZIP entry path is a
// WordprocessingML XML part where okapi's lang/bidiVisual and
// RevisionCrossStructure (moveTo/moveFrom range) stripping applies.
// Other parts (drawings, themes, settings.xml) are untouched.
func shouldStripWMLLang(name string) bool {
	if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
		return false
	}
	switch {
	case name == "word/document.xml",
		name == "word/styles.xml",
		name == "word/footnotes.xml",
		name == "word/endnotes.xml",
		name == "word/comments.xml":
		return true
	case strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml"),
		strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml"):
		return true
	}
	return false
}

// wmlLangValAttrRE matches the w:val attribute on a <w:lang ...> or
// <w:themeFontLang ...> element and captures the existing value.
// Submatches: 1=tag name (lang|themeFontLang), 2=quote char, 3=value.
//
// The match is anchored on the opening "<w:lang" or "<w:themeFontLang"
// followed by a word boundary (so it doesn't accept "<w:langfoo>"), then
// scans up to the element terminator (`>` or `/>`) for any w:val=
// attribute. Single and double quotes are both supported. The character
// class for the value side excludes the quote so we don't cross attribute
// boundaries.
var wmlLangValAttrRE = regexp.MustCompile(
	`(<w:(lang|themeFontLang)\b[^>]*?\bw:val=)(["'])([^"']*)(["'])`,
)

// shouldRewriteWMLLangVal reports whether the given ZIP entry path is a
// WordprocessingML XML part where okapi rewrites <w:lang>/<w:themeFontLang>
// w:val attributes from the source locale to the target locale on
// round-trip (mirroring GenericSkeletonWriter's Property.LANGUAGE
// rewriting; see writer.go SetSourceLocale godoc).
//
// The set is the strip set plus settings.xml — okapi's
// RUN_PROPERTY_LANGUAGE skippable list strips <w:lang/> from rPr in
// document/styles/footnotes/endnotes/comments/header/footer parts (so any
// surviving <w:lang> there must have been outside an rPr and is rewritten
// in the same way), while <w:themeFontLang/> sits in settings.xml only and
// is preserved by okapi but with its w:val retargeted.
func shouldRewriteWMLLangVal(name string) bool {
	if name == "word/settings.xml" {
		return true
	}
	return shouldStripWMLLang(name)
}

// rewriteWMLLangVal rewrites the w:val attribute on every <w:lang> and
// <w:themeFontLang> element when its existing value's primary language
// matches the source locale's primary language. The replacement value
// is the target locale string verbatim (okapi uses LocaleId#toString,
// which is the BCP-47 form).
//
// This mirrors okapi/core/src/main/java/net/sf/okapi/common/skeleton/
// GenericSkeletonWriter.java lines 808-816:
//
//	if ( Property.LANGUAGE.equals(name) ) {
//	    LocaleId locId = LocaleId.fromString(value);
//	    if ( locId.sameLanguageAs(inputLoc) ) {
//	        value = outputLoc.toString();
//	    }
//	}
//
// in combination with okapi/filters/openxml/src/main/java/net/sf/okapi/
// filters/openxml/ContentFilter.java lines 527-537, where the openxml
// filter normalizes the attribute name on <w:lang> and <w:themeFontLang>
// to Property.LANGUAGE so the writer's retargeting kicks in.
//
// Returns the original slice if no eligible attribute was found (so the
// caller can avoid recompressing pass-through entries).
func rewriteWMLLangVal(data []byte, sourceLocale, targetLocale model.LocaleID) []byte {
	if targetLocale.IsEmpty() {
		return data
	}
	// Strict OOXML namespace: upstream's QName-keyed Property.LANGUAGE
	// rewrite does NOT match elements bound to the strict URI
	// "http://purl.oclc.org/ooxml/wordprocessingml/main" — the rewrite
	// hook lives on ContentFilter (lines 527-537) and only fires when
	// the openxml filter has classified the element as a
	// Property.LANGUAGE-carrying WordProcessingML element, which is
	// QName-keyed by the transitional URI (Namespaces.java:26). 858.docx
	// reference output keeps <w:lang w:val="en-US"/> through round-trip
	// even with target=fr, so the native rewrite must also skip strict
	// parts.
	if bytes.Contains(data, []byte(wmlStrictNamespace)) {
		return data
	}
	src := primaryLangOf(sourceLocale)
	if src == "" {
		// Default to "en" — matches okapi OpenXMLFilter's behaviour when
		// no source locale was supplied via setOptions.
		src = "en"
	}
	if !bytes.Contains(data, []byte("<w:lang")) && !bytes.Contains(data, []byte("<w:themeFontLang")) {
		return data
	}
	tgt := []byte(string(targetLocale))
	return wmlLangValAttrRE.ReplaceAllFunc(data, func(match []byte) []byte {
		sub := wmlLangValAttrRE.FindSubmatch(match)
		if sub == nil {
			return match
		}
		// sub[1]=prefix incl. "w:val=", sub[3]=open quote, sub[4]=value, sub[5]=close quote
		existing := string(sub[4])
		if primaryLangOf(model.LocaleID(existing)) != src {
			return match
		}
		out := make([]byte, 0, len(sub[1])+len(sub[3])+len(tgt)+len(sub[5]))
		out = append(out, sub[1]...)
		out = append(out, sub[3]...)
		out = append(out, tgt...)
		out = append(out, sub[5]...)
		return out
	})
}

// primaryLangOf returns the lower-cased primary language subtag of a
// BCP-47 locale ID. Mirrors okapi LocaleId.sameLanguageAs comparison
// semantics (region/script ignored).
func primaryLangOf(l model.LocaleID) string {
	s := strings.ToLower(string(l))
	if i := strings.IndexAny(s, "-_"); i >= 0 {
		s = s[:i]
	}
	return s
}

// Writer implements DataFormatWriter for OpenXML files.
type Writer struct {
	format.BaseFormatWriter
	cfg             *Config
	skeletonStore   *format.SkeletonStore
	originalContent []byte

	// sourceLocale records the input/source locale supplied to the writer
	// (defaults to "en" — okapi's LocaleId.EMPTY default for OpenXMLFilter).
	// Used by the WordprocessingML lang-attribute rewriter to decide whether
	// an existing <w:lang>/<w:themeFontLang> w:val matches the source
	// language and should be retargeted to w.Locale.
	sourceLocale model.LocaleID

	// mediaReplacements maps ZIP entry paths (e.g., "word/media/image1.png")
	// to replacement binary content for locale-variant media substitution (Bowrain AD-007).
	mediaReplacements map[string][]byte

	// blocks holds the current Write call's block index, populated by
	// Write() before invoking renderBlock and consumed by
	// expandDrawingMarkers when renderWMLBlock's TypeImage handler
	// substitutes <!--KAPI-PROP:tu123--> / <!--KAPI-PARA:tu123-->
	// markers inside captured drawing payloads (set by the WML
	// reader via extractDrawingTranslations). Reset at the end of
	// each Write call.
	blocks map[string]*model.Block
}

var _ format.SkeletonStoreConsumer = (*Writer)(nil)
var _ format.OriginalContentSetter = (*Writer)(nil)
var _ format.SourceLocaleSetter = (*Writer)(nil)

// SetMediaReplacement registers a locale-variant media file to substitute
// during output reconstruction. The zipPath should match the original
// entry path (e.g., "word/media/image1.png").
func (w *Writer) SetMediaReplacement(zipPath string, data []byte) {
	if w.mediaReplacements == nil {
		w.mediaReplacements = make(map[string][]byte)
	}
	w.mediaReplacements[zipPath] = data
}

// NewWriter creates a new OpenXML writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "openxml",
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming reconstruction.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// SetOriginalContent sets the original document bytes for reconstruction.
func (w *Writer) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// SetSourceLocale records the source/input locale. Used by the
// WordprocessingML lang-attribute rewriter (mirrors okapi's
// GenericSkeletonWriter behavior at lines 808-816 of okapi/core/src/
// main/java/net/sf/okapi/common/skeleton/GenericSkeletonWriter.java
// which retargets Property.LANGUAGE-named attributes from inputLoc to
// outputLoc when sameLanguageAs(inputLoc) holds).
func (w *Writer) SetSourceLocale(locale model.LocaleID) {
	w.sourceLocale = locale
}

// Write consumes Parts and writes the reconstructed OpenXML document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Collect all blocks keyed by ID
	blocks := make(map[string]*model.Block)
	for part := range parts {
		if part.Type == model.PartBlock {
			if b, ok := part.Resource.(*model.Block); ok {
				blocks[b.ID] = b
			}
		}
	}
	w.blocks = blocks
	defer func() { w.blocks = nil }()

	if w.originalContent == nil {
		return errors.New("openxml: writer requires original content for reconstruction")
	}

	// Open original ZIP
	origZR, err := zip.NewReader(bytes.NewReader(w.originalContent), int64(len(w.originalContent)))
	if err != nil {
		return fmt.Errorf("openxml: invalid original ZIP: %w", err)
	}

	// Parse container
	info, err := parseContainer(origZR, w.cfg)
	if err != nil {
		return err
	}

	// Create output ZIP
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// If we have a skeleton store, use skeleton-based reconstruction
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("openxml: skeleton flush: %w", err)
		}
		if err := w.writeFromSkeleton(origZR, zw, &buf, info, blocks); err != nil {
			return err
		}
		_, err = w.Output.Write(buf.Bytes())
		return err
	}

	// Fallback: copy original unchanged
	if err := w.writeFromReparse(origZR, zw, &buf, blocks); err != nil {
		return err
	}
	_, err = w.Output.Write(buf.Bytes())
	return err
}

// writeFromSkeleton reconstructs translatable XML parts using the skeleton store.
// The skeleton stream contains part-boundary markers (skelPartStartPrefix/skelPartEndPrefix)
// that delimit each XML part's skeleton content. The writer collects each part's
// reconstructed bytes, then writes the output ZIP with replacements.
func (w *Writer) writeFromSkeleton(origZR *zip.Reader, zw *zip.Writer, buf *bytes.Buffer,
	info *containerInfo, blocks map[string]*model.Block) error {

	// Read all skeleton entries, splitting by part-boundary markers
	partContents := make(map[string][]byte)
	var currentPart string
	var currentBuf bytes.Buffer

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("openxml: reading skeleton: %w", err)
		}

		switch entry.Type {
		case format.SkeletonText:
			if currentPart != "" {
				currentBuf.Write(entry.Data)
			}

		case format.SkeletonRef:
			refID := string(entry.Data)

			// Check for part-boundary markers
			if strings.HasPrefix(refID, skelPartStartPrefix) {
				currentPart = strings.TrimPrefix(refID, skelPartStartPrefix)
				currentBuf.Reset()
				continue
			}
			if strings.HasPrefix(refID, skelPartEndPrefix) {
				partPath := strings.TrimPrefix(refID, skelPartEndPrefix)
				if currentBuf.Len() > 0 {
					partContents[partPath] = append([]byte{}, currentBuf.Bytes()...)
				}
				currentPart = ""
				currentBuf.Reset()
				continue
			}

			// Regular block ref — render and write
			if currentPart != "" {
				if block, ok := blocks[refID]; ok {
					currentBuf.WriteString(w.renderBlock(block, info.docType))
				}
			}
		}
	}

	// Write output ZIP: replace translatable parts with skeleton-reconstructed content,
	// and substitute locale-variant media files (Bowrain AD-007).
	isDOCX := info.docType == docTypeDOCX

	// AllowWordStyleOptimisation post-pass: applied per WML part that
	// participates in style synthesis. Styles synthesised across all
	// parts are accumulated in a single map keyed by styleId, then
	// injected into word/styles.xml at the end. This mirrors Okapi's
	// single-IdGenerator-per-filter-invocation scope (see
	// WordStyleDefinitions.readWith line 114).
	var (
		synthesised             map[string]synthesisedStyle
		orderedIDs              []string
		idCounter               int
		existingIDs             map[string]bool
		defaultParagraphStyleID string
		pendingStyles           map[string]pendingStylesEntry
		// hasStylesPart records whether word/styles.xml exists in the
		// source ZIP. When it does NOT, upstream Okapi instantiates
		// `StyleDefinitions.Empty` for the missing styles part
		// (WordDocument.java:115-119, calling styleDefinitions(EMPTY)
		// → new StyleDefinitions.Empty()). The optimiser still runs —
		// inserting <w:pStyle> in pPr and stripping common rPr props
		// from runs — but Empty.place(…) is a no-op and Empty.placedId()
		// returns null (StyleDefinitions.java:53-59), so the inserted
		// pStyle carries an empty w:val and no <w:style> is appended to
		// a styles part (none exists to append to). Per ECMA-376-1
		// §17.7.4, when no styles part is present no style hierarchy
		// exists; the empty-val pStyle is upstream's surfaced form of
		// "synthesis ran but produced no id."
		hasStylesPart bool
	)
	if isDOCX && w.cfg.OptimiseWordStyles {
		synthesised = make(map[string]synthesisedStyle)
		// Pre-load existing styleIds AND the default paragraph styleId
		// from the source styles.xml so generated NF974E24F-* ids don't
		// collide AND so synthesised styles' basedOn (and id parent
		// fragment) point at the document's actual default paragraph
		// style — mirroring upstream WordStyleDefinitions.Ids.defaultBased
		// (WordStyleDefinitions.java:485-491).
		for _, f := range origZR.File {
			if f.Name == "word/styles.xml" {
				hasStylesPart = true
				data, err := readZipFile(f)
				if err == nil {
					existingIDs = extractExistingStyleIDs(data)
					defaultParagraphStyleID = extractDefaultParagraphStyleID(data)
				}
				break
			}
		}
		if existingIDs == nil {
			existingIDs = make(map[string]bool)
		}
	}

	// wsoOptimised stashes the WSO-rewritten bytes for each
	// shouldOptimiseWMLPart part (keyed by ZIP entry name). It is
	// populated in the WSO PRE-PASS below, in canonical Okapi processing
	// order (mainPart first, then headers/footers/footnotes/endnotes/
	// comments — see ZipEntryComparator + reorderedPartPaths in
	// WordDocument.java:74-97). The shared idCounter ticks in that order
	// so the synthesised styleId sequence (NF974E24F-{parent}{N}) lines
	// up with the upstream filter's IdGenerator stream.
	wsoOptimised := map[string][]byte{}
	// Helper: post-process a WML XML payload (after lang strip + lang
	// retargeting, before recompression). WSO is applied in a separate
	// pre-pass — postNonWSOForName runs the field-marker reversal that
	// must always happen, then defers to wsoOptimised when the part has
	// already been WSO'd.
	postNonWSOForName := func(data []byte) []byte {
		if !isDOCX {
			return data
		}
		// Strip the field-rPr keep-empty marker the reader inserted to
		// prevent stripWMLSkippableElements from collapsing
		// `<w:rPr></w:rPr>` inside complex-field runs (see
		// fieldRPrKeepEmptyMarker in wml.go for the upstream-Okapi
		// citation). The marker only appears inside word/*.xml parts;
		// the contains-check is the cheap fast path.
		if bytes.Contains(data, []byte(fieldRPrKeepEmptyMarker)) {
			data = bytes.ReplaceAll(data, []byte(fieldRPrKeepEmptyMarker), nil)
		}
		// Reverse protectFieldPayloadFromStripping (see wml.go for the
		// upstream-Okapi citation): any element renamed with the keep
		// suffix is restored to its original WordprocessingML name now
		// that stripWMLSkippableElements has already run. The contains
		// check is the cheap fast path.
		if bytes.Contains(data, []byte(fieldKeepElementSuffix)) {
			for _, name := range fieldKeepElementNames {
				data = bytes.ReplaceAll(data, []byte("<w:"+name+fieldKeepElementSuffix), []byte("<w:"+name))
				data = bytes.ReplaceAll(data, []byte("</w:"+name+fieldKeepElementSuffix+">"), []byte("</w:"+name+">"))
			}
		}
		return data
	}
	postWML := func(name string, data []byte) []byte {
		data = postNonWSOForName(data)
		if isDOCX && w.cfg.OptimiseWordStyles && shouldOptimiseWMLPart(name) {
			if optimised, ok := wsoOptimised[name]; ok {
				return optimised
			}
		}
		return data
	}

	// WSO pre-pass: visit WSO-eligible parts in the order Okapi's
	// ZipEntryComparator produces (mainPart first; see Okapi
	// WordDocument.java:74-90 / ZipEntryComparator.java:39-44). For each
	// part, fetch the same bytes the file-emit loop would feed to postWML
	// (skeleton-reconstructed content if present, otherwise the raw ZIP
	// content with strip-lang and lang-retarget applied), apply the
	// non-WSO post-processing, and run optimizeWMLPart with the SHARED
	// idCounter so styleId sequence numbers stay in lockstep with the
	// upstream IdGenerator stream.
	if isDOCX && w.cfg.OptimiseWordStyles {
		wsoNames := wsoPartOrder(origZR, info)
		for _, name := range wsoNames {
			f := zipFileByName(origZR, name)
			if f == nil {
				continue
			}
			var data []byte
			if content, ok := partContents[name]; ok && len(content) > 0 {
				data = content
				if shouldStripWMLLang(name) {
					data = stripWMLSkippableElements(data)
				}
				if shouldRewriteWMLLangVal(name) {
					data = rewriteWMLLangVal(data, w.sourceLocale, w.Locale)
				}
			} else if shouldStripWMLLang(name) || shouldRewriteWMLLangVal(name) {
				raw, err := readZipFile(f)
				if err != nil {
					continue
				}
				data = raw
				if shouldStripWMLLang(name) {
					data = stripWMLSkippableElements(data)
				}
				if shouldRewriteWMLLangVal(name) {
					data = rewriteWMLLangVal(data, w.sourceLocale, w.Locale)
				}
			} else {
				continue
			}
			data = postNonWSOForName(data)
			data = optimizeWMLPart(data, existingIDs, defaultParagraphStyleID, hasStylesPart, &idCounter, synthesised, &orderedIDs)
			wsoOptimised[name] = data
		}
	}

	for _, f := range origZR.File {
		if content, ok := partContents[f.Name]; ok && len(content) > 0 {
			// Replace with skeleton-reconstructed content
			if isDOCX && shouldStripWMLLang(f.Name) {
				content = stripWMLSkippableElements(content)
			}
			if isDOCX && shouldRewriteWMLLangVal(f.Name) {
				content = rewriteWMLLangVal(content, w.sourceLocale, w.Locale)
			}
			content = postWML(f.Name, content)
			fh := f.FileHeader
			fh.Method = zip.Deflate
			// Clear data descriptor fields to avoid checksum issues
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(content); err != nil {
				return err
			}
		} else if replacement, ok := w.mediaReplacements[f.Name]; ok {
			// Replace with locale-variant media (Bowrain AD-007).
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(replacement); err != nil {
				return err
			}
		} else if isDOCX && (shouldStripWMLLang(f.Name) || shouldRewriteWMLLangVal(f.Name)) {
			// Pass-through WordprocessingML part (e.g. word/styles.xml,
			// word/settings.xml) that needs okapi-style lang/bidiVisual
			// stripping and/or <w:lang>/<w:themeFontLang> w:val
			// retargeting. Read, transform, re-emit with a recompressed
			// header.
			data, err := readZipFile(f)
			if err != nil {
				return err
			}
			if shouldStripWMLLang(f.Name) {
				data = stripWMLSkippableElements(data)
			}
			if shouldRewriteWMLLangVal(f.Name) {
				data = rewriteWMLLangVal(data, w.sourceLocale, w.Locale)
			}
			data = postWML(f.Name, data)
			// Defer styles.xml emission until all paragraph parts have
			// been visited so we know the synthesised set. We instead
			// stash the post-strip bytes in a sentinel map that's
			// flushed after the loop.
			if w.cfg.OptimiseWordStyles && isDOCX && f.Name == "word/styles.xml" {
				if pendingStyles == nil {
					pendingStyles = map[string]pendingStylesEntry{}
				}
				pendingStyles[f.Name] = pendingStylesEntry{header: f.FileHeader, data: data}
				continue
			}
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(data); err != nil {
				return err
			}
		} else {
			// Copy unchanged — use raw copy to preserve CRC/data descriptors
			if err := zw.Copy(f); err != nil {
				return err
			}
		}
	}

	// Late-emit styles.xml with synthesised <w:style> entries appended.
	if w.cfg.OptimiseWordStyles && isDOCX && pendingStyles != nil {
		for name, ps := range pendingStyles {
			data := ps.data
			if name == "word/styles.xml" && len(orderedIDs) > 0 {
				data = injectSynthesisedStyles(data, synthesised, orderedIDs)
			}
			fh := ps.header
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(data); err != nil {
				return err
			}
		}
	}

	return zw.Close()
}

// pendingStylesEntry holds a WML part (currently only styles.xml) that
// must be deferred until after all other parts have been post-processed
// — the synthesised-style set isn't complete until then.
type pendingStylesEntry struct {
	header zip.FileHeader
	data   []byte
}

// wsoPartOrder returns the ordered list of WSO-eligible part paths,
// in the canonical order Okapi's ZipEntryComparator produces (see
// WordDocument.java:74-97 / ZipEntryComparator.java:39-44 — main
// document part comes first; the rest in original ZIP order).
//
// The shared idCounter must increment in this order so that the
// synthesised styleIds (NF974E24F-{parent}{N}) match upstream's
// IdGenerator stream — otherwise headers/footers that come before
// document.xml in raw ZIP order would consume the low sequence numbers
// the upstream filter assigns to document.xml first.
func wsoPartOrder(origZR *zip.Reader, info *containerInfo) []string {
	var out []string
	seen := make(map[string]struct{})
	// Main part first (Okapi reorderedPartPaths places mainPartPath
	// after relsPath, which is not WSO-eligible — so mainPart is the
	// first WSO target after the early non-WSO entries).
	if info.mainDocumentPart != "" && shouldOptimiseWMLPart(info.mainDocumentPart) {
		out = append(out, info.mainDocumentPart)
		seen[info.mainDocumentPart] = struct{}{}
	}
	// Then everything else in ZIP order.
	for _, f := range origZR.File {
		if _, dup := seen[f.Name]; dup {
			continue
		}
		if shouldOptimiseWMLPart(f.Name) {
			out = append(out, f.Name)
			seen[f.Name] = struct{}{}
		}
	}
	return out
}

// shouldOptimiseWMLPart reports whether a WML XML part participates in
// AllowWordStyleOptimisation (paragraphs are walked, common rPr is
// extracted into synthesised paragraph styles). Mirrors the set of
// parts Okapi's openxml filter routes through WordPart processing.
func shouldOptimiseWMLPart(name string) bool {
	if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
		return false
	}
	switch {
	case name == "word/document.xml",
		name == "word/footnotes.xml",
		name == "word/endnotes.xml",
		name == "word/comments.xml":
		return true
	case strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml"),
		strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml"):
		return true
	}
	return false
}

// writeFromReparse copies the original ZIP, substituting locale-variant media (Bowrain AD-007).
func (w *Writer) writeFromReparse(origZR *zip.Reader, zw *zip.Writer, buf *bytes.Buffer,
	blocks map[string]*model.Block) error {

	for _, f := range origZR.File {
		if replacement, ok := w.mediaReplacements[f.Name]; ok {
			fh := f.FileHeader
			fh.Method = zip.Deflate
			fh.CompressedSize64 = 0
			fh.UncompressedSize64 = 0
			fh.CRC32 = 0
			fw, err := zw.CreateHeader(&fh)
			if err != nil {
				return err
			}
			if _, err := fw.Write(replacement); err != nil {
				return err
			}
		} else {
			if err := zw.Copy(f); err != nil {
				return err
			}
		}
	}

	return zw.Close()
}

// renderBlock converts a block's content back to the appropriate XML dialect.
func (w *Writer) renderBlock(block *model.Block, dt docType) string {
	runs := w.preferredRuns(block)
	if runs == nil {
		return ""
	}

	// Core properties and table column names are plain text (no XML wrapping needed).
	if block.Type == "property" || block.Type == "table-column" {
		return xmlEscapeAttr(model.FlattenRuns(runs))
	}

	// Chart and diagram parts inside a DOCX are DrawingML, not WML —
	// they declare the drawingml/2006/main namespace under the `a:`
	// prefix and use <a:r>/<a:t>. Routing them through renderWMLBlock
	// would emit <w:r>/<w:t>, but the `w:` prefix is undeclared in the
	// chart/diagram XML and the round-trip output would mis-bind it
	// (see TranchartAmpersand.docx / Transmart_art.docx golds, which
	// produce <a:r>/<a:t> inside chart and diagram parts). Keying on
	// partPath keeps the wml writer for body/header/footer parts and
	// switches to the dml writer for chart and diagram parts.
	if dt == docTypeDOCX {
		if pp := block.Properties["partPath"]; isChartPartPath(pp) || isDiagramDataPartPath(pp) {
			return w.renderDMLBlock(runs)
		}
	}

	// Per-source-run rPr preservation (#592). The reader stashes the
	// per-paragraph common rPr children under
	// openxmlSourceRPrAnnotationKey when the source had at least one
	// non-toggle rPr child; the writer prepends this XML to every
	// emitted <w:r>'s <w:rPr>. The WSO post-pass (style_optimization.go)
	// then lifts the redundant rPr into a synthesised paragraph style
	// when the optimisation conditions hold.
	sourceRPr := blockSourceRPrXML(block)

	// Per-text-run rPr sidecar (Phase 2 of the per-run rPr work — see
	// PARITY_NOTES.md "1083-*" cluster). When non-empty the writer
	// prefers each text-run's specific rPr over the paragraph-common
	// sourceRPr, mirroring upstream Okapi RunBuilder.java lines 73-188
	// + RunMerger.java lines 156-229 (per ECMA-376-1 §17.3.2): every
	// source run keeps its full rPr verbatim. When all fragments are
	// identical (or after dedupe-on-collapse), output matches the
	// previous common-rPr path. When heterogeneous, per-run divergences
	// (e.g. rStyle on hyperlink display text) are preserved.
	perRunRPr := blockPerRunRPrFragments(block)
	perRunSrcStart := blockPerRunSrcRunStartFlags(block)

	switch dt {
	case docTypeDOCX:
		return w.renderWMLBlock(runs, sourceRPr, perRunRPr, perRunSrcStart)
	case docTypePPTX:
		return w.renderDMLBlock(runs)
	case docTypeXLSX:
		return w.renderSMLBlock(runs, block)
	default:
		return w.renderWMLBlock(runs, sourceRPr, perRunRPr, perRunSrcStart)
	}
}

// blockSourceRPrXML extracts the per-paragraph common rPr children
// XML from the block annotation populated by the WML reader (#592).
// Returns the empty string when no annotation is present (the writer
// falls through to its toggle-only rPr path).
func blockSourceRPrXML(block *model.Block) string {
	if block == nil || block.Annotations == nil {
		return ""
	}
	a, ok := block.Annotations[openxmlSourceRPrAnnotationKey]
	if !ok {
		return ""
	}
	g, ok := a.(*model.GenericAnnotation)
	if !ok || g == nil || g.Fields == nil {
		return ""
	}
	v, ok := g.Fields["xml"].(string)
	if !ok {
		return ""
	}
	return v
}

// blockPerRunRPrFragments extracts the per-text-run rPr children XML
// fragments from the block annotation populated by the WML reader
// (Phase 1 — see source_rpr.go openxmlPerRunRPrAnnotationKey).
//
// The slice has one entry per text-bearing source run BEFORE
// mergeRuns coalescing — adjacent identical fragments correspond to
// runs that mergeRuns combined into a single model TextRun. The
// writer dedupes adjacent identical fragments at emit time so the
// remaining slice aligns 1:1 with the post-merge model TextRun
// stream emitted by renderWMLBlock.
//
// Returns nil when no annotation is present (the writer falls
// through to the paragraph-common sourceRPr path).
func blockPerRunRPrFragments(block *model.Block) []string {
	if block == nil || block.Annotations == nil {
		return nil
	}
	a, ok := block.Annotations[openxmlPerRunRPrAnnotationKey]
	if !ok {
		return nil
	}
	g, ok := a.(*model.GenericAnnotation)
	if !ok || g == nil || g.Fields == nil {
		return nil
	}
	v, ok := g.Fields["fragments"].([]string)
	if !ok {
		return nil
	}
	// The per-run sidecar is returned raw here. The text-aware
	// bCs/iCs strip — which mirrors upstream Okapi RunParser.java
	// :219-229 (strip bCs/iCs/szCs when runFonts has no detected
	// complex-script content categories) — is applied per text
	// run inside renderWMLBlock where the post-pseudo run text is
	// known. ContentCategoriesDetection (upstream
	// ContentCategoriesDetection.java:134-138) classifies runText
	// against the complex-script Unicode block (U+0590..U+074F
	// plus a few extensions; see containsComplexScriptText for
	// the full inventory derived from ECMA-376-1 §17.3.2.16
	// (bCs) and §17.3.2.17 (iCs)).
	return dedupeAdjacent(v)
}

// blockPerRunSrcRunStartFlags extracts the per-text-run "starts new
// source <w:r>" boolean sidecar from the block annotation populated
// by the WML reader. See source_rpr.go
// openxmlPerRunSrcRunStartAnnotationKey for the contract.
//
// Returns nil when the annotation is absent.
func blockPerRunSrcRunStartFlags(block *model.Block) []bool {
	if block == nil || block.Annotations == nil {
		return nil
	}
	a, ok := block.Annotations[openxmlPerRunSrcRunStartAnnotationKey]
	if !ok {
		return nil
	}
	g, ok := a.(*model.GenericAnnotation)
	if !ok || g == nil || g.Fields == nil {
		return nil
	}
	v, ok := g.Fields["flags"].([]bool)
	if !ok {
		return nil
	}
	return v
}

// stripToggleMirrorChildren removes <w:bCs/> and <w:iCs/> elements
// (with or without attributes) from an rPr children-only XML
// fragment. These are complex-script toggle mirrors that upstream
// Okapi strips at parse time when the run text has NO detected
// complex-script content categories
// (okapi/filters/openxml/RunParser.java:219-229 — when
// !runFonts.containsDetectedComplexScriptContentCategories the
// RUN_PROPERTY_COMPLEX_SCRIPT_BOLD/ITALICS/FONT_SIZE elements are
// added to skippableProperties and dropped from the run's rPr).
//
// Callers in the per-run sidecar path must gate this on the
// post-pseudo run text — see containsComplexScriptText. When the
// text contains complex-script characters, bCs/iCs must be preserved
// per ECMA-376-1 §17.3.2.16 (CT_OnOff bCs — complex-script bold)
// and §17.3.2.17 (CT_OnOff iCs — complex-script italics): each is
// the independent toggle for the complex-script side of the run's
// font triple, and stripping them when text is complex-script-
// bearing would drop legitimate formatting (cluster 1200-*).
func stripToggleMirrorChildren(s string) string {
	if s == "" {
		return s
	}
	for _, name := range []string{"bCs", "iCs"} {
		s = stripWMLElement(s, name)
	}
	return s
}

// containsComplexScriptText reports whether s contains any Unicode
// code point that upstream Okapi's
// ContentCategoriesDetection.Default classifies as a complex-script
// content category (ContentCategoriesDetection.java:71-74,
// 134-138). The ranges mirror Microsoft's "Office Open XML Themes,
// Schemes and Fonts" guidance for the complex-script font slot
// referenced by ECMA-376-1 §17.3.2.16 / .17 / .27.
//
// When this returns false for the post-pseudo run text, upstream
// Okapi strips the complex-script run-property toggle mirrors
// (bCs/iCs/szCs) at parse time — so the writer must do the same
// on the per-run sidecar to round-trip byte-equally with the
// reference.
//
// References:
//   - okapi/filters/openxml/ContentCategoriesDetection.java:71-74,
//     134-138 — COMPLEX_SCRIPT_CHARACTERS Pattern + detection rule.
//   - okapi/filters/openxml/RunParser.java:219-229 — skip
//     bCs/iCs/szCs when no detected CS categories.
//   - ECMA-376-1 §17.3.2.16 (bCs), §17.3.2.17 (iCs).
func containsComplexScriptText(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x0590 && r <= 0x074F: // Hebrew, Arabic, Syriac, …
			return true
		case r >= 0x0780 && r <= 0x07BF: // Thaana
			return true
		case r >= 0x0900 && r <= 0x109F: // Devanagari … Myanmar
			return true
		case r >= 0x1780 && r <= 0x18AF: // Khmer … Mongolian
			return true
		case r >= 0x200C && r <= 0x200F: // ZWJ / ZWNJ / LRM / RLM
			return true
		case r >= 0x202A && r <= 0x202F: // bidi formatting + NNBSP
			return true
		case r >= 0x2670 && r <= 0x2671: // misc symbols
			return true
		case r >= 0xFB1D && r <= 0xFB4F: // Hebrew presentation forms
			return true
		}
	}
	return false
}

// adjustRPrForRunText returns the per-run rPr fragment with bCs/iCs
// removed when the run text has no complex-script characters. This
// mirrors upstream Okapi's parse-time strip
// (okapi/filters/openxml/RunParser.java:219-229) which removes the
// complex-script toggle mirrors when
// !runFonts.containsDetectedComplexScriptContentCategories. We
// apply it at write time because the post-pseudo run text is what
// upstream's ContentCategoriesDetection runs against
// (ContentCategoriesDetection.java:111 — performFor receives the
// run's effective text).
//
// Per ECMA-376-1 §17.3.2.16 (bCs) and §17.3.2.17 (iCs) the
// complex-script side of the bold / italic toggle pair applies
// independently to complex-script runs of the run's text. When
// none of the text classifies as complex-script the toggle mirror
// is a no-op by definition and gets stripped.
func adjustRPrForRunText(fragment, text string) string {
	if fragment == "" {
		return fragment
	}
	if containsComplexScriptText(text) {
		return fragment
	}
	return stripToggleMirrorChildren(fragment)
}

// stripWMLElement removes every occurrence of <w:NAME ...?/> (self-
// closing) from s. The per-run rPr fragments use WML "w:" prefixes
// throughout and the b/i toggle mirrors are always self-closing.
//
// Match terminates at the next ">" after a strict element-name
// boundary (whitespace, "/", or ">") to avoid partial-prefix
// matches like <w:bCsExtension/>.
func stripWMLElement(s, name string) string {
	open := "<w:" + name
	for {
		i := strings.Index(s, open)
		if i < 0 {
			return s
		}
		boundary := i + len(open)
		if boundary >= len(s) {
			return s
		}
		next := s[boundary]
		if next != ' ' && next != '\t' && next != '\n' && next != '\r' && next != '/' && next != '>' {
			// Not an exact element-name match (e.g. matched <w:bCsX/>
			// while looking for <w:bCs/>). Skip past this prefix and
			// keep searching.
			s = s[:i+1] + stripWMLElement(s[i+1:], name)
			return s
		}
		end := strings.Index(s[boundary:], ">")
		if end < 0 {
			return s
		}
		end += boundary + 1
		s = s[:i] + s[end:]
	}
}

// dedupeAdjacent returns a copy of `frags` with adjacent equal
// entries collapsed to a single entry. mergeRuns coalesces adjacent
// source runs whose toggle rPr (b/i/u/strike/vertAlign/vanish/font)
// are equal — when those runs ALSO had byte-equal non-toggle rPr,
// the per-run sidecar carries duplicate adjacent fragments that
// must collapse to align with the post-merge model run sequence
// (one model TextRun per coalesced source-run group). Per upstream
// Okapi RunMerger.java lines 156-229 — adjacent runs fuse only when
// RunProperties.equals, so an output rPr per merged group matches
// upstream's emit cadence.
func dedupeAdjacent(frags []string) []string {
	if len(frags) <= 1 {
		return frags
	}
	out := make([]string, 0, len(frags))
	out = append(out, frags[0])
	for i := 1; i < len(frags); i++ {
		if frags[i] == out[len(out)-1] {
			continue
		}
		out = append(out, frags[i])
	}
	return out
}

// runsHaveInlineCodes reports whether the run sequence contains any
// non-text runs (placeholders or paired codes). The fast path for a
// plain-text block short-circuits the walker below.
func runsHaveInlineCodes(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// countTextRuns returns the number of text-bearing model runs (one
// per non-nil r.Text with non-empty Text). Used to gate the per-run
// rPr sidecar alignment guard in renderWMLBlock — when this count
// matches len(perRunRPr) the writer can index slot-by-slot; when
// it doesn't, the sidecar is suppressed.
func countTextRuns(runs []model.Run) int {
	n := 0
	for _, r := range runs {
		if r.Text != nil && r.Text.Text != "" {
			n++
		}
	}
	return n
}

// renderWMLBlock renders a run sequence as WordprocessingML runs.
//
// sourceRPr is the per-paragraph common rPr children XML stashed by
// the reader (#592). When non-empty it is prepended on every emitted
// <w:r>'s <w:rPr>, mirroring upstream Okapi RunBuilder.java which
// keeps the source's full rPr per run, and giving the WSO post-pass
// (style_optimization.go) material to lift into a synthesised
// paragraph style.
//
// perRunRPr is the per-text-run rPr fragments sidecar (Phase 2 of
// the per-run rPr work — see PARITY_NOTES.md "1083-*" cluster).
// When non-empty AND the slot for the current text-run index is
// non-empty, this fragment REPLACES sourceRPr on that <w:r>; runs
// for which the sidecar slot is empty fall back to sourceRPr. This
// preserves heterogeneous-rPr runs (e.g. hyperlink display runs
// carrying <w:rStyle val="Hyperlink"/> alongside surrounding
// non-hyperlink text) per upstream Okapi RunBuilder.java lines
// 73-188 + RunMerger.java lines 156-229 (RunProperties.equals
// gates run fusion, so heterogeneous rPr surfaces multiple <w:r>
// elements rather than collapsing to a single rPr-less <w:r>).
//
// Per ECMA-376-1 §17.3.2.
func (w *Writer) renderWMLBlock(runs []model.Run, sourceRPr string, perRunRPr []string, perRunSrcStart []bool) string {
	// Alignment guard: the per-run sidecar is one fragment per
	// text-bearing source run AFTER dedupe-on-collapse. The writer
	// emits one <w:r> per text-bearing model.Run.Text. When the two
	// counts disagree, the sidecar cannot be aligned 1:1 to model
	// runs (typically because mergeRuns coalesced source runs whose
	// non-toggle rPr differed — see runProps.equal: it ignores
	// non-toggle children, mergeRuns merges across them). In that
	// case fall back to sourceRPr-only mode rather than risk
	// emitting wrong-rPr-on-wrong-run. This preserves the previous
	// common-rPr behaviour for ambiguous paragraphs.
	if len(perRunRPr) != countTextRuns(runs) {
		perRunRPr = nil
	}
	// Mirror the same alignment guard for the srcRunStart sidecar:
	// it must have one entry per post-merge text-bearing run. If the
	// length disagrees, drop it — the writer falls back to the
	// pre-#592 behaviour (every standalone <w:br/>/<w:tab/> closes
	// the <w:r> envelope immediately, never reused by following text).
	if len(perRunSrcStart) != countTextRuns(runs) {
		perRunSrcStart = nil
	}

	// textRunTexts holds the post-pseudo text per text-bearing model
	// run, aligned with the text-run-idx the writer assigns below.
	// It lets effectiveRPr apply upstream Okapi's parse-time
	// bCs/iCs strip (okapi/filters/openxml/RunParser.java:219-229)
	// against the actual run text, rather than blanket-stripping
	// every occurrence — which would drop legitimate complex-script
	// formatting on Arabic/Hebrew/… runs (cluster 1200-*; see
	// PARITY_NOTES.md for the divergence inventory).
	var textRunTexts []string
	for _, r := range runs {
		if r.Text != nil && r.Text.Text != "" {
			textRunTexts = append(textRunTexts, r.Text.Text)
		}
	}

	// effectiveRPr returns the per-run rPr to emit at text-run index
	// idx (0-based, counting only text-bearing model.Run.Text
	// emissions). Falls back to sourceRPr when the sidecar is empty
	// or the slot is absent/empty.
	//
	// bCs/iCs are stripped on-the-fly when the corresponding run
	// text contains no complex-script characters (mirrors upstream
	// Okapi RunParser.java:219-229 + ContentCategoriesDetection.java
	// :134-138; ECMA-376-1 §17.3.2.16 / .17). When the text DOES
	// carry complex-script content, the source's bCs/iCs survive
	// verbatim — fixing the 1200-* RTL synthesis cluster.
	effectiveRPr := func(idx int) string {
		var base string
		if idx < 0 || idx >= len(perRunRPr) || perRunRPr[idx] == "" {
			base = sourceRPr
		} else {
			base = perRunRPr[idx]
		}
		if base == "" {
			return base
		}
		var text string
		if idx >= 0 && idx < len(textRunTexts) {
			text = textRunTexts[idx]
		}
		return adjustRPrForRunText(base, text)
	}

	// textSrcStart returns true iff the text-run at idx began a fresh
	// source <w:r>. Defaults to true when the sidecar is absent so
	// the writer never accidentally fuses heterogeneous source-run
	// origins (false would invite previously-separate runs to merge
	// into a preceding standalone <w:br/>/<w:tab/>'s <w:r>).
	textSrcStart := func(idx int) bool {
		if idx < 0 || idx >= len(perRunSrcStart) {
			return true
		}
		return perRunSrcStart[idx]
	}

	// Fast paths below collapse the entire run sequence into a single
	// <w:r> via model.FlattenRuns. They are only valid when there is
	// at most one text-bearing model run — otherwise per-run rPr
	// boundaries (Phase 4 split on rPrChildren divergence; sidecar
	// slots from Phase 1/2) would be erased. When countTextRuns > 1
	// fall through to the slow path which emits one <w:r> per text
	// run with the correct effectiveRPr(idx). Mirrors upstream Okapi
	// RunBuilder.java lines 73-188 + RunMerger.java lines 156-229:
	// each distinct rPr boundary becomes a distinct <w:r>. Per
	// ECMA-376-1 §17.3.2.
	if countTextRuns(runs) <= 1 {
		// Fast path: no inline codes AND no rPr at all → single
		// <w:r><w:t> with the flattened text. Pre-#592 behaviour for
		// truly plain paragraphs (e.g. "Heading 1" inside a paragraph
		// whose style already supplies all formatting).
		if !runsHaveInlineCodes(runs) && sourceRPr == "" && effectiveRPr(0) == "" {
			return `<w:r><w:t xml:space="preserve">` + xmlEscape(model.FlattenRuns(runs)) + `</w:t></w:r>`
		}

		// Fast path: no inline codes but we DO have rPr → single
		// <w:r><w:rPr>{rPr}</w:rPr><w:t>. Prefer the per-run sidecar
		// slot 0 over sourceRPr. Mirrors Okapi's "RunMerger merges
		// adjacent same-rPr runs into one <w:r> carrying the shared
		// rPr" behaviour for paragraphs that extracted as a single
		// TextRun (after font-mapping + subtractProps + mergeRuns).
		if !runsHaveInlineCodes(runs) {
			return `<w:r><w:rPr>` + effectiveRPr(0) + `</w:rPr><w:t xml:space="preserve">` +
				xmlEscape(model.FlattenRuns(runs)) + `</w:t></w:r>`
		}
	}

	var buf strings.Builder
	var inRun bool
	// inRunNoText flags an open <w:r> that has emitted a standalone
	// <w:br/> / <w:tab/> but no <w:t> yet. The next text Run, if it
	// shares the same rPr, joins this <w:r> by opening <w:t> inside
	// it rather than spawning a new <w:r>. This preserves the source
	// shape `<w:r><w:br/><w:t>...</w:t></w:r>` (run 3 of
	// 1421-line-break.docx) which upstream Okapi RunBuilder keeps as
	// a single <w:r> per ECMA-376-1 §17.3.2.1 (CT_R: rPr followed by
	// run children — <w:br/> and <w:t> may both appear inside one
	// run). When the next Run is NOT a same-rPr text Run (different
	// rPr, another Ph, or PcOpen/PcClose), this <w:r> closes via
	// closeRunNoText so the open envelope doesn't leak.
	var inRunNoText bool
	var runProps string
	textRunIdx := -1 // pre-increment on each new <w:r> for r.Text

	closeRun := func() {
		if inRun {
			buf.WriteString(`</w:t></w:r>`)
			inRun = false
		}
		if inRunNoText {
			buf.WriteString(`</w:r>`)
			inRunNoText = false
		}
	}

	// emitRPr concatenates the per-text-run rPr (slot at the given
	// idx, falling back to sourceRPr) with the toggle rPr the model
	// accumulated from PcOpen/PcClose runs. The combined block is
	// wrapped in a single <w:rPr>...</w:rPr> per emitted <w:r>.
	emitRPr := func(idx int) {
		base := effectiveRPr(idx)
		if base == "" && runProps == "" {
			return
		}
		buf.WriteString(`<w:rPr>`)
		buf.WriteString(base)
		buf.WriteString(runProps)
		buf.WriteString(`</w:rPr>`)
	}

	// emitNonTextRPr is used by Ph runs (br/tab/footnoteRef) that
	// emit their own <w:r> wrapper. They reuse the paragraph-common
	// sourceRPr (NOT a per-text-run slot) because they are not
	// text-bearing and don't consume a sidecar slot — the sidecar
	// is aligned to text runs only (perRunRPrFragments skips lone
	// "\n" line breaks and sentinel runs by construction; see
	// source_rpr.go).
	emitNonTextRPr := func() {
		if sourceRPr == "" && runProps == "" {
			return
		}
		buf.WriteString(`<w:rPr>`)
		buf.WriteString(sourceRPr)
		buf.WriteString(runProps)
		buf.WriteString(`</w:rPr>`)
	}

	for _, r := range runs {
		switch {
		case r.Text != nil:
			// When the next text run's effectiveRPr differs from the
			// currently-open <w:r>'s rPr, close the current run and open
			// a new one so per-run rPr boundaries (Phase 1-5 sidecar) are
			// preserved on the wire. Mirrors RunBuilder.java:73-188 +
			// RunMerger.canRunPropertiesBeMerged (RunMerger.java:156-229)
			// per ECMA-376-1 §17.3.2.1: each distinct rPr boundary is a
			// distinct <w:r>.
			if inRun {
				nextRPr := effectiveRPr(textRunIdx + 1)
				curRPr := effectiveRPr(textRunIdx)
				if nextRPr != curRPr {
					closeRun()
				}
			}
			// If a prior standalone <w:br/> / <w:tab/> left an <w:r>
			// open without a <w:t>, decide whether this text joins it.
			// Two conditions must hold:
			//   (a) the text was NOT marked as starting a new source
			//       <w:r> (perRunSrcStart sidecar from the reader); and
			//   (b) the text's effectiveRPr matches the rPr the Ph
			//       emitted via emitNonTextRPr (sourceRPr + runProps).
			// Both true → reuse the open <w:r> by opening <w:t> in
			// it, preserving `<w:r><w:br/><w:t>…</w:t></w:r>` (run 3
			// of 1421-line-break.docx). Otherwise close the no-text
			// <w:r> first so the text emits in a fresh <w:r> carrying
			// its own rPr. Per upstream Okapi RunBuilder (lines
			// 73-188) and ECMA-376-1 §17.3.2.1 (CT_R), the <w:r>
			// envelope is preserved per source run.
			if inRunNoText {
				nextRPr := effectiveRPr(textRunIdx + 1)
				if !textSrcStart(textRunIdx+1) && nextRPr == sourceRPr {
					textRunIdx++
					buf.WriteString(`<w:t xml:space="preserve">`)
					inRun = true
					inRunNoText = false
				} else {
					buf.WriteString(`</w:r>`)
					inRunNoText = false
				}
			}
			for _, ch := range r.Text.Text {
				if !inRun {
					textRunIdx++
					buf.WriteString(`<w:r>`)
					emitRPr(textRunIdx)
					buf.WriteString(`<w:t xml:space="preserve">`)
					inRun = true
				}
				xmlEscapeRune(&buf, ch)
			}

		case r.PcOpen != nil:
			if r.PcOpen.Type == TypeHyperlink || r.PcOpen.Type == TypeSmartTag {
				// Opaque paired-code open: emit captured raw XML
				// (the <w:hyperlink ...> or <w:smartTag ...> start
				// element) verbatim, paired with the matching close
				// data emitted by the corresponding PcClose. Per
				// upstream Okapi RunContainer (RunContainer.java
				// lines 29-43, 187-191) hyperlink and smartTag are
				// transparent run-containers preserved as a single
				// pair of codes around their inner runs.
				closeRun()
				buf.WriteString(r.PcOpen.Data)
			} else {
				// A toggle change closes the current <w:r> so the
				// next text emits with the updated rPr — mirrors
				// upstream Okapi RunMerger.canRunPropertiesBeMerged
				// (RunMerger.java lines 156-229), which prevents
				// merging across rPr boundaries.
				closeRun()
				runProps = w.addWMLProp(runProps, r.PcOpen.Type)
			}

		case r.PcClose != nil:
			if r.PcClose.Type == TypeHyperlink || r.PcClose.Type == TypeSmartTag {
				closeRun()
				buf.WriteString(r.PcClose.Data)
			} else {
				closeRun()
				runProps = w.removeWMLProp(runProps, r.PcClose.Type)
			}

		case r.Ph != nil:
			// Inline <w:tab/> / <w:br/> into the open <w:r> when its rPr
			// matches what a free-standing tab/break would use (sourceRPr +
			// runProps). Mirrors upstream Okapi RunBuilder.java:73-188
			// which keeps tab/break as Markup chunks inside the surrounding
			// run rather than spawning a new <w:r>. Per ECMA-376-1
			// §17.3.3.31 (<w:tab/>) and §17.3.3.1 (<w:br/>), both are run
			// children that share the enclosing <w:r>'s rPr context.
			//
			// The SubTypeBreakStandalone / SubTypeTabStandalone subtypes
			// tag Ph chunks that began a fresh source <w:r> (the reader
			// sets textRun.srcRunStart on the first emission of each
			// <w:r> and buildBlock propagates it through the SubType).
			// Those MUST close the current run before emitting so the
			// source-run envelope round-trips intact — RunMerger does
			// not collapse break-bearing runs across <w:r> boundaries
			// (RunMerger.java:156-229). 1421-line-break.docx is the
			// canonical fixture.
			canInline := (r.Ph.Type == TypeTab && r.Ph.SubType != SubTypeTabStandalone) ||
				(r.Ph.Type == TypeBreak && r.Ph.SubType != SubTypeBreakStandalone)
			if canInline && inRun && effectiveRPr(textRunIdx) == sourceRPr {
				if r.Ph.Type == TypeTab {
					buf.WriteString(`</w:t><w:tab/><w:t xml:space="preserve">`)
				} else {
					buf.WriteString(`</w:t><w:br/><w:t xml:space="preserve">`)
				}
				continue
			}
			closeRun()
			switch r.Ph.Type {
			case TypeBreak:
				// A <w:br/> inside a run inherits the surrounding
				// rPr in upstream Okapi (RunBuilder treats <w:br/>
				// as a Markup chunk inside the same <w:r>). For
				// the native renderer we wrap it in its own <w:r>
				// for symmetry with the existing pipeline; the
				// surrounding text runs carry their own rPr.
				//
				// When the Ph is SubTypeBreakStandalone (began a
				// fresh source <w:r>), leave the <w:r> OPEN
				// (inRunNoText=true) so a following text run that
				// originated in the same source <w:r> can join it
				// by opening <w:t> inside this <w:r>. Otherwise
				// close immediately. Mirrors upstream Okapi
				// RunBuilder (RunBuilder.java:73-188) which keeps
				// each source <w:r>'s br + text together in one
				// envelope. 1421-line-break.docx is the canonical
				// fixture (run 3: `<w:r><w:br/><w:t>…</w:t></w:r>`).
				if r.Ph.SubType == SubTypeBreakStandalone {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(`<w:br/>`)
					inRunNoText = true
				} else if sourceRPr != "" || runProps != "" {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(`<w:br/></w:r>`)
				} else {
					buf.WriteString(`<w:r><w:br/></w:r>`)
				}
			case TypeTab:
				if r.Ph.SubType == SubTypeTabStandalone {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(`<w:tab/>`)
					inRunNoText = true
				} else if sourceRPr != "" || runProps != "" {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(`<w:tab/></w:r>`)
				} else {
					buf.WriteString(`<w:r><w:tab/></w:r>`)
				}
			case TypeImage:
				// Drawings/pict/object are opaque — never wrap with
				// our synthesised rPr because the captured payload
				// is the original <w:r>'s body verbatim (see the
				// reader's textRun{text:"", data:raw}).
				//
				// extractDrawingTranslations may have replaced
				// translatable sites (drawing-name attributes,
				// vml-textpath strings, txbx-content paragraph
				// bodies) with <!--KAPI-PROP:tu123--> /
				// <!--KAPI-PARA:tu123--> markers; expand those
				// against the per-Write blocks index here so the
				// captured payload picks up translated content.
				expanded := w.expandDrawingMarkers(r.Ph.Data)
				buf.WriteString(`<w:r>` + expanded + `</w:r>`)
			case TypeFootnoteRef:
				// When the Ph data starts with <w:rPr> the reader
				// embedded the run-specific rPr (e.g.
				// <w:rStyle w:val="FootnoteReference"/>) alongside the
				// marker so the writer keeps the marker inside the same
				// <w:r> as that rPr — mirrors upstream Okapi RunBuilder
				// which never splits the marker from its rPr. Per
				// ECMA-376 Part 1 §17.3.2.1 (CT_R) <w:rPr> precedes the
				// run's other children, so the embedded fragment is
				// already in document order.
				if strings.HasPrefix(r.Ph.Data, `<w:rPr>`) {
					buf.WriteString(`<w:r>` + r.Ph.Data + `</w:r>`)
				} else if sourceRPr != "" || runProps != "" {
					buf.WriteString(`<w:r>`)
					emitNonTextRPr()
					buf.WriteString(r.Ph.Data + `</w:r>`)
				} else {
					buf.WriteString(`<w:r>` + r.Ph.Data + `</w:r>`)
				}
			case TypeField:
				// Complex field markup (fldChar / instrText) and
				// fldSimple — the captured payload already carries its
				// own <w:r>...</w:r> or <w:fldSimple>...</w:fldSimple>
				// wrapper plus rPr, so we emit verbatim with no extra
				// wrapping or per-paragraph rPr injection. Per upstream
				// Okapi (RunParser.parseComplexField, lines 461-542 of
				// okapi/filters/openxml/src/main/java/net/sf/okapi/
				// filters/openxml/RunParser.java; BlockParser.parse for
				// fldSimple, lines 242-250) the run that hosts a field
				// marker is preserved as a single opaque markup chunk.
				buf.WriteString(r.Ph.Data)
			default:
				buf.WriteString(r.Ph.Data)
			}
		}
	}

	closeRun()
	return buf.String()
}

// addWMLProp adds a formatting property element to the accumulated rPr content.
func (w *Writer) addWMLProp(current, spanType string) string {
	switch spanType {
	case TypeBold:
		return current + "<w:b/>"
	case TypeItalic:
		return current + "<w:i/>"
	case TypeUnderline:
		return current + `<w:u w:val="single"/>`
	case TypeStrikethrough:
		return current + "<w:strike/>"
	case TypeSuperscript:
		return current + `<w:vertAlign w:val="superscript"/>`
	case TypeSubscript:
		return current + `<w:vertAlign w:val="subscript"/>`
	}
	return current
}

// removeWMLProp removes a formatting property from the accumulated rPr content.
func (w *Writer) removeWMLProp(current, spanType string) string {
	switch spanType {
	case TypeBold:
		return strings.ReplaceAll(current, "<w:b/>", "")
	case TypeItalic:
		return strings.ReplaceAll(current, "<w:i/>", "")
	case TypeUnderline:
		return strings.ReplaceAll(current, `<w:u w:val="single"/>`, "")
	case TypeStrikethrough:
		return strings.ReplaceAll(current, "<w:strike/>", "")
	case TypeSuperscript:
		return strings.ReplaceAll(current, `<w:vertAlign w:val="superscript"/>`, "")
	case TypeSubscript:
		return strings.ReplaceAll(current, `<w:vertAlign w:val="subscript"/>`, "")
	}
	return current
}

// expandDrawingMarkers replaces <!--KAPI-PROP:id--> /
// <!--KAPI-PARA:id--> marker comments inside a captured drawing
// payload with rendered translations from the current Write call's
// blocks index. PROP markers (set in place of an attribute value
// at READ time) expand to the property block's xml-attr-escaped
// text. PARA markers (set in place of a textbox-body paragraph's
// runs) expand to the paragraph block's renderWMLBlock output —
// `<w:r><w:t>...</w:t></w:r>` plus any inline-code wrapping.
//
// When a marker has no matching block (defensive: e.g. the reader
// emitted blocks but they were filtered out before reaching the
// writer) the marker is replaced with the empty string. This is
// the same behaviour the skeleton flush has for unresolved refs.
func (w *Writer) expandDrawingMarkers(payload string) string {
	if !strings.Contains(payload, drawingMarkerPropPrefix) && !strings.Contains(payload, drawingMarkerParaPrefix) {
		return payload
	}
	return drawingMarkerRE.ReplaceAllStringFunc(payload, func(match string) string {
		m := drawingMarkerRE.FindStringSubmatch(match)
		if len(m) != 3 {
			return ""
		}
		kind, id := m[1], m[2]
		block, ok := w.blocks[id]
		if !ok || block == nil {
			return ""
		}
		runs := w.preferredRuns(block)
		if runs == nil {
			return ""
		}
		switch kind {
		case "PROP":
			return xmlEscapeAttr(model.FlattenRuns(runs))
		case "PARA":
			return w.renderWMLBlock(runs, blockSourceRPrXML(block), blockPerRunRPrFragments(block), blockPerRunSrcRunStartFlags(block))
		default:
			return ""
		}
	})
}

// renderDMLBlock renders a run sequence as DrawingML runs.
func (w *Writer) renderDMLBlock(runs []model.Run) string {
	if !runsHaveInlineCodes(runs) {
		return `<a:r><a:t>` + xmlEscape(model.FlattenRuns(runs)) + `</a:t></a:r>`
	}

	var buf strings.Builder
	var inRun bool
	var runPropsAttrs []string

	closeRun := func() {
		if inRun {
			buf.WriteString(`</a:t></a:r>`)
			inRun = false
		}
	}

	for _, r := range runs {
		switch {
		case r.Text != nil:
			for _, ch := range r.Text.Text {
				if !inRun {
					buf.WriteString(`<a:r>`)
					if len(runPropsAttrs) > 0 {
						buf.WriteString(`<a:rPr `)
						buf.WriteString(strings.Join(runPropsAttrs, " "))
						buf.WriteString(`/>`)
					}
					buf.WriteString(`<a:t>`)
					inRun = true
				}
				xmlEscapeRune(&buf, ch)
			}

		case r.PcOpen != nil:
			runPropsAttrs = w.addDMLProp(runPropsAttrs, r.PcOpen.Type)

		case r.PcClose != nil:
			closeRun()
			runPropsAttrs = w.removeDMLProp(runPropsAttrs, r.PcClose.Type)

		case r.Ph != nil:
			closeRun()
			if r.Ph.Type == TypeBreak {
				buf.WriteString(`<a:br/>`)
			} else {
				buf.WriteString(r.Ph.Data)
			}
		}
	}

	closeRun()
	return buf.String()
}

func (w *Writer) addDMLProp(attrs []string, spanType string) []string {
	switch spanType {
	case TypeBold:
		return append(attrs, `b="1"`)
	case TypeItalic:
		return append(attrs, `i="1"`)
	case TypeUnderline:
		return append(attrs, `u="sng"`)
	case TypeStrikethrough:
		return append(attrs, `strike="sngStrike"`)
	case TypeSuperscript:
		return append(attrs, `baseline="30000"`)
	case TypeSubscript:
		return append(attrs, `baseline="-25000"`)
	}
	return attrs
}

func (w *Writer) removeDMLProp(attrs []string, spanType string) []string {
	var target string
	switch spanType {
	case TypeBold:
		target = `b="1"`
	case TypeItalic:
		target = `i="1"`
	case TypeUnderline:
		target = `u="sng"`
	case TypeStrikethrough:
		target = `strike="sngStrike"`
	case TypeSuperscript:
		target = `baseline="30000"`
	case TypeSubscript:
		target = `baseline="-25000"`
	default:
		return attrs
	}
	var result []string
	for _, a := range attrs {
		if a != target {
			result = append(result, a)
		}
	}
	return result
}

// renderSMLBlock renders a run sequence as SpreadsheetML content.
func (w *Writer) renderSMLBlock(runs []model.Run, block *model.Block) string {
	if block.Type == "shared-string" {
		return w.renderSMLSharedString(runs)
	}

	// Cell content — wrap in <v> element as inline string type. Flatten
	// to plain text: inline codes in cell values are rare and the legacy
	// path stripped markers via Fragment.Text().
	return `<v>` + xmlEscape(model.FlattenRuns(runs)) + `</v>`
}

// renderSMLSharedString renders a run sequence as shared string <si> content.
func (w *Writer) renderSMLSharedString(runs []model.Run) string {
	if !runsHaveInlineCodes(runs) {
		return `<t>` + xmlEscape(model.FlattenRuns(runs)) + `</t>`
	}

	// Rich text shared string — emit <r> elements
	var buf strings.Builder
	var inRun bool
	var currentProps []string

	closeRun := func() {
		if inRun {
			buf.WriteString(`</t></r>`)
			inRun = false
		}
	}

	for _, r := range runs {
		switch {
		case r.Text != nil:
			for _, ch := range r.Text.Text {
				if !inRun {
					buf.WriteString(`<r>`)
					if len(currentProps) > 0 {
						buf.WriteString(`<rPr>`)
						for _, p := range currentProps {
							buf.WriteString(p)
						}
						buf.WriteString(`</rPr>`)
					}
					buf.WriteString(`<t>`)
					inRun = true
				}
				xmlEscapeRune(&buf, ch)
			}

		case r.PcOpen != nil:
			closeRun()
			currentProps = w.addSMLProp(currentProps, r.PcOpen.Type)

		case r.PcClose != nil:
			closeRun()
			currentProps = w.removeSMLProp(currentProps, r.PcClose.Type)

		case r.Ph != nil:
			// Placeholders are skipped in shared strings (legacy behaviour).
		}
	}

	closeRun()
	return buf.String()
}

func (w *Writer) addSMLProp(props []string, spanType string) []string {
	switch spanType {
	case TypeBold:
		return append(props, `<b/>`)
	case TypeItalic:
		return append(props, `<i/>`)
	case TypeUnderline:
		return append(props, `<u/>`)
	case TypeStrikethrough:
		return append(props, `<strike/>`)
	case TypeSuperscript:
		return append(props, `<vertAlign val="superscript"/>`)
	case TypeSubscript:
		return append(props, `<vertAlign val="subscript"/>`)
	}
	return props
}

func (w *Writer) removeSMLProp(props []string, spanType string) []string {
	var target string
	switch spanType {
	case TypeBold:
		target = `<b/>`
	case TypeItalic:
		target = `<i/>`
	case TypeUnderline:
		target = `<u/>`
	case TypeStrikethrough:
		target = `<strike/>`
	case TypeSuperscript:
		target = `<vertAlign val="superscript"/>`
	case TypeSubscript:
		target = `<vertAlign val="subscript"/>`
	default:
		return props
	}
	var result []string
	for _, p := range props {
		if p != target {
			result = append(result, p)
		}
	}
	return result
}

// preferredRuns returns the target runs for the writer's locale when
// present, falling back to the source runs. Returns nil if neither is
// available, matching the earlier getFragment contract.
func (w *Writer) preferredRuns(block *model.Block) []model.Run {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs := block.Targets[w.Locale]
		if len(segs) > 0 && len(segs[0].Runs) > 0 {
			return segs[0].Runs
		}
	}
	if len(block.Source) > 0 && len(block.Source[0].Runs) > 0 {
		return block.Source[0].Runs
	}
	return nil
}
