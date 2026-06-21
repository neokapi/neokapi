package schema

import (
	"fmt"
	"reflect"
	"strings"
)

// FromStruct generates a ComponentSchema by reflecting on a struct.
// It inspects exported fields and maps Go types to JSON Schema types.
// Fields with `schema` struct tags get additional metadata:
//
//	schema:"title=...,description=...,default=...,min=...,max=...,enum=a|b|c,widget=...,group=..."
//
// Interface and function fields are skipped.
func FromStruct(cfg any, meta ToolMeta) *ComponentSchema {
	s := &ComponentSchema{
		ID:          meta.ID,
		Title:       meta.DisplayName,
		Description: meta.Description,
		Type:        "object",
		ToolMeta:    &meta,
		Properties:  make(map[string]PropertySchema),
	}

	v := reflect.ValueOf(cfg)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return s
	}

	t := v.Type()
	groups := make(map[string]*ParameterGroup)
	var groupOrder []string

	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		// Skip fields with schema:"-"
		if field.Tag.Get("schema") == "-" {
			continue
		}

		// Skip interface, func, and channel types
		kind := field.Type.Kind()
		if kind == reflect.Interface || kind == reflect.Func || kind == reflect.Chan {
			continue
		}

		prop := fieldToProperty(field, v.Field(i))
		if prop == nil {
			continue
		}

		name := fieldName(field)
		s.Properties[name] = *prop

		// Process group tag
		tag := field.Tag.Get("schema")
		if groupID := tagValue(tag, "group"); groupID != "" {
			g, ok := groups[groupID]
			if !ok {
				g = &ParameterGroup{
					ID:    groupID,
					Label: groupLabel(groupID),
				}
				groups[groupID] = g
				groupOrder = append(groupOrder, groupID)
			}
			g.Fields = append(g.Fields, name)
		}
	}

	// Build groups in order
	for _, id := range groupOrder {
		s.Groups = append(s.Groups, *groups[id])
	}

	s.BuildRawJSON()
	return s
}

// fieldToProperty converts a struct field to a PropertySchema.
func fieldToProperty(field reflect.StructField, val reflect.Value) *PropertySchema {
	prop := &PropertySchema{}
	ft := field.Type

	// Unwrap pointer
	if ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}

	switch ft.Kind() {
	case reflect.Bool:
		prop.Type = "boolean"
		if val.IsValid() && val.CanInterface() {
			prop.Default = val.Interface()
		}
	case reflect.String:
		prop.Type = "string"
		if val.IsValid() && val.CanInterface() {
			if s := val.String(); s != "" {
				prop.Default = s
			}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		prop.Type = "integer"
		if val.IsValid() && val.CanInterface() {
			if n := val.Int(); n != 0 {
				prop.Default = int(n)
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		prop.Type = "integer"
		if val.IsValid() && val.CanInterface() {
			if n := val.Uint(); n != 0 {
				prop.Default = int(n)
			}
		}
	case reflect.Float32, reflect.Float64:
		prop.Type = "number"
		if val.IsValid() && val.CanInterface() {
			if f := val.Float(); f != 0 {
				prop.Default = f
			}
		}
	case reflect.Slice:
		prop.Type = "array"
		elemType := ft.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		prop.Items = &PropertySchema{Type: goTypeToSchemaType(elemType)}
		if prop.Items.Type == "object" && elemType.Kind() == reflect.Struct {
			prop.Items.Properties = structProperties(elemType)
		}
	case reflect.Map:
		prop.Type = "object"
	case reflect.Struct:
		prop.Type = "object"
		prop.Properties = structProperties(ft)
	default:
		return nil
	}

	// Parse schema tag
	tag := field.Tag.Get("schema")
	if tag != "" {
		applyTag(prop, tag)
	}

	// Don't auto-generate Title or Description from the field name.
	// The frontend derives a label from the JSON key via formatLabel(),
	// and auto-generated descriptions just duplicate that label.
	// Authors should use schema:"description=..." for meaningful help text.

	return prop
}

// structProperties generates PropertySchema entries for a nested struct.
func structProperties(t reflect.Type) map[string]PropertySchema {
	props := make(map[string]PropertySchema)
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		kind := f.Type.Kind()
		if kind == reflect.Interface || kind == reflect.Func || kind == reflect.Chan {
			continue
		}
		p := fieldToProperty(f, reflect.Zero(f.Type))
		if p != nil {
			props[fieldName(f)] = *p
		}
	}
	return props
}

// fieldName returns the JSON-friendly name for a struct field.
// Uses the json tag if present, otherwise converts to camelCase.
func fieldName(f reflect.StructField) string {
	if tag := f.Tag.Get("json"); tag != "" {
		name := strings.Split(tag, ",")[0]
		if name != "" && name != "-" {
			return name
		}
	}
	return toCamelCase(f.Name)
}

// toCamelCase converts a Go PascalCase name to camelCase.
func toCamelCase(s string) string {
	if s == "" {
		return s
	}
	// Find the boundary between leading uppercase and the rest.
	// "XMLParser" -> "xmlParser", "FuzzyThreshold" -> "fuzzyThreshold"
	runes := []rune(s)
	for i := range runes {
		if i == 0 {
			runes[i] = toLower(runes[i])
			continue
		}
		// If current is upper and next is lower, stop lowering
		if isUpper(runes[i]) && i+1 < len(runes) && isLower(runes[i+1]) {
			break
		}
		if isUpper(runes[i]) {
			runes[i] = toLower(runes[i])
		} else {
			break
		}
	}
	return string(runes)
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func toLower(r rune) rune {
	if isUpper(r) {
		return r + ('a' - 'A')
	}
	return r
}

func goTypeToSchemaType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	default:
		return "string"
	}
}

