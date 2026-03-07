package xliff2

import "github.com/gokapi/gokapi/core/config"

// okapiXLIFF2Transformer converts OkfXliff2FilterConfig specs to Xliff2FormatConfig.
// XLIFF 2.0 has no configurable parameters in the native implementation,
// so the transformer drops all Okapi-only parameters silently.
type okapiXLIFF2Transformer struct{}

func (okapiXLIFF2Transformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, val := range spec {
		switch key {
		// Okapi-only params — drop silently (no native equivalent)
		case "useCodeFinder", "codeFinderRules",
			"useSubfilter", "subfilterConfig",
			"maxValidation", "outputSegmentationType",
			"quoteMode", "quoteModeDefined",
			"copySource", "includeNoTranslate":
			continue

		// Shared params — keep as-is
		default:
			result[key] = val
		}
	}

	return result, nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("xliff2"), config.FormatConfigKind("xliff2"),
		okapiXLIFF2Transformer{},
	)
}
