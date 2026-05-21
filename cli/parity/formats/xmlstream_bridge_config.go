//go:build parity

package formats

import (
	"fmt"
)

// xmlstreamBridgeConfig translates a neokapi-keyed xmlstream spec config
// map into the YAML-config key shape the okapi-bridge daemon's
// XmlStreamFilter (okf_xmlstream) expects.
//
// The upstream XmlStreamFilter is configured by a single YAML document
// (an Okapi TaggedFilterConfiguration). The bridge's ParameterApplier
// recognises this — when any incoming value is a complex object/array
// it deep-merges the whole param set into the filter's existing YAML
// config (params.toString() → merge → params.fromString()), keying by
// the Okapi YAML names (`elements`, `attributes`, `exclude_by_default`,
// `preserve_whitespace`). Scalar booleans (`preserve_whitespace`,
// `exclude_by_default`) are applied through the per-key path, which for
// markup params delegates to TaggedFilterConfiguration.setBooleanParameter
// (a live config-property write) — so they take effect either way.
//
// neokapi exposes ergonomic shorthands (camelCase top-level lists /
// booleans) that the upstream filter does not recognise. This
// translator expands each shorthand into the long-form Okapi YAML the
// bridge merges:
//
//	translatableElements: [a,b]    → exclude_by_default: true +
//	                                 elements: {a:{ruleTypes:[INCLUDE]}, …}
//	inlineElements: [a,b]          → elements: {a:{ruleTypes:[INLINE]}, …}
//	excludedElements: [a,b]        → elements: {a:{ruleTypes:[EXCLUDE]}, …}
//	preserveWhitespaceElements:[a] → elements: {a:{ruleTypes:[PRESERVE_WHITESPACE]}}
//	groupElements: [a,b]           → elements: {a:{ruleTypes:[GROUP]}, …}
//	translatableAttributes: [a,b]  → attributes: {a:{ruleTypes:[ATTRIBUTE_TRANS]}, …}
//	excludeByDefault: bool         → exclude_by_default: bool
//	preserveWhitespace: bool       → preserve_whitespace: bool
//	parser: {preserveWhitespace}   → preserve_whitespace: bool
//	elements / attributes (maps)   → merged through verbatim (already Okapi-shaped)
//
// All element-shorthand expansions accumulate into ONE `elements` map
// so multiple shorthands compose; an explicit `elements:` block merges
// last (its keys win on collision). The deep-merge on the bridge side
// preserves the filter's default rules and overlays these.
//
// The translator never mutates its input; it returns a fresh map.
func xmlstreamBridgeConfig(cfg map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(cfg))
	elements := map[string]any{}
	attributes := map[string]any{}
	// excludeByDefault tracks whether any key requires the bridge to flip
	// to opt-in extraction. translatableElements implies it (the native
	// shorthand is a whitelist — text extracts ONLY from listed
	// elements); an explicit excludeByDefault key sets it directly.
	excludeByDefault := false
	excludeByDefaultSet := false

	addElementRule := func(names []string, ruleType string) {
		for _, name := range names {
			elements[name] = map[string]any{"ruleTypes": []any{ruleType}}
		}
	}

	for key, val := range cfg {
		switch key {
		case "translatableElements":
			names, err := asStringList(val, key)
			if err != nil {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: %w", err)
			}
			// Native treats translatableElements as an opt-in whitelist:
			// only the listed elements are translatable, everything else
			// is skipped. The upstream-equivalent is exclude_by_default
			// plus an INCLUDE rule per listed element (declaring TEXTUNIT
			// alone would NOT exclude the rest — the default policy still
			// extracts every other text node).
			addElementRule(names, "INCLUDE")
			if !excludeByDefaultSet {
				excludeByDefault = true
			}

		case "inlineElements":
			names, err := asStringList(val, key)
			if err != nil {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: %w", err)
			}
			addElementRule(names, "INLINE")

		case "excludedElements":
			names, err := asStringList(val, key)
			if err != nil {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: %w", err)
			}
			addElementRule(names, "EXCLUDE")

		case "preserveWhitespaceElements":
			names, err := asStringList(val, key)
			if err != nil {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: %w", err)
			}
			addElementRule(names, "PRESERVE_WHITESPACE")

		case "groupElements":
			names, err := asStringList(val, key)
			if err != nil {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: %w", err)
			}
			addElementRule(names, "GROUP")

		case "translatableAttributes":
			names, err := asStringList(val, key)
			if err != nil {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: %w", err)
			}
			for _, name := range names {
				attributes[name] = map[string]any{"ruleTypes": []any{"ATTRIBUTE_TRANS"}}
			}

		case "excludeByDefault":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: excludeByDefault: expected bool, got %T", val)
			}
			excludeByDefault = b
			excludeByDefaultSet = true

		case "preserveWhitespace":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: preserveWhitespace: expected bool, got %T", val)
			}
			out["preserve_whitespace"] = b

		case "parser":
			m, ok := val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: parser: expected map, got %T", val)
			}
			if pw, ok := m["preserveWhitespace"]; ok {
				b, ok := pw.(bool)
				if !ok {
					return nil, fmt.Errorf("xmlstreamBridgeConfig: parser.preserveWhitespace: expected bool, got %T", pw)
				}
				out["preserve_whitespace"] = b
			}
			// assumeWellformed and other parser sub-keys are no-ops for
			// the bridge transport (Okapi's encoding/xml-equivalent
			// always treats malformed input as a parse error); they are
			// intentionally dropped rather than forwarded.

		case "elements":
			m, ok := val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: elements: expected map, got %T", val)
			}
			for k, v := range m {
				elements[k] = v
			}

		case "attributes":
			m, ok := val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("xmlstreamBridgeConfig: attributes: expected map, got %T", val)
			}
			for k, v := range m {
				attributes[k] = v
			}

		default:
			return nil, fmt.Errorf("xmlstreamBridgeConfig: unknown spec key %q", key)
		}
	}

	if excludeByDefault || excludeByDefaultSet {
		out["exclude_by_default"] = excludeByDefault
	}
	if len(elements) > 0 {
		out["elements"] = elements
	}
	if len(attributes) > 0 {
		out["attributes"] = attributes
	}

	return out, nil
}

// asStringList coerces the YAML decoder's []any / []string into a
// []string, erroring on any non-string element.
func asStringList(v any, label string) ([]string, error) {
	switch x := v.(type) {
	case []string:
		return x, nil
	case []any:
		out := make([]string, len(x))
		for i, item := range x {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d]: expected string, got %T", label, i, item)
			}
			out[i] = s
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s: expected string list, got %T", label, v)
	}
}
