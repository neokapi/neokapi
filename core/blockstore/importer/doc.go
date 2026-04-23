// Package importer lands external translations (from TMX, TBX, XLIFF,
// PO, KLF files, or HTTP webhook payloads) as target overlays on
// existing blocks in a blockstore.Store — without going through the
// full kapi-extract pipeline.
//
// Two entry points:
//
//   - ImportFromFormat wraps any `format.DataFormatReader` (every
//     built-in format — TMX, TBX, XLIFF, PO, KLF, JSON, markdown, …
//     works out of the box). Each incoming block's source text is
//     hashed and matched against existing source blocks in the
//     target store; for every matched block + locale pair, a
//     `targets/<locale>` overlay is upserted.
//
//   - ImportDirect takes an iterator of ImportPair records (already
//     bound to a block hash by the caller — no source-hash matching
//     needed). This is the shape a webhook handler would use: parse
//     the payload, yield {block_hash, locale, text} pairs, and let
//     the importer persist them.
//
// Conflict policy (Options.OnConflict):
//
//   - SkipExisting — write only if the (block, locale) has no overlay
//   - ReplaceExisting (default) — always upsert
//
// The importer never creates new blocks: if an incoming source isn't
// already in the target store, the pair is reported as Unmatched and
// skipped. Adding new source blocks is the extract pipeline's job;
// this is strictly an overlay-import path.
package importer
