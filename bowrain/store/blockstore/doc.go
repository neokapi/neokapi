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
// A `targets/<locale>` overlay is a model.Target, not an opaque body, and it
// goes through bowrain/store's own writer so a target reaching the store
// through this adapter and one reaching it through the block round-trip are
// the same row: the same columns, filed under the same `blocks.id`. The two
// used to disagree on both, and the shape of that was silence — the write
// succeeded, every reader that derives coverage, review state or a decision
// from a target saw nothing, and "no translation yet" is what nothing means.
package blockstore
