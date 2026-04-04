package fixedwidth

import (
	"errors"
	"fmt"
)

// ColumnDef defines a single column in a fixed-width file.
type ColumnDef struct {
	// Name is the column name (used for block/data naming).
	Name string
	// Start is the 0-based start position (in runes) of the column.
	Start int
	// Width is the width (in runes) of the column.
	Width int
	// Translatable indicates if this column contains translatable content.
	Translatable bool
}

// Config holds configuration for the fixed-width column format.
type Config struct {
	// Columns defines the column layout. Each column has a name, start position,
	// width, and translatability flag.
	Columns []ColumnDef
	// HasHeader if true, the first row is treated as a header row.
	HasHeader bool
	// TrimValues if true, leading and trailing whitespace is trimmed from values.
	TrimValues bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "fixedwidth" }

// Reset restores default values.
func (c *Config) Reset() {
	c.Columns = nil
	c.HasHeader = false
	c.TrimValues = false
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	if len(c.Columns) == 0 {
		return errors.New("fixedwidth: at least one column definition is required")
	}
	for i, col := range c.Columns {
		if col.Name == "" {
			return fmt.Errorf("fixedwidth: column %d: name must not be empty", i)
		}
		if col.Width <= 0 {
			return fmt.Errorf("fixedwidth: column %q: width must be positive", col.Name)
		}
		if col.Start < 0 {
			return fmt.Errorf("fixedwidth: column %q: start must not be negative", col.Name)
		}
	}
	return nil
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "columns":
			cols, err := parseColumnDefs(val)
			if err != nil {
				return err
			}
			c.Columns = cols
		case "hasHeader":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("hasHeader: expected bool, got %T", val)
			}
			c.HasHeader = b
		case "trimValues":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("trimValues: expected bool, got %T", val)
			}
			c.TrimValues = b
		default:
			return fmt.Errorf("fixedwidth: unknown parameter: %s", key)
		}
	}
	return nil
}

// parseColumnDefs parses column definitions from a map value.
func parseColumnDefs(val any) ([]ColumnDef, error) {
	arr, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("columns: expected []any, got %T", val)
	}
	var cols []ColumnDef
	for i, elem := range arr {
		m, ok := elem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("columns[%d]: expected map, got %T", i, elem)
		}
		col := ColumnDef{}
		if name, ok := m["name"].(string); ok {
			col.Name = name
		} else {
			return nil, fmt.Errorf("columns[%d]: name is required", i)
		}
		start, err := toInt(m["start"], fmt.Sprintf("columns[%d].start", i))
		if err != nil {
			return nil, err
		}
		col.Start = start
		width, err := toInt(m["width"], fmt.Sprintf("columns[%d].width", i))
		if err != nil {
			return nil, err
		}
		col.Width = width
		if t, ok := m["translatable"].(bool); ok {
			col.Translatable = t
		}
		cols = append(cols, col)
	}
	return cols, nil
}

func toInt(val any, field string) (int, error) {
	switch v := val.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("%s: expected number, got %T", field, val)
	}
}
