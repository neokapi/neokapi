package designtokens

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

// Config holds configuration for the W3C DTCG Design Tokens format.
//
// Design tokens are overwhelmingly non-linguistic: a token's $value is a
// colour, dimension, font family, duration, cubic-bézier, and so on — none of
// which should be translated. The only field that reliably carries
// human-readable, translatable prose is $description (token and group
// documentation). The format therefore configures the generic JSON reader to
// extract *only* $description values, leaving every token value and the entire
// structure as non-translatable passthrough.
//
// The exposed surface is intentionally small: the DTCG layout is fixed by the
// specification, so behaviour is governed by the spec rather than by user
// configuration. The single toggle here decides whether $description values
// are the translatable surface or whether nothing at all is extracted.
type Config struct {
	// ExtractDescriptions controls whether $description values (the
	// human-readable documentation on tokens and groups) are extracted as
	// translatable blocks. Defaults to true.
	//
	// $description is the only DTCG field that holds natural-language prose;
	// $value is always a typed design value (colour, dimension, font name,
	// duration, …) and is never extracted. When this is false the document is
	// read as fully non-translatable structure (useful for validation-only or
	// pass-through flows).
	ExtractDescriptions bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return formatID }

// ConfigKind returns the Kind for the design tokens format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind(formatID) }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		ExtractDescriptions: true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractDescriptions":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractDescriptions: expected bool, got %T", val)
			}
			c.ExtractDescriptions = b
		default:
			return fmt.Errorf("designtokens: unknown parameter: %s", key)
		}
	}
	return nil
}

// descriptionExtractionRule is the regex handed to the JSON reader's
// extractionRules. With UseFullKeyPath enabled the reader matches against the
// slash-joined full key path (always leading-slash for the match target, e.g.
// "/color/primary/$description"), so anchoring on a path boundary ((^|/))
// before the literal "$description" segment and the end of the path selects
// exactly the $description keys at any nesting depth while never matching a
// token named, say, "my$description" or a group whose own name ends in
// "$description".
const descriptionExtractionRule = `(^|/)\$description$`

// applyToJSON realises the design-tokens preset onto an existing generic JSON
// configuration. The passed config is reset first so callers can hand in a live
// reader/writer config.
//
// The key move is extractAllPairs=false combined with an extractionRules regex
// that targets only the $description segment of the full key path. The JSON
// reader's shouldExtract treats a set extractionRules as the *sole* inclusion
// filter (see core/formats/json/config.go shouldExtract): every string whose
// path does not match becomes non-translatable Data, so all $value strings
// (font names, colour hex, …), $type, $extensions content, $deprecated
// messages, and group/token structure pass through untouched. Byte-faithful
// round-trip is inherited from the JSON reader/writer: untranslated values
// replay their original token bytes verbatim.
func (c *Config) applyToJSON(jc *jsonfmt.Config) {
	jc.Reset()

	// Full key-path names so nested $description blocks get meaningful,
	// collision-free names like /color/primary/$description.
	jc.UseFullKeyPath = true
	jc.UseLeadingSlashOnKeyPath = true

	// DTCG files are conventional JSON and commonly ship without slash
	// escaping; emit bare slashes to match typical token tooling output.
	// Byte-faithful round-trip does not depend on this — the writer replays the
	// original token bytes for untranslated values regardless.
	jc.EscapeForwardSlashes = false

	// Extract nothing by default …
	jc.ExtractAllPairs = false
	if c.ExtractDescriptions {
		// … then re-include only $description via the extraction-rules filter.
		jc.ExtractionRules = descriptionExtractionRule
	}
}
