// Package blockstore adapts Bowrain's server-side ContentStore to the
// core/blockstore.Store interface so flows, the automation engine, and
// server handlers can read/write project content through one seam.
//
// This is the in-process adapter described in #385. It keeps the CLI
// ↔ server wire path (AD-009 chunked Merkle sync) and the in-process
// egress (this package) on one Store API.
//
// Layout:
//
//   - Blocks are read from and written to the existing `blocks` table
//     via the ContentStore's StoreBlocks / GetBlocks methods.
//   - Overlays dispatch by kind prefix (#403):
//   - `targets/<locale>`   → `translations` (hot-path indexes on
//     project+locale+updated_at)
//   - `annotations/<name>` → `annotations`   (project+kind+updated_at)
//   - anything else        → `overlays_ext`  (plugin catchall)
//   - The `blockstore.Store` interface stays polymorphic — callers
//     only say `PutOverlay(kind, …)`; the dispatcher picks the table.
//
// A `targets/<locale>` overlay is a model.Target and an `annotations/<name>`
// overlay is a block annotation — neither is an opaque body. Both go through
// bowrain/store's own writer and reader, so an overlay reaching the store
// through this adapter and one reaching it through the block round-trip are the
// same row: the same columns and payload envelope, filed under the same
// `blocks.id`, under the same spelling of the kind. The two used to disagree on
// all three, and the shape of that was silence — the write succeeded, every
// reader that derives coverage, review state, a decision or a block's
// annotations saw nothing, and "no translation yet" is what nothing means.
//
// Every overlay row is filed under the block's own `blocks.id`, whichever name
// the caller addressed the block by. Block and item deletion clear overlays by
// that id; a row filed under anything else survives the block it describes,
// forever.
package blockstore
