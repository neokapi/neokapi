package asciidoc

import (
	"fmt"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format"
)

// Compile-time assertions that Config satisfies the framework config
// interfaces, including the optional schema and config-kind providers used by
// CLI introspection and config decoding.
var (
	_ format.DataFormatConfig   = (*Config)(nil)
	_ format.SchemaProvider     = (*Config)(nil)
	_ format.ConfigKindProvider = (*Config)(nil)
)

// Config holds configuration for the AsciiDoc format.
//
// AsciiDoc (https://docs.asciidoctor.org/asciidoc/latest/, the Eclipse
// AsciiDoc Language working group's evolving specification) is a lightweight
// prose markup language. The reader extracts logical content — headings,
// paragraphs, list items, block titles, admonition text, and table cells —
// and round-trips everything else (verbatim blocks, comments, attribute
// entries, the document header) as non-translatable skeleton. The format
// therefore exposes only a couple of behavioural toggles; the bulk of its
// behaviour is dictated by the grammar rather than by user configuration.
type Config struct {
	// ExtractBlockTitles controls whether AsciiDoc block titles (a line of
	// the form `.Title` attached to a following block) are emitted as
	// translatable blocks. Defaults to true. When false they are preserved
	// verbatim as skeleton.
	ExtractBlockTitles bool

	// ExtractTableCells controls whether the cells of a `|===` table are
	// emitted as translatable blocks (one per cell, grouped into table /
	// table-row groups). Defaults to true. When false the entire table is
	// preserved verbatim as a single skeleton block.
	ExtractTableCells bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "asciidoc" }

// ConfigKind returns the Kind for the asciidoc format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("asciidoc") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		ExtractBlockTitles: true,
		ExtractTableCells:  true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map, rejecting unknown keys.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractBlockTitles":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractBlockTitles: expected bool, got %T", val)
			}
			c.ExtractBlockTitles = b
		case "extractTableCells":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractTableCells: expected bool, got %T", val)
			}
			c.ExtractTableCells = b
		default:
			return fmt.Errorf("asciidoc: unknown parameter: %s", key)
		}
	}
	return nil
}
