package xliff2

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/beevik/etree"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// FileNotePropertyKey is the layer.Properties key used to surface a
// file-level <note> parsed from an XLIFF 2 <file>. One key is written per
// note, combining category + id so multiple notes coexist:
//
//	file-note:<category>:<id>   (empty category and/or id are kept empty)
//
// The kapi batch id, emitted by kapi extract, is carried as
// "file-note:kapi:batch-id" and read back by kapi merge via
// BatchIDFromLayer.
const FileNotePropertyPrefix = "file-note:"

// Reader implements DataFormatReader for XLIFF 2.x files using a
// DOM-based etree parse. The full source DOM is stashed on the first
// emitted Layer via SourceDOMAnnotation so the writer's round-trip
// mode can patch it in place for byte-equal output on unchanged units.
//
// Parse is lossless to neokapi's content model: every spec attribute is
// either decoded into a typed model field or preserved on the source
// DOM (and module/extension subtrees ride along automatically via the
// DOM). See docs/internals/research/xliff2-design.md.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new XLIFF 2.x reader. The reader accepts the
// OASIS 2.0, 2.1 and 2.2 document namespaces as a compatible family.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xliff2",
			FormatDisplayName: "XLIFF 2.x",
			FormatMimeType:    "application/xliff+xml",
			FormatExtensions:  []string{".xlf", ".xliff"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
// When set the reader switches to a streaming token-based parse that
// records byte offsets for source/target placeholders. The DOM-based
// parse is bypassed in this mode (skeleton round-trip is a separate
// flow per the design doc).
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/xliff+xml"},
		Extensions: []string{".xlf", ".xliff"},
		Sniff: func(data []byte) bool {
			s := string(data)
			if !strings.Contains(s, "<xliff") {
				return false
			}
			return strings.Contains(s, "urn:oasis:names:tc:xliff:document:2") ||
				strings.Contains(s, `version="2.0"`) ||
				strings.Contains(s, `version="2.1"`) ||
				strings.Contains(s, `version="2.2"`)
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("xliff2: nil document or reader")
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
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xliff2: reading: %w", err)}
		return
	}
	if r.skeletonStore != nil {
		r.readContentStreaming(ctx, ch, content)
		return
	}

	// Coerce XML 1.1 → 1.0 in the declaration. XLIFF 2.x spec mandates
	// XML 1.0; some real-world tools (and a few okapi-testdata fixtures)
	// emit `version="1.1"` regardless. Rewriting before parse preserves
	// document content and lets stdlib encoding/xml accept it.
	content = coerceXMLDeclTo10(content)

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(content); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xliff2: parsing: %w", err)}
		return
	}

	// We DON'T normalize literal CR (0x0D) → LF here. CRs that originate
	// from numeric character references (&#xD;) survive XML 1.0 §2.11
	// line-end normalization on parse precisely because they were
	// authored as escape sequences; the writer round-trips the DOM by
	// re-escaping every literal CR back to `&#13;` (see
	// writeBytesCREscape), so idempotency holds without us flattening
	// the data here. Flattening here would also lose the entity form
	// that okapi's XLIFFWriter uses for fixtures with explicit CRs in
	// inline markers (e.g. translated.xlf's "IBM Globalization&#xD;
	// Pipeline").

	root := doc.SelectElement("xliff")
	if root == nil {
		// etree's SelectElement is namespace-agnostic by local name.
		// If the XLIFF root isn't found, look for any root element.
		if rootEl := doc.Root(); rootEl != nil && rootEl.Tag == "xliff" {
			root = rootEl
		}
	}
	if root == nil {
		ch <- model.PartResult{Error: errors.New("xliff2: no <xliff> root element found")}
		return
	}

	srcLang := attrValue(root, "srcLang")
	trgLang := attrValue(root, "trgLang")
	version := attrValue(root, "version")

	files := root.SelectElements("file")
	for fileIdx, file := range files {
		fileID := attrValue(file, "id")
		layer := &model.Layer{
			ID:             "file-" + fileID,
			Name:           fileID,
			Format:         "xliff2",
			Locale:         model.LocaleID(srcLang),
			IsMultilingual: true,
			Properties: map[string]string{
				"target-language": trgLang,
			},
		}
		if version != "" {
			layer.Properties["xliff-version"] = version
		}

		// Capture root-level extra attributes (custom namespaces,
		// xml:lang, …) onto the first file's layer for round-trip.
		if fileIdx == 0 {
			setExtraXliffAttrsFromEtree(layer, root)
		}

		// File-level <notes> → file-note:<category>:<id> properties.
		if notesEl := file.SelectElement("notes"); notesEl != nil {
			setFileNotePropertiesFromEtree(layer, notesEl)
		}

		// Stash the source etree.Document AND original bytes on the
		// first file's layer so the writer's round-trip mode can patch
		// it in place — yielding byte-equal output for unmodified
		// inputs (verbatim passthrough of Original) and minimal diffs
		// for modified ones. Only on the first file (single-file XLIFF
		// is the common case; multi-file falls back to generation mode
		// for non-first files).
		if fileIdx == 0 {
			layer.SetAnno("xliff2:source-dom", &SourceDOMAnnotation{
				Doc:      doc,
				Original: content,
			})
		}

		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
			return
		}

		// XLIFF 2 spec: `translate` is inheritable from <xliff>/<file>/
		// <group> down to <unit>. Default at the document level is "yes".
		// Compute this file's effective translate state and pass it to
		// children so units that don't override it inherit it.
		fileTranslate := inheritedTranslate(true, file)

		// Walk file children in order, emitting groups/units.
		for _, child := range file.ChildElements() {
			switch child.Tag {
			case "group":
				r.emitGroup(ctx, ch, child, model.LocaleID(trgLang), fileTranslate)
			case "unit":
				r.emitUnit(ctx, ch, child, model.LocaleID(trgLang), fileTranslate)
			}
		}

		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
	}
}

