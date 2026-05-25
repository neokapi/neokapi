package xliff2

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for XLIFF 2.x files.
//
// The writer builds a fresh etree DOM from the captured Parts and the
// xliff2-native inline IR (SourceBodyAnnotation / TargetBodyAnnotation
// on each Block). This is "generation mode" per the design doc; round-
// trip mode (DOM patching for byte-equal output on unmodified inputs)
// is a future enhancement that requires the source DOM be carried
// across the part stream.
//
// We never call etree's Indent() — it has a known bug where it injects
// whitespace into mixed content despite xml:space="preserve" (etree#88).
// Instead we set Element.Text/Element.Tail explicitly during DOM
// construction, controlling indentation per-element. Mixed-content
// elements (<source>, <target>, <pc>, <mrk>, <data>) get empty Text/
// Tail between their children so significant whitespace is never
// modified.
type Writer struct {
	format.BaseFormatWriter
	cfg            *Config
	skeletonStore  *format.SkeletonStore
	sourceLang     model.LocaleID
	targetLang     model.LocaleID
	fileID         string
	inputVersion   string
	inputExtraAttr []xml.Attr
	fileNotes      []FileNote
	layerFileNotes []FileNote

	// items captures groups (start/end) and blocks in stream order so the
	// writer can reconstruct the <file> nesting structure.
	items []writerItem

	// sourceDoc, when non-nil, is the etree document captured by the
	// reader and ferried via Layer.Annotations["xliff2:source-dom"].
	// Its presence enables round-trip patching mode (byte-equal output
	// for unmodified inputs, minimal-diff for modified ones).
	sourceDoc *etree.Document

	// sourceBytes is the original input the reader saw. When the writer
	// determines no segment was modified, it emits sourceBytes verbatim
	// — bypassing etree's serialization quirks (multi-line attribute
	// collapse, optional-character over-escaping, etc.).
	sourceBytes []byte
}

// writerItem represents one positioned element under a <file>: a group
// boundary or a translatable unit (block).
type writerItem struct {
	kind      writerItemKind
	block     *model.Block
	groupID   string
	groupName string
}

type writerItemKind int

const (
	itemBlock writerItemKind = iota
	itemGroupStart
	itemGroupEnd
)

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new XLIFF 2.x writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xliff2",
		},
		cfg: cfg,
	}
}

// Config returns the writer's configuration (mutable).
func (w *Writer) Config() *Config { return w.cfg }

// SetVersion overrides the emitted XLIFF 2.x version.
func (w *Writer) SetVersion(v string) error {
	if v != "" && !IsSupportedVersion(v) {
		return fmt.Errorf("xliff2: unsupported XLIFF 2.x version %q (expected one of %v)", v, SupportedXLIFFVersions)
	}
	w.cfg.Version = v
	return nil
}

// resolveVersion returns the version this writer should emit.
func (w *Writer) resolveVersion() string {
	if w.cfg != nil && w.cfg.Version != "" {
		return w.cfg.Version
	}
	if w.inputVersion != "" && IsSupportedVersion(w.inputVersion) {
		return w.inputVersion
	}
	return DefaultXLIFFVersion
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// SetFileNotes stamps file-level <note> elements onto the first <file>.
func (w *Writer) SetFileNotes(notes []FileNote) {
	w.fileNotes = append(w.fileNotes[:0], notes...)
}

// AddFileNote appends a single file-level note.
func (w *Writer) AddFileNote(note FileNote) {
	w.fileNotes = append(w.fileNotes, note)
}

// Write consumes Parts from a channel and writes XLIFF 2.x output.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				if w.skeletonStore != nil {
					return w.writeFromSkeleton()
				}
				return w.flush()
			}
			switch part.Type {
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					w.items = append(w.items, writerItem{kind: itemBlock, block: block})
				}
			case model.PartGroupStart:
				if gs, ok := part.Resource.(*model.GroupStart); ok {
					w.items = append(w.items, writerItem{kind: itemGroupStart, groupID: gs.ID, groupName: gs.Name})
				}
			case model.PartGroupEnd:
				w.items = append(w.items, writerItem{kind: itemGroupEnd})
			case model.PartLayerStart:
				if layer, ok := part.Resource.(*model.Layer); ok {
					w.sourceLang = layer.Locale
					w.fileID = layer.Name
					if tl, ok := layer.Properties["target-language"]; ok {
						w.targetLang = model.LocaleID(tl)
					}
					if v, ok := layer.Properties["xliff-version"]; ok {
						w.inputVersion = v
					}
					w.inputExtraAttr = extraAttrsFromLayer(layer)
					w.layerFileNotes = fileNotesFromLayer(layer)
					if a, ok := layer.Annotations["xliff2:source-dom"].(*SourceDOMAnnotation); ok && a != nil && a.Doc != nil && w.sourceDoc == nil {
						w.sourceDoc = a.Doc
						w.sourceBytes = a.Original
					}
				}
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
// (Legacy skeleton path; not exercised by the new DOM writer.)
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("xliff2 writer: flush skeleton: %w", err)
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
	}

	var blocks []*model.Block
	for _, it := range w.items {
		if it.kind == itemBlock {
			blocks = append(blocks, it.block)
		}
	}

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("xliff2 writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			idxStr, refSuffix, ok := strings.Cut(refID, ":")
			if !ok {
				continue
			}
			blockIdx, err := strconv.Atoi(idxStr)
			if err != nil || blockIdx < 0 || blockIdx >= len(blocks) {
				continue
			}
			block := blocks[blockIdx]
			elemType := refSuffix

			var text string
			switch elemType {
			case "source":
				text = block.SourceText()
			case "target":
				if block.HasTarget(targetLang) {
					text = block.TargetText(targetLang)
				} else {
					text = block.SourceText()
				}
			}
			if _, err := io.WriteString(w.Output, xmlEscapeText(text)); err != nil {
				return err
			}
		}
	}
	return nil
}

// flush serializes the captured stream to XLIFF 2.x XML. When the
// source DOM is present (round-trip mode), it patches that DOM in
// place so unmodified content survives byte-equal. Otherwise it
// builds a fresh DOM from the captured items (generation mode).
func (w *Writer) flush() error {
	if w.Output == nil {
		return nil
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
	}

	if w.sourceDoc != nil {
		return w.flushRoundTrip(targetLang)
	}

	version := w.resolveVersion()
	mergedNotes := mergeFileNotes(w.layerFileNotes, w.fileNotes)

	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	xliffEl := doc.CreateElement("xliff")
	w.setRootAttrs(xliffEl, version, targetLang)

	fileEl := xliffEl.CreateElement("file")
	fileEl.CreateAttr("id", w.fileID)

	if len(mergedNotes) > 0 {
		notesEl := fileEl.CreateElement("notes")
		for _, n := range mergedNotes {
			noteEl := notesEl.CreateElement("note")
			if n.ID != "" {
				noteEl.CreateAttr("id", n.ID)
			}
			if n.Category != "" {
				noteEl.CreateAttr("category", n.Category)
			}
			noteEl.SetText(n.Content)
		}
	}

	// Walk items, building the file's children.
	type stackFrame struct {
		el    *etree.Element
		depth int
	}
	stack := []stackFrame{{el: fileEl, depth: 1}}
	for _, it := range w.items {
		top := stack[len(stack)-1]
		switch it.kind {
		case itemGroupStart:
			gEl := top.el.CreateElement("group")
			if it.groupID != "" {
				gEl.CreateAttr("id", it.groupID)
			}
			if it.groupName != "" {
				gEl.CreateAttr("name", it.groupName)
			}
			stack = append(stack, stackFrame{el: gEl, depth: top.depth + 1})
		case itemGroupEnd:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case itemBlock:
			w.appendUnit(top.el, it.block, targetLang)
		}
	}

	// Apply XLIFF-aware indentation to the whole tree.
	indentXliff(doc.Root(), 0)

	doc.WriteSettings = etree.WriteSettings{}
	if err := writeDocCREscape(w.Output, doc); err != nil {
		return fmt.Errorf("xliff2 writer: write: %w", err)
	}
	return nil
}

