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

	// reader is the reader the file was parsed with, kept for PreviewHTML.
	reader format.DataFormatReader
}

// PreviewHTML renders the editor preview of the parsed file (BuildPreview).
// Each block is marked by its durable key as it stands at the call, so a
// caller that stores the blocks calls it after the store resolves their keys
// (the bowrain store's ReplaceItemBlocks), and a producer after it resolves
// them against the venue (host.ResolveIdentity).
func (r *ParseResult) PreviewHTML() string {
	return BuildPreview(r.Parts, r.reader)
}

// ParseItem parses a file using the given reader and builds the editor
// metadata (BlockIndex, block list). The preview is rendered on request
// (ParseResult.PreviewHTML). This is the shared entry point used by both the
// server upload flow and the CLI push flow.
//
// The reader must not have been opened yet — ParseItem calls Open and Close.
func ParseItem(ctx context.Context, reader format.DataFormatReader, doc *model.RawDocument,
	sourceLocale, formatName, itemName string) (*ParseResult, error) {

	if err := format.OpenDocument(ctx, reader, doc); err != nil {
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

	return &ParseResult{
		Parts:          parts,
		Blocks:         blocks,
		BlockIndex:     blockIndex,
		BlockIndexJSON: string(blockIndexJSON),
		reader:         reader,
	}, nil
}
