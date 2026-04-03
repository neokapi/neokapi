package backend

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
)

// RenderDocumentPreview returns pre-rendered HTML for an item.
// Uses stored PreviewHTML if available, falling back to generating
// a preview from the stored BlockIndex.
func (a *App) RenderDocumentPreview(projectID, itemName, targetLocale string) (string, error) {
	ctx := context.Background()

	item, err := a.store.GetItem(ctx, projectID, "main", itemName)
	if err != nil {
		return "", fmt.Errorf("item %q not found in project", itemName)
	}

	if item.PreviewHTML != "" {
		return item.PreviewHTML, nil
	}

	if item.BlockIndex != "" {
		return editor.BuildPreviewFromBlockIndex(item.BlockIndex), nil
	}

	return "", nil
}

// RenderBlockHTML returns the rendered HTML for a single block.
// If targetLocale is non-empty and a translation exists, it returns the
// target text; otherwise it returns the source HTML.
func (a *App) RenderBlockHTML(projectID, itemName, blockID, targetLocale string) (string, error) {
	ctx := context.Background()

	item, err := a.store.GetItem(ctx, projectID, "main", itemName)
	if err != nil {
		return "", fmt.Errorf("item %q not found in project", itemName)
	}

	// Always load the live block from the store for up-to-date targets.
	sb, err := a.store.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return "", fmt.Errorf("block %q not found in item %q", blockID, itemName)
	}

	// Return target translation if available
	if targetLocale != "" {
		if text := sb.Block.TargetText(model.LocaleID(targetLocale)); text != "" {
			return text, nil
		}
	}

	// For source rendering, try the block index for HTML-enriched source
	if item.BlockIndex != "" {
		var blockIndex editor.BlockIndex
		if err := json.Unmarshal([]byte(item.BlockIndex), &blockIndex); err == nil {
			b := blockIndex.BlockByID(blockID)
			if b != nil && b.SourceHTML != "" {
				return b.SourceHTML, nil
			}
		}
	}

	return sb.Block.SourceText(), nil
}
