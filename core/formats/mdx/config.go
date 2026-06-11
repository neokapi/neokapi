package mdx

import (
	"maps"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/formats/markdown"
)

// Config holds configuration for the MDX format. MDX is CommonMark
// Markdown extended with ESM (`import`/`export`), JSX, and `{expression}`
// nodes. The Markdown-prose extraction behaviour is governed entirely by
// the same toggles the markdown format exposes (translateCodeBlocks,
// translateFrontMatter, translateImageAlt, translateURLs,
// translateBlockQuotes, translateHTMLBlocks, useCodeFinder,
// codeFinderRules, …) — these are validated here against a fresh
// markdown.Config and replayed onto each markdown reader the MDX reader
// delegates Markdown spans to.
//
// The MDX-specific constructs (ESM, JSX, expressions) are always treated
// as opaque, non-translatable regions that round-trip byte-for-byte; there
// is intentionally no toggle to translate them in v1.
type Config struct {
	// params is the raw, validated parameter map applied via ApplyMap. It
	// is replayed onto each delegated markdown reader's config so MDX
	// honours exactly the same parameters as `.md`. nil/empty means
	// markdown defaults.
	params map[string]any
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "mdx" }

// ConfigKind declares the config envelope kind for native MDX. Reusing the
// markdown config kind would be wrong (the decoders differ), so MDX
// declares its own.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("mdx") }

// Reset restores default values.
func (c *Config) Reset() { c.params = nil }

// Validate checks configuration validity by replaying the parameters onto
// a throwaway markdown.Config (which performs the per-key type checks).
func (c *Config) Validate() error {
	if len(c.params) == 0 {
		return nil
	}
	probe := &markdown.Config{}
	probe.Reset()
	return probe.ApplyMap(c.params)
}

// ApplyMap validates the values against the markdown config schema (so an
// unknown key or type mismatch is rejected up front) and retains them for
// replay onto delegated markdown readers.
func (c *Config) ApplyMap(values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	probe := &markdown.Config{}
	probe.Reset()
	if err := probe.ApplyMap(values); err != nil {
		return err
	}
	if c.params == nil {
		c.params = make(map[string]any, len(values))
	}
	maps.Copy(c.params, values)
	return nil
}

// applyTo configures the given markdown config with this MDX config's
// retained parameters. Used by the reader when delegating a Markdown span.
func (c *Config) applyTo(md *markdown.Config) error {
	if len(c.params) == 0 {
		return nil
	}
	return md.ApplyMap(c.params)
}
