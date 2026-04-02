package xliff2

import "github.com/neokapi/neokapi/core/config"

// okapiXLIFF2Transformer converts OkfXliff2FilterConfig specs to Xliff2FormatConfig.
type okapiXLIFF2Transformer struct{}

func (okapiXLIFF2Transformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, val := range spec {
		switch key {
		// Supported native params — pass through
		case "forceUniqueIds", "ignoreTagTypeMatch",
			"discardInvalidTargets",
			"writeOriginalData",
			"useCodeFinder", "codeFinderRules":
			result[key] = val

		// Okapi-only params — drop silently (no native equivalent)
		case "needsSegmentation", "simplifyTags",
			"subfilterOverwriteTarget", "maxValidation",
			"useSubfilter", "subfilterConfig",
			"outputSegmentationType",
			"quoteMode", "quoteModeDefined",
			"copySource", "includeNoTranslate",
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
		config.OkapiFilterConfigKind("xliff2"), config.FormatConfigKind("xliff2"),
		okapiXLIFF2Transformer{},
	)
}
