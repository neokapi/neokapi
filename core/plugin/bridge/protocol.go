// Package bridge implements a JVM subprocess manager that wraps Okapi Framework
// Java filters as neokapi DataFormatReader/DataFormatWriter implementations.
// Communication uses gRPC with proto-generated types.
package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/schema"
)

// encodeFilterParams converts map[string]any to map[string]string for proto.
// Complex values are JSON-encoded. Parameters should use the hierarchical
// structure matching the filter's JSON schema (section objects with nested
// properties), not flat Okapi parameter names.
func encodeFilterParams(params map[string]any) map[string]string {
	if len(params) == 0 {
		return nil
	}
	result := make(map[string]string, len(params))
	for k, v := range params {
		switch val := v.(type) {
		case string:
			result[k] = val
		default:
			data, err := json.Marshal(val)
			if err != nil {
				result[k] = fmt.Sprintf("%v", val)
			} else {
				result[k] = string(data)
			}
		}
	}
	return result
}

// extractParamTypes builds a type map from the tool schema for the given params.
// The returned map has entries like {"regEx":"boolean","count":"integer"} which
// tells the Java side what Okapi parameter suffix to use (.b, .i, or bare).
func extractParamTypes(s *schema.ComponentSchema, params map[string]any) map[string]string {
	if s == nil || len(s.Properties) == 0 || len(params) == 0 {
		return nil
	}
	types := make(map[string]string, len(params))
	for k := range params {
		if prop, ok := s.Properties[k]; ok && prop.Type != "" {
			types[k] = prop.Type
		}
	}
	if len(types) == 0 {
		return nil
	}
	return types
}
