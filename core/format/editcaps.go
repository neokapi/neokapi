package format

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/model"
)

// Edit capabilities. A same-format writer replays its skeleton and writes the
// text of the blocks an edit changed. Some edits change more than text: the
// target of a link, or a new bold span over words that had none. A writer that
// can put such a change into the document's bytes declares it, and the change
// service applies those operations only where a format declares them; every
// other request is refused as unsupported (docs/internals/edit-model.md,
// sections 2.4 and 2.7).
//
// Each declaration is probed once when a built-in writer is registered
// (registry.FormatInfo) and read from the cached manifest for a plugin format.
// The operations matrix (core/formats/opsmatrix_capabilities_test.go) holds a
// passing cell for every declaration of every built-in format, and a
// declaration without one fails the build.

// AnyCodeType, as a key of AttrWriter.WritableAttrs, names every code type the
// format reads; AnyAttr, in its list, names every attribute a code's markup
// spells.
const (
	AnyCodeType = "*"
	AnyAttr     = "*"
)

// AttrWriter is implemented by a writer that can write a changed attribute of
// an inline code, such as the href of a link.
type AttrWriter interface {
	// WritableAttrs names, per code type, the attributes whose value the
	// writer can change. A code type is the vocabulary type a reader gives
	// the code (link:hyperlink, media:image); AnyCodeType and AnyAttr widen
	// the declaration.
	WritableAttrs() map[string][]string
	// WriteAttr returns seq with attribute name of the code at seq[at] set to
	// value: the code's attributes record the value, and its native data spell
	// it, escaped for the place it is written. seq[at] is the code's opening
	// half or its placeholder; the writer may respell other runs of seq that
	// hold part of the code's markup, such as the closing half, and returns a
	// sequence of the same runs in the same order, so its length is seq's.
	// WriteAttr copies what it changes and leaves seq as it was. It returns an
	// error whose message gives the reason when this code cannot carry the
	// change, such as a link whose target is defined elsewhere in the
	// document.
	WriteAttr(seq []model.Run, at int, name, value string) ([]model.Run, error)
}

// CodeSite describes a new paired code to a CodeSynthesizer: its vocabulary
// type and attributes, the block it goes into, and the runs around the place
// it opens and closes. Everything it holds is read-only.
type CodeSite struct {
	// Type is the vocabulary type of the code, such as fmt:bold.
	Type string
	// Attrs are the attributes the new code carries, such as a link's href.
	Attrs map[string]string
	// Block is the block whose edition the code goes into.
	Block *model.Block
	// Enclosing are the opening halves of the paired codes the new code sits
	// inside, outermost first.
	Enclosing []model.Run
	// Before are the runs of the sequence before the new code opens, Inner
	// the runs between its halves, and After the runs after it closes.
	Before, Inner, After []model.Run
}

// CodeSynthesizer is implemented by a writer that can write a new paired code
// of a vocabulary type around text: the code a mark operation adds, or a new
// code a runs payload names by type and attributes.
type CodeSynthesizer interface {
	// Synthesizes lists the vocabulary types the writer can write as a new
	// paired code.
	Synthesizes() []string
	// SynthesizeCode returns the two halves of a new paired code at site, as
	// the format's reader would read them: type, subtype, native data spelled
	// and escaped for the format, attributes and the vocabulary's display and
	// constraints. The caller gives them their id. It returns an error whose
	// message gives the reason when the format cannot write the code there,
	// such as a link inside a link.
	SynthesizeCode(site CodeSite) (open, close model.Run, err error)
}

// StructuralWriter is implemented by a writer that can add a block to a
// document or remove one with its shell. Structural lists the operations it
// supports, "insert_block" and "delete_block".
type StructuralWriter interface {
	Structural() []string
}

// NativeOp is a format-specific operation: its name and the JSON Schema of its
// arguments.
type NativeOp struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema,omitempty"`
}

// NativeEditor is implemented by a writer that offers format-specific
// operations.
type NativeEditor interface {
	NativeOps() []NativeOp
}

// EditCapabilities is what a writer declares it can write beyond replaying
// what its reader read. The zero value declares nothing.
type EditCapabilities struct {
	// WritableAttrs maps a code type to the attributes set_attribute may
	// change (AttrWriter).
	WritableAttrs map[string][]string `json:"writable_attrs,omitempty"`
	// Synthesizes lists the vocabulary types mark, and a new code in a runs
	// payload, may create (CodeSynthesizer).
	Synthesizes []string `json:"synthesizes,omitempty"`
	// Structural lists the structural operations the writer supports
	// (StructuralWriter).
	Structural []string `json:"structural,omitempty"`
	// NativeOps lists the format-specific operations (NativeEditor).
	NativeOps []NativeOp `json:"native_ops,omitempty"`
}

// IsZero reports whether the capabilities declare nothing.
func (c EditCapabilities) IsZero() bool {
	return len(c.WritableAttrs) == 0 && len(c.Synthesizes) == 0 && len(c.Structural) == 0 && len(c.NativeOps) == 0
}

// Writable returns the attributes of a code of type typ that set_attribute may
// change, the AnyCodeType declaration included, sorted. AnyAttr in the result
// means every attribute the code's markup spells.
func (c EditCapabilities) Writable(typ string) []string {
	var out []string
	for _, key := range []string{typ, AnyCodeType} {
		for _, name := range c.WritableAttrs[key] {
			if !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
	}
	slices.Sort(out)
	return out
}

// CanWrite reports whether set_attribute may change attribute name of a code
// of type typ.
func (c EditCapabilities) CanWrite(typ, name string) bool {
	w := c.Writable(typ)
	return slices.Contains(w, name) || slices.Contains(w, AnyAttr)
}

// CanSynthesize reports whether the writer can write a new paired code of
// type typ.
func (c EditCapabilities) CanSynthesize(typ string) bool {
	return slices.Contains(c.Synthesizes, typ)
}

// Clone returns a copy that shares nothing with c, with its lists sorted.
func (c EditCapabilities) Clone() EditCapabilities {
	out := EditCapabilities{
		Synthesizes: sortedCopy(c.Synthesizes),
		Structural:  sortedCopy(c.Structural),
		NativeOps:   slices.Clone(c.NativeOps),
	}
	if len(c.WritableAttrs) > 0 {
		out.WritableAttrs = make(map[string][]string, len(c.WritableAttrs))
		for _, k := range slices.Sorted(maps.Keys(c.WritableAttrs)) {
			out.WritableAttrs[k] = sortedCopy(c.WritableAttrs[k])
		}
	}
	slices.SortFunc(out.NativeOps, func(a, b NativeOp) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		}
		return 0
	})
	return out
}

func sortedCopy(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	out := slices.Clone(s)
	slices.Sort(out)
	return slices.Compact(out)
}

// ProbeEditCapabilities returns what w declares through AttrWriter,
// CodeSynthesizer, StructuralWriter and NativeEditor. The registry calls it
// once per built-in writer.
func ProbeEditCapabilities(w DataFormatWriter) EditCapabilities {
	var c EditCapabilities
	if aw, ok := w.(AttrWriter); ok {
		c.WritableAttrs = aw.WritableAttrs()
	}
	if cs, ok := w.(CodeSynthesizer); ok {
		c.Synthesizes = cs.Synthesizes()
	}
	if sw, ok := w.(StructuralWriter); ok {
		c.Structural = sw.Structural()
	}
	if ne, ok := w.(NativeEditor); ok {
		c.NativeOps = ne.NativeOps()
	}
	return c.Clone()
}
