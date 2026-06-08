package flow

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
)

// Data-flow validation (tool/data-model redesign, phase 4). A flow's tools
// declare the facets they consume and produce (schema.IOPort). A flow is valid
// only when every tool's *required* (non-optional) consumed facet is satisfied
// by something upstream: an earlier tool's Produces, the ingest settle stage
// (segmentation persisted at extract, AD-026 §4), or the source binding. A
// required facet with no producer is a hard error at load/build.

// facetKey identifies a stand-off type (overlay, annotation, or pseudo-port) by
// type name and side for availability matching.
func facetKey[T ~string](t T, side model.Side) string {
	return string(t) + "@" + side.String()
}

func ioKey(f schema.IOPort) string { return facetKey(f.Type, f.Side) }

// sourceDerivableFacets are the facets a generic source can already carry —
// committed targets (bilingual interchange), source-side segmentation/alignment
// from extract, and any terms/entities/alt-translations persisted with the
// source. They are assumed available when the flow does not pin its source
// binding (the source is supplied at invocation). Tool-computed facets (qa,
// tm-match, comparison, counts, brand-voice, …) are deliberately NOT here: they
// must be produced by an upstream tool.
func sourceDerivableFacets() map[string]bool {
	return map[string]bool{
		facetKey(schema.PortTarget, model.SideTarget):         true,
		facetKey(model.OverlaySegmentation, model.SideSource): true,
		facetKey(model.OverlayAlignment, model.SideSource):    true,
		facetKey(model.OverlayTerm, model.SideSource):         true,
		facetKey(model.OverlayEntity, model.SideSource):       true,
		facetKey(model.AnnoAltTranslation, model.SideSource):  true,
	}
}

// bindingProvidedFacets returns the facets a declared source binding makes
// available to the first tool. An empty/unspecified source means the binding is
// supplied at invocation, so the generic source-derivable set is assumed.
func bindingProvidedFacets(source string) map[string]bool {
	s := strings.ToLower(strings.TrimSpace(source))
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i] // strip "scheme:path"
	}
	switch s {
	case "":
		// Unspecified: invocation-supplied source. Assume a generic source.
		return sourceDerivableFacets()
	case "file":
		// A plain (monolingual) file carries source content only — no target,
		// no stand-off facets.
		return map[string]bool{}
	case "none":
		return map[string]bool{}
	case "store", "klz":
		// The content store / archive carries the source plus every persisted
		// facet, including tool-computed ones.
		m := sourceDerivableFacets()
		for _, t := range []string{string(model.OverlayQA), model.AnnoTMMatch, model.AnnoWordCount} {
			m[facetKey(t, model.SideTarget)] = true
			m[facetKey(t, model.SideSource)] = true
		}
		return m
	default:
		// Bilingual interchange (xliff, po, tmx, tbx, …): source + committed
		// target + segmentation + alignment.
		return map[string]bool{
			facetKey(schema.PortTarget, model.SideTarget):         true,
			facetKey(model.OverlaySegmentation, model.SideSource): true,
			facetKey(model.OverlayAlignment, model.SideSource):    true,
		}
	}
}

// ValidateDataFlow checks the flow's facet IO contract: every required consumed
// facet must be produced upstream (an earlier tool, the ingest settle stage, or
// the source binding). Tools unknown to the registry (e.g. plugin tools whose
// contract is not loaded) are skipped rather than rejected. A nil registry
// disables the check.
func (d *FlowDefinition) ValidateDataFlow(reg *registry.ToolRegistry) error {
	if reg == nil {
		return nil
	}
	sourceTransforms, main, err := d.StagedToolNodes()
	if err != nil {
		return err
	}

	available := map[string]bool{}
	// Ingest settle stage (AD-026 §4): segmentation/normalization persisted at
	// extract is available to every tool.
	available[facetKey(model.OverlaySegmentation, model.SideSource)] = true
	// Source binding contributes its provided facets.
	source := ""
	if d.Binding != nil {
		source = d.Binding.Source
	}
	for k := range bindingProvidedFacets(source) {
		available[k] = true
	}

	ordered := make([]string, 0, len(sourceTransforms)+len(main))
	ordered = append(ordered, sourceTransforms...)
	ordered = append(ordered, main...)

	for _, name := range ordered {
		info := reg.ToolInfo(registry.ToolID(name))
		if info == nil {
			continue // unknown/plugin tool: contract not available, skip
		}
		for _, c := range info.Consumes {
			if c.Optional {
				continue
			}
			if !available[ioKey(c)] {
				return fmt.Errorf("flow %q: tool %q requires facet %s, but no upstream tool produces it%s",
					d.Name, name, ioKey(c), bindingHint(source))
			}
		}
		for _, p := range info.Produces {
			available[ioKey(p)] = true
		}
	}
	return nil
}

func bindingHint(source string) string {
	if strings.TrimSpace(source) == "" {
		return ""
	}
	return fmt.Sprintf(" and the %q source binding does not provide it", source)
}
