package project

import (
	"context"
	"fmt"
	"os"

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
// project-wide block store (`.kapi/cache/blocks.db`).
//
// Format readers assign *file-local* IDs (e.g. "tu1", "tu2", …) that restart in
// every source file, and most don't populate a content hash — so keying the
// store on the raw ID (or on source text alone) lets blocks from different
// files/collections collide on the same key. Because the store upserts by key
// (last writer wins), an earlier collection's blocks get silently reassigned to
// a later one — the "Website shows 0 blocks" symptom. Namespacing the key by
// the source file's project-relative path keeps every block distinct while
// staying stable across re-extractions (so target overlays keyed off the same
// block survive a re-extract).
func BlockStoreHash(sourceRel, blockID, sourceText string) string {
	seed := sourceRel + "\x00" + blockID
	if blockID == "" {
		seed = sourceRel + "\x00" + sourceText
	}
	return model.ComputeContentHash(seed)
}