// applyTag parses a schema struct tag and applies values to the property.
// Format: schema:"title=...,description=...,default=...,min=...,max=...,enum=a|b|c,widget=..."
func applyTag(prop *PropertySchema, tag string) {
	for part := range strings.SplitSeq(tag, ",") {
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "title":
			prop.Title = val
		case "description":
			prop.Description = val
		case "widget":
			prop.Widget = val
		case "placeholder":
			prop.Placeholder = val
		case "enum":
			for e := range strings.SplitSeq(val, "|") {
				v := strings.TrimSpace(e)
				prop.Options = append(prop.Options, OptionItem{Value: v, Label: v})
			}
		case "min":
			if f, err := parseFloat(val); err == nil {
				prop.Min = &f
			}
		case "max":
			if f, err := parseFloat(val); err == nil {
				prop.Max = &f
			}
		case "default":
			prop.Default = parseDefault(val, prop.Type)
		case "showIf":
			// Format: showIf=field:value (show when field equals value)
			if field, eq, ok := strings.Cut(val, ":"); ok {
				prop.Visible = &ConditionExpr{Field: field, Eq: eq}
			}
		case "showIfEmpty":
			prop.Visible = &ConditionExpr{Field: val, Empty: new(true)}
		case "showIfSet":
			prop.Visible = &ConditionExpr{Field: val, Empty: new(false)}
		case "order":
			var n int
			if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
				prop.Order = &n
			}
		}
	}
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func parseDefault(s string, propType string) any {
	switch propType {
	case "boolean":
		return s == "true"
	case "integer":
		var n int
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
			return n
		}
		return 0
	case "number":
		var f float64
		if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
			return f
		}
		return 0.0
	default:
		return s
	}
}

// tagValue extracts a specific key's value from a schema tag.
func tagValue(tag, key string) string {
	for part := range strings.SplitSeq(tag, ",") {
		k, v, ok := strings.Cut(part, "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// groupLabel converts a group ID like "validation" to "Validation".
func groupLabel(id string) string {
	if id == "" {
		return ""
	}
	runes := []rune(id)
	runes[0] = toUpper(runes[0])
	return string(runes)
}

func toUpper(r rune) rune {
	if isLower(r) {
		return r - ('a' - 'A')
	}
	return r
}
