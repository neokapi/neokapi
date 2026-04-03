package xml

import "github.com/neokapi/neokapi/core/config"

// okapiXMLTransformer converts OkfXmlFilterConfig and OkfXmlstreamFilterConfig
// specs to XmlFormatConfig.
type okapiXMLTransformer struct{}

func (okapiXMLTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, val := range spec {
		switch key {
		// Okapi-only params — drop silently
		case "configFile", "configId":
			continue
		case "useCodeFinder", "codeFinderRules":
			continue
		case "quoteMode", "quoteModeDefined":
			continue
		case "global_cdata_subfilter", "global_pcdata_subfilter":
			continue
		case "exclude_by_default":
			result["excludeByDefault"] = val

		// Parser section: keep shared params, drop okapi-only
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
					// Go XML parser always handles this — drop
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
	// Register transforms for both okf_xml and okf_xmlstream -> xml
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("xml"), config.FormatConfigKind("xml"),
		okapiXMLTransformer{},
	)
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("xmlstream"), config.FormatConfigKind("xml"),
		okapiXMLTransformer{},
	)
}
