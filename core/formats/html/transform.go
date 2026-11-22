package html

import (
	"github.com/neokapi/neokapi/core/config"

	coreschema "github.com/neokapi/neokapi/core/schema"
)

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
				result[coreschema.SnakeCase(key)] = val
				continue
			}
			cleaned := make(map[string]any)
			for pk, pv := range m {
				switch pk {
				case "assumeWellformed":
					// Go parser always handles malformed HTML — drop
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
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("html"), config.FormatConfigKind("html"),
		okapiHTMLTransformer{},
	)
}
