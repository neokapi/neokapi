package cli

import (
	"github.com/bmatcuk/doublestar/v4"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/project"
)

// mergedFormatConfig returns the format config the project declares for a
// content item: defaults.formats[<format>].config overlaid by the item's own
// format.config (item wins per key). nil when nothing is configured.
func mergedFormatConfig(proj *project.KapiProject, formatName string, item *project.ContentItem) map[string]any {
	if proj == nil {
		return nil
	}
	var merged map[string]any
	add := func(cfg map[string]any) {
		for k, v := range cfg {
			if merged == nil {
				merged = map[string]any{}
			}
			merged[k] = v
		}
	}
	if fd, ok := proj.Defaults.Formats[formatName]; ok {
		add(fd.Config)
	}
	if item != nil && item.Format != nil {
		add(item.Format.Config)
	}
	return merged
}

// formatConfigForSource resolves the merged format config for a source file
// by matching it against the project's content items (doublestar, like
// content resolution). Used by merge, where only the relative source path —
// not the resolved item — survives in the extraction manifest.
func formatConfigForSource(proj *project.KapiProject, formatName, relSource string) map[string]any {
	if proj == nil {
		return nil
	}
	for _, coll := range proj.Content {
		for _, item := range coll.EffectiveItems() {
			if ok, _ := doublestar.Match(item.Path, relSource); ok {
				itemCopy := item
				return mergedFormatConfig(proj, formatName, &itemCopy)
			}
		}
	}
	return mergedFormatConfig(proj, formatName, nil)
}

// applyFormatConfig applies a merged config map onto a reader's typed
// config. A nil/empty map is a no-op; readers without a Config are skipped.
func applyFormatConfig(reader format.DataFormatReader, cfg map[string]any) error {
	if len(cfg) == 0 {
		return nil
	}
	c := reader.Config()
	if c == nil {
		return nil
	}
	return c.ApplyMap(cfg)
}
