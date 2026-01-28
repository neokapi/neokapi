package xml

// Config holds configuration for the XML format.
type Config struct {
	// TranslatableElements lists element names whose text content is translatable.
	// If empty, all text content is considered translatable.
	TranslatableElements []string

	// TranslatableAttributes lists attribute names that are translatable.
	TranslatableAttributes []string
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "xml" }

// Reset restores default values.
func (c *Config) Reset() {
	c.TranslatableElements = nil
	c.TranslatableAttributes = nil
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }
