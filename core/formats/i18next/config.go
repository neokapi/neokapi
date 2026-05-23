// Package i18next implements a first-class neokapi format for the i18next /
// react-i18next JSON localization layout — the conventions of the dominant web
// i18n library (https://www.i18next.com/misc/json-format).
//
// The format is a thin wrapper over the generic JSON reader/writer. i18next
// files are ordinary JSON resource bundles, so the heavy lifting (tokenizing,
// byte-faithful round-trip, key-path naming, embedded-markup subfiltering,
// inline-code detection) is delegated to core/formats/json. This package adds
// only the i18next-specific semantics that the generic reader cannot infer:
//
//   - interpolation ({{var}}, {{var, format}}) and nesting ($t(key)) are
//     protected as inline codes via a preset code-finder configuration;
//   - v4 CLDR plural sibling keys (key_one / key_other / …, plus the legacy
//     key_plural and key_0 / key_1 forms) are annotated with their base key and
//     plural category and grouped under a shared plural-group id;
//   - context sibling keys (key_male / key_female / …) are annotated with their
//     base key and context value;
//   - nested namespaces become key-path block names (reusing the JSON reader's
//     full-key-path option).
//
// Each i18next key remains a one-to-one Block so the JSON writer's per-value
// token replacement keeps the round-trip byte-faithful: plural and context
// siblings are annotated, never merged, on the read side and preserved verbatim
// on write.
package i18next

import (
	"fmt"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
)

// Compile-time assertions that Config satisfies the framework config
// interfaces, including the optional schema and config-kind providers used by
// CLI introspection and config decoding.
var (
	_ format.DataFormatConfig   = (*Config)(nil)
	_ format.SchemaProvider     = (*Config)(nil)
	_ format.ConfigKindProvider = (*Config)(nil)
)

// Config holds configuration for the i18next JSON format.
//
// The exposed surface is intentionally small: i18next files have a
// well-understood layout, so most behaviour is fixed by the conventions of the
// library rather than by user configuration. The toggles here govern whether
// interpolation/nesting are protected as inline codes, whether embedded HTML
// values (the `_html` key suffix) are handed to the HTML subfilter, and how the
// reader recognises plural sibling keys.
type Config struct {
	// ProtectInterpolation controls whether i18next interpolation
	// ({{var}}, {{var, format}}) and nesting ($t(key)) are detected and
	// protected as opaque inline codes so they are never translated.
	// Defaults to true.
	ProtectInterpolation bool

	// SubfilterHTMLValues controls whether values whose key ends in `_html`
	// (the i18next convention for values containing markup) are handed to the
	// HTML subfilter so their tags are protected and their text remains
	// translatable.
	//
	// Defaults to false. The HTML subfilter is not byte-faithful for the bare
	// markup fragments i18next stores (the HTML format normalises a fragment by
	// wrapping it in a document body), so enabling it trades exact round-trip
	// for tag protection. Leave it off when faithful round-trip matters; enable
	// it for translation workflows where protecting embedded tags is worth the
	// re-serialization.
	SubfilterHTMLValues bool

	// LegacyPluralForms controls whether the legacy v1–v3 plural sibling keys
	// (`key_plural` and the numeric `key_0` / `key_1` / `key_2` … forms) are
	// recognised as plurals in addition to the v4 CLDR suffixes
	// (`_zero` / `_one` / `_two` / `_few` / `_many` / `_other`). Defaults to
	// true.
	LegacyPluralForms bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return formatID }

// ConfigKind returns the Kind for the i18next format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind(formatID) }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		ProtectInterpolation: true,
		SubfilterHTMLValues:  false,
		LegacyPluralForms:    true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "protectInterpolation":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("protectInterpolation: expected bool, got %T", val)
			}
			c.ProtectInterpolation = b
		case "subfilterHtmlValues":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("subfilterHtmlValues: expected bool, got %T", val)
			}
			c.SubfilterHTMLValues = b
		case "legacyPluralForms":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("legacyPluralForms: expected bool, got %T", val)
			}
			c.LegacyPluralForms = b
		default:
			return fmt.Errorf("i18next: unknown parameter: %s", key)
		}
	}
	return nil
}

// applyToJSON realises the i18next preset onto an existing generic JSON
// configuration: full key-path names for nested namespaces, the code-finder
// rules that protect {{interpolation}} / $t() nesting, and the `_html` HTML
// subfilter mapping — the same knobs the JSON format's built-in "i18next"
// preset exposes, applied programmatically here. The passed config is reset
// first so callers can hand in a live reader/writer config.
func (c *Config) applyToJSON(jc *jsonfmt.Config) {
	jc.Reset()

	// Nested namespaces → key-path block names.
	jc.UseFullKeyPath = true
	jc.UseLeadingSlashOnKeyPath = true

	// i18next files commonly ship without slash-escaping; emit bare slashes so
	// the writer round-trips typical i18next bundles. Byte-faithful round-trip
	// does not depend on this — the writer replays the original token bytes for
	// untranslated values regardless — but it matches the conventional output.
	jc.EscapeForwardSlashes = false

	if c.ProtectInterpolation {
		jc.UseCodeFinder = true
		jc.CodeFinderRules = interpolationCodeFinderRules()
	}

	if c.SubfilterHTMLValues {
		jc.SubfilterFormat = "html"
		jc.SubfilterRules = `_html$`
	}
}

// interpolationCodeFinderRules returns the regular expressions that match
// i18next interpolation and nesting so the JSON code-finder protects them as
// inline codes. Order matters only in that all matches are collected and sorted
// by position by the JSON reader; overlapping alternatives are fine.
//
//   - {{ … }}  — interpolation, including formatted {{var, format}} and keys
//     with namespace/keyseparator characters. The non-greedy body stops at the
//     first closing }} so adjacent placeholders are matched independently.
//   - $t( … )  — nesting references to other keys, with optional options object.
func interpolationCodeFinderRules() []string {
	return []string{
		`\{\{[^}]*\}\}`,
		`\$t\([^)]*\)`,
	}
}
