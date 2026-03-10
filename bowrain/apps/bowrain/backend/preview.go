package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gokapi/gokapi/core/editor"
	"github.com/gokapi/gokapi/core/model"
)

// RenderDocumentPreview returns pre-rendered HTML for an item.
func (a *App) RenderDocumentPreview(projectID, itemName, targetLocale string) (string, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return "", err
	}

	item, err := a.store.GetItem(ctx, projectID, "main", itemName)
	if err != nil {
		return "", fmt.Errorf("item %q not found in project", itemName)
	}

	if len(item.SourceBytes) == 0 {
		return "", nil
	}

	// Re-parse source bytes to generate preview
	reader, err := a.formatReg.NewReader(item.Format)
	if err != nil {
		return "", fmt.Errorf("no reader for %q: %w", item.Format, err)
	}

	doc := &model.RawDocument{
		URI:          itemName,
		SourceLocale: proj.SourceLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(item.SourceBytes)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return "", fmt.Errorf("parse source: %w", err)
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			return "", fmt.Errorf("read source: %w", result.Error)
		}
		parts = append(parts, result.Part)
	}
	reader.Close()

	preview := editor.BuildPreview(parts, item.SourceBytes, item.Format, proj.SourceLocale)
	return preview, nil
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
