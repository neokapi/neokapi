package editor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// ParseResult holds the output of parsing a file for the editor.
type ParseResult struct {
	// Parts is the full Part stream from the reader.
	Parts []*model.Part

	// Blocks are the translatable blocks extracted from Parts.
	Blocks []*model.Block

	// BlockIndex is the structured index for the editor.
	BlockIndex *BlockIndex

	// BlockIndexJSON is the JSON-serialized BlockIndex.
	BlockIndexJSON string

	// PreviewHTML is the pre-rendered editor preview HTML.
	PreviewHTML string
}

// ParseItem parses a file using the given reader and builds all editor metadata
// (BlockIndex, PreviewHTML, block list). This is the shared entry point used by
// both the server upload flow and the CLI push flow.
//
// The reader must not have been opened yet — ParseItem calls Open and Close.
func ParseItem(ctx context.Context, reader format.DataFormatReader, doc *model.RawDocument,
	sourceLocale, formatName, itemName string) (*ParseResult, error) {

	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("parse %q: %w", itemName, err)
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			return nil, fmt.Errorf("read %q: %w", itemName, result.Error)
		}
		parts = append(parts, result.Part)
	}
	reader.Close()

	var blocks []*model.Block
	for _, pt := range parts {
		if pt.Type == model.PartBlock {
			if b, ok := pt.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}

	blockIndex := BuildBlockIndex(parts, sourceLocale, formatName, itemName)
	blockIndexJSON, err := json.Marshal(blockIndex)
	if err != nil {
		return nil, fmt.Errorf("marshal block index for %q: %w", itemName, err)
	}

	previewHTML := BuildPreview(parts, reader)

	return &ParseResult{
		Parts:          parts,
		Blocks:         blocks,
		BlockIndex:     blockIndex,
		BlockIndexJSON: string(blockIndexJSON),
		PreviewHTML:    previewHTML,
	}, nil
}
