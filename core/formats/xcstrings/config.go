package xcstrings

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

// Config holds configuration for the Apple String Catalog format.
//
// Apple String Catalogs (.xcstrings) are produced by Xcode's localization
// tooling and have a fixed, well-defined JSON schema. The format therefore
// exposes only a small number of toggles; the bulk of its behaviour is
// dictated by the schema rather than by user configuration.
type Config struct {
	// ExtractStale controls whether entries whose extractionState is
	// "stale" (the source string no longer appears in the code base) are
	// emitted as translatable blocks. Defaults to true — stale entries are
	// still localizable until removed.
	ExtractStale bool

	// MarkTranslatedState controls the state value written into a
	// localization's stringUnit when a target translation is produced for a
	// previously untranslated locale. Apple uses "translated". Existing
	// states are preserved verbatim on round-trip; this only governs newly
	// populated localizations.
	MarkTranslatedState string

	// disableNonTranslatableContent, when set, keeps non-translatable
	// contextual content out of the part stream. Specifically it suppresses the
	// entry-level fallback Block that surfaces an entry's developer comment when
	// the entry produces no translatable leaf (no localizations, an empty
	// localizations object, or a stale entry skipped because ExtractStale is
	// off). Zero value = surfacing ON (the opt-out default). When off, the part
	// stream is byte-identical to the prior behavior (parity-faithful).
	disableNonTranslatableContent bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "xcstrings" }

// ConfigKind returns the Kind for the xcstrings format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("xcstrings") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		ExtractStale:        true,
		MarkTranslatedState: "translated",
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	return nil
}

// ExtractNonTranslatableContent reports whether non-translatable contextual
// content (the entry-level developer-comment fallback Block for entries with no
// translatable leaf) is surfaced into the part stream. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content. The parity runner type-asserts this method and turns
// surfacing off so the emitted part stream stays byte-identical to the prior
// behavior.
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractStale":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractStale: expected bool, got %T", val)
			}
			c.ExtractStale = b
		case "markTranslatedState":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("markTranslatedState: expected string, got %T", val)
			}
			c.MarkTranslatedState = s
		case "extractNonTranslatableContent":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractNonTranslatableContent: expected bool, got %T", val)
			}
			c.disableNonTranslatableContent = !b
		default:
			return fmt.Errorf("xcstrings: unknown parameter: %s", key)
		}
	}
	return nil
}
