package po

import "github.com/gokapi/gokapi/core/config"

// okapiPOTransformer converts okf_po filter config specs to native PO format config.
// The PO filter has few config parameters, so most Okapi-only params are dropped.
type okapiPOTransformer struct{}

func (okapiPOTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, val := range spec {
		switch key {
		// Okapi-only params — drop silently
		case "useCodeFinder", "codeFinderRules",
			"bilingualMode", "protectApproved",
			"outputBOMonMsgid", "genericOutput",
			"localizeHeader", "wrapContent":
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
		config.OkapiFilterConfigKind("po"), config.FormatConfigKind("po"),
		okapiPOTransformer{},
	)
}
