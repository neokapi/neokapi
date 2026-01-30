package backend

import (
	"fmt"

	"github.com/asgeirf/gokapi/core/kaz"
	"github.com/asgeirf/gokapi/core/model"
)

// RenderDocumentPreview returns the pre-rendered HTML for an item.
// If previewHTML is already cached, it returns that; otherwise it
// generates preview on-the-fly.
func (a *App) RenderDocumentPreview(projectID, itemName, targetLocale string) (string, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return "", err
	}

	id, ok := p.items[itemName]
	if !ok {
		return "", fmt.Errorf("item %q not found in project", itemName)
	}

	// Return cached preview if available
	if id.previewHTML != "" {
		return id.previewHTML, nil
	}

	// Generate on-the-fly
	preview := kaz.BuildPreview(id.parts, id.sourceBytes, id.format, model.LocaleID(p.info.SourceLocale))
	id.previewHTML = preview
	return preview, nil
}

// RenderBlockHTML returns the rendered HTML for a single block.
// If targetLocale is non-empty and a translation exists, it returns the
// target text; otherwise it returns the source HTML.
func (a *App) RenderBlockHTML(projectID, itemName, blockID, targetLocale string) (string, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return "", err
	}

	id, ok := p.items[itemName]
	if !ok {
		return "", fmt.Errorf("item %q not found in project", itemName)
	}

	// Look up block in block index (fast path)
	if id.blockIndex != nil {
		b := id.blockIndex.BlockByID(blockID)
		if b == nil {
			return "", fmt.Errorf("block %q not found in item %q", blockID, itemName)
		}

		// Return target translation if available
		if targetLocale != "" {
			if text, ok := b.Targets[targetLocale]; ok && text != "" {
				return text, nil
			}
		}

		// Return source HTML if available, else plain source
		if b.SourceHTML != "" {
			return b.SourceHTML, nil
		}
		return b.Source, nil
	}

	// Fallback: search Part stream
	for _, pt := range id.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok || block.ID != blockID {
			continue
		}

		// Return target translation if available
		if targetLocale != "" {
			if text := block.TargetText(model.LocaleID(targetLocale)); text != "" {
				return text, nil
			}
		}

		// Return source text
		return block.SourceText(), nil
	}

	return "", fmt.Errorf("block %q not found in item %q", blockID, itemName)
}