// flushRoundTrip implements byte-equal-on-untouched round-trip:
//   - Walk every <unit>/<segment> in the source DOM. For each, compare
//     the model's content against the DOM's; if equal, leave it alone;
//     if different, patch just that <source>/<target> element's children.
//   - If NO segment was patched (and no explicit file notes were
//     stamped), short-circuit: emit the original input bytes verbatim.
//     This bypasses etree's serialization quirks (multi-line attribute
//     collapse, optional-character over-escaping like `>` → `&gt;` and
//     `"` → `&quot;`) that would otherwise change bytes outside the
//     model's reach.
//   - Otherwise, serialize the (mutated) DOM via etree.
func (w *Writer) flushRoundTrip(targetLang model.LocaleID) error {
	blocksByID := make(map[string]*model.Block, len(w.items))
	for _, it := range w.items {
		if it.kind == itemBlock && it.block != nil {
			blocksByID[it.block.ID] = it.block
		}
	}

	root := w.sourceDoc.Root()
	if root == nil {
		// Defensive: no root means the DOM was wiped. Fall through
		// to generation mode.
		w.sourceDoc = nil
		return w.flush()
	}

	// Honor an explicit version override: patch <xliff version="…"> and
	// the matching default namespace. Without an override we leave the
	// captured version untouched (Config.Version == "" means "auto:
	// preserve input").
	patchedRoot := false
	if w.cfg != nil && w.cfg.Version != "" && IsSupportedVersion(w.cfg.Version) {
		v := w.cfg.Version
		if attrValue(root, "version") != v {
			root.CreateAttr("version", v) // CreateAttr replaces existing
			ns := NamespaceForVersion(v)
			if attrValue(root, "xmlns") != ns {
				root.CreateAttr("xmlns", ns)
			}
			patchedRoot = true
		}
	}

	// Patch root srcLang/trgLang when the writer was handed locales
	// that differ from the source DOM. This matters when the harness or
	// caller deliberately overrides the target locale (e.g. parity test
	// pseudo-translating an existing bilingual fixture into a different
	// language) — otherwise the model has the new locale's segments but
	// the root <xliff> still advertises the old one.
	if !w.sourceLang.IsEmpty() && attrValue(root, "srcLang") != string(w.sourceLang) {
		root.CreateAttr("srcLang", string(w.sourceLang))
		patchedRoot = true
	}
	if !targetLang.IsEmpty() && attrValue(root, "trgLang") != string(targetLang) {
		root.CreateAttr("trgLang", string(targetLang))
		patchedRoot = true
	}

	patched := patchedRoot
	for _, fileEl := range root.SelectElements("file") {
		if walkUnitsRoundTrip(fileEl, blocksByID, targetLang) {
			patched = true
		}
	}

	notesAdded := false
	if len(w.fileNotes) > 0 {
		if firstFile := root.SelectElement("file"); firstFile != nil {
			applyExplicitFileNotes(firstFile, w.fileNotes)
			notesAdded = true
		}
	}

	// Byte-equal short-circuit: when nothing was changed, emit the
	// original bytes the reader saw. This is the v2 contract for
	// untouched round-trip.
	if !patched && !notesAdded && len(w.sourceBytes) > 0 {
		if _, err := w.Output.Write(w.sourceBytes); err != nil {
			return fmt.Errorf("xliff2 writer: passthrough: %w", err)
		}
		return nil
	}

	// Once we're committed to re-serialising the DOM, normalize spec-
	// default attributes the way okapi's XLIFFWriter does. okapi's
	// writer emits inline-code and note attributes only when they
	// differ from the spec defaults (e.g. `priority="1"` on <note>,
	// `canOverlap="no"` on <pc>); the source DOM may carry the explicit
	// defaults verbatim. Stripping here lets fixtures with default-
	// valued attributes reach canonical-equal vs the okapi reference.
	// We only strip defaults on the patch path, never on the byte-equal
	// short-circuit, so the v2 byte-equal-on-untouched contract still
	// holds for unmodified inputs.
	stripOkapiDefaults(root)
	propagateXMLSpaceToFragments(root, "")

	if err := writeDocCREscape(w.Output, w.sourceDoc); err != nil {
		return fmt.Errorf("xliff2 writer: round-trip write: %w", err)
	}
	return nil
}

// writeDocCREscape serializes doc to w, escaping any literal carriage
// return bytes as numeric character references (`&#13;`). etree's
// serializer leaves `\r` raw in both attribute values and CharData;
// XML 1.0 §2.11 line-end normalization would silently rewrite those to
// `\n` on the next read, losing the round-trip distinction between an
// authored newline and an authored CR. okapi's XLIFF Toolkit always
// emits the entity form (e.g. fixtures with `&#x000D;` in inline
// markers like `IBM Globalization&#xD;Pipeline`), so mirroring that
// here closes the parity gap on translated.xlf, translated_with_mrk.xlf,
// and original_en.xlf — and is the right thing to do regardless: any
// downstream tool that parses our output now sees the same character
// sequence it would for okapi's.
//
// We post-process the rendered byte stream rather than walking the DOM
// because etree does not expose an "emit text with custom escaping"
// hook; replacing in raw bytes is safe because etree never emits a
// literal `\r` inside tag/attribute syntax (those are pure ASCII), so
// every `\r` in the buffer originates from user-supplied text.
func writeDocCREscape(w io.Writer, doc *etree.Document) error {
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		return err
	}
	return writeBytesCREscape(w, buf.Bytes())
}

// writeBytesCREscape writes data to w, replacing each 0x0D (CR) with
// the literal entity bytes "&#13;". Single-pass, allocation-free for
// the common no-CR case.
func writeBytesCREscape(w io.Writer, data []byte) error {
	if !bytes.ContainsRune(data, '\r') {
		_, err := w.Write(data)
		return err
	}
	const repl = "&#13;"
	start := 0
	for i, b := range data {
		if b != '\r' {
			continue
		}
		if i > start {
			if _, err := w.Write(data[start:i]); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, repl); err != nil {
			return err
		}
		start = i + 1
	}
	if start < len(data) {
		if _, err := w.Write(data[start:]); err != nil {
			return err
		}
	}
	return nil
}

