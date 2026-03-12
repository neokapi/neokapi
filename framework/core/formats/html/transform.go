package html

import "github.com/gokapi/gokapi/core/config"

// okapiHTMLTransformer converts OkfHtmlFilterConfig specs to HtmlFormatConfig.
// It keeps shared parameters (parser, elements, attributes, codeFinder) and
// drops okapi-only parameters that have no native equivalent.
type okapiHTMLTransformer struct{}

func (okapiHTMLTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, val := range spec {
		switch key {
		// Okapi-only params — drop silently
		case "quoteMode", "quoteModeDefined":
			continue

		// Parser section: keep shared params, drop okapi-only ones
		case "parser":
			m, ok := val.(map[string]any)
			if !ok {
				result[key] = val
				continue
			}
			cleaned := make(map[string]any)
			for pk, pv := range m {
				switch pk {
				case "assumeWellformed":
					// Go parser always handles malformed HTML — drop
				default:
					cleaned[pk] = pv
				}
			}
			if len(cleaned) > 0 {
				result[key] = cleaned
			}

		// Shared params — keep as-is
		default:
			result[key] = val
		}
	}

	return result, nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("html"), config.FormatConfigKind("html"),
		okapiHTMLTransformer{},
	)
}
