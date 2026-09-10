package model

// BlockRef identifies a block in an inspected document. Snapshot-bound ranges
// retain both the reader's ID and its structural address, alongside the content
// hash used by ordinary block edits and check findings.
type BlockRef struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Address     string      `json:"address,omitempty"`
	ContentHash string      `json:"content_hash"`
	Source      *SourceSpan `json:"source,omitempty"`
}

// BlockRange describes a heading section in the reader's document order.
// Heading is preserved; Body names content blocks being replaced. Structural
// source between these boundaries, including non-block data, belongs to the
// section as well. EndBefore is absent at the end of the containing structure.
// The enclosing edit plan supplies the source snapshot that scopes these IDs.
type BlockRange struct {
	Heading   BlockRef   `json:"heading"`
	Body      []BlockRef `json:"body"`
	EndBefore *BlockRef  `json:"end_before,omitempty"`
}

// RefForBlock projects a reader block into an edit-range anchor.
func RefForBlock(block *Block) BlockRef {
	ref := BlockRef{
		ID: block.ID, Name: block.Name, Address: block.StructuralAddress(),
		ContentHash: ComputeContentHash(block.SourceText()),
	}
	if span, ok := block.SourceSpan(); ok {
		ref.Source = &span
	}
	return ref
}
