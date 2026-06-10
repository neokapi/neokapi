package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// textPlan builds the EditPlan for a text-level transform applied to the
// source and/or a set of target locales — the shared shape of the simple
// transformers (case, line-break, URI, full-width, encoding, search-replace,
// regex rules). The source rewrite is the opaque ReplaceAll path: these tools
// operate on the flattened text, so inline codes and run-anchored source
// overlays cannot follow the rewrite (AD-006). A locale without a committed
// target is skipped; an unchanged text produces no edit.
func textPlan(v tool.BlockView, applySource bool, targets []model.LocaleID, fn func(string) (string, error)) (tool.EditPlan, error) {
	var plan tool.EditPlan
	if applySource {
		text := v.SourceText()
		converted, err := fn(text)
		if err != nil {
			return tool.EditPlan{}, fmt.Errorf("source: %w", err)
		}
		if converted != text {
			plan.ReplaceAll = &converted
		}
	}
	for _, loc := range targets {
		if !v.HasTarget(loc) {
			continue
		}
		text := v.TargetText(loc)
		converted, err := fn(text)
		if err != nil {
			return tool.EditPlan{}, fmt.Errorf("target %s: %w", loc, err)
		}
		if converted != text {
			plan.SetTarget(loc, []model.Run{{Text: &model.TextRun{Text: converted}}})
		}
	}
	return plan, nil
}
