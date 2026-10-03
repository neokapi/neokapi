package store

import (
	"context"

	"github.com/neokapi/neokapi/core/venue"
)

// BlockWriteStore is the optional capability the change service's stream home
// writes through: a read of one item's blocks by key, and a write that holds
// the rows it read from that read until it commits, so a change decided on the
// rows is the change stored on them.
//
// Assert for it rather than for a concrete store type: the event-emitting
// decorator forwards it.
type BlockWriteStore interface {
	// ItemBlocks reads the blocks of one item in document order. keys names
	// the blocks wanted by their durable key (source id), their name or their
	// row id; empty reads every block of the item. It holds nothing.
	ItemBlocks(ctx context.Context, projectID, stream, itemName string, keys []string) ([]*venue.StoredBlock, error)
	// BeginBlockWrite starts a write to one stream. It takes the stream's
	// shared write lock, so a push applying to the stream commits first or
	// waits for the write to.
	BeginBlockWrite(ctx context.Context, projectID, stream string) (BlockWrite, error)
}

// BlockWrite is a write to some blocks of one stream, open until Commit or
// Rollback.
type BlockWrite interface {
	// Hold reads the blocks of one item that keys name, as ItemBlocks does,
	// and holds each row until the write ends: another write to them waits.
	Hold(ctx context.Context, itemName string, keys []string) ([]*venue.StoredBlock, error)
	// Store writes blocks Hold read back to their rows, with the history and
	// change-log rows of what changed, on the write's transaction. A
	// translation or a block annotation a held row carried and its block no
	// longer does is removed. Each block carries the content hash Hold read as
	// ContentHash. It creates no row.
	Store(ctx context.Context, blocks []*venue.StoredBlock) error
	// Commit makes the write visible and releases the rows.
	Commit() error
	// Rollback discards the write and releases the rows. It is safe after
	// Commit and more than once.
	Rollback() error
}