// walkUnitsRoundTrip recurses into <group>/<unit> children of parent,
// patching each unit against the matching model.Block. Returns true if
// any segment was patched.
func walkUnitsRoundTrip(parent *etree.Element, blocksByID map[string]*model.Block, targetLang model.LocaleID) bool {
	patched := false
	for _, child := range parent.ChildElements() {
		switch child.Tag {
		case "group":
			if walkUnitsRoundTrip(child, blocksByID, targetLang) {
				patched = true
			}
		case "unit":
			id := attrValue(child, "id")
			block, ok := blocksByID[id]
			if !ok {
				continue // unit removed from model (rare)
			}
			if patchUnit(child, block, targetLang) {
				patched = true
				// Once a unit is being re-serialised, normalize its
				// originalData ids to d1, d2, … and synthesize empty
				// targets for ignorables that lack them (mirroring
				// okapi's X2ToOkpConverter pass). We only do these on
				// patched units so the byte-equal-on-untouched contract
				// still holds for unmodified inputs.
				renumberDataIDsInUnit(child)
				synthesizeIgnorableTargets(child)
			}
		}
	}
	return patched
}

// patchUnit reconciles one <unit> element in the source DOM with the
// model.Block, returning true if any segment was patched.
func patchUnit(unitEl *etree.Element, block *model.Block, targetLang model.LocaleID) bool {
	srcSegs := sourceSegsFromBlock(block)
	srcByID := make(map[string]*seg, len(srcSegs))
	srcByPos := make([]*seg, len(srcSegs))
	for i := range srcSegs {
		srcByPos[i] = &srcSegs[i]
		srcByID[srcSegs[i].ID] = &srcSegs[i]
	}
	var trgByID map[string]*seg
	if trgSegs := targetSegsFromBlock(block, targetLang); len(trgSegs) > 0 {
		trgByID = make(map[string]*seg, len(trgSegs))
		for i := range trgSegs {
			trgByID[trgSegs[i].ID] = &trgSegs[i]
		}
	}

	// Capture pre-patch inline ids on every <target> in the unit. Okapi's
	// Tag store remembers tags it parsed even after they're replaced (the
	// store outlives a single Source/Target swap), so when Store.suggestId
	// later picks a fresh ignorable id it skips not just live ids but the
	// pre-replacement ghosts too. Without this, we synthesise ignorable
	// ids one slot lower than okapi whenever a pseudo pass replaces a
	// target whose inline used a higher-numbered id (icu_message: ph
	// id="2" in the source target → ignorable id="3" in okapi vs "2" in
	// native). See lib-xliff2 Store.suggestId/Unit.isIdUsed.
	ghostIDs := map[string]bool{}
	for _, segEl := range unitEl.ChildElements() {
		if segEl.Tag != "segment" && segEl.Tag != "ignorable" {
			continue
		}
		if tgtEl := segEl.SelectElement("target"); tgtEl != nil {
			walkXliff2El(tgtEl, func(el *etree.Element) {
				if el == tgtEl {
					return
				}
				if id := attrValue(el, "id"); id != "" {
					ghostIDs[id] = true
				}
			})
		}
	}

	// xliff2 makes segment ids optional. Match DOM <segment>/<ignorable>
	// elements to model.Segments by DOM id first, then fall back to
	// document-order position. The reader synthesizes collision-free ids
	// for unkeyed elements ("s<n>" or "_xliff2_seg_<n>" when "s<n>" is
	// already explicitly used) and assigns them to model.Segment.ID, so
	// positional fallback recovers the correspondence the DOM lacks.
	patched := false
	domSegIdx := 0
	segIDCounter := 0
	for _, segEl := range unitEl.ChildElements() {
		if segEl.Tag != "segment" && segEl.Tag != "ignorable" {
			continue
		}
		segID := attrValue(segEl, "id")
		modelSrc := srcByID[segID]
		modelTgt := trgByID[segID]
		if modelSrc == nil && segID == "" && domSegIdx < len(srcByPos) {
			modelSrc = srcByPos[domSegIdx]
			if modelSrc != nil && modelTgt == nil {
				modelTgt = trgByID[modelSrc.ID]
			}
		}
		domSegIdx++

		// xliff2 segment ids are optional in source, but okapi
		// XLIFF2Filter materializes them on round-trip ("s1", "s2", …
		// for unkeyed <segment>). Mirror that here so re-emitted
		// segments match okapi byte-for-byte. Ignorable id synthesis is
		// deferred to a second pass below so it only fires when the
		// unit was already going to be patched anyway — otherwise the
		// byte-equal-on-untouched contract breaks for fixtures with
		// unkeyed ignorables that the model didn't modify.
		//
		// The counter increments only on UNKEYED segments — segments
		// with an explicit id (e.g. "1239bca", "abc") don't consume a
		// slot, mirroring okapi's Store.suggestId(false) which only
		// numbers segments that need a synthesized id. Without this,
		// the second unkeyed segment after an explicit-id one would
		// land on "s2" while okapi emits "s1".
		if segEl.Tag == "segment" && segID == "" {
			segIDCounter++
			segEl.CreateAttr("id", fmt.Sprintf("s%d", segIDCounter))
			patched = true
		}

		if srcEl := segEl.SelectElement("source"); srcEl != nil && modelSrc != nil {
			if !segmentMatchesDOM(srcEl, modelSrc) {
				replaceInlineChildren(srcEl, modelSrc)
				patched = true
			}
		}

		tgtEl := segEl.SelectElement("target")
		if modelTgt != nil {
			// Mirror OkpToX2Converter (line 191): emit <target> only when
			// the segment has a non-"initial" state OR the target has
			// non-empty content. An "empty target on a state-less segment"
			// is dropped on round-trip — okapi treats the missing state
			// as the default "initial", and unmodified empty targets carry
			// no information worth re-emitting. Only applies to <segment>;
			// <ignorable> follows separate rules and always keeps its
			// target when one was authored.
			segState := attrValue(segEl, "state")
			suppressTarget := segEl.Tag == "segment" &&
				segmentTargetIsEmpty(modelTgt, tgtEl) &&
				(segState == "" || segState == "initial")
			if suppressTarget {
				if tgtEl != nil {
					segEl.RemoveChild(tgtEl)
					patched = true
				}
				// Skip the rest of target patching — there's no <target>
				// element to maintain.
				continue
			}
			if tgtEl == nil {
				tgtEl = etree.NewElement("target")
				if srcEl := segEl.SelectElement("source"); srcEl != nil {
					srcIdx := childIndex(segEl, srcEl)
					segEl.InsertChildAt(srcIdx+1, tgtEl)
				} else {
					segEl.AddChild(tgtEl)
				}
				replaceInlineChildren(tgtEl, modelTgt)
				patched = true
			} else if !segmentMatchesDOM(tgtEl, modelTgt) {
				replaceInlineChildren(tgtEl, modelTgt)
				patched = true
			}
			// xml:space="preserve" is significant for XLIFF white-
			// space handling; okapi propagates it from <source> to
			// <target> on round-trip when the target lacks one.
			if srcEl := segEl.SelectElement("source"); srcEl != nil {
				if srcSpace := attrValue(srcEl, "space"); srcSpace == "preserve" && attrValue(tgtEl, "space") == "" {
					tgtEl.CreateAttr("xml:space", "preserve")
					patched = true
				}
			}
		}

		// Drop the empty initial-state <target> from <segment> elements
		// when the model target is also empty AND this segment was
		// already going to be patched for some other reason. Okapi's
		// XLIFF2 writer elides empty initial-state targets on round-
		// trip — a missing <target> within a <segment> is the canonical
		// "untranslated" representation (state defaults to "initial").
		// Gating on prior-patched preserves the byte-equal-on-untouched
		// contract: when a segment is otherwise unchanged we still emit
		// its source <target></target> verbatim. Restricted to
		// <segment>: <ignorable> targets carry inline whitespace/spans
		// that may legitimately be empty-but-present.
		if patched && segEl.Tag == "segment" && tgtEl != nil && segmentTargetIsEmpty(modelTgt, tgtEl) {
			segEl.RemoveChild(tgtEl)
		}
	}

	// Drop orphan <data> entries from <originalData> when no inline
	// element references them. Okapi's xliff2 toolkit garbage-collects
	// originalData after subfilter inlining and pseudo-translation: when
	// a <ph dataRef="d2"/> in the source target is replaced (e.g. with
	// the source-side <ph dataRef="d1"/>), the now-unreferenced
	// <data id="d2"> is removed from the unit. Without this we keep the
	// orphan and diverge by one element.
	if patched {
		pruneOrphanData(unitEl)
	}

	// Second pass — synthesise ids on unkeyed <ignorable> elements only
	// when the unit is already going to be re-serialised (patched is
	// true). This mirrors okapi's Store.suggestId(false): increment a
	// counter starting from 1, retrying when the candidate collides
	// with an in-use id elsewhere in the unit (segment/ignorable ids,
	// inline element ids, originalData ids) OR a ghost id remembered
	// from the pre-patch target. Skipping when nothing else changed
	// preserves the v2 byte-equal-on-untouched contract for fixtures
	// whose source has unkeyed ignorables that the model didn't touch.
	if patched {
		usedIDs := collectUsedUnitIDs(unitEl)
		for id := range ghostIDs {
			usedIDs[id] = true
		}
		ignorableIDCounter := 0
		for _, segEl := range unitEl.ChildElements() {
			if segEl.Tag != "ignorable" {
				continue
			}
			if attrValue(segEl, "id") != "" {
				continue
			}
			ignorableIDCounter++
			for usedIDs[strconv.Itoa(ignorableIDCounter)] {
				ignorableIDCounter++
			}
			newID := strconv.Itoa(ignorableIDCounter)
			segEl.CreateAttr("id", newID)
			usedIDs[newID] = true
		}
	}

	return patched
}

