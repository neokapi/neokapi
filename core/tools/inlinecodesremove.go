package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// InlineCodesRemoveConfig holds configuration for the inline codes remove tool.
type InlineCodesRemoveConfig struct {
	ApplySource           bool           `schema:"title=Apply to Source,description=Apply to source segments"`                                                                                            // Apply to source segments (default: false)
	ApplyTarget           bool           `schema:"title=Apply to Target,description=Apply to target segments,default=true"`                                                                               // Apply to target segments (default: true)
	TargetLocale          model.LocaleID `schema:"title=Target Locale,description=Target locale for processing,showIfSet=ApplyTarget"`                                                                  // Target locale (required when ApplyTarget is true)
	IncludeNonTranslatable bool          `schema:"title=Include Non-Translatable,description=Apply the removal action even to text units marked as non-translatable,default=true"`                                  // Include non-translatable blocks
	ReplaceWithSpace      bool           `schema:"title=Replace With Space,description=Replace line-break inline codes with spaces instead of removing them entirely"`                                        // Replace line-break codes with spaces
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
		return fmt.Errorf("inline-codes-remove: target locale is required when ApplyTarget is true")
	}
	if !c.ApplySource && !c.ApplyTarget {
		return fmt.Errorf("inline-codes-remove: at least one of ApplySource or ApplyTarget must be true")
	}
	return nil
}

// NewInlineCodesRemoveTool creates a new tool that strips inline codes/spans
// from fragment content, producing clean plain text.
func NewInlineCodesRemoveTool(cfg *InlineCodesRemoveConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "inline-codes-remove",
		ToolDescription: "Strips inline codes/spans from fragment content, producing clean plain text",
		Cfg:             cfg,
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
			for _, seg := range block.Source {
				stripSpans(seg.Content)
			}
		}

		if conf.ApplyTarget {
			segs, ok := block.Targets[conf.TargetLocale]
			if ok {
				for _, seg := range segs {
					stripSpans(seg.Content)
				}
			}
		}

		return part, nil
	}
	return t
}

// stripSpans removes all span markers from the fragment's coded text
// and clears the Spans slice, leaving only plain text.
func stripSpans(frag *model.Fragment) {
	if frag == nil || !frag.HasSpans() {
		return
	}
	frag.CodedText = frag.Text()
	frag.Spans = nil
}
