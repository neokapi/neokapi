package project

import (
	"github.com/neokapi/neokapi/core/format"
)

// A recipe declares a format's configuration in two places, and both are
// statements about the same reader/writer pair: `defaults.formats.<format>.config`
// for every item that uses the format, and a content item's own `format.config`
// for that item alone. Whoever reads or writes a file has to consult both, or it
// reads the document under a configuration the recipe never described — and the
// keys that live here are exactly the ones that say which leaves of a document
// are content (`extractionRules`, `keyPathPatterns`, `frontMatterKeys`). A path
// that drops them extracts a document's identifiers and slugs as prose and
// translates them.
//
// MergeFormatConfig is therefore the one merge, and every path that builds a
// reader or a writer for a project file goes through it.

// MergeFormatConfig returns the format configuration a recipe declares for one
// content item: `defaults.formats[formatName].config` overlaid by the item's own
// `format.config`, the item winning per key. A nil item asks only for the
// defaults. Returns nil when the recipe declares nothing.
//
// The overlay is shallow — a key an item names replaces the default outright
// rather than merging into it, so an item that narrows `extractionRules` is not
// silently widened by the project default it meant to override.
func MergeFormatConfig(defaults map[string]FormatDefaults, formatName string, item *ContentItem) map[string]any {
	var merged map[string]any
	add := func(cfg map[string]any) {
		for k, v := range cfg {
			if merged == nil {
				merged = make(map[string]any, len(cfg))
			}
			merged[k] = v
		}
	}
	if fd, ok := defaults[formatName]; ok {
		add(fd.Config)
	}
	if item != nil && item.Format != nil {
		add(item.Format.Config)
	}
	return merged
}

// FormatConfigFor is MergeFormatConfig over this context's format defaults.
func (ctx *ProjectContext) FormatConfigFor(formatName string, item *ContentItem) map[string]any {
	return MergeFormatConfig(ctx.FormatDefaults, formatName, item)
}

// ApplyReaderConfig applies a merged format config onto a reader's typed
// config, stripping the reserved `output.*` writer options first — readers
// normalize BOM, charset and newlines at parse time, so those keys are not
// theirs and would fail the reader's own unknown-key check.
func ApplyReaderConfig(reader Configurable, cfg map[string]any) error {
	if len(cfg) == 0 {
		return nil
	}
	_, rest, err := format.SplitOutputConfig(cfg)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return nil
	}
	c := reader.Config()
	if c == nil {
		return nil
	}
	return c.ApplyMap(rest)
}

// ApplyWriterConfig applies a merged format config onto a writer: the reserved
// `output.*` byte-level options every writer embedding format.BaseFormatWriter
// understands, plus the format-specific serialization keys for a writer that
// exposes its typed config through format.WriterConfigurable.
//
// A writer that exposes no config takes the output options and ignores the
// rest: the recipe's remaining keys govern extraction, which is the reader's
// half of the pair.
func ApplyWriterConfig(writer format.DataFormatWriter, cfg map[string]any) error {
	if len(cfg) == 0 {
		return nil
	}
	opts, rest, err := format.SplitOutputConfig(cfg)
	if err != nil {
		return err
	}
	if !opts.IsZero() {
		if oc, ok := writer.(format.OutputConfigurable); ok {
			if serr := oc.SetOutputOptions(opts); serr != nil {
				return serr
			}
		}
	}
	if len(rest) == 0 {
		return nil
	}
	wc, ok := writer.(format.WriterConfigurable)
	if !ok {
		return nil
	}
	c := wc.WriterConfig()
	if c == nil {
		return nil
	}
	return c.ApplyMap(rest)
}