// emitGroup emits PartGroupStart/PartGroupEnd around the group's
// contents, recursing into nested groups and units. parentTranslate is
// the inheritable translate flag from the enclosing <file>/<group>;
// this group's own translate attribute, if present, overrides it before
// being passed to children.
func (r *Reader) emitGroup(ctx context.Context, ch chan<- model.PartResult, group *etree.Element, trgLang model.LocaleID, parentTranslate bool) {
	gs := &model.GroupStart{
		ID:   attrValue(group, "id"),
		Name: attrValue(group, "name"),
		Type: attrValue(group, "type"),
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: gs}) {
		return
	}
	groupTranslate := inheritedTranslate(parentTranslate, group)
	for _, child := range group.ChildElements() {
		switch child.Tag {
		case "group":
			r.emitGroup(ctx, ch, child, trgLang, groupTranslate)
		case "unit":
			r.emitUnit(ctx, ch, child, trgLang, groupTranslate)
		}
	}
	r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: gs.ID}})
}

// emitUnit builds a Block from a <unit> element and emits it.
// parentTranslate is the inheritable translate flag from the enclosing
// <file>/<group>; the unit's own translate attribute (if present)
// overrides it. Default at the document level is "yes".
func (r *Reader) emitUnit(ctx context.Context, ch chan<- model.PartResult, unit *etree.Element, trgLang model.LocaleID, parentTranslate bool) {
	translatable := inheritedTranslate(parentTranslate, unit)

	block := &model.Block{
		ID:           attrValue(unit, "id"),
		Name:         attrValue(unit, "name"),
		Translatable: translatable,
		Properties:   make(map[string]string),
		Targets:      make(map[model.VariantKey]*model.Target),
	}

	// Unit-level <notes>: store as note-N properties (preserves order).
	if notesEl := unit.SelectElement("notes"); notesEl != nil {
		for i, n := range notesEl.SelectElements("note") {
			block.Properties[fmt.Sprintf("note-%d", i)] = strings.TrimSpace(n.Text())
		}
	}

	// <originalData> capture.
	if odEl := unit.SelectElement("originalData"); odEl != nil {
		entries := make(map[string]*Content)
		for _, dataEl := range odEl.SelectElements("data") {
			id := attrValue(dataEl, "id")
			if id == "" {
				continue
			}
			entries[id] = &Content{Inlines: parseInlines(dataEl)}
		}
		if len(entries) > 0 {
			block.SetAnno("xliff2:original-data", &OriginalDataAnnotation{Entries: entries})
		}
	}

	// Walk segments and ignorables in document order.
	srcSegs := []seg{}
	tgtSegs := []seg{}

	// Collect explicit segment ids first so synthesized ids for
	// unkeyed elements don't collide with them. xliff2 makes the id
	// attribute optional on <segment>/<ignorable>, so a unit can have
	// an unkeyed <ignorable> immediately followed by an explicit
	// <segment id="s2"> — without this guard our synthesized "s2" for
	// the ignorable would shadow the real segment in srcByID lookups.
	explicitIds := make(map[string]bool)
	for _, child := range unit.ChildElements() {
		if child.Tag != "segment" && child.Tag != "ignorable" {
			continue
		}
		if id := attrValue(child, "id"); id != "" {
			explicitIds[id] = true
		}
	}

	segIdx := 0
	for _, child := range unit.ChildElements() {
		if child.Tag != "segment" && child.Tag != "ignorable" {
			continue
		}
		segIdx++
		segID := attrValue(child, "id")
		if segID == "" {
			// Synthesize a non-colliding id: "s<n>" first, then
			// "_xliff2_seg_<n>" if a real segment already claimed it.
			segID = fmt.Sprintf("s%d", segIdx)
			if explicitIds[segID] {
				segID = fmt.Sprintf("_xliff2_seg_%d", segIdx)
			}
		}

		// State on segment becomes a property (last writer wins for
		// multi-segment units; rare to differ). subState is preserved
		// only via the source DOM round-trip.
		if state := attrValue(child, "state"); state != "" {
			block.Properties["state"] = state
		}

		srcEl := child.SelectElement("source")
		if srcEl == nil {
			continue // spec violation but tolerate
		}
		srcInlines := parseInlines(srcEl)
		srcSeg := seg{
			ID:      segID,
			Runs:    inlinesToRuns(srcInlines),
			Content: &Content{Inlines: srcInlines},
		}
		// Preserve <ignorable> vs <segment> distinction for downstream
		// pipelines that need it (e.g. parity native engine seeds
		// targets for ignorables only, mirroring okapi's
		// X2ToOkpConverter line 200).
		if child.Tag == "ignorable" {
			srcSeg.Ignorable = true
		}
		srcSegs = append(srcSegs, srcSeg)

		if tgtEl := child.SelectElement("target"); tgtEl != nil {
			tgtInlines := parseInlines(tgtEl)
			if hasNonEmptyInline(tgtInlines) {
				tgtSegs = append(tgtSegs, seg{
					ID:      segID,
					Runs:    inlinesToRuns(tgtInlines),
					Content: &Content{Inlines: tgtInlines},
					// Mirror the source <ignorable> marker onto the
					// target seg so the target segmentation overlay
					// carries the same kind — downstream consumers
					// (e.g. the parity pseudo) must keep an
					// <ignorable>'s target verbatim, never translate it.
					Ignorable: child.Tag == "ignorable",
				})
			}
		}
	}

	applySegmentsToBlock(block, srcSegs, tgtSegs, trgLang)

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// parseInlines walks an etree.Element's children and builds the
// xliff2 Inline IR. Text and CharData come through as Text nodes;
// element children dispatch on local name to typed Inline variants.
// <cp hex="X"/> is resolved to its code point and merged into the
// preceding (or following) Text node.
func parseInlines(parent *etree.Element) []Inline {
	var out []Inline
	for _, tok := range parent.Child {
		switch t := tok.(type) {
		case *etree.CharData:
			s := t.Data
			if s == "" {
				continue
			}
			out = appendText(out, s)
		case *etree.Element:
			switch t.Tag {
			case "cp":
				if hex := attrValue(t, "hex"); hex != "" {
					if n, err := strconv.ParseInt(hex, 16, 32); err == nil {
						out = appendText(out, string(rune(n)))
					}
				}
			case "ph":
				out = append(out, Inline{Ph: &Ph{CodeAttrs: parseCodeAttrs(t)}})
			case "pc":
				out = append(out, Inline{Pc: &Pc{
					CodeAttrs: parseCodeAttrs(t),
					Children:  parseInlines(t),
				}})
			case "sc":
				out = append(out, Inline{Sc: &Sc{CodeAttrs: parseCodeAttrs(t)}})
			case "ec":
				out = append(out, Inline{Ec: &Ec{CodeAttrs: parseCodeAttrs(t)}})
			case "mrk":
				out = append(out, Inline{Mrk: &Mrk{
					MrkAttrs: parseMrkAttrs(t),
					Children: parseInlines(t),
				}})
			case "sm":
				out = append(out, Inline{Sm: &Sm{MrkAttrs: parseMrkAttrs(t)}})
			case "em":
				out = append(out, Inline{Em: &Em{StartRef: attrValue(t, "startRef")}})
			default:
				// Unknown inline element (extension namespace?) — skip
				// silently. The source DOM still carries it for
				// round-trip; this just means it won't surface to
				// downstream tools that walk the IR.
			}
		}
	}
	return out
}

// appendText merges adjacent Text nodes so <cp> resolution doesn't
// create text fragmentation. CR characters (0x0D) are preserved as-is:
// they only reach the IR via numeric character references (`&#xD;` /
// `&#x000D;`) on the input side — bare CR bytes were already normalized
// to LF by encoding/xml on parse per XML 1.0 §2.11. The writer's CR
// escape (writeBytesCREscape) round-trips literal `\r` back to `&#13;`,
// so preserving the entity-decoded `\r` in the IR keeps the
// pseudo-translation pipeline transparent: a tool that walks runs and
// rewrites letters won't touch `\r`, and the writer re-emits it.
func appendText(out []Inline, s string) []Inline {
	if n := len(out); n > 0 && out[n-1].Text != nil {
		out[n-1].Text.Content += s
		return out
	}
	return append(out, Inline{Text: &Text{Content: s}})
}

// parseCodeAttrs reads all spec-defined inline-code attributes off
// an etree.Element into a CodeAttrs struct.
func parseCodeAttrs(el *etree.Element) CodeAttrs {
	return CodeAttrs{
		ID:            attrValue(el, "id"),
		CanCopy:       attrValue(el, "canCopy"),
		CanDelete:     attrValue(el, "canDelete"),
		CanReorder:    attrValue(el, "canReorder"),
		CanOverlap:    attrValue(el, "canOverlap"),
		CopyOf:        attrValue(el, "copyOf"),
		DataRef:       attrValue(el, "dataRef"),
		DataRefStart:  attrValue(el, "dataRefStart"),
		DataRefEnd:    attrValue(el, "dataRefEnd"),
		Dir:           attrValue(el, "dir"),
		Disp:          attrValue(el, "disp"),
		DispStart:     attrValue(el, "dispStart"),
		DispEnd:       attrValue(el, "dispEnd"),
		Equiv:         attrValue(el, "equiv"),
		EquivStart:    attrValue(el, "equivStart"),
		EquivEnd:      attrValue(el, "equivEnd"),
		SubFlows:      attrValue(el, "subFlows"),
		SubFlowsStart: attrValue(el, "subFlowsStart"),
		SubFlowsEnd:   attrValue(el, "subFlowsEnd"),
		SubType:       attrValue(el, "subType"),
		Type:          attrValue(el, "type"),
		Isolated:      attrValue(el, "isolated"),
		StartRef:      attrValue(el, "startRef"),
	}
}

// parseMrkAttrs reads annotation-marker attributes off an etree.Element.
func parseMrkAttrs(el *etree.Element) MrkAttrs {
	return MrkAttrs{
		ID:        attrValue(el, "id"),
		Type:      attrValue(el, "type"),
		Translate: attrValue(el, "translate"),
		Ref:       attrValue(el, "ref"),
		Value:     attrValue(el, "value"),
	}
}

// inlinesToRuns downconverts the xliff2 Inline IR to the framework's
// generic model.Run sequence. Lossy by design — Run is a simpler
// abstraction; the lossless path is the SourceBodyAnnotation /
// TargetBodyAnnotation IR. Downstream tools that need full attribute
// fidelity reach for the annotation; tools that only care about text
// and placeholder equivs use Runs.
func inlinesToRuns(inls []Inline) []model.Run {
	var out []model.Run
	for _, in := range inls {
		switch {
		case in.Text != nil:
			out = append(out, model.Run{Text: &model.TextRun{Text: in.Text.Content}})
		case in.Ph != nil:
			out = append(out, model.Run{Ph: &model.PlaceholderRun{
				ID:      in.Ph.ID,
				Type:    in.Ph.Type,
				SubType: in.Ph.SubType,
				Equiv:   in.Ph.Equiv,
				Disp:    in.Ph.Disp,
			}})
		case in.Pc != nil:
			out = append(out, model.Run{PcOpen: &model.PcOpenRun{
				ID:      in.Pc.ID,
				Type:    in.Pc.Type,
				SubType: in.Pc.SubType,
				Equiv:   in.Pc.EquivStart,
				Disp:    in.Pc.DispStart,
			}})
			out = append(out, inlinesToRuns(in.Pc.Children)...)
			out = append(out, model.Run{PcClose: &model.PcCloseRun{
				ID:    in.Pc.ID,
				Type:  in.Pc.Type,
				Equiv: in.Pc.EquivEnd,
			}})
		case in.Sc != nil:
			out = append(out, model.Run{Ph: &model.PlaceholderRun{
				ID:      in.Sc.ID,
				Type:    in.Sc.Type,
				SubType: in.Sc.SubType,
				Equiv:   in.Sc.Equiv,
				Disp:    in.Sc.Disp,
			}})
		case in.Ec != nil:
			out = append(out, model.Run{Ph: &model.PlaceholderRun{
				ID:      in.Ec.ID,
				Type:    in.Ec.Type,
				SubType: in.Ec.SubType,
				Equiv:   in.Ec.Equiv,
				Disp:    in.Ec.Disp,
			}})
		case in.Mrk != nil:
			// Annotation markers don't have a direct Run analogue; we
			// fold their text content through.
			out = append(out, inlinesToRuns(in.Mrk.Children)...)
		case in.Sm != nil, in.Em != nil:
			// No Run for span markers — they're metadata.
		}
	}
	return out
}

// hasNonEmptyInline reports whether any inline node has actual content.
// Used to suppress empty <target/> from emitting a Targets entry that
// would imply an empty translation.
func hasNonEmptyInline(inls []Inline) bool {
	for _, in := range inls {
		if in.Text != nil && in.Text.Content != "" {
			return true
		}
		if in.Ph != nil || in.Pc != nil || in.Sc != nil || in.Ec != nil ||
			in.Mrk != nil || in.Sm != nil || in.Em != nil {
			return true
		}
	}
	return false
}

// attrValue returns the value of a local-name attribute, ignoring
// namespace. Returns "" when absent.
func attrValue(el *etree.Element, local string) string {
	for _, a := range el.Attr {
		if a.Key == local {
			return a.Value
		}
	}
	return ""
}

// inheritedTranslate returns the effective `translate` flag for an
// XLIFF 2 element (file, group, or unit). XLIFF 2 §3.4.1 makes
// `translate` an inheritable attribute: an element's own `translate`
// attribute (if present) wins, otherwise the parent's effective value
// is inherited. The document default is "yes".
func inheritedTranslate(parent bool, el *etree.Element) bool {
	v := attrValue(el, "translate")
	if v == "" {
		return parent
	}
	return !strings.EqualFold(v, "no")
}

// extraAttrPropKeyPrefix prefixes Layer.Properties keys that carry XLIFF
// root attributes the reader captured but didn't interpret.
const extraAttrPropKeyPrefix = "xliff-xattr-"

// extraAttrPropKey returns the property key for the i-th captured extra attr.
func extraAttrPropKey(i int) string {
	return fmt.Sprintf("%s%d", extraAttrPropKeyPrefix, i)
}

// setExtraXliffAttrsFromEtree captures root-level extra attributes
// (custom namespace declarations, xml:lang, …) into Layer.Properties
// for round-trip via the writer's generation mode. Round-trip mode
// reads them off the source DOM directly and ignores these keys.
func setExtraXliffAttrsFromEtree(layer *model.Layer, root *etree.Element) {
	n := 0
	for _, a := range root.Attr {
		if isXliffCoreAttr(a) {
			continue
		}
		layer.Properties[extraAttrPropKey(n)] = encodeEtreeAttr(a)
		n++
	}
}

// isXliffCoreAttr reports whether an etree.Attr on the <xliff> root is
// one we explicitly interpret (and therefore don't need to preserve as
// an "extra" attribute). The default-namespace xmlns also belongs to
// the writer's responsibility (it picks the right URI for the chosen
// version).
func isXliffCoreAttr(a etree.Attr) bool {
	if a.Space == "" {
		switch a.Key {
		case "version", "srcLang", "trgLang", "xmlns":
			return true
		}
	}
	return false
}

// encodeEtreeAttr serializes an etree.Attr as "space|local=value".
func encodeEtreeAttr(a etree.Attr) string {
	return a.Space + "|" + a.Key + "=" + a.Value
}

// decodeExtraAttr inverts encodeEtreeAttr. Returns an xml.Attr because
// the writer's generation mode currently still uses encoding/xml types
// in some helpers (kept for compatibility).
func decodeExtraAttr(s string) (xml.Attr, bool) {
	bar := strings.IndexByte(s, '|')
	if bar < 0 {
		return xml.Attr{}, false
	}
	eq := strings.IndexByte(s[bar+1:], '=')
	if eq < 0 {
		return xml.Attr{}, false
	}
	space := s[:bar]
	local := s[bar+1 : bar+1+eq]
	value := s[bar+1+eq+1:]
	return xml.Attr{Name: xml.Name{Space: space, Local: local}, Value: value}, true
}

// extraAttrsFromLayer reconstructs captured extra attrs from a Layer's
// Properties, preserving source order via the numeric index.
func extraAttrsFromLayer(layer *model.Layer) []xml.Attr {
	if layer == nil {
		return nil
	}
	var out []xml.Attr
	for i := 0; ; i++ {
		v, ok := layer.Properties[extraAttrPropKey(i)]
		if !ok {
			break
		}
		if a, ok := decodeExtraAttr(v); ok {
			out = append(out, a)
		}
	}
	return out
}

// setFileNotePropertiesFromEtree copies <notes><note>... contents into
// layer.Properties under the file-note:<category>:<id> key convention.
func setFileNotePropertiesFromEtree(layer *model.Layer, notesEl *etree.Element) {
	if layer == nil || notesEl == nil {
		return
	}
	if layer.Properties == nil {
		layer.Properties = make(map[string]string)
	}
	for _, n := range notesEl.SelectElements("note") {
		content := strings.TrimSpace(n.Text())
		if content == "" {
			continue
		}
		category := attrValue(n, "category")
		id := attrValue(n, "id")
		if category == "" && id == "" {
			continue
		}
		layer.Properties[FileNotePropertyPrefix+category+":"+id] = content
	}
}

// coerceXMLDeclTo10 rewrites the XML declaration to version="1.0" if
// the input says 1.1. XLIFF 2.x mandates XML 1.0; XML 1.1 in the wild
// is virtually always a tooling glitch and the document is otherwise
// 1.0-compatible. Returns the input unchanged when the declaration is
// already 1.0 or absent.
func coerceXMLDeclTo10(in []byte) []byte {
	const decl11 = `<?xml version="1.1"`
	const decl10 = `<?xml version="1.0"`
	idx := bytes.Index(in, []byte(decl11))
	if idx < 0 {
		return in
	}
	out := make([]byte, 0, len(in))
	out = append(out, in[:idx]...)
	out = append(out, decl10...)
	out = append(out, in[idx+len(decl11):]...)
	return out
}

// xmlEscapeText escapes XML special characters in text content.
func xmlEscapeText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
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

// emit sends a Part downstream, returning false if the context is
// canceled.
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

// =====================================================================
// Streaming skeleton path (preserved from legacy reader)
// =====================================================================

// elemPos tracks the byte position of a source or target element's inner content.
type elemPos struct {
	startOffset int    // byte offset after opening tag
	endOffset   int    // byte offset before closing tag
	blockIdx    int    // 0-based block index
	elemType    string // "source" or "target"
}

// xliff2StreamState holds the mutable state for streaming XLIFF 2.x parsing.
type xliff2StreamState struct {
	reader  *Reader
	ctx     context.Context
	ch      chan<- model.PartResult
	rawText string
	decoder *xml.Decoder

	srcLang       string
	trgLang       string
	version       string
	extraAttrs    []xml.Attr
	fileID        string
	inFile        bool
	inUnit        bool
	inSegment     bool
	inSource      bool
	inTarget      bool
	inNotes       bool
	inNote        bool
	unitID        string
	unitName      string
	unitTranslate string
	// translateStack is the inheritable XLIFF 2 `translate` flag stack:
	// one entry per open <file>/<group> from outer to inner. Top of
	// stack is the effective parent value for the current element.
	// Document default is "yes" (true).
	translateStack []bool
	segID          string
	blockCount     int
	elemPositions  []elemPos
	elemStartOff   int64

	// Accumulators
	sourceInnerXML strings.Builder
	targetInnerXML strings.Builder
	noteBuilder    strings.Builder
	sourceDepth    int
	targetDepth    int

	// Current unit data
	sourceSegs []seg
	targets    map[model.LocaleID][]seg
	notes      []string
	states     []string
}

// parentTranslate returns the effective `translate` flag inherited
// from the closest enclosing <file>/<group> on the stack. Document
// default ("yes") is returned when the stack is empty.
func (s *xliff2StreamState) parentTranslate() bool {
	if n := len(s.translateStack); n > 0 {
		return s.translateStack[n-1]
	}
	return true
}

// readContentStreaming uses streaming XML parsing for skeleton byte-offset tracking.
func (r *Reader) readContentStreaming(ctx context.Context, ch chan<- model.PartResult, content []byte) {
	rawText := string(content)
	decoder := xml.NewDecoder(strings.NewReader(rawText))
	decoder.Strict = false

	s := &xliff2StreamState{
		reader:  r,
		ctx:     ctx,
		ch:      ch,
		rawText: rawText,
		decoder: decoder,
	}

	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xliff2: parsing: %w", err)}
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			s.handleStartElement(t)
		case xml.EndElement:
			s.handleEndElement(t)
		case xml.CharData:
			text := string(t)
			if s.inNote {
				s.noteBuilder.WriteString(text)
			} else if s.inSource {
				s.sourceInnerXML.WriteString(text)
			} else if s.inTarget {
				s.targetInnerXML.WriteString(text)
			}
		}
	}
	s.buildSkeleton()
}

