package markdown

import "github.com/gokapi/gokapi/core/config"

// okapiMarkdownTransformer converts OkfMarkdownFilterConfig specs to
// MarkdownFormatConfig. Most parameters pass through with key mapping
// from Okapi naming to native naming. Okapi-only parameters that have
// no native equivalent are dropped silently.
type okapiMarkdownTransformer struct{}

func (okapiMarkdownTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for key, val := range spec {
		switch key {
		// Okapi-only params — drop silently
		case "useCodeFinder", "generateHeaderAnchors", "bom":
			continue

		// Okapi "extractImageAltText" maps to native "translateImageAlt"
		case "extractImageAltText":
			result["translateImageAlt"] = val

		// Okapi "translateMetadataHeader" maps to native "translateFrontMatter"
		case "translateMetadataHeader":
			result["translateFrontMatter"] = val

		// Shared params — keep as-is
		default:
			result[key] = val
		}
	}
	return result, nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("markdown"), config.FormatConfigKind("markdown"),
		okapiMarkdownTransformer{},
	)
}