// segmentTargetIsEmpty reports whether tgtEl can be safely elided from a
// <segment> because both the model target and the DOM target hold no
// translatable content. The model side is considered empty when modelTgt
// is nil or its Runs contain no text and no inline codes; the DOM side
// is considered empty when the element has no element children and no
// non-whitespace text content. Attributes (e.g. xml:space, state) on the
// element itself are intentionally ignored — they only matter when the
// element survives.
func segmentTargetIsEmpty(modelTgt *seg, tgtEl *etree.Element) bool {
	if modelTgt != nil {
		for _, r := range modelTgt.Runs {
			if r.Text != nil && r.Text.Text != "" {
				return false
			}
			if r.Text == nil {
				// Any non-text run (Ph/PcOpen/PcClose/Sub/Plural/Select)
				// counts as content even when the wrapping text is empty.
				return false
			}
		}
	}
	if tgtEl == nil {
		return true
	}
	if len(tgtEl.ChildElements()) > 0 {
		return false
	}
	if strings.TrimSpace(domElementText(tgtEl)) != "" {
		return false
	}
	return true
}

// pruneOrphanData removes <data> children of any <originalData> in the
// unit whose id is not referenced by any inline element's dataRef /
// dataRefStart / dataRefEnd attribute anywhere in the unit's
// source/target trees. Mirrors okapi's xliff2 toolkit behaviour where
// originalData entries that no longer back an inline code are dropped
// on serialization (see lib-xliff2 Unit.checkOriginalData and the
// XLIFF2Filter post-processing pass after pseudo-translation).
//
// When pruning empties an <originalData> element, the wrapper itself is
// removed too — okapi never emits an empty <originalData/>.
func pruneOrphanData(unitEl *etree.Element) {
	referenced := map[string]bool{}
	for _, child := range unitEl.ChildElements() {
		if child.Tag == "originalData" {
			continue
		}
		walkXliff2El(child, func(el *etree.Element) {
			for _, attr := range []string{"dataRef", "dataRefStart", "dataRefEnd"} {
				if v := attrValue(el, attr); v != "" {
					referenced[v] = true
				}
			}
		})
	}
	for _, odEl := range unitEl.SelectElements("originalData") {
		var keep []*etree.Element
		for _, dataEl := range odEl.SelectElements("data") {
			id := attrValue(dataEl, "id")
			if id == "" || referenced[id] {
				keep = append(keep, dataEl)
			} else {
				odEl.RemoveChild(dataEl)
			}
		}
		if len(keep) == 0 {
			unitEl.RemoveChild(odEl)
		}
	}
}

// collectUsedUnitIDs returns the set of every id string already in use
// **inside** the unit element (segment/ignorable ids plus inline
// element ids — pc/sc/ec/ph/mrk/sm/em — anywhere within source/target,
// plus data ids in originalData). The unit element's own id is a
// different scope (units are unique within file, parts are unique
// within unit) and is NOT included. Used by patchUnit when synthesising
// ids for unkeyed <ignorable> elements so the new id avoids collisions,
// mirroring okapi's Store.suggestId(false) retry-on-collision
// behaviour.
func collectUsedUnitIDs(unitEl *etree.Element) map[string]bool {
	used := map[string]bool{}
	for _, child := range unitEl.ChildElements() {
		walkXliff2El(child, func(el *etree.Element) {
			if id := attrValue(el, "id"); id != "" {
				used[id] = true
			}
		})
	}
	return used
}

// propagateXMLSpaceToFragments stamps `xml:space="preserve"` on every
// <source> / <target> whose inherited xml:space is "preserve" but where
// the attribute isn't already present on the element itself. This
// mirrors okapi's writeFragment behaviour: when the parent unit's
// preserveWS is true (inherited from <file xml:space="preserve">) the
// writer always emits the attribute, even though the attribute would
// inherit through XML 1.0's xml:space rules anyway. Without the explicit
// stamp, encoding/xml's canonical normalizer (which doesn't propagate
// xml:space) sees the attribute on okapi's output but not on ours.
//
// Walks top-down so nested overrides win: a <segment xml:space="default">
// child of a <file xml:space="preserve"> uses "default" for its
// source/target.
func propagateXMLSpaceToFragments(el *etree.Element, inheritedSpace string) {
	if el == nil {
		return
	}
	curSpace := inheritedSpace
	if v := attrValue(el, "space"); v != "" {
		curSpace = v
	}
	switch el.Tag {
	case "source", "target":
		if curSpace == "preserve" && attrValue(el, "space") == "" {
			el.CreateAttr("xml:space", "preserve")
		}
		// Don't recurse into source/target — inline-content elements
		// (<pc>, <mrk>, <sc>, <ec>, …) have their own xml:space rules
		// that we don't want to disturb here.
		return
	}
	for _, child := range el.ChildElements() {
		propagateXMLSpaceToFragments(child, curSpace)
	}
}

