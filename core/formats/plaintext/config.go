package plaintext

import (
	"errors"

	"github.com/neokapi/neokapi/core/format"
)

// DefaultSpliceMarker is the continuation marker recognized at the end of
// a line when splice mode is enabled and no custom marker is configured.
const DefaultSpliceMarker = `\`

// Config holds configuration for the plain text format.
type Config struct {
	// SegmentByLine if true, each line is a Block. If false, paragraphs
	// (separated by blank lines) are Blocks.
	SegmentByLine bool `json:"segmentByLine" schema:"title=Segment by Line,description=If true each line becomes a separate block; if false paragraphs separated by blank lines are blocks,default=true"`
	// Paragraphs if true, blank-line-separated paragraphs are Blocks
	// (the behavior of the retired paraplaintext format). Equivalent to
	// segmentByLine=false; either switch enables paragraph mode.
	Paragraphs bool `json:"paragraphs" schema:"title=Paragraphs,description=If true blank-line-separated paragraphs become blocks instead of individual lines,default=false"`
	// SpliceLines if true, lines ending with the splice marker are joined
	// with the following line into a single Block (the behavior of the
	// retired splicedlines format).
	SpliceLines bool `json:"spliceLines" schema:"title=Splice Lines,description=If true lines ending with the splice marker are joined with the next line into one block,default=false"`
	// SpliceMarker is the continuation marker recognized at the end of a
	// line in splice mode. Defaults to a single backslash.
	SpliceMarker string `json:"spliceMarker" schema:"title=Splice Marker,description=Continuation marker recognized at the end of a line in splice mode,default=\\"`
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "plaintext" }

// Reset restores default values.
func (c *Config) Reset() {
	c.SegmentByLine = true
	c.Paragraphs = false
	c.SpliceLines = false
	c.SpliceMarker = DefaultSpliceMarker
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	if c.Paragraphs && c.SpliceLines {
		return errors.New("plaintext: paragraphs and spliceLines are mutually exclusive")
	}
	return nil
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	return format.ApplyMapViaJSON(c, values)
}

// ParagraphMode reports whether paragraph extraction is active — either
// via the paragraphs switch or the legacy segmentByLine=false setting.
func (c *Config) ParagraphMode() bool {
	return c.Paragraphs || !c.SegmentByLine
}

// Marker returns the effective splice continuation marker.
func (c *Config) Marker() string {
	if c.SpliceMarker == "" {
		return DefaultSpliceMarker
	}
	return c.SpliceMarker
}
