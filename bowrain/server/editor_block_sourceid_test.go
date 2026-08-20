package server

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
)

// The blocks payload carries the id the document knows the block by.
//
// The store mints its own id on ingest and keeps the format reader's as
// SourceID. The rendered document — built by the format's own preview builder —
// marks its blocks with the reader's. A surface addressing a block inside that
// document therefore has to translate, and it can only do that if the payload
// says what the other name is.
//
// Without this the two id spaces never met: every message the document preview
// posted was addressed to an element that does not exist, and postMessage has
// no delivery receipt, so the document went on rendering its source while the
// surface believed it had been updated.
func TestStoredBlockToInfoResponse_CarriesTheDocumentsIDForTheBlock(t *testing.T) {
	sb := &venue.StoredBlock{
		Block:    &model.Block{ID: "0CERYaui", Translatable: true},
		ItemName: "ui/src/components/AutomationsPage.kbf.json",
		SourceID: "../ui/src/components/AutomationsPage.tsx:135:0",
	}

	got := storedBlockToInfoResponse(sb, nil)

	assert.Equal(t, "0CERYaui", got.ID, "the store's id stays the block's identity")
	assert.Equal(t, "../ui/src/components/AutomationsPage.tsx:135:0", got.SourceID,
		"and the document's name for it travels alongside")
}

// A block stored without an item has no document to disagree with, and the
// field is omitted rather than sent empty — a surface reading it falls back to
// the block's own id, which is what such a document is marked with.
func TestStoredBlockToInfoResponse_OmitsTheDocumentIDWhenThereIsNoDocument(t *testing.T) {
	sb := &venue.StoredBlock{Block: &model.Block{ID: "b1", Translatable: true}}

	assert.Empty(t, storedBlockToInfoResponse(sb, nil).SourceID)
}
