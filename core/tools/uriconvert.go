package tools

import (
	"errors"
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
	Mode         URIConvertMode `schema:"title=Conversion Direction,description=URI conversion direction,enum=encode|decode,default=decode"` // encode or decode (default: "decode")
	ApplySource  bool           `schema:"title=Apply to Source,description=Apply to source text"`                                            // Apply to source (default: false)
	ApplyTarget  bool           `schema:"title=Apply to Target,description=Apply to target text,default=true"`                               // Apply to target (default: true)
	TargetLocale model.LocaleID `schema:"title=Target Locale,description=Target locale for processing,showIfSet=ApplyTarget"`                // Target locale to process (required when ApplyTarget)
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
		return errors.New("uri-convert: TargetLocale required when ApplyTarget is true")
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
	// Transform producer: returns the URI-escape rewrite as an edit plan; the
	// framework applier rewrites the block (AD-006).
	t.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		if !v.Translatable() {
			return tool.EditPlan{}, nil
		}
		conf := t.Cfg.(*URIConvertConfig)
		var targets []model.LocaleID
		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() {
			targets = []model.LocaleID{conf.TargetLocale}
		}
		plan, err := textPlan(v, conf.ApplySource, targets, func(s string) (string, error) {
			return convertURI(s, conf.Mode)
		})
		if err != nil {
			return tool.EditPlan{}, fmt.Errorf("uri-convert: %w", err)
		}
		return plan, nil
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
