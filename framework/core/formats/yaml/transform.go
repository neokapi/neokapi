package yaml

import "github.com/gokapi/gokapi/core/config"

// okapiYAMLTransformer converts okf_yaml filter config specs to native YAML
// format config. Most Okapi YAML filter parameters map directly to native
// equivalents. Okapi-only params (subFilterProcessLiteralAsBlock, bom) are
// dropped since they have no native equivalent.
type okapiYAMLTransformer struct{}

func (okapiYAMLTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for key, val := range spec {
		switch key {
		// Okapi-only params — drop silently
		case "subFilterProcessLiteralAsBlock", "bom", "escapeExtendedChars":
			continue
		// Map okapi subfilter values to native equivalents
		case "subfilter":
			if s, ok := val.(string); ok {
				switch s {
				case "okf_html":
					result["subfilter"] = "html"
				default:
					result["subfilter"] = s
				}
			}
		default:
			result[key] = val
		}
	}
	return result, nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("yaml"), config.FormatConfigKind("yaml"),
		okapiYAMLTransformer{},
	)
}