func (s *xliff2StreamState) handleStartElement(t xml.StartElement) {
	switch t.Name.Local {
	case "xliff":
		for _, a := range t.Attr {
			switch a.Name.Local {
			case "srcLang":
				s.srcLang = a.Value
			case "trgLang":
				s.trgLang = a.Value
			case "version":
				s.version = a.Value
			default:
				if isXliffExtraAttrLegacy(a) {
					s.extraAttrs = append(s.extraAttrs, a)
				}
			}
		}
	case "file":
		s.inFile = true
		s.fileID = ""
		fileTranslate := true
		for _, a := range t.Attr {
			if a.Name.Local == "id" {
				s.fileID = a.Value
			}
			if a.Name.Local == "translate" && strings.EqualFold(a.Value, "no") {
				fileTranslate = false
			}
		}
		s.translateStack = append(s.translateStack, fileTranslate)
		layer := &model.Layer{
			ID:             "file-" + s.fileID,
			Name:           s.fileID,
			Format:         "xliff2",
			Locale:         model.LocaleID(s.srcLang),
			IsMultilingual: true,
			Properties: map[string]string{
				"target-language": s.trgLang,
			},
		}
		if s.version != "" {
			layer.Properties["xliff-version"] = s.version
		}
		for i, a := range s.extraAttrs {
			layer.Properties[extraAttrPropKey(i)] = encodeXMLAttr(a)
		}
		s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartLayerStart, Resource: layer})
	case "group":
		gs := &model.GroupStart{ID: attrValueXML(t, "id"), Name: attrValueXML(t, "name")}
		s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartGroupStart, Resource: gs})
		groupTranslate := s.parentTranslate()
		if v := attrValueXML(t, "translate"); v != "" {
			groupTranslate = !strings.EqualFold(v, "no")
		}
		s.translateStack = append(s.translateStack, groupTranslate)
	case "unit":
		s.inUnit = true
		s.unitID = ""
		s.unitName = ""
		s.unitTranslate = ""
		s.sourceSegs = nil
		s.targets = make(map[model.LocaleID][]seg)
		s.notes = nil
		s.states = nil
		for _, a := range t.Attr {
			switch a.Name.Local {
			case "id":
				s.unitID = a.Value
			case "name":
				s.unitName = a.Value
			case "translate":
				s.unitTranslate = a.Value
			}
		}
	case "notes":
		if s.inUnit {
			s.inNotes = true
		}
	case "note":
		if s.inNotes {
			s.inNote = true
			s.noteBuilder.Reset()
		}
	case "segment":
		if s.inUnit {
			s.inSegment = true
			s.segID = ""
			segState := ""
			for _, a := range t.Attr {
				switch a.Name.Local {
				case "id":
					s.segID = a.Value
				case "state":
					segState = a.Value
				}
			}
			if segState != "" {
				s.states = append(s.states, segState)
			}
		}
	case "source":
		if s.inSegment {
			s.inSource = true
			s.sourceDepth = 0
			s.sourceInnerXML.Reset()
			s.elemStartOff = s.decoder.InputOffset()
		}
	case "target":
		if s.inSegment {
			s.inTarget = true
			s.targetDepth = 0
			s.targetInnerXML.Reset()
			s.elemStartOff = s.decoder.InputOffset()
		}
	default:
		s.writeNestedStartTag(t)
	}
}

