package tmx

import "github.com/neokapi/neokapi/core/config"

// okapiTMXTransformer converts OkfTmxFilterConfig specs to TmxFormatConfig.
type okapiTMXTransformer struct{}

func (okapiTMXTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, val := range spec {
		switch key {
		// Supported native params — pass through
		case "processAllTargets", "exitOnInvalid",
			"escapeGT",
			"useCodeFinder", "codeFinderRules":
			result[key] = val

		// Okapi-only params — drop silently
		case "segType", "consolidateDpSkeleton", "propValueSep",
			"mergeAdjacentCodes",
			"moveLeadingAndTrailingCodesToSkeleton", "simplifierRules":
			continue

		default:
			result[key] = val
		}
	}

	return result, nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("tmx"), config.FormatConfigKind("tmx"),
		okapiTMXTransformer{},
	)
}
