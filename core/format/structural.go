package format

import (
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/model"
)

// Structural edits. A same-format writer replays its skeleton, which has a slot
// for every block the reader read and no slot for a block nobody read. Adding
// a block to a document, or removing one together with the markup around it,
// is therefore a change to the document's structure that only the format can
// make: in a key-value catalog the shell of a block is its key and the value
// beside it. A writer that can make such a change declares it, and the change
// service applies insert_block and delete_block only where a format declares
// them (docs/internals/edit-model.md, sections 2.2 and 7.1).

// The structural operations a writer can declare.
const (
	StructuralInsertBlock = "insert_block"
	StructuralDeleteBlock = "delete_block"
)

// StructureEditor is implemented by a writer that can add a block to a
// document or remove one with its shell. Structural lists the operations it
// writes under its configuration, StructuralInsertBlock and
// StructuralDeleteBlock. EditStructure returns doc, a whole document in the
// writer's format, with each edit made in order: an edit sees the document the
// edits before it left. Every byte outside the shells it adds or removes stays
// as it was. It returns a *StructureError naming the edit it could not make,
// and then nothing of the result is used.
type StructureEditor interface {
	Structural() []string
	EditStructure(doc []byte, edits []StructuralEdit) ([]byte, error)
}

// StructuralEdit is one block a structural edit adds to a document or removes
// from it. Blocks are named by the key the format's reader gives them: a JSON
// key path, a YAML key path, an ARB message id.
type StructuralEdit struct {
	// Op is StructuralInsertBlock or StructuralDeleteBlock.
	Op string
	// Key names the block removed, or the new block's key.
	Key string
	// Block is the block removed, as the format's reader read it from this
	// document, so the writer can find it by what the reader recorded. Nil
	// leaves the writer to find the block by Key.
	Block *model.Block
	// Anchor names the block the new one goes after, or before it when Before
	// is set; empty puts the new block last in the document. AnchorBlock is
	// that block as the reader read it, nil when an earlier edit added it.
	Anchor      string
	AnchorBlock *model.Block
	Before      bool
	// Value is the new block's content: the text the format writes as its
	// value, which its reader reads back as the block's content.
	Value string
	// Runs is the new block's content as runs, Value's source. A writer that
	// spells a value from its runs (ValueSpeller) writes the value from them,
	// so a plural or select in them is written as the format writes one.
	Runs []model.Run
}

// ValueSpeller is implemented by a writer whose reader reads syntax inside a
// value as structure, as an ARB message's plurals and selects are read as
// plural and select runs, so model.RenderRunsWithData, which writes one branch
// of a structure, does not give back the value. SpellValue returns the value
// the writer writes for runs: b is the block the runs are an edition of, as
// the format's reader read it, or nil for content no reader has read.
type ValueSpeller interface {
	SpellValue(b *model.Block, runs []model.Run) string
}

// SpellValue returns the value w writes for runs: its own spelling where it
// spells values (ValueSpeller), else model.RenderRunsWithData.
func SpellValue(w any, b *model.Block, runs []model.Run) string {
	if s, ok := w.(ValueSpeller); ok {
		return s.SpellValue(b, runs)
	}
	return model.RenderRunsWithData(runs)
}

// StructureReason says why a StructureEditor could not make an edit.
type StructureReason string

const (
	// StructureExists: the document already has an entry under the new
	// block's key, although no block of it may answer to the key (a number,
	// or a nested object, sits there).
	StructureExists StructureReason = "exists"
	// StructureNotFound: the document holds no entry the edit names.
	StructureNotFound StructureReason = "not_found"
	// StructureUnsupported: the format has no way to make the edit there, such
	// as beside a value in an array or a flow collection.
	StructureUnsupported StructureReason = "unsupported"
)

// StructureError is the edit a StructureEditor could not make, and why.
type StructureError struct {
	// Edit is the index of the edit in the list EditStructure was given.
	Edit    int
	Reason  StructureReason
	Message string
}

func (e *StructureError) Error() string { return e.Message }

// StructureErrorf returns a *StructureError for edit i.
func StructureErrorf(i int, reason StructureReason, msg string, args ...any) *StructureError {
	return &StructureError{Edit: i, Reason: reason, Message: fmt.Sprintf(msg, args...)}
}

// StructuralOps returns the structural operations w declares and can write:
// those it lists, when it also implements StructureEditor. A writer that
// declares an operation without the write half declares nothing.
func StructuralOps(w DataFormatWriter) []string {
	se, ok := w.(StructureEditor)
	if !ok {
		return nil
	}
	var out []string
	for _, op := range se.Structural() {
		if (op == StructuralInsertBlock || op == StructuralDeleteBlock) && !slices.Contains(out, op) {
			out = append(out, op)
		}
	}
	slices.Sort(out)
	return out
}