// synthesizeIgnorableTargets adds an empty <target> sibling to every
// <ignorable> element under unitEl that lacks one, when at least one
// of the unit's <segment> siblings already has a <target>. This mirrors
// okapi's X2ToOkpConverter behaviour at line 200 ("apply the source
// ignorable content to target unless there exists target ignorable
// content, but only if we had a target segment"): okapi materializes a
// target on every ignorable so source/target part counts match,
// preventing downstream tools from getting offset segments. The XLIFF
// 2 round-trip then re-emits the empty target.
//
// The synthesized target inherits xml:space from the source (the
// propagateXMLSpaceToFragments pass picks up the new attribute later).
func synthesizeIgnorableTargets(unitEl *etree.Element) {
	hasAnyTarget := false
	for _, child := range unitEl.ChildElements() {
		if child.Tag != "segment" {
			continue
		}
		if child.SelectElement("target") != nil {
			hasAnyTarget = true
			break
		}
	}
	if !hasAnyTarget {
		return
	}
	for _, child := range unitEl.ChildElements() {
		if child.Tag != "ignorable" {
			continue
		}
		if child.SelectElement("target") != nil {
			continue
		}
		srcEl := child.SelectElement("source")
		if srcEl == nil {
			continue
		}
		tgtEl := etree.NewElement("target")
		// Place the new target right after <source>.
		srcIdx := childIndex(child, srcEl)
		child.InsertChildAt(srcIdx+1, tgtEl)
	}
}

// renumberDataIDsInUnit rewrites a unit's <originalData>/<data> ids to
// the sequential `d1, d2, …` form okapi's XLIFFWriter emits, and patches
// every inline `dataRef` / `dataRefStart` / `dataRefEnd` reference to
// match. This closes the parity gap on fixtures like code_id_mismatch.xlf
// where the source uses sparse ids (`d1, d7`) and okapi's output collapses
// them (`d1, d2`).
//
// We renumber based on the document order of <data> entries inside
// <originalData>. Okapi's actual algorithm is content-keyed (see
// Store.calculateDataToIdsMap), but the document-order pass produces the
// same result for typical fixtures because <data> entries are emitted in
// first-use order. If a fixture ever surfaces where the two orders
// disagree, we can promote this to the content-keyed variant.
func renumberDataIDsInUnit(unitEl *etree.Element) {
	odEl := unitEl.SelectElement("originalData")
	if odEl == nil {
		return
	}
	dataEls := odEl.SelectElements("data")
	if len(dataEls) == 0 {
		return
	}
	// Build oldID → newID map; only renumber entries whose current id
	// doesn't already match the canonical "d<n>" form for its slot.
	rename := make(map[string]string, len(dataEls))
	for i, dataEl := range dataEls {
		oldID := attrValue(dataEl, "id")
		newID := fmt.Sprintf("d%d", i+1)
		if oldID == newID {
			continue
		}
		rename[oldID] = newID
		dataEl.CreateAttr("id", newID)
	}
	if len(rename) == 0 {
		return
	}
	// Walk the unit's source/target/inline subtree and patch every
	// referencing attribute. data references appear on <ph>, <sc>,
	// <ec>, <pc> via dataRef / dataRefStart / dataRefEnd.
	for _, child := range unitEl.ChildElements() {
		if child.Tag != "segment" && child.Tag != "ignorable" {
			continue
		}
		walkXliff2El(child, func(el *etree.Element) {
			for _, attrName := range [...]string{"dataRef", "dataRefStart", "dataRefEnd"} {
				v := attrValue(el, attrName)
				if v == "" {
					continue
				}
				if newV, ok := rename[v]; ok {
					el.CreateAttr(attrName, newV)
				}
			}
		})
	}
}

// stripOkapiDefaults walks the entire DOM and removes attributes whose
// values match the spec default that okapi's XLIFFWriter omits on
// output. This narrows the gap between our DOM round-trip (which
// preserves source attributes verbatim) and okapi's read-then-rewrite
// (which drops defaults). The list mirrors the explicit emission gates
// in okapi.lib.xliff2.writer.XLIFFWriter:
//
//   - <note priority="1"> — Note.priority defaults to 1 (XLIFFWriter line 760)
//   - <pc canOverlap="no"> — pc default is "no" (per XLIFF 2.2 §3.3)
//   - <sc canOverlap="yes">, <ec canOverlap="yes"> — sc/ec default is
//     "yes"
//   - <unit canResegment="…" translate="…"> when the value matches the
//     parent file/group's effective value (XLIFFWriter.writeInheritedAttributes,
//     line 931 — "if data.getCanResegment() != parentData.getCanResegment()")
//   - same for <segment canResegment="…">
//
// canCopy/canDelete/canReorder defaults ("yes") are NOT stripped here:
// fixtures in the wild rarely emit those defaults explicitly, and when
// they do okapi preserves them. We only strip what okapi actively drops.
func stripOkapiDefaults(root *etree.Element) {
	if root == nil {
		return
	}
	stripInheritedAttrs(root, "yes", "yes")
	walkXliff2El(root, func(el *etree.Element) {
		switch el.Tag {
		case "note":
			if attrValue(el, "priority") == "1" {
				el.RemoveAttr("priority")
			}
		case "pc":
			if attrValue(el, "canOverlap") == "no" {
				el.RemoveAttr("canOverlap")
			}
		case "sc", "ec":
			if attrValue(el, "canOverlap") == "yes" {
				el.RemoveAttr("canOverlap")
			}
		case "target":
			// okapi's X2ToOkpConverter / OkpToX2Converter do not preserve
			// <target order=…>, so the round-tripped output never carries
			// the attribute even when the source has it (see fixtures
			// test01.xlf and test04.xlf, which use `order="3"` /
			// `order="1"` on segments to declare a different logical
			// emission sequence). Mirror that here.
			if attrValue(el, "order") != "" {
				el.RemoveAttr("order")
			}
		}
	})
}

// stripInheritedAttrs walks the file/group/unit/segment hierarchy and
// strips canResegment / translate attributes when their value matches
// the inherited (parent) value. Top-down recursion tracks the effective
// inherited values; the spec defaults at the root <xliff> level are
// "yes"/"yes" for both attributes.
func stripInheritedAttrs(el *etree.Element, inheritedCanReseg, inheritedTranslate string) {
	if el == nil {
		return
	}
	curCanReseg := inheritedCanReseg
	curTranslate := inheritedTranslate
	switch el.Tag {
	case "file", "group", "unit":
		if v := attrValue(el, "canResegment"); v != "" {
			if v == inheritedCanReseg {
				el.RemoveAttr("canResegment")
			} else {
				curCanReseg = v
			}
		}
		if v := attrValue(el, "translate"); v != "" {
			if v == inheritedTranslate {
				el.RemoveAttr("translate")
			} else {
				curTranslate = v
			}
		}
	case "segment":
		if v := attrValue(el, "canResegment"); v != "" && v == inheritedCanReseg {
			el.RemoveAttr("canResegment")
		}
		// translate is not a segment-level attribute in XLIFF 2.
		return
	}
	for _, child := range el.ChildElements() {
		stripInheritedAttrs(child, curCanReseg, curTranslate)
	}
}

