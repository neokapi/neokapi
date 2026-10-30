package project

import (
	"context"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// ReadSourceBlocks opens path with the given format reader, applies per-item
// format config, and drains the reader into its blocks (translatable and not)
// plus the first layer it emits. It is the single source-reading path shared by
// the CLI (merge, extract, the block cache `kapi up` fills) and kapi-desktop
// (the block cache it fills on open), so both read and number blocks
// identically. The reader gets the skeleton store a write would give it
// (WireReadSkeleton), so the blocks are the ones a check reports and an edit
// names.
func ReadSourceBlocks(
	ctx context.Context,
	reg *registry.FormatRegistry,
	formatName, path string,
	src, tgt model.LocaleID,
	cfg map[string]any,
) ([]*model.Block, *model.Layer, error) {
	reader, err := reg.NewReader(registry.FormatID(formatName))
	if err != nil {
		return nil, nil, err
	}
	// Per-item format config (e.g. translateFrontMatter on a docs item) must
	// apply on every re-read exactly as it did at extract time.
	if err := ApplyReaderConfig(reader, cfg); err != nil {
		return nil, nil, fmt.Errorf("apply format config: %w", err)
	}
	// The reader closes before the skeleton store it writes into.
	if store := WireReadSkeleton(reg, formatName, reader); store != nil {
		defer func() { _ = store.Close() }()
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: src,
		TargetLocale: tgt,
		FormatID:     formatName,
		Reader:       f,
	}
	if err := format.OpenDocument(ctx, reader, doc); err != nil {
		return nil, nil, err
	}
	defer reader.Close()

	var blocks []*model.Block
	var layer *model.Layer
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return nil, nil, res.Error
		}
		switch res.Part.Type {
		case model.PartBlock:
			if b, ok := res.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		case model.PartLayerStart:
			if l, ok := res.Part.Resource.(*model.Layer); ok && layer == nil {
				layer = l
			}
		}
	}
	return blocks, layer, nil
}

// WireReadSkeleton gives reader the skeleton store a write of the document
// would give it, one that keeps nothing (format.NewWiredReadSkeleton), and
// returns it, or nil when the format has no skeleton path. A reader can read a
// document differently with a skeleton wired: the HTML reader keeps character
// references as codes, and the MDX reader reads a JSX element's block-level
// children as markdown. The change service reads every document that way, so
// every read that reports blocks wires one too.
func WireReadSkeleton(reg *registry.FormatRegistry, formatName string, reader format.DataFormatReader) *format.SkeletonStore {
	if _, ok := reader.(format.SkeletonStoreEmitter); !ok || !reg.HasWriter(registry.FormatID(formatName)) {
		return nil
	}
	writer, err := reg.NewWriter(registry.FormatID(formatName))
	if err != nil {
		return nil
	}
	return format.NewWiredReadSkeleton(reader, writer)
}

// BlockStoreHash returns a stable, globally-unique key for a block in a
// project-wide block store — the source file's block id namespaced by its
// project-relative path so blocks (and their target overlays) from different
// files can't collide. It delegates to blockstore.StoreKey, the single
// definition shared by the extract, run, and merge paths.
func BlockStoreHash(sourceRel, blockID, sourceText string) string {
	return blockstore.StoreKey(sourceRel, blockID, sourceText)
}
