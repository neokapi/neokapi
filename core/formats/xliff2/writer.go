package xliff2

import (
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

// flush builds an etree DOM from captured items and serializes it.
func (w *Writer) flush() error {
	if w.Output == nil {
		return nil
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
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
	if _, err := doc.WriteTo(w.Output); err != nil {
		return fmt.Errorf("xliff2 writer: write: %w", err)
	}
	return nil
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

	// Build segments. Per-segment inline IR (SegmentInlineAnnotation)
	// drives full-fidelity emission of source/target bodies; segments
	// without that annotation fall back to plain Run text.
	for _, srcSeg := range block.Source {
		segEl := unitEl.CreateElement("segment")
		if srcSeg.ID != "" {
			segEl.CreateAttr("id", srcSeg.ID)
		}

		srcEl := segEl.CreateElement("source")
		writeSegmentInline(srcEl, srcSeg)

		if trgSegs, ok := block.Targets[targetLang]; ok {
			for _, ts := range trgSegs {
				if ts.ID == srcSeg.ID {
					tgtEl := segEl.CreateElement("target")
					writeSegmentInline(tgtEl, ts)
					break
				}
			}
		}
	}
}

// writeSegmentInline writes the segment's body into el using the
// per-segment Inline IR when available, falling back to the segment's
// Run text otherwise.
func writeSegmentInline(el *etree.Element, seg *model.Segment) {
	if seg == nil {
		return
	}
	if a, ok := seg.Annotations["xliff2:segment-inline"].(*SegmentInlineAnnotation); ok && a != nil && a.Content != nil {
		renderInlinesInto(el, a.Content.Inlines)
		return
	}
	el.SetText(model.RenderRunsWithData(seg.Runs))
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
		cd := etree.NewCharData(s)
		parent.InsertChildAt(len(parent.Child), cd)
		_ = t
	case *etree.CharData:
		t.Data += s
	default:
		cd := etree.NewCharData(s)
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
	} else {
		if a.DataRef != "" {
			el.CreateAttr("dataRef", a.DataRef)
		}
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
	parent.InsertChildAt(len(parent.Child), etree.NewCharData(closePrefix))
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
			cd := etree.NewCharData(s)
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
