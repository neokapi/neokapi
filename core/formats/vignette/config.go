package vignette

import (
	"fmt"
	"strings"
)

// Default Okapi VignetteFilter configuration.
//
// These mirror the upstream `Parameters` defaults in the Java
// VignetteFilter. The lists are order-significant: each entry in
// `PartsNames` pairs by index with the corresponding entry in
// `PartsConfigurations` to identify which sub-filter (or `default` for
// none) processes that attribute's payload.
const (
	DefaultPartsNames = "SMCCONTENT-TITLE, SMCCONTENT-ABSTRACT, SMCCONTENT-BODY, SMCCONTENT-ALT, " +
		"SMCCHANNELDESCRIPTOR-TITLE, SMCCHANNELDESCRIPTOR-ABSTRACT, SMCCHANNELDESCRIPTOR-ALT, " +
		"SMCLINKCOLLECTIONS-LINKCOLLECTION-TITLE, SMCLINKCOLLECTIONS-LINKCOLLECTION-DESCRIPTION, " +
		"SMCLINKS-TITLE, SMCLINKS-ABSTRACT, SMCLINKS-BODY, SMCLINKS-ALT"

	DefaultPartsConfigurations = "default, okf_html, okf_html, default, " +
		"default, okf_html, default, " +
		"default, okf_html, " +
		"default, okf_html, okf_html, default"

	DefaultSourceID = "SOURCE_ID"
	DefaultLocaleID = "LOCALE_ID"
)

// Config holds configuration for the Vignette CMS export/import XML format.
type Config struct {
	// PartsNames is the comma-separated list of `<attribute name="…">`
	// values to extract from each `importContentInstance`. Defaults to
	// the standard SMC attribute set.
	PartsNames string

	// PartsConfigurations is the comma-separated list of sub-filter
	// configuration ids (one per entry in PartsNames). Use "default" for
	// no sub-filtering (the payload is treated as a single literal Block).
	// Currently the native reader recognises "okf_html" (decode HTML
	// entities + strip outer <p> wrapping) and treats every other value
	// — including "default" — as no sub-filtering.
	PartsConfigurations string

	// SourceID is the `name` attribute value of the `<attribute>` that
	// holds the source-id linking source and target instances.
	SourceID string

	// LocaleID is the `name` attribute value of the `<attribute>` that
	// holds the locale identifier.
	LocaleID string

	// Monolingual disables source/target pairing and extracts every
	// `importContentInstance` independently.
	Monolingual bool

	// UseCDATA wraps written payloads in `<![CDATA[…]]>` (write-side
	// only — has no effect on reading).
	UseCDATA bool

	// disableNonTranslatableContent, when set, keeps non-source-locale
	// content instances (the other-locale / unpaired importContentInstance
	// payloads that bilingual mode does not extract as translatable) in
	// opaque skeleton instead of surfacing them as Translatable:false
	// content Blocks (visible to ingestion, skipped by MT). Zero value =
	// surfacing ON (the opt-out default). Has no effect in monolingual mode
	// (every instance is already extracted there).
	disableNonTranslatableContent bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "vignette" }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		PartsNames:          DefaultPartsNames,
		PartsConfigurations: DefaultPartsConfigurations,
		SourceID:            DefaultSourceID,
		LocaleID:            DefaultLocaleID,
		Monolingual:         false,
		UseCDATA:            true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ExtractNonTranslatableContent reports whether non-source-locale content
// instances (the other-locale / unpaired importContentInstance payloads that
// bilingual mode does not extract as translatable) are surfaced as
// Translatable:false content Blocks. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-source-locale
// content instances as content Blocks (used by the parity runner to match the
// Okapi bridge, which keeps such content in skeleton).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "partsNames":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("partsNames: expected string, got %T", val)
			}
			c.PartsNames = s
		case "partsConfigurations":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("partsConfigurations: expected string, got %T", val)
			}
			c.PartsConfigurations = s
		case "sourceId":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("sourceId: expected string, got %T", val)
			}
			c.SourceID = s
		case "localeId":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("localeId: expected string, got %T", val)
			}
			c.LocaleID = s
		case "monolingual":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("monolingual: expected bool, got %T", val)
			}
			c.Monolingual = b
		case "useCDATA":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useCDATA: expected bool, got %T", val)
			}
			c.UseCDATA = b
		case "extractNonTranslatableContent":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractNonTranslatableContent: expected bool, got %T", val)
			}
			c.disableNonTranslatableContent = !b
		default:
			return fmt.Errorf("vignette: unknown parameter: %s", key)
		}
	}
	return nil
}

// PartsMap returns a map from attribute name to sub-filter
// configuration id, paired by index in the comma-separated lists.
// Names without a corresponding configuration entry default to
// "default" (no sub-filtering).
func (c *Config) PartsMap() map[string]string {
	names := splitCommaList(c.PartsNames)
	configs := splitCommaList(c.PartsConfigurations)
	out := make(map[string]string, len(names))
	for i, name := range names {
		cfg := "default"
		if i < len(configs) {
			cfg = configs[i]
		}
		out[name] = cfg
	}
	return out
}

func splitCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
