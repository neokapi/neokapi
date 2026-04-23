// Package blockstore adapts Bowrain's server-side ContentStore to the
// core/blockstore.Store interface so flows, the automation engine, and
// server handlers can read/write project content through one seam.
//
// This is the in-process adapter described in #385. It keeps the CLI
// ↔ server wire path (AD-009 chunked Merkle sync) and the in-process
// egress (this package) on one Store API.
//
// Layout today:
//
//   - Blocks are read from and written to the existing `blocks` table
//     via the ContentStore's StoreBlocks / GetBlocks methods.
//   - Overlays are stored in a new `block_overlays` table keyed by
//     (project_id, stream, block_id, kind). The kind is opaque to the
//     store; conventions are "targets/<locale>", "annotations/<name>",
//     "skeletons/<format>" (see core/blockstore.Overlay).
//
// Layout tomorrow (#385 Phase 2):
//
// Overlay reads/writes will dispatch by kind to purpose-built tables
// (translations, annotations, automation_runs) with declarative
// project_id partitioning. The `block_overlays` table stays as the
// catchall for plugin-supplied kinds. The Go interface in this package
// is unchanged across that migration — callers only see blockstore.Store.
package blockstore
