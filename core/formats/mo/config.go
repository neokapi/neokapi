package mo

// Config holds configuration for the MO writer. Empty by default — MO is
// a binary gettext catalog; its shape is determined by the incoming Blocks
// (one entry per translated Block, with msgctxt from Block.Name or
// Properties["context"]). Reserved for future knobs such as selecting
// between little- and big-endian encoding.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "mo" }

// Reset restores default values.
func (c *Config) Reset() { *c = Config{} }

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map. No knobs today.
func (c *Config) ApplyMap(values map[string]any) error {
	for key := range values {
		_ = key
	}
	return nil
}
