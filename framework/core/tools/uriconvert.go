package tools

import (
	"fmt"
	"net/url"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// URIConvertMode controls whether text is encoded or decoded.
type URIConvertMode string

const (
	URIEncode URIConvertMode = "encode"
	URIDecode URIConvertMode = "decode"
)

// URIConvertConfig holds configuration for the URI conversion tool.
type URIConvertConfig struct {
	Mode         URIConvertMode // encode or decode (default: "decode")
	ApplySource  bool           // Apply to source (default: false)
	ApplyTarget  bool           // Apply to target (default: true)
	TargetLocale model.LocaleID // Target locale to process (required when ApplyTarget)
}

// ToolName returns the tool name this config applies to.
func (c *URIConvertConfig) ToolName() string { return "uri-convert" }

// Reset restores default values.
func (c *URIConvertConfig) Reset() {
	c.Mode = URIDecode
	c.ApplySource = false
	c.ApplyTarget = true
	c.TargetLocale = ""
}

// Validate checks configuration validity.
func (c *URIConvertConfig) Validate() error {
	switch c.Mode {
	case URIEncode, URIDecode:
	default:
		return fmt.Errorf("uri-convert: invalid Mode %q (use encode or decode)", c.Mode)
	}
	if c.ApplyTarget && c.TargetLocale.IsEmpty() {
		return fmt.Errorf("uri-convert: TargetLocale required when ApplyTarget is true")
	}
	return nil
}

// NewURIConvertTool creates a tool that encodes or decodes URI escape sequences
// in block text. Uses url.PathEscape/url.PathUnescape for the conversion.
func NewURIConvertTool(cfg *URIConvertConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "uri-convert",
		ToolDescription: "Encodes or decodes URI escape sequences in text",
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

		conf := t.Cfg.(*URIConvertConfig)

		if conf.ApplySource {
			sourceText := block.SourceText()
			converted, err := convertURI(sourceText, conf.Mode)
			if err != nil {
				return nil, fmt.Errorf("uri-convert source: %w", err)
			}
			block.SetSourceText(converted)
		}

		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() && block.HasTarget(conf.TargetLocale) {
			targetText := block.TargetText(conf.TargetLocale)
			converted, err := convertURI(targetText, conf.Mode)
			if err != nil {
				return nil, fmt.Errorf("uri-convert target: %w", err)
			}
			block.SetTargetText(conf.TargetLocale, converted)
		}

		return part, nil
	}
	return t
}

// convertURI encodes or decodes URI escape sequences in text.
func convertURI(text string, mode URIConvertMode) (string, error) {
	switch mode {
	case URIEncode:
		return url.PathEscape(text), nil
	case URIDecode:
		return url.PathUnescape(text)
	default:
		return text, nil
	}
}
