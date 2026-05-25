package tools

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// LineBreakMode specifies the target line ending style.
type LineBreakMode string

const (
	LineBreakLF   LineBreakMode = "lf"   // Unix \n
	LineBreakCRLF LineBreakMode = "crlf" // Windows \r\n
	LineBreakCR   LineBreakMode = "cr"   // Classic Mac \r
)

// LineBreakConvertConfig holds configuration for the line break conversion tool.
type LineBreakConvertConfig struct {
	Mode        LineBreakMode `schema:"title=Line Break Type,description=Target line break style,enum=lf|crlf|cr,default=lf"` // Target line break style (default: "lf")
	ApplySource bool          `schema:"title=Apply to Source,description=Apply to source text,default=true"`                  // Apply to source text (default: true)
	ApplyTarget bool          `schema:"title=Apply to Target,description=Apply to target text,default=true"`                  // Apply to target text (default: true)
}

// ToolName returns the tool name this config applies to.
func (c *LineBreakConvertConfig) ToolName() string { return "linebreak-convert" }

// Reset restores default values.
func (c *LineBreakConvertConfig) Reset() {
	c.Mode = LineBreakLF
	c.ApplySource = true
	c.ApplyTarget = true
}

// Validate checks configuration validity.
func (c *LineBreakConvertConfig) Validate() error {
	switch c.Mode {
	case LineBreakLF, LineBreakCRLF, LineBreakCR:
	default:
		return fmt.Errorf("linebreak-convert: invalid Mode %q (use lf, crlf, or cr)", c.Mode)
	}
	return nil
}

// NewLineBreakConvertTool creates a tool that normalizes line endings in block text.
func NewLineBreakConvertTool(cfg *LineBreakConvertConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "linebreak-convert",
		ToolDescription: "Normalizes line endings in source and/or target text of blocks",
		Cfg:             cfg,
	}
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}
		if !block.Translatable {
			return part, nil
		}

		conf := t.Cfg.(*LineBreakConvertConfig)

		if conf.ApplySource {
			sourceText := block.SourceText()
			block.SetSourceText(convertLineBreaks(sourceText, conf.Mode))
		}

		if conf.ApplyTarget {
			for _, locale := range block.TargetLocales() {
				targetText := block.TargetText(locale)
				block.SetTargetText(locale, convertLineBreaks(targetText, conf.Mode))
			}
		}

		return part, nil
	}
	return t
}

// convertLineBreaks first normalizes all line endings to \n, then converts to the target style.
func convertLineBreaks(text string, mode LineBreakMode) string {
	// Normalize: replace \r\n first (to avoid double-replacing), then \r.
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	switch mode {
	case LineBreakLF:
		return normalized
	case LineBreakCRLF:
		return strings.ReplaceAll(normalized, "\n", "\r\n")
	case LineBreakCR:
		return strings.ReplaceAll(normalized, "\n", "\r")
	default:
		return normalized
	}
}
