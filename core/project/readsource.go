package project

import (
	"context"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// ReadSourceBlocks opens path with the given format reader, applies per-item
// format config, and drains the reader into its blocks (translatable and not)
// plus the first layer it emits. It is the single source-reading path shared by
// the CLI (merge, extract) and kapi-desktop (extract → block store) so both
// read and number blocks identically — block numbering depends on the same
// reader + config, so any divergence would desync merge and coverage.
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
	if len(cfg) > 0 {
		if c := reader.Config(); c != nil {
			if err := c.ApplyMap(cfg); err != nil {
				return nil, nil, fmt.Errorf("apply format config: %w", err)
			}
		}
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
	if err := reader.Open(ctx, doc); err != nil {
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

// BlockStoreHash returns a stable, globally-unique key for a block in a
// project-wide block store — the source file's block id namespaced by its
// project-relative path so blocks (and their target overlays) from different
// files can't collide. It delegates to blockstore.StoreKey, the single
// definition shared by the extract, run, and merge paths.
func BlockStoreHash(sourceRel, blockID, sourceText string) string {
	return blockstore.StoreKey(sourceRel, blockID, sourceText)
}