// walkXliff2El invokes f on el and every descendant element. Recursive
// DFS — xliff2 trees are shallow (typically <unit>/<segment>/<source>/
// <pc>/<mrk> — at most a half-dozen nested levels) so the stack usage
// is bounded.
func walkXliff2El(el *etree.Element, f func(*etree.Element)) {
	if el == nil {
		return
	}
	f(el)
	for _, child := range el.ChildElements() {
		walkXliff2El(child, f)
	}
}

// segmentMatchesDOM reports whether the seg's content matches what's
// already in the DOM element. The comparison uses the segment's inline IR
// when it is **fresh** (its text content agrees with seg.Runs); otherwise
// falls back to text-only comparison via seg.Runs. Self-detecting freshness
// removes the caller-contract footgun where a tool modifies seg.Runs but
// leaves a stale IR behind — we'd otherwise silently skip patching.
func segmentMatchesDOM(domEl *etree.Element, s *seg) bool {
	if s == nil {
		return true
	}
	if ir := freshInlineIR(s); ir != nil {
		return inlinesEqual(ir.Inlines, parseInlines(domEl))
	}
	return domElementText(domEl) == runsFlatText(s.Runs)
}

// freshInlineIR returns the segment's inline IR Content when it is fresh —
// i.e., its concatenated text equals the segment's Runs' concatenated text.
// Returns nil when no IR is attached or when it has been invalidated by a
// Run-side modification.
func freshInlineIR(s *seg) *Content {
	if s == nil || s.Content == nil {
		return nil
	}
	if inlinesFlatText(s.Content.Inlines) != runsFlatText(s.Runs) {
		return nil
	}
	return s.Content
}

// inlinesFlatText returns the concatenated text content of an Inline
// tree, recursing into pc/mrk children. Placeholder elements (ph/sc/
// ec/sm/em) and code-point markers contribute nothing — they're
// position-stable across modifications and don't carry comparable text.
func inlinesFlatText(inls []Inline) string {
	var sb strings.Builder
	collectInlineText(&sb, inls)
	return sb.String()
}

func collectInlineText(sb *strings.Builder, inls []Inline) {
	for _, in := range inls {
		switch {
		case in.Text != nil:
			sb.WriteString(in.Text.Content)
		case in.Pc != nil:
			collectInlineText(sb, in.Pc.Children)
		case in.Mrk != nil:
			collectInlineText(sb, in.Mrk.Children)
		}
	}
}

// runsFlatText returns the concatenated text content of a Run sequence,
// counting only TextRun nodes. Used as the freshness ground truth for
// SegmentInlineAnnotation: tools that modify text-bearing Runs change
// this output, signaling that the annotation is stale.
func runsFlatText(runs []model.Run) string {
	var sb strings.Builder
	for _, r := range runs {
		if r.Text != nil {
			sb.WriteString(r.Text.Text)
		}
	}
	return sb.String()
}

// inlinesEqual reports structural equality of two Inline trees.
func inlinesEqual(a, b []Inline) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !inlineEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func inlineEqual(a, b Inline) bool {
	switch {
	case a.Text != nil && b.Text != nil:
		return a.Text.Content == b.Text.Content
	case a.Ph != nil && b.Ph != nil:
		return a.Ph.CodeAttrs == b.Ph.CodeAttrs
	case a.Pc != nil && b.Pc != nil:
		return a.Pc.CodeAttrs == b.Pc.CodeAttrs && inlinesEqual(a.Pc.Children, b.Pc.Children)
	case a.Sc != nil && b.Sc != nil:
		return a.Sc.CodeAttrs == b.Sc.CodeAttrs
	case a.Ec != nil && b.Ec != nil:
		return a.Ec.CodeAttrs == b.Ec.CodeAttrs
	case a.Mrk != nil && b.Mrk != nil:
		return a.Mrk.MrkAttrs == b.Mrk.MrkAttrs && inlinesEqual(a.Mrk.Children, b.Mrk.Children)
	case a.Sm != nil && b.Sm != nil:
		return a.Sm.MrkAttrs == b.Sm.MrkAttrs
	case a.Em != nil && b.Em != nil:
		return a.Em.StartRef == b.Em.StartRef
	}
	return false
}

// domElementText returns the concatenated CharData content of an
// element including descendants — used for best-effort text-only
// comparison when the model lacks a SegmentInlineAnnotation.
func domElementText(el *etree.Element) string {
	var sb strings.Builder
	collectText(&sb, el)
	return sb.String()
}

func collectText(sb *strings.Builder, el *etree.Element) {
	for _, c := range el.Child {
		switch t := c.(type) {
		case *etree.CharData:
			sb.WriteString(t.Data)
		case *etree.Element:
			collectText(sb, t)
		}
	}
}

// replaceInlineChildren wipes the element's children and re-renders
// them from the segment's content. Prefers the fresh IR (preserves
// inline-code attribute fidelity); falls back to the Runs' text when
// the IR is stale or absent (loses inline attributes on the patched
// segment but keeps text correct).
func replaceInlineChildren(el *etree.Element, s *seg) {
	el.Child = nil
	if ir := freshInlineIR(s); ir != nil {
		renderInlinesInto(el, ir.Inlines)
		return
	}
	el.SetText(model.RenderRunsWithData(s.Runs))
}

// childIndex returns the index of target in parent.Child, or -1.
func childIndex(parent *etree.Element, target *etree.Element) int {
	for i, c := range parent.Child {
		if c == target {
			return i
		}
	}
	return -1
}

// applyExplicitFileNotes appends notes stamped via SetFileNotes /
// AddFileNote to the file's existing <notes>, deduplicating by
// (category, id) — explicit notes win over carried-through notes.
func applyExplicitFileNotes(fileEl *etree.Element, explicit []FileNote) {
	notesEl := fileEl.SelectElement("notes")
	if notesEl == nil {
		notesEl = etree.NewElement("notes")
		// Insert after <skeleton> if present, otherwise at the start.
		insertAt := 0
		for i, c := range fileEl.Child {
			if e, ok := c.(*etree.Element); ok && e.Tag == "skeleton" {
				insertAt = i + 1
			}
		}
		fileEl.InsertChildAt(insertAt, notesEl)
	}
	// Build a (category, id) → element index for dedup-overwrite.
	existing := make(map[[2]string]*etree.Element)
	for _, n := range notesEl.SelectElements("note") {
		key := [2]string{attrValue(n, "category"), attrValue(n, "id")}
		existing[key] = n
	}
	for _, n := range explicit {
		key := [2]string{n.Category, n.ID}
		if e, ok := existing[key]; ok {
			e.SetText(n.Content)
			continue
		}
		ne := notesEl.CreateElement("note")
		if n.ID != "" {
			ne.CreateAttr("id", n.ID)
		}
		if n.Category != "" {
			ne.CreateAttr("category", n.Category)
		}
		ne.SetText(n.Content)
	}
}

