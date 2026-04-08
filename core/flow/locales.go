package flow

import (
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
)

// ResolveFlowLocales inspects a flow's tool chain and determines which locale
// sets to process. Returns a slice of locale sets — one per execution pass.
//
// Return semantics:
//   - nil → source-only flow (all tools are monolingual), run once with no target
//   - [["en","qps"]] → one pass with a fixed default locale
//   - [["en","de"],["en","fr"],...] → one pass per project target
//   - [["en","de","fr","ja",...]] → one multilingual pass with all locales
//
// Resolution rules:
//  1. If all tools are monolingual → nil
//  2. If any bilingual tool has no default → one pass per project target (paired with source)
//  3. If all bilingual tools have defaults → one pass per unique default
//  4. If any tool is multilingual → one pass with source + all project targets
//  5. Mixed → union of all needed passes
func ResolveFlowLocales(spec *StepsSpec, toolInfos map[registry.ToolID]registry.ToolInfo, sourceLocale string, projectTargets []string) [][]string {
	if spec == nil || len(spec.Steps) == 0 {
		return nil
	}

	// Collect tool names from the flow steps.
	toolNames := collectToolNames(spec.Steps)

	hasMonoOnly := true
	hasBilingualNoDefault := false
	hasMultilingual := false
	var bilingualDefaults []string

	for _, name := range toolNames {
		info, ok := toolInfos[registry.ToolID(name)]
		if !ok {
			// Unknown tool — assume bilingual (conservative, iterates all targets).
			hasBilingualNoDefault = true
			hasMonoOnly = false
			continue
		}

		switch info.Cardinality {
		case schema.Monolingual:
			// Doesn't affect target iteration.
		case schema.Bilingual:
			hasMonoOnly = false
			if info.DefaultLocale != "" {
				bilingualDefaults = append(bilingualDefaults, string(info.DefaultLocale))
			} else {
				hasBilingualNoDefault = true
			}
		case schema.Multilingual:
			hasMonoOnly = false
			hasMultilingual = true
		default:
			// No cardinality set — assume bilingual (conservative).
			hasBilingualNoDefault = true
			hasMonoOnly = false
		}
	}

	// Rule 1: all monolingual → source-only, run once.
	if hasMonoOnly {
		return nil
	}

	// Rule 4: any multilingual → one pass with all locales.
	if hasMultilingual {
		all := []string{sourceLocale}
		all = append(all, projectTargets...)
		return [][]string{all}
	}

	// Collect target locales.
	var passes [][]string
	seen := make(map[string]bool)

	// Rule 2: any bilingual without default → include all project targets.
	if hasBilingualNoDefault {
		for _, t := range projectTargets {
			if !seen[t] {
				seen[t] = true
				passes = append(passes, []string{sourceLocale, t})
			}
		}
	}

	// Rule 3/5: bilingual defaults → include each default locale.
	for _, def := range bilingualDefaults {
		if !seen[def] {
			seen[def] = true
			passes = append(passes, []string{sourceLocale, def})
		}
	}

	if len(passes) == 0 {
		return nil
	}
	return passes
}

// collectToolNames extracts all tool names from a step list, recursing into
// parallel steps.
func collectToolNames(steps []FlowStep) []string {
	var names []string
	for _, step := range steps {
		if step.Tool != "" {
			names = append(names, step.Tool)
		}
		if len(step.Parallel) > 0 {
			names = append(names, collectToolNames(step.Parallel)...)
		}
	}
	return names
}

// BuildToolInfoMap creates a tool name → ToolInfo lookup from the registry.
func BuildToolInfoMap(reg *registry.ToolRegistry) map[registry.ToolID]registry.ToolInfo {
	m := make(map[registry.ToolID]registry.ToolInfo)
	for _, info := range reg.ListWithSchemas() {
		m[info.Name] = info
	}
	return m
}
