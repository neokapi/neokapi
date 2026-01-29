package csv

import "fmt"

// Config holds configuration for the CSV format.
type Config struct {
	// Separator is the field delimiter character. Default is ','.
	Separator rune
	// HasHeader if true, the first row is treated as headers.
	HasHeader bool
	// TranslatableColumns specifies which column indices (0-based) to extract
	// as translatable content. If empty, all columns are translatable.
	TranslatableColumns []int
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "csv" }

// Reset restores default values.
func (c *Config) Reset() {
	c.Separator = ','
	c.HasHeader = true
	c.TranslatableColumns = nil
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	if c.Separator == 0 {
		return fmt.Errorf("csv: separator must not be zero")
	}
	return nil
}