// setRootAttrs writes the <xliff> element's attributes in spec-canonical
// order: xmlns first, then any captured xmlns:* prefix declarations
// (sorted), then version/srcLang/trgLang, then any captured non-namespace
// extra attributes.
func (w *Writer) setRootAttrs(root *etree.Element, version string, targetLang model.LocaleID) {
	root.CreateAttr("xmlns", NamespaceForVersion(version))

	var nsAttrs []xml.Attr
	var otherAttrs []xml.Attr
	for _, a := range w.inputExtraAttr {
		if isXmlnsAttr(a) {
			if a.Name.Local == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
				continue
			}
			nsAttrs = append(nsAttrs, a)
		} else {
			otherAttrs = append(otherAttrs, a)
		}
	}
	sort.SliceStable(nsAttrs, func(i, j int) bool {
		return nsAttrs[i].Name.Local < nsAttrs[j].Name.Local
	})
	for _, a := range nsAttrs {
		root.CreateAttr("xmlns:"+a.Name.Local, a.Value)
	}

	root.CreateAttr("version", version)
	if w.sourceLang != "" {
		root.CreateAttr("srcLang", string(w.sourceLang))
	}
	if !targetLang.IsEmpty() {
		root.CreateAttr("trgLang", string(targetLang))
	}

	for _, a := range otherAttrs {
		name := a.Name.Local
		if a.Name.Space == "xml" {
			name = "xml:" + a.Name.Local
		} else if a.Name.Space != "" && a.Name.Space != "xmlns" {
			name = a.Name.Space + ":" + a.Name.Local
		}
		root.CreateAttr(name, a.Value)
	}
}

// appendUnit builds a <unit> element for the given Block and appends it
// to parent.
func (w *Writer) appendUnit(parent *etree.Element, block *model.Block, targetLang model.LocaleID) {
	unitEl := parent.CreateElement("unit")
	unitEl.CreateAttr("id", block.ID)
	if block.Name != "" {
		unitEl.CreateAttr("name", block.Name)
	}
	if !block.Translatable {
		unitEl.CreateAttr("translate", "no")
	}

	// Unit-level notes (block.Properties keys "note-N").
	noteVals := unitNotesFromProperties(block.Properties)
	if len(noteVals) > 0 {
		notesEl := unitEl.CreateElement("notes")
		for _, content := range noteVals {
			noteEl := notesEl.CreateElement("note")
			noteEl.SetText(content)
		}
	}

	// <originalData> emission, only when the unit declares one.
	if odAnn, ok := block.Annotations["xliff2:original-data"].(*OriginalDataAnnotation); ok && odAnn != nil && len(odAnn.Entries) > 0 {
		odEl := unitEl.CreateElement("originalData")
		// Emit data entries in id-sorted order for deterministic output.
		ids := make([]string, 0, len(odAnn.Entries))
		for id := range odAnn.Entries {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			dataEl := odEl.CreateElement("data")
			dataEl.CreateAttr("id", id)
			renderInlinesInto(dataEl, odAnn.Entries[id].Inlines)
		}
	}

	// Build segments. Per-segment inline IR drives full-fidelity emission
	// of source/target bodies; segments without an IR fall back to plain
	// Run text. The source/target seg lists are reconstructed from the
	// block's flat runs + segmentation overlays.
	srcSegs := sourceSegsFromBlock(block)
	tgtSegs := targetSegsFromBlock(block, targetLang)
	for i := range srcSegs {
		srcSeg := &srcSegs[i]
		// XLIFF 2 distinguishes <segment> (translatable) from <ignorable>
		// (non-translatable inter-segment material). Emit the right tag.
		tag := "segment"
		if srcSeg.Ignorable {
			tag = "ignorable"
		}
		segEl := unitEl.CreateElement(tag)
		if srcSeg.ID != "" {
			segEl.CreateAttr("id", srcSeg.ID)
		}

		srcEl := segEl.CreateElement("source")
		writeSegmentInline(srcEl, srcSeg)

		for j := range tgtSegs {
			if tgtSegs[j].ID == srcSeg.ID && srcSeg.ID != "" {
				tgtEl := segEl.CreateElement("target")
				writeSegmentInline(tgtEl, &tgtSegs[j])
				break
			}
		}
	}
}

// writeSegmentInline writes the segment's body into el using the
// fresh per-segment Inline IR when available, falling back to the
// segment's Run text otherwise. See freshInlineIR for staleness
// detection.
func writeSegmentInline(el *etree.Element, s *seg) {
	if s == nil {
		return
	}
	if ir := freshInlineIR(s); ir != nil {
		renderInlinesInto(el, ir.Inlines)
		return
	}
	el.SetText(model.RenderRunsWithData(s.Runs))
}

// renderInlinesInto walks an Inline IR and appends each node to parent
// as etree children.
func renderInlinesInto(parent *etree.Element, inls []Inline) {
	for _, in := range inls {
		switch {
		case in.Text != nil:
			appendCharData(parent, in.Text.Content)
		case in.Ph != nil:
			el := parent.CreateElement("ph")
			writeCodeAttrs(el, in.Ph.CodeAttrs, codeAttrPh)
		case in.Pc != nil:
			el := parent.CreateElement("pc")
			writeCodeAttrs(el, in.Pc.CodeAttrs, codeAttrPc)
			renderInlinesInto(el, in.Pc.Children)
		case in.Sc != nil:
			el := parent.CreateElement("sc")
			writeCodeAttrs(el, in.Sc.CodeAttrs, codeAttrSc)
		case in.Ec != nil:
			el := parent.CreateElement("ec")
			writeCodeAttrs(el, in.Ec.CodeAttrs, codeAttrEc)
		case in.Mrk != nil:
			el := parent.CreateElement("mrk")
			writeMrkAttrs(el, in.Mrk.MrkAttrs)
			renderInlinesInto(el, in.Mrk.Children)
		case in.Sm != nil:
			el := parent.CreateElement("sm")
			writeMrkAttrs(el, in.Sm.MrkAttrs)
		case in.Em != nil:
			el := parent.CreateElement("em")
			if in.Em.StartRef != "" {
				el.CreateAttr("startRef", in.Em.StartRef)
			}
		}
	}
}

// appendCharData adds a CharData node to parent. etree's Element.SetText
// only sets the leading character data; for subsequent text-after-elements
// we use the Tail of the previous child OR an explicit CharData node.
func appendCharData(parent *etree.Element, s string) {
	if s == "" {
		return
	}
	if len(parent.Child) == 0 {
		parent.SetText(s)
		return
	}
	// Place the text on the trailing Tail of the last child element.
	last := parent.Child[len(parent.Child)-1]
	switch t := last.(type) {
	case *etree.Element:
		// etree models inter-sibling text as a CharData token, NOT as
		// a Tail field. Insert a CharData token after the element.
		cd := etree.NewText(s)
		parent.InsertChildAt(len(parent.Child), cd)
		_ = t
	case *etree.CharData:
		t.Data += s
	default:
		cd := etree.NewText(s)
		parent.InsertChildAt(len(parent.Child), cd)
	}
}

// codeAttrFlavor selects which CodeAttrs fields apply for a given inline
// element (different elements support different subsets per spec).
type codeAttrFlavor int

const (
	codeAttrPh codeAttrFlavor = iota
	codeAttrPc
	codeAttrSc
	codeAttrEc
)

