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
	// KeyColumns specifies which column indices (0-based) provide the block ID
	// (source ID / record ID). Values from these columns are concatenated with "."
	// to form the block ID. If empty, row-based IDs are used.
	KeyColumns []int
	// CommentColumns specifies which column indices (0-based) contain comments
	// or notes. These are stored as block properties.
	CommentColumns []int
	// TrimValues if true, leading and trailing whitespace is removed from cell values.
	TrimValues bool
	// ValuesStartRow is the 1-based row number where data values begin.
	// Default is 0 (auto: row after header, or row 1 if no header).
	ValuesStartRow int
	// ColumnNamesRow is the 1-based row number that contains column names.
	// Default is 0 (auto: row 1 if HasHeader is true).
	ColumnNamesRow int
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "csv" }

// Reset restores default values.
func (c *Config) Reset() {
	c.Separator = ','
	c.HasHeader = true
	c.TranslatableColumns = nil
	c.KeyColumns = nil
	c.CommentColumns = nil
	c.TrimValues = false
	c.ValuesStartRow = 0
	c.ColumnNamesRow = 0
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
			cols, err := parseIntSlice(val, key)
			if err != nil {
				return err
			}
			c.TranslatableColumns = cols
		case "keyColumns":
			cols, err := parseIntSlice(val, key)
			if err != nil {
				return err
			}
			c.KeyColumns = cols
		case "commentColumns":
			cols, err := parseIntSlice(val, key)
			if err != nil {
				return err
			}
			c.CommentColumns = cols
		case "trimValues":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("trimValues: expected bool, got %T", val)
			}
			c.TrimValues = b
		case "valuesStartRow":
			n, err := parseIntValue(val, key)
			if err != nil {
				return err
			}
			c.ValuesStartRow = n
		case "columnNamesRow":
			n, err := parseIntValue(val, key)
			if err != nil {
				return err
			}
			c.ColumnNamesRow = n
		default:
			return fmt.Errorf("csv: unknown parameter: %s", key)
		}
	}
	return nil
}

// parseIntSlice parses an []any of numbers into []int.
func parseIntSlice(val any, key string) ([]int, error) {
	arr, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected []any, got %T", key, val)
	}
	cols := make([]int, 0, len(arr))
	for i, elem := range arr {
		switch v := elem.(type) {
		case float64:
			cols = append(cols, int(v))
		case int:
			cols = append(cols, v)
		default:
			return nil, fmt.Errorf("%s[%d]: expected number, got %T", key, i, elem)
		}
	}
	return cols, nil
}

// parseIntValue parses a single number value.
func parseIntValue(val any, key string) (int, error) {
	switch v := val.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("%s: expected number, got %T", key, val)
	}
}
