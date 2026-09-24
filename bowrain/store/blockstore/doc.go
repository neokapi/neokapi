// Package blockstore adapts Bowrain's ContentStore to core/blockstore.Store for
// flows, automation and server handlers.
//
// Blocks use ContentStore.StoreBlocks and GetBlocks. Overlay kinds determine
// storage: targets/<locale> uses translations, annotations/<name> uses
// annotations, and other kinds use overlays_ext.
//
// Target and annotation overlays use the same writers, readers, payload shapes
// and kind names as block round-trips. Every overlay is stored under blocks.id,
// regardless of the identifier used to address the block, so block and item
// deletion can remove all associated overlays.
package blockstore
