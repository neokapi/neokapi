package xml

import (
	"github.com/neokapi/neokapi/core/config"

	coreschema "github.com/neokapi/neokapi/core/schema"
)

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
			result["exclude_by_default"] = val

		// Parser section: keep shared params, drop okapi-only
		case "parser":
			m, ok := val.(map[string]any)
			if !ok {
				result[coreschema.SnakeCase(key)] = val
				continue
			}
			cleaned := make(map[string]any)
			for pk, pv := range m {
				switch pk {
				case "assumeWellformed":
					// Go XML parser always handles this — drop
				default:
					cleaned[coreschema.SnakeCase(pk)] = pv
				}
			}
			if len(cleaned) > 0 {
				result[coreschema.SnakeCase(key)] = cleaned
			}

		// Element and attribute rules: one rule map per name, whose
		// keys (ruleTypes, idAttributes, ...) are spelled in snake_case.
		case "elements", "attributes":
			result[key] = snakeCaseRules(val)

		// Shared params — keep as-is
		default:
			result[coreschema.SnakeCase(key)] = val
		}
	}

	return result, nil
}

// snakeCaseRules spells the keys of each rule map in an Okapi elements or
// attributes section in snake_case, leaving the element and attribute names
// that key the section as they are.
func snakeCaseRules(val any) any {
	rules, ok := val.(map[string]any)
	if !ok {
		return val
	}
	out := make(map[string]any, len(rules))
	for name, rule := range rules {
		if m, ok := rule.(map[string]any); ok {
			out[name] = coreschema.SnakeCaseKeys(m)
			continue
		}
		out[name] = rule
	}
	return out
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