func (s *xliff2StreamState) writeNestedStartTag(t xml.StartElement) {
	if s.inSource {
		s.sourceDepth++
		s.sourceInnerXML.WriteString("<")
		s.sourceInnerXML.WriteString(t.Name.Local)
		for _, a := range t.Attr {
			s.sourceInnerXML.WriteString(" ")
			if a.Name.Space != "" {
				s.sourceInnerXML.WriteString(a.Name.Space)
				s.sourceInnerXML.WriteString(":")
			}
			s.sourceInnerXML.WriteString(a.Name.Local)
			s.sourceInnerXML.WriteString(`="`)
			s.sourceInnerXML.WriteString(xmlEscapeAttr(a.Value))
			s.sourceInnerXML.WriteString(`"`)
		}
		s.sourceInnerXML.WriteString(">")
	} else if s.inTarget {
		s.targetDepth++
		s.targetInnerXML.WriteString("<")
		s.targetInnerXML.WriteString(t.Name.Local)
		for _, a := range t.Attr {
			s.targetInnerXML.WriteString(" ")
			if a.Name.Space != "" {
				s.targetInnerXML.WriteString(a.Name.Space)
				s.targetInnerXML.WriteString(":")
			}
			s.targetInnerXML.WriteString(a.Name.Local)
			s.targetInnerXML.WriteString(`="`)
			s.targetInnerXML.WriteString(xmlEscapeAttr(a.Value))
			s.targetInnerXML.WriteString(`"`)
		}
		s.targetInnerXML.WriteString(">")
	}
}

