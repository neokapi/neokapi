package xliff

import "github.com/neokapi/neokapi/core/config"

// okapiXLIFFTransformer converts okf_xliff filter specs to xliff format config.
type okapiXLIFFTransformer struct{}

func (okapiXLIFFTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for key, val := range spec {
		switch key {
		// Supported native params — pass through
		case "preserveSpaceByDefault",
			"includeExtensions", "includeIts",
			"ignoreInputSegmentation", "fallbackToID", "forceUniqueIds",
			"useTranslationTargetState", "targetStateValue",
			"editAltTrans", "addAltTrans",
			"addTargetLanguage", "overrideTargetLanguage",
			"allowEmptyTargets", "alwaysAddTargets",
			"useCodeFinder", "codeFinderRules":
			result[key] = val

		// Okapi-only params — drop silently
		case "inlineCdata", "bilingualMode", "generateTarget",
			"escapeGT", "copySource",
			"useCustomParser", "protectApproved",
			"cdataSubfilter", "pcdataSubfilter",
			"escapingOutput", "useSdlProperties",
			"segmentationType", "useSegSource",
			"factoryClass", "quoteModeDefined", "quoteMode",
			"useSdlXliffWriter", "useSegsForSdlProps",
			"sdlSegConfValue", "sdlSegLockedValue", "sdlSegOriginValue",
			"useIwsXliffWriter", "iwsBlockFinished", "iwsBlockLockStatus",
			"iwsBlockMultipleExact", "iwsBlockTmScore", "iwsBlockTmScoreValue",
			"iwsIncludeMultipleExact", "iwsRemoveTmOrigin",
			"iwsTransStatusValue", "iwsTransTypeValue",
			"subAsTextUnit", "skipNoMrkSegSource",
			"alwaysUseSegSource", "outputSegmentationType",
			"targetStateMode", "addAltTransGMode",
			"balanceCodes", "mergeAdjacentCodes",
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
		config.OkapiFilterConfigKind("xliff"), config.FormatConfigKind("xliff"),
		okapiXLIFFTransformer{},
	)
}
