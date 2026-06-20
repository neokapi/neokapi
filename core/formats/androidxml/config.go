package androidxml

import (
	"fmt"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format"
)

// Compile-time assertions that Config satisfies the framework config interfaces,
// including the optional schema and config-kind providers used by CLI
// introspection and config decoding.
var (
	_ format.DataFormatConfig   = (*Config)(nil)
	_ format.SchemaProvider     = (*Config)(nil)
	_ format.ConfigKindProvider = (*Config)(nil)
)

// Config holds configuration for the Android string-resources format.
//
// Android resource files have a fixed, well-defined schema (the <resources>
// vocabulary). The translatable surface is narrow — <string>, <string-array>
// items, and <plurals> items — so the format exposes only a handful of toggles;
// the bulk of its behaviour is dictated by the schema.
type Config struct {
	// ExtractComments controls whether an XML comment immediately preceding an
	// entry is surfaced as a translator note on the emitted Block(s). Defaults to
	// true. Comments always round-trip verbatim regardless of this setting.
	ExtractComments bool

	// SkipNonTranslatable controls whether entries marked translatable="false"
	// are excluded from extraction. Defaults to true — Android treats such
	// resources as developer-owned and the lint tooling forbids translating them.
	// When false the flag is ignored and the entry is extracted anyway.
	SkipNonTranslatable bool

	// SkipResourceReferences controls whether <string> values that are a bare
	// resource reference (e.g. @string/foo, ?attr/bar) are excluded from
	// extraction. Defaults to true — a reference is an alias, not UI text.
	SkipResourceReferences bool

	// disableNonTranslatableContent, when set, keeps entries marked
	// translatable="false" in opaque skeleton instead of surfacing them as
	// non-translatable content Blocks (Translatable:false, visible to ingestion,
	// skipped by MT). Zero value = surfacing ON (the opt-out default). This is
	// orthogonal to SkipNonTranslatable, which decides whether such entries are
	// *translatable*: with SkipNonTranslatable on (default) a translatable="false"
	// entry is never translated, but its value is still surfaced as contextual
	// content unless this flag turns the surfacing off. Bare resource references
	// are unaffected — they always round-trip as skeleton.
	disableNonTranslatableContent bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "androidxml" }

// ConfigKind returns the Kind for the androidxml format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("androidxml") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		ExtractComments:        true,
		SkipNonTranslatable:    true,
		SkipResourceReferences: true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ExtractNonTranslatableContent reports whether entries marked
// translatable="false" are surfaced as non-translatable content Blocks
// (Translatable:false, visible to ingestion, skipped by MT). Default true. When
// false such entries stay in opaque skeleton and round-trip verbatim — the
// historical behaviour.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of translatable="false"
// entries as non-translatable content blocks. The parity runner type-asserts
// this to force the flag off so the canonical part stream stays byte-identical
// to a non-surfacing baseline.
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractComments":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractComments: expected bool, got %T", val)
			}
			c.ExtractComments = b
		case "skipNonTranslatable":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("skipNonTranslatable: expected bool, got %T", val)
			}
			c.SkipNonTranslatable = b
		case "skipResourceReferences":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("skipResourceReferences: expected bool, got %T", val)
			}
			c.SkipResourceReferences = b
		case "extractNonTranslatableContent":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractNonTranslatableContent: expected bool, got %T", val)
			}
			c.disableNonTranslatableContent = !b
		default:
			return fmt.Errorf("androidxml: unknown parameter: %s", key)
		}
	}
	return nil
}
