package sievepen

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"os"
)

// TMXMapping resolves a TMX inline element (with optional `type` attribute)
// to a semantic vocabulary type used by neokapi's inline-span model.
//
// Resolution precedence:
//  1. element_type_attr[element][typeAttr] — most specific
//  2. element_types[element] — fall-back per element
//  3. Fallback — final terminal fall-back
type TMXMapping struct {
	ElementTypes    map[string]string            `json:"element_types"`
	ElementTypeAttr map[string]map[string]string `json:"element_type_attr"`
	Fallback        string                       `json:"fallback"`
}

//go:embed tmx_mappings/default.json
var defaultTMXMappingJSON []byte

// DefaultTMXMapping returns the embedded default TMX mapping.
func DefaultTMXMapping() (*TMXMapping, error) {
	var m TMXMapping
	if err := json.Unmarshal(defaultTMXMappingJSON, &m); err != nil {
		return nil, fmt.Errorf("parse default TMX mapping: %w", err)
	}
	return &m, nil
}

// LoadTMXMapping loads the default mapping and merges user overrides from
// a JSON file at the given path. An empty path returns the default.
func LoadTMXMapping(path string) (*TMXMapping, error) {
	base, err := DefaultTMXMapping()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return base, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read TMX mapping %s: %w", path, err)
	}
	var override TMXMapping
	if err := json.Unmarshal(raw, &override); err != nil {
		return nil, fmt.Errorf("parse TMX mapping %s: %w", path, err)
	}
	mergeMapping(base, &override)
	return base, nil
}

func mergeMapping(base, override *TMXMapping) {
	for k, v := range override.ElementTypes {
		if base.ElementTypes == nil {
			base.ElementTypes = make(map[string]string)
		}
		base.ElementTypes[k] = v
	}
	for el, attrs := range override.ElementTypeAttr {
		if base.ElementTypeAttr == nil {
			base.ElementTypeAttr = make(map[string]map[string]string)
		}
		if base.ElementTypeAttr[el] == nil {
			base.ElementTypeAttr[el] = make(map[string]string)
		}
		maps.Copy(base.ElementTypeAttr[el], attrs)
	}
	if override.Fallback != "" {
		base.Fallback = override.Fallback
	}
}

// Resolve returns the semantic type for a TMX element with an optional
// `type` attribute. It applies the precedence rules documented on TMXMapping.
func (m *TMXMapping) Resolve(element, typeAttr string) string {
	if m == nil {
		return ""
	}
	if typeAttr != "" {
		if attrMap, ok := m.ElementTypeAttr[element]; ok {
			if t, ok := attrMap[typeAttr]; ok {
				return t
			}
		}
	}
	if t, ok := m.ElementTypes[element]; ok {
		return t
	}
	return m.Fallback
}
