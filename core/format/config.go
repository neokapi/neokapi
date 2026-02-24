package format

// DataFormatConfig holds configuration for a data format.
type DataFormatConfig interface {
	// FormatName returns the format this config applies to.
	FormatName() string

	// Reset restores default values.
	Reset()

	// Validate checks configuration validity.
	Validate() error

	// ApplyMap applies configuration values from a map.
	// Unknown keys or type mismatches return an error.
	ApplyMap(values map[string]any) error
}
