package idml

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Reader implements DataFormatReader for Adobe InDesign IDML files.
//
// IDML is a ZIP package containing XML story files (Stories/Story_*.xml),
// spread files, master spread files, and various resources. The reader
// extracts translatable text from story XML files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new IDML reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "idml",
			FormatDisplayName: "Adobe InDesign Markup Language",
			FormatMimeType:    "application/vnd.adobe.indesign-idml-package",
			FormatExtensions:  []string{".idml"},
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
		MIMETypes:  []string{"application/vnd.adobe.indesign-idml-package"},
		Extensions: []string{".idml"},
		MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}}, // PK ZIP header
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("idml: nil document or reader")
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

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Read all content into memory (ZIP requires random access)
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("idml: reading: %w", err)}
		return
	}

	// Open as ZIP
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("idml: not a valid ZIP archive: %w", err)}
		return
	}

	// Validate the archive against the shared safeio budget before reading any
	// part; per-entry reads are additionally bounded in readZipFile.
	if err := safeio.DefaultZipLimits.CheckReader(zr); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("idml: %w", err)}
		return
	}

	// Find story files. A valid IDML package may legitimately contain no
	// Stories/ entries — e.g. a layout-only template (master spreads, styles,
	// preferences) whose only "story" is the empty XML/BackingStory, or a
	// document with no placed copy. Okapi's IDML filter tolerates this (it
	// folds the BackingStory into its part-name list and simply yields no
	// translatable units — DesignMapFragments.java:165,335-336); the native
	// reader must likewise emit an empty (root-layer-only) document rather than
	// hard-erroring, so the writer can still copy the package through verbatim.
	storyFiles := r.findStoryFiles(zr)

	// Pre-scan visibility (designmap layers + spread/master TextFrames)
	// to learn which stories should be skipped during extraction.
	// Mirrors okapi's PasteboardItem.VisibilityFilter#filterVisible
	// (PasteboardItem.java:192-211): a pasteboard item is dropped when
	// its parent layer is invisible (extractHiddenLayers=false) OR when
	// the textual spread item itself carries Visible="false"
	// (extractHiddenPasteboardItems=false). Stories survive when ANY
	// referencing TextFrame is visible (linked-frame chains).
	hiddenStories, err := r.scanHiddenStories(zr)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("idml: pre-scan: %w", err)}
		return
	}

	// Pre-scan intrinsic geometry (A2): map each story to its anchoring
	// TextFrame's page-space box, so parseStory can attach it to every block.
	storyGeom, err := r.scanStoryGeometry(zr)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("idml: geometry pre-scan: %w", err)}
		return
	}

	// Emit root layer
	rootLayer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "idml",
		Locale:   locale,
		Encoding: "UTF-8",
		MimeType: "application/vnd.adobe.indesign-idml-package",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: rootLayer}) {
		return
	}

	blockCounter := 0

	// Process each story file
	for _, sf := range storyFiles {
		zf := zipFileByName(zr, sf)
		if zf == nil {
			continue
		}

		// Skip stories whose parent TextFrame (or layer) is hidden
		// per the visibility pre-scan. The story file stays in the
		// archive — only its translatable Content is suppressed; the
		// writer copies the original Story_*.xml bytes through to the
		// output zip. This matches okapi's behavior of preserving the
		// document structure while excluding hidden frames from the
		// translation surface.
		if storyID := storyIDFromPath(sf); storyID != "" {
			if hiddenStories[storyID] {
				continue
			}
		}

		storyData, err := readZipFile(zf)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("idml: reading %s: %w", sf, err)}
			return
		}

		// Emit child layer for this story
		childLayer := &model.Layer{
			ID:       "layer-" + sf,
			Name:     sf,
			Locale:   locale,
			ParentID: rootLayer.ID,
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
			return
		}

		// Emit skeleton part-boundary marker
		r.skelPartStart(sf)

		// Parse story XML and extract blocks
		if err := r.parseStory(ctx, ch, storyData, sf, &blockCounter, storyGeom); err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("idml: parsing %s: %w", sf, err)}
			return
		}

		r.skelPartEnd(sf)

		// End child layer
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer}) {
			return
		}
	}

	// End root layer
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
}

// findStoryFiles returns sorted list of story XML file paths from the ZIP.
func (r *Reader) findStoryFiles(zr *zip.Reader) []string {
	var stories []string
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "Stories/") && strings.HasSuffix(f.Name, ".xml") {
			stories = append(stories, f.Name)
		}
	}
	slices.Sort(stories)
	return stories
}

// parseStory parses a single story XML file and emits blocks for translatable content.
//
// IDML story XML structure:
//
//	<Story>
//	  <ParagraphStyleRange AppliedParagraphStyle="...">
//	    <Content>direct child content</Content>     <!-- valid: bare Content -->
//	    <CharacterStyleRange AppliedCharacterStyle="...">
//	      <Content>translatable text</Content>
//	      <Content>another sibling Content</Content> <!-- multiple per CSR allowed -->
//	    </CharacterStyleRange>
//	    <Br/> <!-- line break -->
//	    <CharacterStyleRange>
//	      <Content>more text</Content>
//	      <Footnote>
//	        <ParagraphStyleRange>
//	          <CharacterStyleRange>
//	            <Content>footnote text</Content>
//	          </CharacterStyleRange>
//	        </ParagraphStyleRange>
//	      </Footnote>
//	    </CharacterStyleRange>
//	  </ParagraphStyleRange>
//	</Story>
//
// The walker emits one Block per <Content> element, in document order,
// regardless of whether the element is a direct child of a
// ParagraphStyleRange or nested inside one or more CharacterStyleRange
// elements. Real-world IDML stories (e.g. Adobe InDesign output) routinely
// mix bare-PSR <Content> children with CSR-wrapped <Content> siblings
// inside the same ParagraphStyleRange — see the upstream
// 06-hello-world-14.idml test fixture for an example.
func (r *Reader) parseStory(ctx context.Context, ch chan<- model.PartResult,
	data []byte, storyPath string, blockCounter *int, storyGeom map[string]frameBox) error {

	// Intrinsic geometry (A2): the anchoring frame box for this story, if any,
	// attached to every block the story emits.
	geom := storyGeom[storyIDFromPath(storyPath)]

	d := xml.NewDecoder(bytes.NewReader(data))

	var skelBuf bytes.Buffer
	var contentDepth int    // >0 when inside a <Content> element
	var noteDepth int       // >0 when inside a Note/Footnote/Endnote element
	var stickyNoteDepth int // >0 when inside a <Note> (InDesign editor sticky note, not Footnote/Endnote)
	// pendingStickyNotes holds the bodies of unextracted InDesign sticky
	// <Note> elements seen since the last translatable Block. They ride
	// along on the next translatable Block as parity-safe NoteAnnotations
	// so the comment text is visible to ingestion without becoming MT
	// payload (and without changing the emitted part stream — annotations
	// are not part of the parity canonical stream). The text also stays in
	// the skeleton, keeping round-trip byte-exact. Scoped per story, so a
	// trailing note never leaks onto the next story's first block; a note
	// with no following block in its story stays skeleton-only (as before).
	var pendingStickyNotes []string
	var textBuf strings.Builder
	// rootOpened tracks whether the root element start tag has been
	// emitted. Whitespace between the `<?xml ?>` declaration and the
	// root element (CharData tokens fired by the decoder for the
	// inter-prologue newlines/tabs) is suppressed because okapi strips
	// it on round-trip and our Story_*.xml diffs were dominated by
	// that single difference.
	var rootOpened bool

	// Style tracking uses a stack so nested ParagraphStyleRange/CharacterStyleRange
	// (e.g. inside footnotes, tables) are handled correctly.
	type styleState struct {
		paragraphStyle string
		charStyle      string
	}
	var styleStack []styleState
	currentStyle := styleState{}

	// Empty-element self-close tracking: when a start tag is followed
	// immediately by its matching end tag (no text, no nested element,
	// no skel ref), okapi emits `<X/>` rather than `<X></X>`. We
	// achieve byte-equal parity by buffering the trailing `>` offset
	// of the most recent start tag; on a matching EndElement we
	// truncate that `>` to `/>`. Any other skel write commits the
	// pending start tag (clears it).
	var pending struct {
		name   xml.Name
		offset int
		active bool
	}
	commitPending := func() { pending.active = false }
	emitStart := func(t xml.StartElement) {
		commitPending()
		if r.skeletonStore == nil {
			return
		}
		r.skelWriteStartElement(&skelBuf, t)
		pending.name = t.Name
		pending.offset = skelBuf.Len() - 1
		pending.active = true
	}
	emitEnd := func(t xml.EndElement) {
		if pending.active && pending.name == t.Name {
			skelBuf.Truncate(pending.offset)
			skelBuf.WriteString("/>")
			pending.active = false
			return
		}
		r.skelWriteEndElement(&skelBuf, t)
	}

	// Per-PSR bare-CSR wrapping. Okapi's IDML round-trip wraps any
	// run of bare <Content>/<Br>/<TextFrame>/<HyperlinkTextSource>
	// children of a <ParagraphStyleRange> in a synthetic
	// <CharacterStyleRange AppliedCharacterStyle="…/[No character
	// style]"> element. Real CSR siblings are preserved as-is. We
	// track per-PSR-frame whether a synthetic CSR is currently open
	// so the run can be opened/closed exactly once around each
	// consecutive bare-child group. inCSR > 0 disables wrapping
	// while we're already inside a real CharacterStyleRange.
	// elemDepth tracks XML nesting depth so the bare-child detector
	// only triggers on direct children of the active PSR (not e.g.
	// <Properties> nested inside a wrapped <TextFrame>).
	type psrFrame struct {
		bareCSROpen  bool
		depth        int        // elemDepth at which this PSR was opened
		attrs        []xml.Attr // start-tag attributes (for cross-PSR merge equality check)
		bareCSRAttrs []xml.Attr // attrs used for the synth CSR wrapping bare children (nil → default)
	}
	// Cross-PSR merge tracking. Mirrors upstream
	// StoryChildElementsWriter (writeAsStyledTextElement, lines 70-89)
	// which only opens a new <ParagraphStyleRange> when the next
	// styled-text element's paragraphStyleRange differs from the
	// current one. Adjacent PSRs with identical attributes get
	// collapsed into one PSR with the children of all merged PSRs
	// concatenated. The canonical normalizer's merge-csrs +
	// mergeAdjacentContentsInCSR passes then fuse the now-adjacent
	// CSRs and Contents.
	//
	// Implementation: at PSR end we record the closed PSR's
	// attributes per parent scope ("scopeKey" = the parent
	// element's elemDepth). At PSR start we check whether the
	// previous closed PSR at the SAME parent scope had identical
	// attributes AND no intervening non-whitespace token has been
	// emitted since. If both hold, we truncate the previous
	// `</ParagraphStyleRange>` end tag from skelBuf and skip
	// emitting this PSR's `<ParagraphStyleRange>` start tag. The
	// state is keyed by parent scope so adjacent PSRs inside a
	// footnote / cell merge independently from PSRs at the story
	// level.
	type psrCloseRecord struct {
		attrs       []xml.Attr
		closeOffset int // skelBuf offset where `</ParagraphStyleRange>` starts
	}
	// lastClosedPSR maps parent-scope elemDepth → record of the last
	// PSR closed at that scope. The merge attempt at the next PSR
	// start verifies the recorded close is still the very last bytes
	// in skelBuf — any intervening write naturally invalidates the
	// merge (closeOffset+len(closeTag) != skelBuf.Len() check below).
	// The map is keyed by elemDepth so adjacent PSRs inside a
	// footnote / cell merge independently from PSRs at the story
	// level.
	lastClosedPSR := map[int]*psrCloseRecord{}
	// Per-HTS bare-CSR wrapping. Mirrors okapi's
	// HyperlinkTextSourceStyledTextReferenceElementParser
	// (StoryChildElementsParser.java:464-569) — every direct child
	// of <HyperlinkTextSource> is routed through
	// parseFromCharacterStyleRange (line 535-543), and bare
	// Content/Br children get assembled under the HTS's effective
	// character-style ranges. We replicate this by wrapping bare
	// children in a synthetic <CharacterStyleRange> whose
	// AppliedCharacterStyle is taken from the HTS itself (or
	// "[No character style]" when the HTS carries the special
	// `n` value, mirroring upstream's STYLE_NONE_VALUE fallback at
	// StoryChildElementsParser.java:491-505).
	//
	// Additionally, a <ParagraphStyleRange> directly inside an HTS
	// is unwrapped — only its CSR children survive — because
	// upstream's parseFromParagraphStyleRange (StoryChildElementsParser.java:105-130)
	// flattens PSR children into a list of CSRs without re-emitting
	// the wrapping PSR tag. We achieve this by suppressing emission
	// of the PSR start/end tags when they appear as a direct HTS
	// child, while keeping the inner CSR/Content tokens flowing.
	type htsFrame struct {
		bareCSROpen      bool
		depth            int        // elemDepth at which this HTS was opened
		appliedCharStyle string     // AppliedCharacterStyle attribute of the HTS
		savedInCSR       int        // inCSR depth before HTS opened (restored on close)
		parentCSRAttrs   []xml.Attr // attrs of the immediately enclosing CSR (if any)
		// suppressBareWrap is set by the HTS-start pre-scan when
		// the entire HTS body would canonicalise to a single uniform
		// style range (HTS effective char-style equals parent CSR's
		// char-style AND no real CSR/PSR/etc. siblings appear). In
		// that case upstream's
		// HyperlinkTextSourceStyledTextReferenceElementParser
		// (StoryChildElementsParser.java:514-520) selects the
		// "empty-style" writer that emits children verbatim with no
		// CharacterStyleRange wrapper. We mirror that by skipping
		// the synth-CSR wrap entirely.
		suppressBareWrap bool
		// suppressedPSRDepth records the elemDepth where we elided a
		// nested ParagraphStyleRange start tag. The matching end tag
		// is also suppressed when elemDepth-1 == this value.
		// Multiple suppressions are tracked via a stack so adjacent
		// PSR children inside the same HTS each get unwrapped.
		suppressedPSRStack []int
	}
	var psrStack []psrFrame
	var htsStack []htsFrame
	// Synthetic PSR wrapping for bare styled-text children of
	// <Story> / <EndnoteRange> / <Footnote> / <Note> elements.
	// Mirrors upstream StoryParser (StoryParser.java:73-78) and
	// StyledTextReferenceElementParser (StoryChildElementsParser.java:404-462)
	// which both route every direct child through
	// StoryChildElementsParser.parseWith — bare Content/Br/CSR
	// siblings are then assigned the default (paragraph + character)
	// StyleRanges. The writer (StoryChildElementsWriter.writeAsStyledTextElement,
	// lines 70-89) emits a <ParagraphStyleRange AppliedParagraphStyle=
	// "...NormalParagraphStyle"> wrapper around any run of styled-text
	// children whose paragraphStyleRange is the default. Adjacent
	// real PSRs whose AppliedParagraphStyle equals the default merge
	// into the same wrapper via the cross-PSR merge logic above.
	//
	// We push a frame on this stack at every Story / EndnoteRange /
	// Footnote / Note start. The frame tracks the child scope
	// (elemDepth at which direct children sit) and whether a synth
	// PSR wrapper is currently open at that scope. Bare Content/Br/
	// CSR/HTS children of the scope-owning element trigger
	// openSynthPSR; the next real PSR / nested wrapper closes it.
	type synthScopeFrame struct {
		childScope     int        // elemDepth at which the scope owner's children sit
		synthPSROpen   bool       // synth <ParagraphStyleRange> currently open at this scope
		psrAttrs       []xml.Attr // attrs of the open synth PSR (recorded for cross-PSR merge)
		savedInCSR     int        // inCSR depth before this scope opened (restored on close)
		inheritPSRAttr []xml.Attr // PSR attrs to use for synth wrapping (parent context); nil → default NormalParagraphStyle
		inheritCSRAttr []xml.Attr // CSR attrs to use for the inner synth CSR; nil → default
	}
	var synthScopeStack []synthScopeFrame
	// csrAttrStack tracks the open CSR start-element attributes for
	// each currently-open real <CharacterStyleRange>. The top of the
	// stack is the immediate parent CSR; HTS uses this to seed its
	// synthetic CSR attributes (so e.g. Underline="false" carried by
	// the parent CSR survives onto the wrapper around bare HTS
	// children, mirroring upstream's mergedWith semantics in
	// StoryChildElementsParser.java:486-505 +
	// StyleRange.java:134-181).
	var csrAttrStack [][]xml.Attr
	// csrHadContent[i] reports whether csrAttrStack[i]'s CSR has
	// observed any styled-text child (Content/Br/HTS/Footnote/etc.)
	// since opening. Used to suppress runningCSRAttrs updates from
	// CONTENT-EMPTY CSR siblings — an empty CSR contributes no text
	// to the output and its attrs should not poison the running
	// effective character style for the next bare-HTS wrap. Mirrors
	// upstream's StoryChildElementsParser, where empty CSR siblings
	// either get folded into the next styled child or never reach
	// the lastStyledTextElementIn(...) accumulator that updates
	// currentStyleRanges (StoryChildElementsParser.java:288-292).
	var csrHadContent []bool
	// cellDepthStack tracks the elemDepth of every currently-open
	// <Cell> element. Mirrors upstream's
	// StoryChildElement.StyledTextReferenceElement.Table.Cell
	// (StoryChildElement.java:367-382), whose CellBuilder.build()
	// always constructs a Cell with `new Properties.Empty(eventFactory)`
	// — i.e. it deliberately discards any <Properties> direct child of
	// <Cell> (e.g. <AllCellGradientAttrList>) on read, so the
	// StoryChildElementsWriter never emits them on round-trip.
	// We mirror that here by skipping any <Properties> subtree whose
	// parent elemDepth equals the topmost open cell's elemDepth.
	// Properties nested deeper (inside the Cell's PSR/CSR/HyperlinkText
	// Source/...) are NOT direct Cell children and must survive.
	var cellDepthStack []int
	// runningEffectiveCSRAttrs mirrors upstream's
	// StoryChildElementsParser.this.currentStyleRanges.characterStyleRange()
	// (StoryChildElementsParser.java:57). It is the LAST styled-text
	// element's effective character-style attributes, carried across
	// PSR boundaries (StoryParser.java:79 propagates the inner
	// parser's currentStyleRanges to the next sibling parser). Used
	// when an HTS opens with AppliedCharacterStyle="n" (STYLE_NONE):
	// upstream merges referenceElementStyleRanges with this running
	// scope (StoryChildElementsParser.java:504), so the synthetic CSR
	// wrapping bare HTS children must carry the merged
	// AppliedCharacterStyle (= running's, since merge picks the
	// argument's name on collision per StyleRange.java:139-147).
	// Updated at every real CSR close that observed content. nil →
	// no prior styled-text element seen, falls back to default
	// "[No character style]" (the StoryParser initial value at
	// StoryParser.java:53-54).
	var runningEffectiveCSRAttrs []xml.Attr
	inCSR := 0
	elemDepth := 0
	openSynthCSRWith := func(appliedCharStyle string) {
		commitPending()
		if appliedCharStyle == "" {
			appliedCharStyle = "CharacterStyle/$ID/[No character style]"
		}
		if r.skeletonStore != nil {
			skelBuf.WriteString(`<CharacterStyleRange AppliedCharacterStyle="`)
			skelBuf.WriteString(xmlEscapeAttr(appliedCharStyle))
			skelBuf.WriteString(`">`)
		}
		// Update running effective char style to reflect this synth
		// wrap. Mirrors upstream's parseContent / parseBreak setting
		// currentStyleRanges = styleRanges (StoryChildElementsParser.java:370,
		// 378) — bare children inside this synth wrap will see this as
		// the running. The wrap effectively represents a styled-text
		// scope whose chars = appliedCharStyle.
		runningEffectiveCSRAttrs = []xml.Attr{{
			Name:  xml.Name{Local: "AppliedCharacterStyle"},
			Value: appliedCharStyle,
		}}
	}
	openSynthCSR := func() {
		openSynthCSRWith("CharacterStyle/$ID/[No character style]")
	}
	// markEnclosingCSRSeenContent flags the innermost open real CSR
	// as content-bearing. Called when a Content / Br / HTS / Footnote /
	// Endnote / Note / Table / Change starts inside a real CSR; used
	// at CSR-end to decide whether to update the enclosing PSR's
	// runningCSRAttrs.
	markEnclosingCSRSeenContent := func() {
		if n := len(csrHadContent); n > 0 {
			csrHadContent[n-1] = true
		}
	}
	// updateRunningFromBareContent is called for each bare
	// Content/Br element processed DIRECTLY inside a real CSR (not
	// PSR-direct, not HTS-direct — those go through synth wraps that
	// update running themselves). Mirrors upstream's parseContent /
	// parseBreak (StoryChildElementsParser.java:370, 378) which set
	// this.currentStyleRanges = styleRanges, where styleRanges =
	// (paragraphStyleRange, characterStyleRange) of the enclosing CSR
	// (parseFromCharacterStyleRange line 256, 261). The resulting
	// running.characterStyleRange = enclosing CSR's chars, which then
	// propagates to the next sibling's HTS-with-"n" merge
	// (childElementsBaseStyleRanges argument).
	//
	// MUST be called AFTER openBareIfDirect so the HTS/PSR-direct
	// synth wrap (if any) has set running first; this function then
	// no-ops because content sitting in a synth wrap is not direct
	// inside the outer real CSR.
	updateRunningFromBareContent := func() {
		if len(csrAttrStack) == 0 {
			return
		}
		// Inside an HTS-direct synth wrap (HTS body, no inner real
		// CSR): the synth wrap has already updated running. We're
		// not direct inside the outer real CSR.
		if len(htsStack) > 0 && htsStack[len(htsStack)-1].bareCSROpen {
			return
		}
		// Inside a PSR-direct synth wrap: the synth wrap has already
		// updated running.
		if len(psrStack) > 0 && psrStack[len(psrStack)-1].bareCSROpen {
			return
		}
		runningEffectiveCSRAttrs = csrAttrStack[len(csrAttrStack)-1]
	}
	// openSynthCSRWithAttrs emits a synthetic CSR with the supplied
	// attribute set, with AppliedCharacterStyle overridden by the
	// supplied value (mirrors upstream's mergedAttributesWith semantics
	// at StyleRange.java:149-181 — the synthetic argument's
	// AppliedCharacterStyle wins on collision). All other attrs from
	// `attrs` (e.g. Underline, KerningMethod) survive intact. Updates
	// runningEffectiveCSRAttrs to the resulting attr set so subsequent
	// bare children inside this wrap see the correct running.
	openSynthCSRWithAttrs := func(attrs []xml.Attr, appliedCharStyle string) {
		commitPending()
		if appliedCharStyle == "" {
			appliedCharStyle = "CharacterStyle/$ID/[No character style]"
		}
		if r.skeletonStore != nil {
			skelBuf.WriteString(`<CharacterStyleRange AppliedCharacterStyle="`)
			skelBuf.WriteString(xmlEscapeAttr(appliedCharStyle))
			skelBuf.WriteString(`"`)
			for _, a := range attrs {
				if a.Name.Local == "AppliedCharacterStyle" {
					continue
				}
				skelBuf.WriteString(" ")
				writeAttrName(&skelBuf, a.Name)
				skelBuf.WriteString(`="`)
				skelBuf.WriteString(xmlEscapeAttr(a.Value))
				skelBuf.WriteString(`"`)
			}
			skelBuf.WriteString(">")
		}
		merged := make([]xml.Attr, 0, len(attrs)+1)
		merged = append(merged, xml.Attr{
			Name:  xml.Name{Local: "AppliedCharacterStyle"},
			Value: appliedCharStyle,
		})
		for _, a := range attrs {
			if a.Name.Local == "AppliedCharacterStyle" {
				continue
			}
			merged = append(merged, a)
		}
		runningEffectiveCSRAttrs = merged
	}
	closeSynthCSR := func() {
		commitPending()
		if r.skeletonStore != nil {
			skelBuf.WriteString(`</CharacterStyleRange>`)
		}
	}
	// htsAppliedCharStyle resolves the AppliedCharacterStyle to use
	// for synthetic CSRs wrapping bare children of the innermost
	// HTS. Mirrors StoryChildElementsParser.java:486-505 — when the
	// HTS carries "n" (STYLE_NONE_VALUE), the merge of
	// referenceElementStyleRanges with childElementsBaseStyleRanges
	// (line 504) picks up the running effective character style's
	// name (per StyleRange.mergedWith returning the argument's name,
	// StyleRange.java:139-147). When the HTS carries an explicit
	// AppliedCharacterStyle, that wins (line 491-502).
	//
	// Subsequent inner styled-text children of the HTS update
	// childElementsCurrentStyleRanges
	// (StoryChildElementsParser.java:541-542), so subsequent synth
	// wraps for bare children that follow an inner real CSR pick up
	// the CURRENT running effective char style — not a snapshot from
	// HTS-open. We mirror that by reading the LIVE
	// runningEffectiveCSRAttrs, which is updated at every CSR close
	// (real CSR closes propagate via the global update path).
	htsAppliedCharStyle := func() string {
		if len(htsStack) == 0 {
			return ""
		}
		// Always use the LIVE running effective character style.
		// At HTS open, running was already set to the merged
		// childElementsCurrentStyleRanges (per HTS-open update
		// below), so the FIRST wrap reads the correct initial value.
		// After inner real CSRs / bare content update running, the
		// NEXT wrap reads the updated value. Mirrors upstream's
		// childElementsCurrentStyleRanges tracking
		// (StoryChildElementsParser.java:541-542) which is the same
		// state for every wrap inside the HTS body.
		if pa := attrVal(runningEffectiveCSRAttrs, "AppliedCharacterStyle"); pa != "" {
			return pa
		}
		// No prior styled-text element seen → default
		// "[No character style]" (StoryParser.java:53-54 initial).
		return "CharacterStyle/$ID/[No character style]"
	}
	// isPSRDirect reports whether the element about to be entered is
	// a direct child of the innermost open PSR (and not inside a real
	// CSR). Must be called BEFORE incrementing elemDepth for the new
	// element.
	isPSRDirect := func() bool {
		if len(psrStack) == 0 || inCSR > 0 {
			return false
		}
		return elemDepth == psrStack[len(psrStack)-1].depth
	}
	// isHTSDirect reports whether the element about to be entered is
	// a direct child of the innermost open HTS (and not inside a real
	// CSR). Mirrors isPSRDirect's semantics: must be called before
	// elemDepth is incremented.
	isHTSDirect := func() bool {
		if len(htsStack) == 0 || inCSR > 0 {
			return false
		}
		return elemDepth == htsStack[len(htsStack)-1].depth
	}
	openBareIfDirect := func() {
		if isHTSDirect() {
			top := &htsStack[len(htsStack)-1]
			if top.suppressBareWrap {
				return
			}
			if !top.bareCSROpen {
				// HTS bare-child synth wrap: emit a CSR carrying the
				// LIVE running effective char style attrs. Mirrors
				// upstream's StoryChildElementsWriter — every Content
				// inside an HTS body carries its own styleRanges (set
				// by parseContent at line 370 from current
				// childElementsCurrentStyleRanges), and the writer
				// emits a CSR per styleRanges.
				//
				// For the FIRST wrap (before any inner real CSR), the
				// HTS-open update has set running to (parent CSR's
				// other attrs + AppliedCharacterStyle from HTS-effective).
				// For SUBSEQUENT wraps after inner CSR closes, running
				// reflects the inner CSR's attrs (typically just
				// AppliedCharacterStyle, no other parent attrs).
				openSynthCSRWithAttrs(runningEffectiveCSRAttrs, htsAppliedCharStyle())
				top.bareCSROpen = true
			}
			return
		}
		if isPSRDirect() && !psrStack[len(psrStack)-1].bareCSROpen {
			top := &psrStack[len(psrStack)-1]
			switch {
			case len(top.bareCSRAttrs) > 0:
				// Synth scope (Story-direct, EndnoteRange, …) carries
				// the inherited CSR attrs from the parent context.
				// Emit a CSR with that attribute set so the bare
				// children inherit the correct effective char-style.
				commitPending()
				if r.skeletonStore != nil {
					skelBuf.WriteString(`<CharacterStyleRange`)
					for _, a := range top.bareCSRAttrs {
						skelBuf.WriteString(` `)
						writeAttrName(&skelBuf, a.Name)
						skelBuf.WriteString(`="`)
						skelBuf.WriteString(xmlEscapeAttr(a.Value))
						skelBuf.WriteString(`"`)
					}
					skelBuf.WriteString(`>`)
				}
				// Mirror running update on the wrap's effective attrs.
				runningEffectiveCSRAttrs = top.bareCSRAttrs
			case attrVal(runningEffectiveCSRAttrs, "AppliedCharacterStyle") != "":
				// GLOBAL running effective char style. Mirrors
				// upstream StoryChildElementsParser.parseAsFromCharacterStyleRange
				// (StoryChildElementsParser.java:132-136) which uses
				// `this.currentStyleRanges.characterStyleRange()` —
				// the LAST styled-text element's CSR attrs — to wrap
				// the next non-CSR sibling. Tracking is global (not
				// per-PSR) because Java's StoryParser.java:79
				// propagates currentStyleRanges across Story child
				// parsers (so PSR boundaries don't reset it).
				openSynthCSRWith(attrVal(runningEffectiveCSRAttrs, "AppliedCharacterStyle"))
			default:
				openSynthCSR()
			}
			top.bareCSROpen = true
		}
	}
	closeBareIfDirect := func() {
		if isHTSDirect() && htsStack[len(htsStack)-1].bareCSROpen {
			closeSynthCSR()
			htsStack[len(htsStack)-1].bareCSROpen = false
			return
		}
		if isPSRDirect() && psrStack[len(psrStack)-1].bareCSROpen {
			closeSynthCSR()
			psrStack[len(psrStack)-1].bareCSROpen = false
		}
	}
	closeBareOnPSREnd := func() {
		if len(psrStack) > 0 && psrStack[len(psrStack)-1].bareCSROpen {
			closeSynthCSR()
			psrStack[len(psrStack)-1].bareCSROpen = false
		}
	}
	closeBareOnHTSEnd := func() {
		if len(htsStack) > 0 && htsStack[len(htsStack)-1].bareCSROpen {
			closeSynthCSR()
			htsStack[len(htsStack)-1].bareCSROpen = false
		}
	}
	// tryMergePSRStart attempts to elide the boundary between a
	// previously-closed PSR (at the current parent scope) and this
	// PSR-about-to-open. Returns true if the merge succeeded — in
	// that case the caller MUST NOT emit the `<ParagraphStyleRange>`
	// start tag (the corresponding `</ParagraphStyleRange>` end tag
	// has been truncated from skelBuf).
	tryMergePSRStart := func(attrs []xml.Attr) bool {
		// PSR directly inside an HTS is handled separately (unwrapped).
		// Only consider merges at scopes outside HTS bodies.
		if isHTSDirect() {
			return false
		}
		// Pending start-tag optimisation must be committed before we
		// can safely inspect / mutate the trailing bytes of skelBuf.
		// (A pending self-close on an empty PSR would have already
		// truncated `>` to `/>`, breaking the close-tag match.)
		commitPending()
		rec, ok := lastClosedPSR[elemDepth]
		if !ok {
			return false
		}
		if !samePSRAttrs(rec.attrs, attrs) {
			return false
		}
		const closeTag = "</ParagraphStyleRange>"
		// Verify the recorded close tag is still the very last bytes
		// in skelBuf — any intervening token committed by another
		// branch would have appended after this offset and the merge
		// is no longer safe.
		if rec.closeOffset+len(closeTag) != skelBuf.Len() {
			return false
		}
		buf := skelBuf.Bytes()
		if string(buf[rec.closeOffset:]) != closeTag {
			return false
		}
		skelBuf.Truncate(rec.closeOffset)
		delete(lastClosedPSR, elemDepth)
		return true
	}
	// recordPSRClose stores the just-emitted `</ParagraphStyleRange>`
	// end tag's position so the next PSR start at the same parent
	// scope can attempt a merge. attrs is the START tag's attribute
	// set captured when the PSR was opened.
	recordPSRClose := func(attrs []xml.Attr) {
		// elemDepth has already been decremented by the end branch;
		// the parent scope is the current elemDepth.
		const closeTag = "</ParagraphStyleRange>"
		// The close tag was appended via emitEnd → skelWriteEndElement,
		// which doesn't go through the self-close pending optimisation
		// for end tags (only start tags accumulate pending state).
		// So skelBuf.Len() - len(closeTag) is the offset of the
		// close tag's `<` byte.
		offset := skelBuf.Len() - len(closeTag)
		if offset < 0 {
			return
		}
		buf := skelBuf.Bytes()
		if string(buf[offset:]) != closeTag {
			// Defensive: someone appended after the end tag emission;
			// don't try to merge.
			return
		}
		lastClosedPSR[elemDepth] = &psrCloseRecord{
			attrs:       attrs,
			closeOffset: offset,
		}
	}
	// activeSynthScope returns the innermost open synth scope frame
	// AND whether the element about to be entered is a direct child
	// of that scope owner (not inside any nested PSR/HTS/CSR). Must
	// be called BEFORE elemDepth is incremented for the new element.
	// Returns nil when no scope is active or the about-to-emit
	// element isn't directly under one.
	activeSynthScope := func() *synthScopeFrame {
		if len(synthScopeStack) == 0 {
			return nil
		}
		scope := &synthScopeStack[len(synthScopeStack)-1]
		// Direct child of the scope owner: depth matches and we
		// aren't currently inside a CSR/HTS opened deeper.
		if elemDepth != scope.childScope {
			return nil
		}
		if inCSR > 0 || len(htsStack) > 0 {
			return nil
		}
		// A real PSR is "deeper than this scope" when its frame.depth
		// > scope.childScope. The synth PSR for this scope (if any)
		// has frame.depth == scope.childScope. Bare children only
		// trigger when no real PSR is between us and the scope.
		if len(psrStack) > 0 {
			topPSR := psrStack[len(psrStack)-1]
			if topPSR.depth > scope.childScope {
				return nil
			}
		}
		return scope
	}
	// synthPSRDefaultAttrs returns the default attribute set used by
	// the synthetic <ParagraphStyleRange>. Mirrors upstream
	// StyleRange.defaultParagraphStyleRange (StyleRange.java) which
	// carries only AppliedParagraphStyle = NormalParagraphStyle.
	synthPSRDefaultAttrs := func() []xml.Attr {
		return []xml.Attr{{
			Name:  xml.Name{Local: "AppliedParagraphStyle"},
			Value: "ParagraphStyle/$ID/NormalParagraphStyle",
		}}
	}
	// openSynthPSR opens a synthetic <ParagraphStyleRange> at the
	// given scope's child level. The synth PSR uses default
	// NormalParagraphStyle. A psrFrame is pushed so the existing
	// PSR-direct bare-CSR wrapping (openBareIfDirect /
	// closeBareIfDirect) handles bare Content/Br/HTS children
	// automatically, and a bare CSR child slots in as a normal
	// PSR-child CSR.
	openSynthPSR := func(scope *synthScopeFrame) {
		if scope.synthPSROpen {
			return
		}
		attrs := scope.inheritPSRAttr
		if attrs == nil {
			attrs = synthPSRDefaultAttrs()
		}
		// Cross-PSR merge: if the previously-closed sibling PSR at
		// this scope had the same attrs, elide the boundary (truncate
		// the previous </ParagraphStyleRange> and don't emit this
		// synth PSR's start tag).
		if !tryMergePSRStart(attrs) {
			commitPending()
			if r.skeletonStore != nil {
				skelBuf.WriteString(`<ParagraphStyleRange`)
				for _, a := range attrs {
					skelBuf.WriteString(` `)
					writeAttrName(&skelBuf, a.Name)
					skelBuf.WriteString(`="`)
					skelBuf.WriteString(xmlEscapeAttr(a.Value))
					skelBuf.WriteString(`"`)
				}
				skelBuf.WriteString(`>`)
			}
		}
		scope.synthPSROpen = true
		scope.psrAttrs = attrs
		// Push a psrFrame so existing PSR-direct logic (bare-CSR
		// wrapping, isPSRDirect, etc.) handles inner children. The
		// bareCSRAttrs carries the inherited character-style attrs
		// (from the enclosing CSR, when this scope is for an
		// EndnoteRange/Footnote/Note inside one).
		psrStack = append(psrStack, psrFrame{
			depth:        scope.childScope,
			attrs:        attrs,
			bareCSRAttrs: scope.inheritCSRAttr,
		})
	}
	closeSynthPSR := func(scope *synthScopeFrame) {
		if !scope.synthPSROpen {
			return
		}
		// First close any open bare-CSR inside the synth PSR.
		if len(psrStack) > 0 && psrStack[len(psrStack)-1].bareCSROpen {
			closeSynthCSR()
			psrStack[len(psrStack)-1].bareCSROpen = false
		}
		commitPending()
		if r.skeletonStore != nil {
			skelBuf.WriteString(`</ParagraphStyleRange>`)
		}
		// Pop the synthetic psrFrame.
		if len(psrStack) > 0 {
			psrStack = psrStack[:len(psrStack)-1]
		}
		// Record this synthetic PSR's close so a real PSR with
		// matching default attrs can absorb it via cross-PSR merge.
		const closeTag = "</ParagraphStyleRange>"
		offset := skelBuf.Len() - len(closeTag)
		if offset >= 0 {
			lastClosedPSR[elemDepth] = &psrCloseRecord{
				attrs:       scope.psrAttrs,
				closeOffset: offset,
			}
		}
		scope.synthPSROpen = false
		scope.psrAttrs = nil
	}
	// openSynthIfBare opens the active scope's synthetic PSR wrapper
	// when a bare styled-text element (Content/Br/CSR/HTS) is about
	// to be emitted directly under the scope owner.
	openSynthIfBare := func() {
		scope := activeSynthScope()
		if scope == nil {
			return
		}
		openSynthPSR(scope)
	}
	// closeActiveSynth closes the innermost scope's synth PSR (if
	// any) without popping the scope itself. Called when a non-bare
	// sibling (real PSR / Footnote / etc.) starts, to terminate the
	// synth wrapper before processing the new element.
	closeActiveSynth := func() {
		if len(synthScopeStack) == 0 {
			return
		}
		top := &synthScopeStack[len(synthScopeStack)-1]
		// Only close if the about-to-emit element is at the scope's
		// child level (not deeper). This guards against accidentally
		// closing the synth wrapper from a non-direct sibling — e.g.
		// elements deeper inside a real PSR shouldn't close a synth
		// PSR at story level. But the close should fire for siblings
		// at the SAME depth as bare children would be.
		if elemDepth != top.childScope {
			return
		}
		closeSynthPSR(top)
	}

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("parsing XML: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			rootOpened = true
			switch t.Name.Local {
			case "ParagraphStyleRange":
				// A PSR directly inside an HTS is unwrapped — only its
				// CSR/Content children survive in the output, mirroring
				// upstream parseFromParagraphStyleRange (StoryChildElementsParser.java:105-130)
				// which flattens PSR children into a list of CSRs.
				// We track the suppressed PSR depth so the matching
				// EndElement is also elided. Style stack still pushes
				// because the PSR's AppliedParagraphStyle still
				// influences inner Block properties.
				if isHTSDirect() {
					closeBareIfDirect()
					styleStack = append(styleStack, currentStyle)
					currentStyle.paragraphStyle = attrVal(t.Attr, "AppliedParagraphStyle")
					elemDepth++
					top := &htsStack[len(htsStack)-1]
					top.suppressedPSRStack = append(top.suppressedPSRStack, elemDepth)
					break
				}
				closeBareIfDirect()
				// Close any open story-direct synth PSR before
				// processing this real PSR. The synth PSR's recorded
				// close attrs may match this real PSR's attrs, in
				// which case tryMergePSRStart will absorb the synth
				// PSR's wrapper into this real PSR.
				closeActiveSynth()
				styleStack = append(styleStack, currentStyle)
				currentStyle.paragraphStyle = attrVal(t.Attr, "AppliedParagraphStyle")
				// Cross-PSR merge: when the previous closed sibling
				// PSR at this scope had identical attributes and no
				// intervening token, elide the boundary by truncating
				// the previous `</ParagraphStyleRange>` and skipping
				// this start tag. Mirrors upstream
				// StoryChildElementsWriter.writeAsStyledTextElement
				// (StoryChildElementsWriter.java:70-89) which only
				// emits a new <ParagraphStyleRange> when the next
				// styled-text element's paragraphStyleRange differs
				// from the current one.
				if !tryMergePSRStart(t.Attr) {
					emitStart(t)
				}
				elemDepth++
				psrStack = append(psrStack, psrFrame{depth: elemDepth, attrs: t.Attr})

			case "CharacterStyleRange":
				closeBareIfDirect()
				openSynthIfBare()
				inCSR++
				styleStack = append(styleStack, currentStyle)
				currentStyle.charStyle = attrVal(t.Attr, "AppliedCharacterStyle")
				// Track CSR start-element attributes so a child HTS
				// can seed its synthetic-CSR wrapper with the parent's
				// attribute set (Underline, KerningMethod, …).
				csrAttrStack = append(csrAttrStack, t.Attr)
				csrHadContent = append(csrHadContent, false)
				emitStart(t)
				elemDepth++

			case "Content":
				// IDML <Content> elements are always leaf text nodes (no
				// nesting), but we use a depth counter rather than a
				// boolean so a malformed input can't silently lose state.
				//
				// Okapi's IDML round-trip rewrites every translatable
				// <Content> start tag as `<Content xml:space="preserve">`.
				//
				// `<Note>` (InDesign editor sticky note, distinct from
				// `<Footnote>`/`<Endnote>` publication notes) is treated
				// as an isolated inline code by upstream when
				// extractNotes=false (the default):
				// StyledTextElementsMapping.java:177-181. Its child
				// Content stays in the source language with the source's
				// original attributes — no `xml:space="preserve"`
				// rewrite. We mirror that by emitting the source start
				// tag verbatim; the close branch keeps the original
				// CharData in skeleton so no Block ever fires.
				markEnclosingCSRSeenContent()
				openSynthIfBare()
				openBareIfDirect()
				updateRunningFromBareContent()
				commitPending()
				if r.skeletonStore != nil {
					if stickyNoteDepth > 0 && !r.cfg.ExtractNotes {
						r.skelWriteStartElement(&skelBuf, t)
					} else {
						skelBuf.WriteString(`<Content xml:space="preserve">`)
					}
				}
				contentDepth++
				textBuf.Reset()
				elemDepth++

			case "Br":
				// Line break — skeleton only
				markEnclosingCSRSeenContent()
				openSynthIfBare()
				openBareIfDirect()
				updateRunningFromBareContent()
				emitStart(t)
				elemDepth++

			case "TextFrame":
				markEnclosingCSRSeenContent()
				openSynthIfBare()
				openBareIfDirect()
				emitStart(t)
				elemDepth++

			case "HyperlinkTextSource":
				markEnclosingCSRSeenContent()
				openSynthIfBare()
				// Snapshot whether THIS HTS is about to be wrapped in
				// a synth CSR (bare-HTS-direct-child-of-PSR case). If
				// so, the just-opened synth CSR's effective char-style
				// comes from the GLOBAL running effective char style
				// (mirrors StoryChildElementsParser.parseAsFromCharacterStyleRange
				// at line 132-136 + StoryParser.java:79's
				// cross-PSR propagation). Propagate it as the HTS's
				// parentCSRAttrs so the HTS's bare-child inner synth
				// wrap inherits the same style.
				var synthWrapAppliedStyle string
				if isPSRDirect() && len(psrStack) > 0 && !psrStack[len(psrStack)-1].bareCSROpen {
					if s := attrVal(runningEffectiveCSRAttrs, "AppliedCharacterStyle"); s != "" {
						synthWrapAppliedStyle = s
					}
				}
				openBareIfDirect()
				// Capture the immediately-enclosing CSR's attrs (if
				// any) so synthetic CSRs wrapping bare HTS children
				// inherit them. When the HTS isn't inside a CSR (e.g.
				// directly under a PSR), parentCSRAttrs is nil and
				// the synthetic CSR uses just its AppliedCharacterStyle.
				var parentCSRAttrs []xml.Attr
				if len(csrAttrStack) > 0 {
					parentCSRAttrs = csrAttrStack[len(csrAttrStack)-1]
				} else if synthWrapAppliedStyle != "" {
					// HTS sits inside a just-emitted bare-PSR-direct
					// synth CSR with the running effective style. Make
					// that style visible to the HTS's bare-child
					// synth wrap as if it were a real parent CSR.
					parentCSRAttrs = []xml.Attr{{
						Name:  xml.Name{Local: "AppliedCharacterStyle"},
						Value: synthWrapAppliedStyle,
					}}
				}
				htsAppliedStyle := attrVal(t.Attr, "AppliedCharacterStyle")
				// Pre-scan remaining HTS body to decide whether
				// synthetic-CSR wrapping is needed. Mirrors
				// StoryChildElementsParser.java:514-520 — when the
				// HTS's effective style equals the parent's AND every
				// child element is bare (Content/Br only, no real
				// CSR/PSR/...), upstream emits the empty-style writer
				// that produces no wrapping tags.
				suppressBareWrap := shouldSuppressHTSBareWrap(data, d.InputOffset(),
					htsAppliedStyle, parentCSRAttrs)
				emitStart(t)
				elemDepth++
				// Save and clear inCSR. HTS introduces a new
				// CSR-wrapping scope: bare children of HTS need to be
				// detected as "direct HTS children" even though the
				// HTS itself is nested inside a parent CSR. Real
				// CSR children of HTS will re-establish inCSR via
				// their own start-element handling.
				htsStack = append(htsStack, htsFrame{
					depth:            elemDepth,
					appliedCharStyle: htsAppliedStyle,
					savedInCSR:       inCSR,
					parentCSRAttrs:   parentCSRAttrs,
					suppressBareWrap: suppressBareWrap,
				})
				inCSR = 0
				// Update runningEffectiveCSRAttrs to the merged
				// childElementsCurrentStyleRanges that upstream
				// initialises at HyperlinkTextSourceStyledTextReferenceElementParser
				// (StoryChildElementsParser.java:490-505). The merge of
				// referenceElementStyleRanges (= parent CSR's StyleRange)
				// with either:
				//   - synthetic{X} when HTS has explicit X (line 491-502): result.name = X
				//   - childElementsBaseStyleRanges (= running) when HTS has "n" (line 503-505): result.name = running's name
				// in both cases, OTHER parent attrs (Underline, KerningMethod, …)
				// survive intact (mergedAttributesWith at StyleRange.java:149-181).
				// The merged set becomes the LIVE running for the FIRST
				// bare-child wrap inside the HTS body.
				wrapName := htsAppliedStyle
				if wrapName == "" || wrapName == "n" {
					// "n" → name from running. If running is empty, fall
					// back to default (StoryParser initial value).
					wrapName = attrVal(runningEffectiveCSRAttrs, "AppliedCharacterStyle")
					if wrapName == "" {
						wrapName = "CharacterStyle/$ID/[No character style]"
					}
				}
				merged := make([]xml.Attr, 0, len(parentCSRAttrs)+1)
				merged = append(merged, xml.Attr{
					Name:  xml.Name{Local: "AppliedCharacterStyle"},
					Value: wrapName,
				})
				for _, a := range parentCSRAttrs {
					if a.Name.Local == "AppliedCharacterStyle" {
						continue
					}
					merged = append(merged, a)
				}
				runningEffectiveCSRAttrs = merged

			case "Note", "Footnote", "Endnote", "EndnoteRange":
				markEnclosingCSRSeenContent()
				closeBareIfDirect()
				closeActiveSynth()
				noteDepth++
				if t.Name.Local == "Note" {
					stickyNoteDepth++
				}
				// Capture the parent PSR/CSR attrs BEFORE emitting —
				// the synth wrapping inside the reference element
				// inherits these so its children carry the same
				// effective styled-text context.
				var parentPSRAttrs, parentCSRAttrs []xml.Attr
				if len(psrStack) > 0 {
					parentPSRAttrs = psrStack[len(psrStack)-1].attrs
				}
				if len(csrAttrStack) > 0 {
					parentCSRAttrs = csrAttrStack[len(csrAttrStack)-1]
				}
				emitStart(t)
				elemDepth++
				// Push a synth scope frame: bare children of these
				// reference elements need synthetic PSR+CSR wrapping
				// just like Story-direct children. Mirrors upstream
				// StyledTextReferenceElementParser
				// (StoryChildElementsParser.java:404-462) which routes
				// every direct child through
				// StoryChildElementsParser.parseWith with the parent
				// styleRanges — bare Content/Br get assigned those
				// inherited StyleRanges. The writer
				// (StoryChildElementsWriter, line 434) emits PSR + CSR
				// wrappers around them.
				//
				// Save and clear inCSR. The reference element's
				// interior is a fresh styled-text scope; the
				// surrounding CSR (if any) doesn't apply to direct
				// children — upstream creates new wrappers via the
				// inherited styleRanges.
				synthScopeStack = append(synthScopeStack, synthScopeFrame{
					childScope:     elemDepth,
					savedInCSR:     inCSR,
					inheritPSRAttr: parentPSRAttrs,
					inheritCSRAttr: parentCSRAttrs,
				})
				inCSR = 0

			case "Story":
				// Both the outer wrapper <idPkg:Story> and the inner
				// content-bearing <Story> have local name "Story", so
				// distinguish by namespace. The outer is in the
				// idPkg namespace; the inner uses the default
				// namespace (or no namespace). Only the inner one
				// gates story-direct synth-PSR wrapping — the outer
				// wrapper just contains the inner Story as its sole
				// child.
				closeBareIfDirect()
				closeActiveSynth()
				emitStart(t)
				elemDepth++
				if !strings.Contains(t.Name.Space, "packaging") {
					synthScopeStack = append(synthScopeStack, synthScopeFrame{childScope: elemDepth})
				}

			case "XMLElement":
				// Mirrors upstream's untagXmlStructures=true default
				// (Parameters.java:195) where parseFromElementRange
				// (StoryChildElementsParser.java:271-294) flattens
				// XMLElement wrappers — only their styled children
				// survive in the resulting StoryChildElement list.
				// We achieve the same by NOT emitting the start/end
				// tags AND not incrementing elemDepth, so the wrapper
				// effectively disappears: its children become direct
				// siblings of XMLElement's parent for all
				// depth-aware logic (PSR-direct, story-direct,
				// cross-PSR merge).
				closeBareIfDirect()
				closeActiveSynth()
				// No emitStart, no elemDepth++.

			case "Change":
				// Mirrors upstream parseFromChangedRange
				// (StoryChildElementsParser.java:342-351). Track-
				// changes wrappers come in three flavours:
				//   - DeletedText: skipRange — drop the entire
				//     subtree (the deleted content does not survive).
				//   - InsertedText / MovedText / others:
				//     acceptChanges → parseFromElementRange — unwrap
				//     the wrapper, leaving its children inline.
				closeBareIfDirect()
				closeActiveSynth()
				if attrVal(t.Attr, "ChangeType") == "DeletedText" {
					// Skip the entire <Change> subtree.
					if err := skipElementSubtree(d, t.Name); err != nil {
						return fmt.Errorf("skipping deleted Change: %w", err)
					}
					break
				}
				// InsertedText / MovedText / etc.: unwrap (no emit,
				// no elemDepth++). The matching </Change> end tag
				// also produces no output (handled in the EndElement
				// branch).

			case "XMLAttribute", "XMLComment", "XMLInstruction":
				// Mirrors upstream's untagXmlStructures=true default
				// (Parameters.java:195) where parseAsFromCharacterStyleRange
				// (StoryChildElementsParser.java:138-146) skips these
				// XML-projection elements entirely (skipRange).
				// We consume the entire subtree from the decoder and
				// emit nothing.
				if err := skipElementSubtree(d, t.Name); err != nil {
					return fmt.Errorf("skipping %s: %w", t.Name.Local, err)
				}

			case "Cell":
				// IDML Tables/Cells: track cell depth so a direct
				// <Properties> child can be elided. Mirrors upstream
				// StoryChildElement.StyledTextReferenceElement.Table.Cell
				// (StoryChildElement.java:367-382): CellBuilder.build()
				// always constructs a Cell with a Properties.Empty,
				// dropping any <Properties> child the parser saw — the
				// writer therefore never emits them. Cell content
				// (PSRs / CSRs / Properties INSIDE those) is unaffected.
				closeBareIfDirect()
				closeActiveSynth()
				emitStart(t)
				elemDepth++
				cellDepthStack = append(cellDepthStack, elemDepth)

			case "Properties":
				// Drop <Properties> when it is a direct child of the
				// most recently opened <Cell> (Adobe IDML §3.7 Tables
				// Cell properties: AllCellGradientAttrList,
				// MetadataPacketPreference, …). Upstream Okapi's
				// CellBuilder ignores parsed Properties, so the
				// reference round-trip never contains them. All other
				// <Properties> contexts (PSR, CSR, HyperlinkTextSource,
				// Table itself, …) are preserved by falling through to
				// the default branch.
				if n := len(cellDepthStack); n > 0 && cellDepthStack[n-1] == elemDepth {
					if err := skipElementSubtree(d, t.Name); err != nil {
						return fmt.Errorf("skipping Cell Properties: %w", err)
					}
					break
				}
				closeBareIfDirect()
				closeActiveSynth()
				emitStart(t)
				elemDepth++

			default:
				closeBareIfDirect()
				closeActiveSynth()
				emitStart(t)
				elemDepth++
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "Content":
				if contentDepth > 0 {
					text := textBuf.String()
					if r.cfg.SkipDiscretionaryHyphens {
						text = strings.ReplaceAll(text, "\u00AD", "")
					}

					trimmed := strings.TrimSpace(text)

					// Footnote/Endnote <Content> text is always extracted
					// as a translatable Block — matching okapi's IDML
					// round-trip, which translates footnote/endnote
					// bodies regardless of the ExtractNotes flag.
					//
					// `<Note>` (InDesign editor sticky note) is
					// different: upstream treats it as an isolated
					// inline code when extractNotes=false (the default,
					// StyledTextElementsMapping.java:177-181), so its
					// child Content stays in the skeleton untranslated.
					inUnextractedStickyNote := stickyNoteDepth > 0 && !r.cfg.ExtractNotes

					if trimmed == "" || inUnextractedStickyNote {
						// Non-translatable: write to skeleton as text
						commitPending()
						r.skelText(&skelBuf, xmlEscape(text))

						// Capture the sticky <Note> body so it rides along
						// on the next translatable Block as a NoteAnnotation
						// (developer/translator context, not MT payload).
						// The text stays in the skeleton above, so round-trip
						// is unchanged; the annotation is not part of the
						// parity canonical stream, so no flag is needed.
						if inUnextractedStickyNote && trimmed != "" {
							pendingStickyNotes = append(pendingStickyNotes, trimmed)
						}
					} else {
						// Translatable content: emit block
						*blockCounter++
						blockID := fmt.Sprintf("tu%d", *blockCounter)

						commitPending()
						r.skelRef(&skelBuf, blockID)

						block := &model.Block{
							ID:           blockID,
							Translatable: true,
							Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
							Targets:      make(map[model.VariantKey]*model.Target),
							Properties: map[string]string{
								"storyPath":      storyPath,
								"paragraphStyle": currentStyle.paragraphStyle,
								"characterStyle": currentStyle.charStyle,
							},
						}
						// Attach any pending sticky-note bodies as
						// parity-safe NoteAnnotations on the adjacent block.
						for _, note := range pendingStickyNotes {
							block.AddNote(&model.NoteAnnotation{
								Text:      note,
								From:      "idml-note",
								Annotates: "general",
							})
						}
						pendingStickyNotes = nil
						applyStoryGeometry(block, geom)
						if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
							return nil
						}
					}

					if r.skeletonStore != nil {
						skelBuf.WriteString(`</Content>`)
					}

					contentDepth--
					textBuf.Reset()
				}
				elemDepth--

			case "ParagraphStyleRange":
				// If this PSR end matches a suppressed-PSR-start
				// (PSR directly inside HTS that we elided), don't
				// emit the end tag either. Just unwind state.
				if len(htsStack) > 0 {
					top := &htsStack[len(htsStack)-1]
					if n := len(top.suppressedPSRStack); n > 0 && top.suppressedPSRStack[n-1] == elemDepth {
						closeBareIfDirect()
						top.suppressedPSRStack = top.suppressedPSRStack[:n-1]
						elemDepth--
						if len(styleStack) > 0 {
							currentStyle = styleStack[len(styleStack)-1]
							styleStack = styleStack[:len(styleStack)-1]
						}
						break
					}
				}
				closeBareOnPSREnd()
				var closingAttrs []xml.Attr
				if len(psrStack) > 0 {
					closingAttrs = psrStack[len(psrStack)-1].attrs
					psrStack = psrStack[:len(psrStack)-1]
				}
				elemDepth--
				emitEnd(t)
				// Record this PSR's close so the next PSR start at the
				// same parent scope can attempt a cross-PSR merge.
				// recordPSRClose silently no-ops when the close was
				// truncated (self-closing tags) or otherwise mutated.
				if closingAttrs != nil {
					recordPSRClose(closingAttrs)
				}
				if len(styleStack) > 0 {
					currentStyle = styleStack[len(styleStack)-1]
					styleStack = styleStack[:len(styleStack)-1]
				}

			case "CharacterStyleRange":
				// Pop CSR tracking state. runningEffectiveCSRAttrs is
				// updated at Content/Br time (updateRunningFromBareContent),
				// at HTS open, and at synth-CSR wrap emission — NOT
				// here. Updating at close would overwrite the inner
				// HTS's last-styled value with the outer CSR's attrs,
				// breaking upstream's parseHyperlinkTextSource
				// (StoryChildElementsParser.java:312) where the
				// parser's currentStyleRanges takes the HTS's
				// childElementsCurrentStyleRanges.
				if len(csrHadContent) > 0 {
					csrHadContent = csrHadContent[:len(csrHadContent)-1]
				}
				if inCSR > 0 {
					inCSR--
				}
				if len(csrAttrStack) > 0 {
					csrAttrStack = csrAttrStack[:len(csrAttrStack)-1]
				}
				elemDepth--
				emitEnd(t)
				if len(styleStack) > 0 {
					currentStyle = styleStack[len(styleStack)-1]
					styleStack = styleStack[:len(styleStack)-1]
				}

			case "HyperlinkTextSource":
				closeBareOnHTSEnd()
				if len(htsStack) > 0 {
					inCSR = htsStack[len(htsStack)-1].savedInCSR
					htsStack = htsStack[:len(htsStack)-1]
				}
				elemDepth--
				emitEnd(t)

			case "Note", "Footnote", "Endnote", "EndnoteRange":
				// Close any still-open synth PSR at this scope before
				// popping the scope frame and emitting </X>.
				if len(synthScopeStack) > 0 {
					top := &synthScopeStack[len(synthScopeStack)-1]
					closeSynthPSR(top)
					inCSR = top.savedInCSR
					synthScopeStack = synthScopeStack[:len(synthScopeStack)-1]
				}
				noteDepth--
				if t.Name.Local == "Note" && stickyNoteDepth > 0 {
					stickyNoteDepth--
				}
				elemDepth--
				emitEnd(t)

			case "Story":
				// Close any still-open synth PSR at this scope before
				// emitting the </Story> close tag — the synth PSR
				// might have absorbed bare children at the very end of
				// Story with no following real PSR / Footnote / etc.
				// to trigger the close. Both outer <idPkg:Story> and
				// inner <Story> end tags share the local name "Story";
				// only the inner one pops the synth scope frame.
				if !strings.Contains(t.Name.Space, "packaging") &&
					len(synthScopeStack) > 0 {
					top := &synthScopeStack[len(synthScopeStack)-1]
					closeSynthPSR(top)
					synthScopeStack = synthScopeStack[:len(synthScopeStack)-1]
				}
				elemDepth--
				emitEnd(t)

			case "XMLElement":
				// Matching close for the XMLElement wrapper start —
				// no emit, no elemDepth--. See start branch comment.
				closeBareIfDirect()
				// Mirror closeActiveSynth so trailing bare children
				// of the unwrapped XMLElement get their synth PSR
				// closed before any sibling outside the unwrapped
				// region picks up.
				closeActiveSynth()

			case "Change":
				// Matching close for an InsertedText / MovedText /
				// etc. <Change> wrapper that we elided in the start
				// branch. (DeletedText changes are consumed
				// wholesale by skipElementSubtree, so their close
				// tag is never seen here.) No emit, no elemDepth--.
				closeBareIfDirect()
				closeActiveSynth()

			case "Cell":
				// Pop the cell-depth stack so a sibling Cell at the
				// same Table scope correctly tracks ITS own direct
				// Properties subsequently. Mirrors upstream
				// StoryChildElement.StyledTextReferenceElement.Table.Cell
				// build behaviour.
				if n := len(cellDepthStack); n > 0 {
					cellDepthStack = cellDepthStack[:n-1]
				}
				elemDepth--
				emitEnd(t)

			default:
				elemDepth--
				emitEnd(t)
			}

		case xml.CharData:
			if contentDepth > 0 {
				textBuf.Write(t)
			} else if rootOpened {
				// Inter-element whitespace inside the Story document is
				// dropped to match okapi's XML round-trip — okapi
				// re-serializes structure without preserving the
				// pretty-printed indentation/newlines, and our skel
				// would otherwise leak `\n\t` runs between every
				// sibling element.
				if !isWhitespaceOnly(t) {
					commitPending()
					r.skelText(&skelBuf, xmlEscape(string(t)))
				}
			}
			// Pre-root char data (whitespace between <?xml ?> and the
			// root element) is dropped — see rootOpened comment above.

		case xml.ProcInst:
			inst := string(t.Inst)
			// Okapi's IDML round-trip rewrites the XML declaration's
			// pseudo-attributes from double to single quotes
			// (`version='1.0' encoding='UTF-8'`). The XML 1.0 spec
			// treats either form as equivalent, but byte-equal parity
			// requires us to follow the same convention for the
			// `<?xml ?>` PI specifically.
			if t.Target == "xml" {
				inst = strings.ReplaceAll(inst, `"`, `'`)
			}
			commitPending()
			r.skelText(&skelBuf, "<?"+t.Target+" "+inst+"?>")
		}
	}

	// Flush remaining skeleton data
	commitPending()
	r.skelFlush(&skelBuf)

	return nil
}

