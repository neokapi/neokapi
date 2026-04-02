package cli

import (
	"strings"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/spf13/cobra"
)

// RegisterSchemaFlags registers cobra flags from a ComponentSchema.
// Each property in the schema becomes a CLI flag with appropriate type,
// default value, and description.
func RegisterSchemaFlags(cmd *cobra.Command, s *schema.ComponentSchema) {
	if s == nil {
		return
	}
	for name, prop := range s.Properties {
		flagName := toKebabCase(name)
		desc := prop.Description
		if desc == "" {
			desc = name
		}

		// Append option labels to description
		if len(prop.Options) > 0 {
			vals := make([]string, len(prop.Options))
			for i, opt := range prop.Options {
				vals[i] = opt.Label
			}
			desc += " (" + strings.Join(vals, ", ") + ")"
		}

		switch prop.Type {
		case "boolean":
			def := false
			if b, ok := prop.Default.(bool); ok {
				def = b
			}
			cmd.Flags().Bool(flagName, def, desc)
		case "string":
			def := ""
			if s, ok := prop.Default.(string); ok {
				def = s
			}
			cmd.Flags().String(flagName, def, desc)
		case "integer":
			def := 0
			switch v := prop.Default.(type) {
			case int:
				def = v
			case float64:
				def = int(v)
			case int64:
				def = int(v)
			}
			cmd.Flags().Int(flagName, def, desc)
		case "number":
			def := 0.0
			switch v := prop.Default.(type) {
			case float64:
				def = v
			case int:
				def = float64(v)
			}
			cmd.Flags().Float64(flagName, def, desc)
		case "array":
			cmd.Flags().StringSlice(flagName, nil, desc)
		}
	}
}

// ReadSchemaFlags reads flag values from a cobra command using the schema
// to determine types. Only returns flags that were explicitly set.
func ReadSchemaFlags(cmd *cobra.Command, s *schema.ComponentSchema) map[string]any {
	if s == nil {
		return nil
	}
	result := make(map[string]any)
	for name, prop := range s.Properties {
		flagName := toKebabCase(name)
		f := cmd.Flags().Lookup(flagName)
		if f == nil || !f.Changed {
			continue
		}
		switch prop.Type {
		case "boolean":
			if v, err := cmd.Flags().GetBool(flagName); err == nil {
				result[name] = v
			}
		case "string":
			if v, err := cmd.Flags().GetString(flagName); err == nil {
				result[name] = v
			}
		case "integer":
			if v, err := cmd.Flags().GetInt(flagName); err == nil {
				result[name] = v
			}
		case "number":
			if v, err := cmd.Flags().GetFloat64(flagName); err == nil {
				result[name] = v
			}
		case "array":
			if v, err := cmd.Flags().GetStringSlice(flagName); err == nil {
				result[name] = v
			}
		}
	}
	return result
}

// ReadAllSchemaFlags reads all flag values (including defaults) from a cobra
// command using the schema to determine types.
func ReadAllSchemaFlags(cmd *cobra.Command, s *schema.ComponentSchema) map[string]any {
	if s == nil {
		return nil
	}
	result := make(map[string]any)
	for name, prop := range s.Properties {
		flagName := toKebabCase(name)
		switch prop.Type {
		case "boolean":
			if v, err := cmd.Flags().GetBool(flagName); err == nil {
				result[name] = v
			}
		case "string":
			if v, err := cmd.Flags().GetString(flagName); err == nil {
				result[name] = v
			}
		case "integer":
			if v, err := cmd.Flags().GetInt(flagName); err == nil {
				result[name] = v
			}
		case "number":
			if v, err := cmd.Flags().GetFloat64(flagName); err == nil {
				result[name] = v
			}
		case "array":
			if v, err := cmd.Flags().GetStringSlice(flagName); err == nil {
				result[name] = v
			}
		}
	}
	return result
}

// toKebabCase converts a camelCase name to kebab-case for CLI flags.
// "fuzzyThreshold" -> "fuzzy-threshold"
// "applyTarget" -> "apply-target"
func toKebabCase(s string) string {
	if s == "" {
		return s
	}
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '-')
		}
		if r >= 'A' && r <= 'Z' {
			result = append(result, r+('a'-'A'))
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}
