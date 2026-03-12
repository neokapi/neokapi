package ttx

import (
	"fmt"

	"github.com/neokapi/neokapi/core/config"
)

// SegmentMode controls how the TTX reader handles segmentation.
type SegmentMode int

const (
	// SegmentModeAuto auto-detects existing segments: if found, extract only
	// those; otherwise extract all text parts. This is the default.
	SegmentModeAuto SegmentMode = 0

	// SegmentModeExistingOnly extracts only pre-existing segments (Tu elements).
	SegmentModeExistingOnly SegmentMode = 1

	// SegmentModeAll extracts all text, including both existing segments and
	// un-segmented text parts.
	SegmentModeAll SegmentMode = 2
)

// Config holds configuration for the Trados TagEditor TTX format.
// Options mirror the Okapi Framework TTX filter parameters.
type Config struct {
	// SegmentMode controls extraction behavior for segments.
	// See SegmentMode constants for valid values.
	// Defaults to SegmentModeAuto (0).
	SegmentMode SegmentMode

	// EscapeGT controls whether greater-than characters (>) are escaped as
	// &gt; in the output. Most XML processors don't require this, but some
	// legacy systems do.
	// Defaults to false.
	EscapeGT bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "ttx" }

// ConfigKind returns the Kind for TTX format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("ttx") }

// Reset restores default values matching Okapi's TTX filter defaults.
func (c *Config) Reset() {
	*c = Config{
		SegmentMode: SegmentModeAuto,
		EscapeGT:    false,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	if c.SegmentMode < 0 || c.SegmentMode > 2 {
		return fmt.Errorf("ttx: invalid segmentMode %d (must be 0, 1, or 2)", c.SegmentMode)
	}
	return nil
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "segmentMode":
			switch v := val.(type) {
			case int:
				c.SegmentMode = SegmentMode(v)
			case float64:
				c.SegmentMode = SegmentMode(int(v))
			default:
				return fmt.Errorf("segmentMode: expected int, got %T", val)
			}
		case "escapeGT":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("escapeGT: expected bool, got %T", val)
			}
			c.EscapeGT = b
		default:
			return fmt.Errorf("ttx: unknown parameter: %s", key)
		}
	}
	return nil
}