// Skeleton part-boundary markers compatible with the OpenXML pattern.
const (
	skelPartStartPrefix = "@@SKEL_PART_START@@"
	skelPartEndPrefix   = "@@SKEL_PART_END@@"
)

func (r *Reader) skelPartStart(partPath string) {
	if r.skeletonStore != nil {
		_ = r.skeletonStore.WriteRef(skelPartStartPrefix + partPath)
	}
}

func (r *Reader) skelPartEnd(partPath string) {
	if r.skeletonStore != nil {
		_ = r.skeletonStore.WriteRef(skelPartEndPrefix + partPath)
	}
}

func (r *Reader) skelText(buf *bytes.Buffer, s string) {
	if r.skeletonStore != nil {
		buf.WriteString(s)
	}
}

func (r *Reader) skelRef(buf *bytes.Buffer, id string) {
	if r.skeletonStore != nil {
		if buf.Len() > 0 {
			_ = r.skeletonStore.WriteText(buf.Bytes())
			buf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

func (r *Reader) skelFlush(buf *bytes.Buffer) {
	if r.skeletonStore != nil && buf.Len() > 0 {
		_ = r.skeletonStore.WriteText(buf.Bytes())
		buf.Reset()
	}
}

func (r *Reader) skelWriteStartElement(buf *bytes.Buffer, t xml.StartElement) {
	if r.skeletonStore == nil {
		return
	}
	buf.WriteString("<")
	writeElementName(buf, t.Name)
	for _, a := range t.Attr {
		buf.WriteString(" ")
		writeAttrName(buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
}

func (r *Reader) skelWriteEndElement(buf *bytes.Buffer, t xml.EndElement) {
	if r.skeletonStore == nil {
		return
	}
	buf.WriteString("</")
	writeElementName(buf, t.Name)
	buf.WriteString(">")
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

// Helper functions

func readZipFile(f *zip.File) ([]byte, error) {
	// Bounded by the shared safeio zip limits (per-entry uncompressed size +
	// inflate-ratio zip-bomb guard on the actual decompressed stream).
	return safeio.DefaultZipLimits.ReadEntry(f)
}

func zipFileByName(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func attrVal(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// skipElementSubtree consumes tokens from the decoder until the
// matching EndElement for the given start name is found. Tracks
// generic XML nesting depth so unrelated nested children don't
// confuse the boundary detection. Mirrors upstream's skipRange
// (StoryChildElementsParser.java:353-363) used for XMLAttribute /
// XMLComment / XMLInstruction when untagXmlStructures is true.
func skipElementSubtree(d *xml.Decoder, name xml.Name) error {
	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tt := tok.(type) {
		case xml.StartElement:
			depth++
			_ = tt
		case xml.EndElement:
			depth--
			if depth == 0 && tt.Name != name {
				// Mismatched close — XML wasn't balanced as expected.
				return fmt.Errorf("unexpected end %s while skipping %s", tt.Name.Local, name.Local)
			}
		}
	}
	return nil
}

// samePSRAttrs reports whether two ParagraphStyleRange attribute
// sets are equivalent. Two PSR-attribute lists are equivalent when
// they cover the same (name, value) pairs regardless of order.
//
// This is the equality test driving the cross-PSR merge — adjacent
// PSRs whose start tags carry the same attributes get collapsed
// into a single PSR wrapper. Mirrors upstream
// StoryChildElementsWriter.writeAsStyledTextElement
// (StoryChildElementsWriter.java:75) which compares
// `currentStyleRanges.paragraphStyleRange().equals(...)` — StyleRange
// equality is structural over attribute pairs.
func samePSRAttrs(a, b []xml.Attr) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		found := false
		for _, y := range b {
			if x.Name.Space == y.Name.Space &&
				x.Name.Local == y.Name.Local &&
				x.Value == y.Value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// shouldSuppressHTSBareWrap pre-scans the remainder of an HTS body
// (starting at the byte offset right after the HTS start tag) to
// decide whether bare-content wrapping is needed. Returns true when
// the HTS contains only bare Content/Br/Properties children (no
// real CharacterStyleRange / ParagraphStyleRange / Table /
// HyperlinkTextSource / etc.) AND the HTS's effective character-
// style equals the parent CSR's character-style.
//
// This mirrors upstream's
// HyperlinkTextSourceStyledTextReferenceElementParser.parse
// (StoryChildElementsParser.java:514-520) which selects an
// "empty-style" writer (no CSR wrapping) when
// childElementsCurrentStyleRanges.equals(mergedReferenceAndChildElementsStyleRanges)
// AND sameStyleRangesFor(mergedStoryChildElements). When the
// condition fails — different child styles or HTS overrides parent —
// the wrapped writer kicks in and the synthetic CSRs we emit during
// the main pass match upstream's output.
func shouldSuppressHTSBareWrap(data []byte, fromOffset int64,
	htsAppliedStyle string, parentCSRAttrs []xml.Attr) bool {
	// Determine effective HTS char style. Mirrors
	// StoryChildElementsParser.java:486-505 — STYLE_NONE_VALUE ("n")
	// or empty falls back to parent CSR's char style.
	effective := htsAppliedStyle
	if effective == "" || effective == "n" {
		effective = attrVal(parentCSRAttrs, "AppliedCharacterStyle")
	}
	parentStyle := attrVal(parentCSRAttrs, "AppliedCharacterStyle")
	// Suppression requires effective == parent. When they differ,
	// upstream's mergedReferenceAndChildElementsStyleRanges differs
	// from the parent's reference, so the empty-style writer is not
	// selected.
	if effective != parentStyle {
		return false
	}
	if int(fromOffset) >= len(data) {
		return false
	}
	// Wrap the remainder in a synthetic <HyperlinkTextSource> root so
	// Go's xml.Decoder doesn't choke on the unbalanced
	// </HyperlinkTextSource> close tag at the end of the slice.
	var buf bytes.Buffer
	buf.WriteString("<HyperlinkTextSource>")
	buf.Write(data[fromOffset:])
	scan := xml.NewDecoder(&buf)
	// Consume our synthetic root start.
	if _, err := scan.Token(); err != nil {
		return false
	}
	depth := 0 // depth relative to the HTS body
	for {
		tok, err := scan.Token()
		if errors.Is(err, io.EOF) {
			return true
		}
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			// Direct children of HTS sit at depth 0 when encountered.
			// `Content`, `Br`, and `Properties` are bare/leaf-style
			// elements; anything else (CharacterStyleRange,
			// ParagraphStyleRange, Footnote, Endnote, EndnoteRange,
			// Note, Table, HyperlinkTextSource, Change, …) forces the
			// wrapped writer per
			// StyledTextElementsMapping.java:172-201 + the
			// referent-emission paths in
			// HyperlinkTextSourceStyledTextReferenceElementParser.parse
			// loop body (StoryChildElementsParser.java:535-543) which
			// re-enter parseFromCharacterStyleRange and surface
			// non-bare children.
			if depth == 0 {
				switch t.Name.Local {
				case "Content", "Br", "Properties":
					// bare child — fine
				default:
					return false
				}
			}
			depth++
		case xml.EndElement:
			if depth == 0 {
				// Reached HTS close.
				return true
			}
			depth--
		}
	}
}

// xmlEscape escapes the three required XML text-content characters
// (`&`, `<`, `>`). It deliberately does NOT escape whitespace
// characters: Go's xml.EscapeText converts `\n`/`\t`/`\r` into the
// `&#xA;`/`&#x9;`/`&#xD;` numeric entities, which is legal but breaks
// byte-for-byte parity with okapi (whose Story_*.xml output keeps
// literal newlines after the `<?xml ?>` declaration).
func xmlEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isWhitespaceOnly(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return false
		}
	}
	return true
}

func xmlEscapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func writeElementName(buf *bytes.Buffer, name xml.Name) {
	if name.Space != "" {
		prefix := nsPrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
	}
	buf.WriteString(name.Local)
}

func writeAttrName(buf *bytes.Buffer, name xml.Name) {
	// `xmlns:foo="..."` namespace declarations come through Go's XML
	// decoder as Attr{Name{Space:"xmlns", Local:"foo"}}. Without this
	// case, the writer would emit just `foo="..."` and break the XML's
	// namespace bindings — exactly the symptom okapi flagged on idml
	// Story_*.xml round-trip ("idPkg=" instead of "xmlns:idPkg=").
	if name.Space == "xmlns" {
		buf.WriteString("xmlns:")
		buf.WriteString(name.Local)
		return
	}
	if name.Space != "" {
		prefix := nsPrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
	}
	buf.WriteString(name.Local)
}

// storyIDFromPath extracts the Self id of a story file path, e.g.
// "Stories/Story_u29d.xml" → "u29d". Returns "" when the path doesn't
// match the expected layout.
func storyIDFromPath(p string) string {
	const prefix = "Stories/Story_"
	const suffix = ".xml"
	if !strings.HasPrefix(p, prefix) || !strings.HasSuffix(p, suffix) {
		return ""
	}
	return p[len(prefix) : len(p)-len(suffix)]
}

// scanHiddenStories returns the set of story Self ids whose extraction
// must be suppressed because every TextFrame referencing them is hidden
// (TextFrame Visible="false") and/or because their parent layer is
// hidden. Mirrors okapi's PasteboardItem.VisibilityFilter#filterVisible
// (PasteboardItem.java:192-211) plus DesignMapFragments' Layer ingest
// (DesignMapFragments.java:306-313).
//
// Spec: Adobe IDML File Format Specification §"Spread" / "TextFrame"
// (TextFrame inherits SpreadItem; Visible attribute, default true) and
// §"Layer" (designmap.xml's Layer Visible attribute, default true).
//
// A story is "hidden" when:
//   - !ExtractHiddenLayers AND every referencing TextFrame's ItemLayer is
//     a hidden layer (per designmap.xml), AND
//   - !ExtractHiddenPasteboardItems AND every referencing TextFrame
//     carries Visible="false".
//
// More precisely, we follow okapi's per-frame filter: each TextFrame is
// dropped if the layer rule OR the frame rule excludes it; a story is
// extracted iff at least one referencing frame survives both rules.
func (r *Reader) scanHiddenStories(zr *zip.Reader) (map[string]bool, error) {
	// Always-extract shortcut: nothing to do.
	if r.cfg.ExtractHiddenLayers && r.cfg.ExtractHiddenPasteboardItems {
		return nil, nil
	}

	// Layer visibility (designmap.xml). Default for a missing layer is
	// "visible" — okapi treats unknown layer ids as an error
	// (PasteboardItem.java:213-222), but real-world IDMLs occasionally
	// reference ItemLayer ids that aren't declared (e.g. promoted from
	// master spreads). Defaulting to visible avoids spurious skips.
	layerVisible := map[string]bool{}
	if dm := zipFileByName(zr, "designmap.xml"); dm != nil {
		data, err := readZipFile(dm)
		if err != nil {
			return nil, fmt.Errorf("read designmap.xml: %w", err)
		}
		dec := xml.NewDecoder(bytes.NewReader(data))
		for {
			tok, err := dec.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("parse designmap.xml: %w", err)
			}
			se, ok := tok.(xml.StartElement)
			if !ok || se.Name.Local != "Layer" {
				continue
			}
			self := attrVal(se.Attr, "Self")
			if self == "" {
				continue
			}
			// IDML default for a missing Visible attribute is true
			// (Adobe IDML spec §"Layer", Visible XSD default).
			layerVisible[self] = parseBoolDefault(attrVal(se.Attr, "Visible"), true)
		}
	}

	// Track per-story:
	//   storyHasChainStart — at least one referencing frame is the
	//     chain-start (PreviousTextFrame == "n"). Anchored-only stories
	//     (no chain start, only inline-anchor TextFrames) lack this.
	//   storyVisibleChainStart — at least one chain-start frame
	//     survives the visibility filter (visible Layer + visible frame).
	//
	// A story is hidden iff it has chain-start references AND none of
	// the chain-start frames are visible. This mirrors upstream
	// DesignMapFragments.visibleStoryPartNames (lines 192-204 of
	// DesignMapFragments.java) which:
	//   1. computes visiblePasteboardItems via VisibilityFilter,
	//   2. collects visibleStoryIds via OrderingIdioms.getOrderedStoryIds
	//      (OrderingIdioms.java:148-152) — that helper ONLY adds a story
	//      ID when the item's PreviousTextFrameId is NO_VALUE (i.e. the
	//      item is the FIRST frame in its thread). Frames mid-chain
	//      contribute zero story IDs.
	//   3. anchoredStoryPartNames are stories with no chain-start at all
	//      (referenced only from inline anchors); these get included via
	//      the union at line 204.
	//
	// Net behaviour: a story whose chain-start frame is invisible is
	// dropped EVEN IF mid-chain frames are visible. (Layout-wise, the
	// hidden chain-start owns the whole thread; mid-chain visibility
	// alone doesn't surface story content.)
	storyHasChainStart := map[string]bool{}
	storyVisibleChainStart := map[string]bool{}
	storyHasRef := map[string]bool{}

	scanSpread := func(name string) error {
		zf := zipFileByName(zr, name)
		if zf == nil {
			return nil
		}
		data, err := readZipFile(zf)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		dec := xml.NewDecoder(bytes.NewReader(data))
		for {
			tok, err := dec.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("parse %s: %w", name, err)
			}
			se, ok := tok.(xml.StartElement)
			if !ok || se.Name.Local != "TextFrame" {
				continue
			}
			storyID := attrVal(se.Attr, "ParentStory")
			if storyID == "" {
				continue
			}
			storyHasRef[storyID] = true
			frameVisible := parseBoolDefault(attrVal(se.Attr, "Visible"), true)
			itemLayer := attrVal(se.Attr, "ItemLayer")
			// Adobe IDML §"TextFrame": PreviousTextFrame == "n" marks
			// the head of a threaded-frame chain. Empty / missing also
			// counts as no-previous (defensive — IDML always emits "n"
			// in practice).
			prev := attrVal(se.Attr, "PreviousTextFrame")
			isChainStart := prev == "" || prev == "n"
			if isChainStart {
				storyHasChainStart[storyID] = true
			}

			// Per-frame layer/visibility filter (mirrors okapi
			// PasteboardItem.java:199-206).
			if !r.cfg.ExtractHiddenLayers && itemLayer != "" {
				if vis, known := layerVisible[itemLayer]; known && !vis {
					continue
				}
			}
			if !r.cfg.ExtractHiddenPasteboardItems && !frameVisible {
				continue
			}
			// Frame survived visibility. Story is visible only if THIS
			// frame is the chain-start; mid-chain visible frames can't
			// rescue an invisible chain-start.
			if isChainStart {
				storyVisibleChainStart[storyID] = true
			}
		}
		return nil
	}

	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "Spreads/") && strings.HasSuffix(f.Name, ".xml") {
			if err := scanSpread(f.Name); err != nil {
				return nil, err
			}
		} else if strings.HasPrefix(f.Name, "MasterSpreads/") && strings.HasSuffix(f.Name, ".xml") {
			if err := scanSpread(f.Name); err != nil {
				return nil, err
			}
		}
	}

	hidden := map[string]bool{}
	for storyID := range storyHasRef {
		// Anchored-only stories (no chain start) are always extracted
		// per DesignMapFragments line 201-204 (anchoredStoryPartNames
		// gets unioned into visibleStoryPartNames).
		if !storyHasChainStart[storyID] {
			continue
		}
		if !storyVisibleChainStart[storyID] {
			hidden[storyID] = true
		}
	}
	return hidden, nil
}

// parseBoolDefault parses an XML boolean attribute, returning def when
// the value is empty. Recognises the IDML convention of "true"/"false"
// (case-insensitive). Any other value returns def — okapi parses with
// `Boolean.parseBoolean`, which only matches "true" (case-insensitive)
// and treats every other value as false; we mirror that semantic but
// keep the def fallback so a missing attribute returns the spec default.
func parseBoolDefault(s string, def bool) bool {
	if s == "" {
		return def
	}
	switch strings.ToLower(s) {
	case "true":
		return true
	case "false":
		return false
	}
	// Any non-empty non-recognized value — Java Boolean.parseBoolean
	// returns false here.
	return false
}

// nsPrefix returns a namespace prefix for known IDML namespaces.
func nsPrefix(ns string) string {
	switch ns {
	case "http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging":
		return "idPkg"
	case "http://www.w3.org/XML/1998/namespace":
		return "xml"
	default:
		return ""
	}
}
