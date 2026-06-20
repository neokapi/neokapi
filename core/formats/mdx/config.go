package mdx

import (
	"fmt"
	"maps"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/formats/markdown"
)

// extractNonTranslatableContentKey is the parameter that toggles MDX's own
// non-translatable content surfacing (block-level JSX text children, GFM table
// cell prose, and the markdown-opaque fallback blocks). It is intercepted by
// the MDX config and NOT forwarded to the delegated markdown reader, whose
// own non-translatable surfacing (code fences) is governed separately and kept
// off for the embedded spans (see reader.emitMarkdownProse).
const extractNonTranslatableContentKey = "extractNonTranslatableContent"

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
	// markdown defaults. The extractNonTranslatableContent key is never
	// stored here — it is intercepted into disableNonTranslatableContent.
	params map[string]any

	// disableNonTranslatableContent, when set, keeps MDX-specific
	// non-translatable content (block-level JSX text children, GFM table
	// cell prose, markdown-opaque fallback blocks) in opaque skeleton/Data
	// instead of surfacing it as Translatable:false content blocks (visible
	// to ingestion, skipped by MT). Zero value = surfacing ON (the opt-out
	// default). Parity forces it off via SetExtractNonTranslatableContent.
	disableNonTranslatableContent bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "mdx" }

// ConfigKind declares the config envelope kind for native MDX. Reusing the
// markdown config kind would be wrong (the decoders differ), so MDX
// declares its own.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("mdx") }

// Reset restores default values.
func (c *Config) Reset() {
	c.params = nil
	c.disableNonTranslatableContent = false
}

// ExtractNonTranslatableContent reports whether MDX-specific non-translatable
// content (JSX text children, table cell prose, markdown-opaque fallback
// blocks) is surfaced as Translatable:false content blocks. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of MDX-specific
// non-translatable content as content blocks. The parity runner type-asserts
// this method and sets it false so the canonical part stream matches the
// opaque-only baseline.
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// Validate checks configuration validity by replaying the (already
// flag-stripped) parameters onto a throwaway markdown.Config (which performs
// the per-key type checks).
func (c *Config) Validate() error {
	if len(c.params) == 0 {
		return nil
	}
	probe := &markdown.Config{}
	probe.Reset()
	return probe.ApplyMap(c.params)
}

// ApplyMap intercepts the MDX-owned extractNonTranslatableContent flag, then
// validates the remaining values against the markdown config schema (so an
// unknown key or type mismatch is rejected up front) and retains them for
// replay onto delegated markdown readers. The flag is deliberately NOT
// forwarded to markdown — MDX's surfacing is independent of the embedded
// markdown reader's code-fence surfacing.
func (c *Config) ApplyMap(values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	forward := make(map[string]any, len(values))
	for key, val := range values {
		if key == extractNonTranslatableContentKey {
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("mdx: %s: expected bool, got %T", extractNonTranslatableContentKey, val)
			}
			c.disableNonTranslatableContent = !b
			continue
		}
		forward[key] = val
	}
	if len(forward) == 0 {
		return nil
	}
	probe := &markdown.Config{}
	probe.Reset()
	if err := probe.ApplyMap(forward); err != nil {
		return err
	}
	if c.params == nil {
		c.params = make(map[string]any, len(forward))
	}
	maps.Copy(c.params, forward)
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
