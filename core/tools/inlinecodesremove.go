package tools

import (
	"errors"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// InlineCodesRemoveConfig holds configuration for the inline codes remove tool.
type InlineCodesRemoveConfig struct {
	ApplySource            bool           `schema:"title=Apply to Source,description=Apply to source segments"`                                                                     // Apply to source segments (default: false)
	ApplyTarget            bool           `schema:"title=Apply to Target,description=Apply to target segments,default=true"`                                                        // Apply to target segments (default: true)
	TargetLocale           model.LocaleID `schema:"title=Target Locale,description=Target locale for processing,showIfSet=ApplyTarget"`                                             // Target locale (required when ApplyTarget is true)
	IncludeNonTranslatable bool           `schema:"title=Include Non-Translatable,description=Apply the removal action even to text units marked as non-translatable,default=true"` // Include non-translatable blocks
	ReplaceWithSpace       bool           `schema:"title=Replace With Space,description=Replace line-break inline codes with spaces instead of removing them entirely"`             // Replace line-break codes with spaces
}

// ToolName returns the tool name this config applies to.
func (c *InlineCodesRemoveConfig) ToolName() string { return "inline-codes-remove" }

// Reset restores default values.
func (c *InlineCodesRemoveConfig) Reset() {
	c.ApplySource = false
	c.ApplyTarget = true
	c.TargetLocale = ""
	c.IncludeNonTranslatable = true
	c.ReplaceWithSpace = false
}

// Validate checks configuration validity.
func (c *InlineCodesRemoveConfig) Validate() error {
	if c.ApplyTarget && c.TargetLocale == "" {
		return errors.New("inline-codes-remove: target locale is required when ApplyTarget is true")
	}
	if !c.ApplySource && !c.ApplyTarget {
		return errors.New("inline-codes-remove: at least one of ApplySource or ApplyTarget must be true")
	}
	return nil
}

// NewInlineCodesRemoveTool creates a new tool that strips inline-code
// runs (Ph / PcOpen / PcClose) from segment content, producing clean
// plain text.
func NewInlineCodesRemoveTool(cfg *InlineCodesRemoveConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "inline-codes-remove",
		ToolDescription: "Strips inline-code runs from segment content, producing clean plain text",
		Cfg:             cfg,
		WritesSource:    true,
		WritesTarget:    true,
	}
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}

		conf := t.Cfg.(*InlineCodesRemoveConfig)

		if !block.Translatable && !conf.IncludeNonTranslatable {
			return part, nil
		}

		if conf.ApplySource {
			block.SetSourceRuns(stripInlineRuns(block.Source, conf.ReplaceWithSpace))
		}

		if conf.ApplyTarget {
			if runs := block.TargetRuns(conf.TargetLocale); runs != nil {
				block.SetTargetRuns(conf.TargetLocale, stripInlineRuns(runs, conf.ReplaceWithSpace))
			}
		}

		return part, nil
	}
	return t
}

// stripInlineRuns walks a run sequence and removes every non-text
// run (placeholder, paired code, sub). Consecutive text runs are
// coalesced. When replaceWithSpace is true, line-break-style codes
// contribute a single space so "foo<br/>bar" collapses to "foo bar"
// rather than "foobar".
//
// Plural and select runs recurse through their forms/cases so
// inline codes inside structured constructs are also stripped.
func stripInlineRuns(runs []model.Run, replaceWithSpace bool) []model.Run {
	var out []model.Run
	var buf string
	flush := func() {
		if buf == "" {
			return
		}
		out = append(out, model.Run{Text: &model.TextRun{Text: buf}})
		buf = ""
	}
	for _, r := range runs {
		switch {
		case r.Text != nil:
			buf += r.Text.Text
		case r.Ph != nil:
			if replaceWithSpace && isLineBreakType(r.Ph.Type, r.Ph.SubType) {
				buf += " "
			}
		case r.PcOpen != nil, r.PcClose != nil:
			// Drop paired codes; their content (text runs between
			// them) is already in the stream.
		case r.Sub != nil:
			if replaceWithSpace {
				buf += " "
			}
		case r.Plural != nil:
			flush()
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, v := range r.Plural.Forms {
				forms[k] = stripInlineRuns(v, replaceWithSpace)
			}
			out = append(out, model.Run{Plural: &model.PluralRun{Pivot: r.Plural.Pivot, Forms: forms}})
		case r.Select != nil:
			flush()
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for k, v := range r.Select.Cases {
				cases[k] = stripInlineRuns(v, replaceWithSpace)
			}
			out = append(out, model.Run{Select: &model.SelectRun{Pivot: r.Select.Pivot, Cases: cases}})
		}
	}
	flush()
	return out
}

// isLineBreakType returns true when a placeholder run represents a
// line break (so stripping can replace it with a space to preserve
// token boundaries).
func isLineBreakType(typ, subType string) bool {
	switch typ {
	case "html:br", "md:break", "break", "line-break":
		return true
	}
	switch subType {
	case "br", "line-break":
		return true
	}
	return false
}
