package json

import (
	"github.com/neokapi/neokapi/core/config"

	coreschema "github.com/neokapi/neokapi/core/schema"
)

// okapiJSONTransformer converts OkfJsonFilterConfig specs to JsonFormatConfig.
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
			result[coreschema.SnakeCase(key)] = val
		}
	}
	return result, nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("json"), config.FormatConfigKind("json"),
		okapiJSONTransformer{},
	)
}
