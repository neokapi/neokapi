package json

import "github.com/gokapi/gokapi/core/config"

// okapiJSONTransformer converts okapi/json-v1 specs to gokapi/json-v1.
// The native JSON format supports the same parameter set as the okapi bridge,
// so most parameters pass through unchanged. Only okapi-specific wrapper
// params are dropped.
type okapiJSONTransformer struct{}

func (okapiJSONTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for key, val := range spec {
		switch key {
		// Okapi-only params — drop silently
		case "escapeExtendedChars", "bom":
			continue
		default:
			result[key] = val
		}
	}
	return result, nil
}

func init() {
	config.DefaultTransforms.Register(
		"okapi/json-v1", "gokapi/json-v1",
		okapiJSONTransformer{},
	)
}
