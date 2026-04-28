package i18n

import (
	"maps"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/schema"
)

// Leaf field names on a translatable metadata node. Mirrors the canonical
// metadata JSON keys produced by the //go:generate builtin generator and
// the shape plugins ship in their manifest.json. Extraction regexes in
// core/i18n/i18n.kapi target these same leaves.
const (
	fieldDisplayName = "displayName"
	fieldDescription = "description"
	fieldTitle       = "title"
	fieldLabel       = "label"
)

// LocalizeComponentSchema returns a deep copy of s with every translatable
// leaf rewritten via t. Non-translatable fields (IDs, defaults, types,
// conditions, layout hints, validation constraints) are preserved.
//
// Scope derivation:
//   - If s.ToolMeta != nil: base scope = "tools.<ToolMeta.ID>".
//   - Else: base scope = "" (schema with no identifiable owner — lookups
//     will miss and fall back to source).
//
// Property-level fields extend the scope: tools.<id>.properties.<name>.title,
// tools.<id>.groups.<id>.label, etc. Matches the dot-separated full key
// path the JSON filter emits (useKeyAsName=true, default path separator).
func LocalizeComponentSchema(s *schema.ComponentSchema, t Translator) *schema.ComponentSchema {
	if s == nil {
		return nil
	}
	if t == nil {
		t = NoopTranslator{}
	}

	out := *s
	base := baseScope(s)

	if s.ToolMeta != nil {
		tm := *s.ToolMeta
		if tm.DisplayName != "" {
			tm.DisplayName = t.T(join(base, fieldDisplayName), tm.DisplayName)
		}
		if tm.Description != "" {
			tm.Description = t.T(join(base, fieldDescription), tm.Description)
		}
		out.ToolMeta = &tm
	}

	// Title / Description on the component itself share the same leaf keys.
	if out.Title != "" {
		out.Title = t.T(join(base, fieldTitle), out.Title)
	}
	if out.Description != "" {
		out.Description = t.T(join(base, fieldDescription), out.Description)
	}

	if len(s.Groups) > 0 {
		groups := make([]schema.ParameterGroup, len(s.Groups))
		for i, g := range s.Groups {
			gs := g
			groupScope := join(base, "groups", g.ID)
			if gs.Label != "" {
				gs.Label = t.T(join(groupScope, fieldLabel), gs.Label)
			}
			if gs.Description != "" {
				gs.Description = t.T(join(groupScope, fieldDescription), gs.Description)
			}
			groups[i] = gs
		}
		out.Groups = groups
	}

	if len(s.Properties) > 0 {
		out.Properties = make(map[string]schema.PropertySchema, len(s.Properties))
		for name, p := range s.Properties {
			out.Properties[name] = localizeProperty(p, t, join(base, "properties", name))
		}
	}

	return &out
}

// localizeProperty translates a single property schema's translatable leaves
// under the given base scope. Nested properties (object types) recurse.
func localizeProperty(p schema.PropertySchema, t Translator, base Scope) schema.PropertySchema {
	if p.Title != "" {
		p.Title = t.T(join(base, fieldTitle), p.Title)
	}
	if p.Description != "" {
		p.Description = t.T(join(base, fieldDescription), p.Description)
	}
	if len(p.Options) > 0 {
		opts := make([]schema.OptionItem, len(p.Options))
		for i, o := range p.Options {
			if o.Label != "" {
				// Option scope uses the option's value as the segment; for
				// non-string values fall back to the index. Deterministic and
				// matches what the JSON filter emits when iterating options.
				segment := stringifyValue(o.Value, i)
				o.Label = t.T(join(base, "options", segment, fieldLabel), o.Label)
			}
			opts[i] = o
		}
		p.Options = opts
	}
	if len(p.EnumDescriptions) > 0 {
		out := make(map[string]string, len(p.EnumDescriptions))
		for k, v := range p.EnumDescriptions {
			out[k] = t.T(join(base, "enumDescriptions", k), v)
		}
		p.EnumDescriptions = out
	}
	if len(p.Properties) > 0 {
		children := make(map[string]schema.PropertySchema, len(p.Properties))
		for name, child := range p.Properties {
			children[name] = localizeProperty(child, t, join(base, "properties", name))
		}
		p.Properties = children
	}
	if p.Items != nil {
		item := localizeProperty(*p.Items, t, join(base, "items"))
		p.Items = &item
	}
	// Defensive copy of any remaining reference types on the returned value
	// so callers can't mutate the registry's backing data by accident.
	if len(p.WidgetOptions) > 0 {
		p.WidgetOptions = maps.Clone(p.WidgetOptions)
	}
	return p
}

func baseScope(s *schema.ComponentSchema) Scope {
	if s.ToolMeta != nil && s.ToolMeta.ID != "" {
		return Scope("tools." + s.ToolMeta.ID)
	}
	return ""
}

// join builds a dot-separated Scope, skipping empty segments. Matches
// the format produced by the JSON filter's default useKeyAsName output
// (parent.child.leaf) so msgctxt lookups hit without re-normalization.
func join(parts ...any) Scope {
	segs := make([]string, 0, len(parts))
	for _, p := range parts {
		var s string
		switch v := p.(type) {
		case Scope:
			s = string(v)
		case string:
			s = v
		}
		if s != "" {
			segs = append(segs, s)
		}
	}
	return Scope(strings.Join(segs, "."))
}

func stringifyValue(v any, fallbackIndex int) string {
	switch x := v.(type) {
	case string:
		if x != "" {
			return x
		}
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.Itoa(int(x))
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return strconv.Itoa(fallbackIndex)
}
