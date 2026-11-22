package xliff2

import (
	"github.com/neokapi/neokapi/core/config"

	coreschema "github.com/neokapi/neokapi/core/schema"
)

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
			result[coreschema.SnakeCase(key)] = val

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
			result[coreschema.SnakeCase(key)] = val
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