// writeCodeAttrs emits the spec-applicable CodeAttrs onto el, in
// canonical order. Skips empty attributes and spec-default values.
func writeCodeAttrs(el *etree.Element, a CodeAttrs, flavor codeAttrFlavor) {
	if a.ID != "" {
		el.CreateAttr("id", a.ID)
	}
	if flavor == codeAttrEc && a.StartRef != "" {
		el.CreateAttr("startRef", a.StartRef)
	}
	if a.Type != "" {
		el.CreateAttr("type", a.Type)
	}
	if a.SubType != "" {
		el.CreateAttr("subType", a.SubType)
	}
	if a.CanCopy != "" && a.CanCopy != "yes" {
		el.CreateAttr("canCopy", a.CanCopy)
	}
	if a.CanDelete != "" && a.CanDelete != "yes" {
		el.CreateAttr("canDelete", a.CanDelete)
	}
	if a.CanReorder != "" && a.CanReorder != "yes" {
		el.CreateAttr("canReorder", a.CanReorder)
	}
	switch flavor {
	case codeAttrPc:
		if a.CanOverlap != "" && a.CanOverlap != "no" {
			el.CreateAttr("canOverlap", a.CanOverlap)
		}
	case codeAttrSc, codeAttrEc:
		if a.CanOverlap != "" && a.CanOverlap != "yes" {
			el.CreateAttr("canOverlap", a.CanOverlap)
		}
	}
	if a.CopyOf != "" {
		el.CreateAttr("copyOf", a.CopyOf)
	}
	if flavor == codeAttrPc {
		if a.DataRefStart != "" {
			el.CreateAttr("dataRefStart", a.DataRefStart)
		}
		if a.DataRefEnd != "" {
			el.CreateAttr("dataRefEnd", a.DataRefEnd)
		}
	} else if a.DataRef != "" {
		el.CreateAttr("dataRef", a.DataRef)
	}
	if (flavor == codeAttrSc || flavor == codeAttrEc || flavor == codeAttrPc) && a.Dir != "" {
		el.CreateAttr("dir", a.Dir)
	}
	if flavor == codeAttrPc {
		if a.DispStart != "" {
			el.CreateAttr("dispStart", a.DispStart)
		}
		if a.DispEnd != "" {
			el.CreateAttr("dispEnd", a.DispEnd)
		}
	} else if a.Disp != "" {
		el.CreateAttr("disp", a.Disp)
	}
	if flavor == codeAttrPc {
		if a.EquivStart != "" {
			el.CreateAttr("equivStart", a.EquivStart)
		}
		if a.EquivEnd != "" {
			el.CreateAttr("equivEnd", a.EquivEnd)
		}
	} else if a.Equiv != "" {
		el.CreateAttr("equiv", a.Equiv)
	}
	if flavor == codeAttrPc {
		if a.SubFlowsStart != "" {
			el.CreateAttr("subFlowsStart", a.SubFlowsStart)
		}
		if a.SubFlowsEnd != "" {
			el.CreateAttr("subFlowsEnd", a.SubFlowsEnd)
		}
	} else if a.SubFlows != "" {
		el.CreateAttr("subFlows", a.SubFlows)
	}
	if (flavor == codeAttrSc || flavor == codeAttrEc) && a.Isolated != "" && a.Isolated != "no" {
		el.CreateAttr("isolated", a.Isolated)
	}
}

// writeMrkAttrs emits MrkAttrs in canonical order.
func writeMrkAttrs(el *etree.Element, a MrkAttrs) {
	if a.ID != "" {
		el.CreateAttr("id", a.ID)
	}
	if a.Type != "" {
		el.CreateAttr("type", a.Type)
	}
	if a.Translate != "" {
		el.CreateAttr("translate", a.Translate)
	}
	if a.Ref != "" {
		el.CreateAttr("ref", a.Ref)
	}
	if a.Value != "" {
		el.CreateAttr("value", a.Value)
	}
}

// indentXliff applies 2-space indentation to a tree, recursing past
// structural elements (xliff/file/group/unit/segment/notes/originalData)
// and treating XLIFF mixed-content elements (source/target/pc/mrk/data)
// as opaque — their children's whitespace is left untouched.
//
// We DON'T use etree.Document.Indent() because it has a known bug
// (etree#88) that injects whitespace into mixed content even when
// xml:space="preserve" is set.
func indentXliff(el *etree.Element, depth int) {
	if el == nil || isMixedContentElement(el.Tag) {
		return
	}
	children := el.ChildElements()
	if len(children) == 0 {
		return
	}
	prefix := "\n" + strings.Repeat("  ", depth+1)
	closePrefix := "\n" + strings.Repeat("  ", depth)
	// Wipe existing inter-element whitespace so we can place ours.
	stripWhitespaceCharData(el)
	for i, c := range el.ChildElements() {
		// Insert leading whitespace before each child.
		insertCharDataBefore(el, c, prefix)
		_ = i
		indentXliff(c, depth+1)
	}
	// Trailing whitespace before the closing tag.
	parent := el
	parent.InsertChildAt(len(parent.Child), etree.NewText(closePrefix))
}

// isMixedContentElement reports whether the named element is one whose
// children may include significant whitespace per XLIFF 2 spec. We
// must not re-indent inside these.
func isMixedContentElement(tag string) bool {
	switch tag {
	case "source", "target", "pc", "mrk", "data", "note", "ph", "sc", "ec", "sm", "em", "cp":
		return true
	}
	return false
}

// stripWhitespaceCharData removes pure-whitespace CharData tokens that
// are direct children of el. Element-bound character data (Text on the
// element with mixed content) is left alone — we only strip the
// inter-element whitespace tokens.
func stripWhitespaceCharData(el *etree.Element) {
	out := el.Child[:0]
	for _, c := range el.Child {
		if cd, ok := c.(*etree.CharData); ok {
			if strings.TrimSpace(cd.Data) == "" {
				continue
			}
		}
		out = append(out, c)
	}
	el.Child = out
}

// insertCharDataBefore inserts a CharData token immediately before
// target inside parent.
func insertCharDataBefore(parent *etree.Element, target *etree.Element, s string) {
	for i, c := range parent.Child {
		if c == target {
			cd := etree.NewText(s)
			parent.InsertChildAt(i, cd)
			return
		}
	}
}

// isXmlnsAttr reports whether an xml.Attr is an XML namespace declaration.
func isXmlnsAttr(a xml.Attr) bool {
	if a.Name.Space == "xmlns" {
		return true
	}
	if a.Name.Space == "" && a.Name.Local == "xmlns" {
		return true
	}
	return false
}

// unitNotesFromProperties returns the values of block.Properties keys
// "note-0", "note-1", … in numeric order.
func unitNotesFromProperties(props map[string]string) []string {
	if len(props) == 0 {
		return nil
	}
	type indexed struct {
		idx int
		val string
	}
	var notes []indexed
	for k, v := range props {
		if !strings.HasPrefix(k, "note-") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(k, "note-"))
		if err != nil {
			continue
		}
		notes = append(notes, indexed{idx: n, val: v})
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].idx < notes[j].idx })
	out := make([]string, len(notes))
	for i, n := range notes {
		out[i] = n.val
	}
	return out
}
