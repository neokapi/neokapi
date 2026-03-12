package properties

import "github.com/gokapi/gokapi/core/config"

// okapiPropertiesTransformer converts OkfPropertiesFilterConfig specs to
// PropertiesFormatConfig. Most parameters pass through; okapi-specific
// params that have no native equivalent are dropped silently.
type okapiPropertiesTransformer struct{}

func (okapiPropertiesTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for key, val := range spec {
		switch key {
		// Okapi-only params — drop silently
		case "escapeExtendedChars", "bom",
			"subfilter",
			"useKeyCondition", "extractOnlyMatchingKey", "keyCondition",
			"commentsAreNotes", "extractAllPairs":
			continue
		default:
			result[key] = val
		}
	}
	return result, nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("properties"), config.FormatConfigKind("properties"),
		okapiPropertiesTransformer{},
	)
}
