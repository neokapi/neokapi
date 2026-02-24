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

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "separator":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("separator: expected string, got %T", val)
			}
			runes := []rune(s)
			if len(runes) != 1 {
				return fmt.Errorf("separator: expected single character, got %q", s)
			}
			c.Separator = runes[0]
		case "hasHeader":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("hasHeader: expected bool, got %T", val)
			}
			c.HasHeader = b
		case "translatableColumns":
			arr, ok := val.([]any)
			if !ok {
				return fmt.Errorf("translatableColumns: expected []any, got %T", val)
			}
			cols := make([]int, 0, len(arr))
			for i, elem := range arr {
				switch v := elem.(type) {
				case float64:
					cols = append(cols, int(v))
				case int:
					cols = append(cols, v)
				default:
					return fmt.Errorf("translatableColumns[%d]: expected number, got %T", i, elem)
				}
			}
			c.TranslatableColumns = cols
		default:
			return fmt.Errorf("csv: unknown parameter: %s", key)
		}
	}
	return nil
}
