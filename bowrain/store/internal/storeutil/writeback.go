package storeutil

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// WriteBack is one write of blocks back to the rows they were read from. It
// holds, per block id, the content hash the caller read, and records the blocks
// that may not land.
type WriteBack struct {
	base    map[string]string
	skipped []string
}

// NewWriteBack prepares a write-back of reads and returns the blocks to write,
// one per id. A later read of the same id replaces an earlier one.
func NewWriteBack(reads []*venue.StoredBlock) (*WriteBack, []*model.Block) {
	wb := &WriteBack{base: make(map[string]string, len(reads))}
	order := make([]string, 0, len(reads))
	byID := make(map[string]*model.Block, len(reads))
	for _, r := range reads {
		if r == nil || r.Block == nil || r.Block.ID == "" {
			continue
		}
		id := r.Block.ID
		if _, seen := byID[id]; !seen {
			order = append(order, id)
		}
		byID[id] = r.Block
		wb.base[id] = r.ContentHash
	}
	blocks := make([]*model.Block, 0, len(order))
	for _, id := range order {
		blocks = append(blocks, byID[id])
	}
	return wb, blocks
}

// Base is the content hash the caller read for a block.
func (w *WriteBack) Base(id string) string {
	return w.base[id]
}

// Admits reports whether a block may be written: its row exists and still holds
// the content hash the caller read. A block that may not is recorded as skipped.
func (w *WriteBack) Admits(id string, exists bool, storedHash string) bool {
	if exists && storedHash == w.base[id] {
		return true
	}
	w.Skip(id)
	return false
}

// Skip records a block that did not land.
func (w *WriteBack) Skip(id string) {
	w.skipped = append(w.skipped, id)
}

// Skipped lists the blocks that did not land, in the order they were met.
func (w *WriteBack) Skipped() []string {
	return w.skipped
}