func (s *xliff2StreamState) handleEndElement(t xml.EndElement) {
	switch t.Name.Local {
	case "file":
		if s.inFile {
			layer := &model.Layer{ID: "file-" + s.fileID, Name: s.fileID}
			s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
			s.inFile = false
		}
		if n := len(s.translateStack); n > 0 {
			s.translateStack = s.translateStack[:n-1]
		}
	case "group":
		s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{}})
		if n := len(s.translateStack); n > 0 {
			s.translateStack = s.translateStack[:n-1]
		}
	case "unit":
		if s.inUnit {
			s.emitUnit()
		}
	case "notes":
		s.inNotes = false
	case "note":
		if s.inNote {
			s.notes = append(s.notes, s.noteBuilder.String())
			s.inNote = false
		}
	case "segment":
		s.inSegment = false
	case "source":
		if s.inSource {
			s.finishSource()
		}
	case "target":
		if s.inTarget {
			s.finishTarget()
		}
	default:
		if s.inSource && s.sourceDepth > 0 {
			s.sourceDepth--
			s.sourceInnerXML.WriteString("</")
			s.sourceInnerXML.WriteString(t.Name.Local)
			s.sourceInnerXML.WriteString(">")
		} else if s.inTarget && s.targetDepth > 0 {
			s.targetDepth--
			s.targetInnerXML.WriteString("</")
			s.targetInnerXML.WriteString(t.Name.Local)
			s.targetInnerXML.WriteString(">")
		}
	}
}

