package po

// Config holds configuration for the PO format.
type Config struct {
	// PreserveUntranslated if true, emits blocks for entries with empty msgstr.
	PreserveUntranslated bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "po" }

// Reset restores default values.
func (c *Config) Reset() {
	c.PreserveUntranslated = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }
