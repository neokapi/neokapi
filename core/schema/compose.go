package schema

import (
	"maps"
	"slices"
	"sort"
)

// Variant is one selectable backend of a pluggable-backend tool — a
// segmentation engine, a translation provider, a QA check-set — that contributes
// its own parameter sub-schema under a shared discriminator field.
//
// Variants are the building block for the canonical "umbrella tool" shape: the
// tool owns only a small common schema plus a discriminator (e.g. "engine"),
// and each backend evolves its own [Variant.Params] independently in its own
// package. [ComposeVariants] assembles them into one form where the discriminator
// is a labeled select and each backend's fields appear, grouped, only when that
// backend is selected — so the user configures exactly one backend at a time and
// no per-field visibility is hand-written.
type Variant struct {
	Name        string // discriminator value that selects this variant
	Label       string // human label for the selector option
	Description string // selector option help text (→ ui:enum-descriptions)

	// GroupLabel labels the variant's own field group; empty falls back to Label.
	GroupLabel string

	// Params is the variant's own parameter schema (typically built with
	// [FromStruct] over the variant's config struct). nil means the variant has
	// no configurable parameters (it contributes only a selector option).
	Params *ComponentSchema

	// When overrides the visibility condition applied to this variant's section.
	// The default (nil) gates it on the discriminator equalling the variant's
	// Name. Set it when a backend's config is shared across several discriminator
	// values — e.g. an LLM param set that applies to both "llm" and "hybrid"
	// engines: When = {Any: [{Field:"engine",Eq:"llm"}, {Field:"engine",Eq:"hybrid"}]}.
	When *ConditionExpr
}

// ComposeVariants builds a single tool schema from a common base schema plus a
// set of variants selected by a discriminator field.
//
//   - The discriminator property (which must already exist on base, e.g. as the
//     "engine" field) becomes a labeled select whose options and descriptions
//     come from the variants, with defaultName as its default.
//   - Each variant's parameters are merged in and placed in their own group(s),
//     gated at the group level so the whole section appears only when the
//     discriminator selects that variant (master-detail; no empty headers). The
//     variant groups are inserted immediately after the group that holds the
//     discriminator, so a backend's config sits right under the selector.
//
// The base schema is not mutated; a new composed schema is returned.
func ComposeVariants(base *ComponentSchema, discriminator, defaultName string, variants []Variant) *ComponentSchema {
	out := *base
	out.Properties = make(map[string]PropertySchema, len(base.Properties))
	maps.Copy(out.Properties, base.Properties)
	out.Groups = append([]ParameterGroup(nil), base.Groups...)

	// Discriminator → labeled select sourced from the variants.
	if d, ok := out.Properties[discriminator]; ok {
		d.Widget = "select"
		if defaultName != "" {
			d.Default = defaultName
		}
		d.Options = make([]OptionItem, 0, len(variants))
		descs := map[string]string{}
		for _, v := range variants {
			d.Options = append(d.Options, OptionItem{Value: v.Name, Label: v.Label})
			if v.Description != "" {
				descs[v.Name] = v.Description
			}
		}
		if len(descs) > 0 {
			d.EnumDescriptions = descs
		}
		out.Properties[discriminator] = d
	}

	// Build the variant groups. Gating is at the GROUP level (master-detail): a
	// variant's whole section — header and fields — is shown only while its gate
	// holds, so an unselected backend renders nothing at all (no empty header).
	// Collected groups are inserted after the discriminator's group.
	var variantGroups []ParameterGroup
	for _, v := range variants {
		if v.Params == nil || len(v.Params.Properties) == 0 {
			continue
		}
		// The gate that makes this variant's section visible: its custom When, or
		// the default discriminator == Name.
		gate := v.When
		if gate == nil {
			gate = &ConditionExpr{Field: discriminator, Eq: v.Name}
		}

		// Merge the variant's properties (the group gates the section, so fields
		// carry no per-field visibility of their own).
		merged := map[string]bool{}
		for _, name := range sortedVariantFields(v.Params.Properties) {
			if _, dup := out.Properties[name]; dup {
				// A field name shared with the base or another variant would
				// clobber a distinct definition; skip rather than silently
				// overwrite. Variant authors keep parameter names unique.
				continue
			}
			out.Properties[name] = v.Params.Properties[name]
			merged[name] = true
		}
		if len(merged) == 0 {
			continue
		}

		// Preserve the variant's own groups (namespaced to avoid collisions with
		// the base or other variants), each gated; then collect anything ungrouped
		// into a single group labelled for the variant, also gated. A flat-param
		// variant (the common case — an engine) yields one group.
		inGroup := map[string]bool{}
		for _, g := range v.Params.Groups {
			fields := make([]string, 0, len(g.Fields))
			for _, f := range g.Fields {
				if merged[f] {
					fields = append(fields, f)
					inGroup[f] = true
				}
			}
			if len(fields) == 0 {
				continue
			}
			ng := g
			ng.ID = v.Name + ":" + g.ID
			ng.Fields = fields
			ng.Visible = gate
			variantGroups = append(variantGroups, ng)
		}
		var ungrouped []string
		for _, name := range sortedVariantFields(v.Params.Properties) {
			if merged[name] && !inGroup[name] {
				ungrouped = append(ungrouped, name)
			}
		}
		if len(ungrouped) > 0 {
			label := v.GroupLabel
			if label == "" {
				label = v.Label
			}
			variantGroups = append(variantGroups, ParameterGroup{ID: v.Name, Label: label, Fields: ungrouped, Visible: gate})
		}
	}

	out.Groups = insertAfterDiscriminator(out.Groups, discriminator, variantGroups)
	out.RawJSON = nil
	out.BuildRawJSON()
	return &out
}

// sortedVariantFields orders a variant's property names by ui:order then name,
// so the composed schema is deterministic regardless of map iteration order.
func sortedVariantFields(props map[string]PropertySchema) []string {
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		oi, oj := order(props[names[i]]), order(props[names[j]])
		if oi != oj {
			return oi < oj
		}
		return names[i] < names[j]
	})
	return names
}

func order(p PropertySchema) int {
	if p.Order != nil {
		return *p.Order
	}
	return 1 << 30
}

// insertAfterDiscriminator places the variant groups directly after the group
// that contains the discriminator field, falling back to appending when no group
// holds it.
func insertAfterDiscriminator(groups []ParameterGroup, discriminator string, variantGroups []ParameterGroup) []ParameterGroup {
	if len(variantGroups) == 0 {
		return groups
	}
	at := -1
	for i := range groups {
		if slices.Contains(groups[i].Fields, discriminator) {
			at = i
			break
		}
	}
	if at < 0 {
		return append(groups, variantGroups...)
	}
	out := make([]ParameterGroup, 0, len(groups)+len(variantGroups))
	out = append(out, groups[:at+1]...)
	out = append(out, variantGroups...)
	out = append(out, groups[at+1:]...)
	return out
}