func (s *xliff2StreamState) emitUnit() {
	// Inherit translate from the enclosing <file>/<group> stack;
	// the unit's own attribute (if present) overrides.
	translatable := s.parentTranslate()
	if s.unitTranslate != "" {
		translatable = !strings.EqualFold(s.unitTranslate, "no")
	}
	block := &model.Block{
		ID:           s.unitID,
		Name:         s.unitName,
		Translatable: translatable,
		Properties:   make(map[string]string),
		Targets:      make(map[model.VariantKey]*model.Target),
	}
	for _, st := range s.states {
		if st != "" {
			block.Properties["state"] = st
		}
	}
	for i, note := range s.notes {
		block.Properties[fmt.Sprintf("note-%d", i)] = note
	}
	// The streaming skeleton path tracks at most one target locale.
	trgLang := model.LocaleID(s.trgLang)
	var tgtSegs []seg
	if segs, ok := s.targets[trgLang]; ok {
		tgtSegs = segs
	}
	applySegmentsToBlock(block, s.sourceSegs, tgtSegs, trgLang)
	s.reader.emit(s.ctx, s.ch, &model.Part{Type: model.PartBlock, Resource: block})
	s.blockCount++
	s.inUnit = false
}

func (s *xliff2StreamState) finishSource() {
	endOff := s.decoder.InputOffset()
	closeTag := "</source>"
	endPos := int(endOff) - len(closeTag)
	if endPos < 0 {
		endPos = 0
	}
	s.elemPositions = append(s.elemPositions, elemPos{
		startOffset: int(s.elemStartOff),
		endOffset:   endPos,
		blockIdx:    s.blockCount,
		elemType:    "source",
	})
	sid := s.segID
	if sid == "" {
		sid = fmt.Sprintf("s%d", len(s.sourceSegs)+1)
	}
	sourceText := strings.TrimSpace(s.sourceInnerXML.String())
	s.sourceSegs = append(s.sourceSegs, seg{
		ID:   sid,
		Runs: []model.Run{{Text: &model.TextRun{Text: sourceText}}},
	})
	s.inSource = false
}

