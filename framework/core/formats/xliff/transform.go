package xliff

import "github.com/neokapi/neokapi/core/config"

// okapiXLIFFTransformer converts okf_xliff filter specs to xliff format config.
// The native XLIFF format currently has no configurable parameters,
// so all Okapi-specific wrapper params are dropped.
type okapiXLIFFTransformer struct{}

func (okapiXLIFFTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for key, val := range spec {
		switch key {
		// Okapi-only params — drop silently
		case "inlineCdata", "bilingualMode", "generateTarget",
			"escapeGT", "addTargetLanguage", "copySource",
			"allowEmptyTargets", "overrideTargetLanguage",
			"useCustomParser", "protectApproved",
			"cdataSubfilter", "pcdataSubfilter",
			"useCodeFinder", "codeFinderRules",
			"escapingOutput", "useSdlProperties",
			"segmentationType", "useSegSource":
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
