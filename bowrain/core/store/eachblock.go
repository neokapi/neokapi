package store

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/venue"
)

// Reading a project's blocks without holding them all.
//
// A block query scoped to a project and nothing else answers with the whole
// corpus, and BlockQuery.Limit of zero means no limit at all — so the reader's
// peak memory is a function of how much content the customer has, with no
// ceiling and no backpressure. That is not a hypothetical: it took the
// production server down on a project of roughly a thousand items. The server
// idles at ~16 MiB; one such read drove it past 934 MiB against a 1 GiB task
// and the container was OOM-killed. The same corpus had just travelled through
// the worker's chunked ingest at ~108 MiB, which is the shape to copy.
//
// EachBlockBatch is that shape. It walks in id order through the keyset cursor
// (BlockQuery.AfterID), so peak memory is the batch rather than the project,
// and the cost of the thousandth batch equals the cost of the first.

// DefaultBlockBatch is how many blocks a walk holds at once.
//
// Large enough that a walk over a big project is a modest number of queries,
// small enough that a batch of fully hydrated blocks — source runs, properties,
// overlays — is megabytes rather than hundreds of them.
const DefaultBlockBatch = 500

// EachBlockBatch calls fn with each batch of blocks the query matches, in id
// order, holding only one batch at a time.
//
// The query's own Limit, Offset and AfterID are the walk's to set: a caller
// passes the SCOPE (project, stream, and any filters) and lets this own the
// paging. A batch of zero uses DefaultBlockBatch.
//
// fn must not retain the slice it is handed beyond its own call — the point of
// the walk is that a batch becomes garbage once it has been dealt with.
//
// The walk assumes rows are not being inserted below the cursor while it runs.
// Settling and stamping update rows in place, which the cursor is immune to;
// a concurrent push that adds blocks may have some of them land after the walk
// has passed their id, and they are picked up by the next run rather than by
// this one.
func EachBlockBatch(
	ctx context.Context,
	cs ContentStore,
	scope BlockQuery,
	batch int,
	fn func(blocks []*venue.StoredBlock) error,
) error {
	if cs == nil {
		return nil
	}
	if batch <= 0 {
		batch = DefaultBlockBatch
	}

	q := scope
	q.Offset = 0
	q.Limit = batch

	for {
		page, err := cs.GetBlocks(ctx, q)
		if err != nil {
			return fmt.Errorf("read blocks after %q: %w", q.AfterID, err)
		}
		if len(page) == 0 {
			return nil
		}
		if err := fn(page); err != nil {
			return err
		}
		// A short page is the last one: asking again would return nothing.
		if len(page) < batch {
			return nil
		}

		last := page[len(page)-1]
		next := blockCursorID(last)
		if next == "" || next == q.AfterID {
			// Without a cursor to advance past, asking again returns the same
			// page forever. Stopping is the only safe answer, and it is loud:
			// a store that cannot name its rows cannot be walked.
			return fmt.Errorf("block walk stalled after %q: batch ends with a block that has no id", q.AfterID)
		}
		q.AfterID = next
	}
}

// blockCursorID is the id the walk resumes after — the stored row's own id,
// which is what the query orders by.
func blockCursorID(sb *venue.StoredBlock) string {
	if sb == nil {
		return ""
	}
	if sb.Block != nil && sb.Block.ID != "" {
		return sb.Block.ID
	}
	return ""
}