func (s *xliff2StreamState) finishTarget() {
	endOff := s.decoder.InputOffset()
	closeTag := "</target>"
	endPos := int(endOff) - len(closeTag)
	if endPos < 0 {
		endPos = 0
	}
	s.elemPositions = append(s.elemPositions, elemPos{
		startOffset: int(s.elemStartOff),
		endOffset:   endPos,
		blockIdx:    s.blockCount,
		elemType:    "target",
	})
	targetText := strings.TrimSpace(s.targetInnerXML.String())
	tl := model.LocaleID(s.trgLang)
	if targetText != "" && !tl.IsEmpty() {
		sid := s.segID
		if sid == "" {
			sid = fmt.Sprintf("s%d", len(s.sourceSegs))
		}
		s.targets[tl] = append(s.targets[tl], seg{
			ID:   sid,
			Runs: []model.Run{{Text: &model.TextRun{Text: targetText}}},
		})
	}
	s.inTarget = false
}

func (s *xliff2StreamState) buildSkeleton() {
	if len(s.elemPositions) == 0 {
		return
	}
	skelPos := 0
	for _, ep := range s.elemPositions {
		if ep.startOffset > skelPos {
			s.reader.skelText(s.rawText[skelPos:ep.startOffset])
		}
		refID := fmt.Sprintf("%d:%s", ep.blockIdx, ep.elemType)
		s.reader.skelRef(refID)
		skelPos = ep.endOffset
	}
	if skelPos < len(s.rawText) {
		s.reader.skelText(s.rawText[skelPos:])
	}
	s.reader.skelFlush()
}

// Reader-side skeleton helpers (used only in streaming mode).
var _ = bytes.Equal

func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		_ = r.skeletonStore.WriteText([]byte(s))
	}
}

func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		_ = r.skeletonStore.WriteRef(id)
	}
}

func (r *Reader) skelFlush() {
	// no-op: we no longer buffer (each text write goes straight through)
}

// isXliffExtraAttrLegacy is the legacy streaming-path equivalent of
// isXliffCoreAttr (negated). Kept distinct because the streaming path
// uses encoding/xml types whereas the DOM path uses etree types.
func isXliffExtraAttrLegacy(a xml.Attr) bool {
	if a.Name.Space == "" {
		switch a.Name.Local {
		case "version", "srcLang", "trgLang", "xmlns":
			return false
		}
	}
	return true
}

func attrValueXML(t xml.StartElement, local string) string {
	for _, a := range t.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func encodeXMLAttr(a xml.Attr) string {
	return a.Name.Space + "|" + a.Name.Local + "=" + a.Value
}
