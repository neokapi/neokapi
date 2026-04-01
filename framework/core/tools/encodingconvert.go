package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"
)

// PropEncodingTarget is the block property key for the target encoding.
const PropEncodingTarget = "encoding-target"

// EncodingConvertConfig holds configuration for the encoding conversion tool.
type EncodingConvertConfig struct {
	TargetEncoding string         `schema:"description=Target encoding name (e.g. utf-8 or iso-8859-1 or shift-jis)"` // Target encoding name (e.g., "utf-8", "iso-8859-1", "shift-jis")
	ApplySource    bool           `schema:"description=Apply encoding conversion to source text"` // Apply to source (default: false)
	ApplyTarget    bool           `schema:"description=Apply encoding conversion to target text,default=true"` // Apply to target (default: true)
	TargetLocale   model.LocaleID `schema:"description=Target locale for processing,showIfSet=ApplyTarget"` // Required when ApplyTarget is true
}

// ToolName returns the tool name this config applies to.
func (c *EncodingConvertConfig) ToolName() string { return "encoding-convert" }

// Reset restores default values.
func (c *EncodingConvertConfig) Reset() {
	c.TargetEncoding = ""
	c.ApplySource = false
	c.ApplyTarget = true
	c.TargetLocale = ""
}

// Validate checks configuration validity.
func (c *EncodingConvertConfig) Validate() error {
	if c.TargetEncoding == "" {
		return fmt.Errorf("encoding-convert: TargetEncoding is required")
	}
	if _, err := ianaindex.IANA.Encoding(c.TargetEncoding); err != nil {
		return fmt.Errorf("encoding-convert: unsupported encoding %q: %w", c.TargetEncoding, err)
	}
	if c.ApplyTarget && c.TargetLocale.IsEmpty() {
		return fmt.Errorf("encoding-convert: TargetLocale required when ApplyTarget is true")
	}
	return nil
}

// NewEncodingConvertTool creates a tool that converts text through a target encoding.
// It encodes text to the target encoding and decodes back to UTF-8, which validates
// and normalizes the text for that encoding (replacing unmappable characters).
// It also stores the target encoding name in block properties for downstream writers.
func NewEncodingConvertTool(cfg *EncodingConvertConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "encoding-convert",
		ToolDescription: "Converts character encoding of text content",
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

		conf := t.Cfg.(*EncodingConvertConfig)

		enc, err := ianaindex.IANA.Encoding(conf.TargetEncoding)
		if err != nil {
			return nil, fmt.Errorf("encoding-convert: unsupported encoding %q: %w", conf.TargetEncoding, err)
		}

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}
		block.Properties[PropEncodingTarget] = conf.TargetEncoding

		if conf.ApplySource {
			sourceText := block.SourceText()
			converted, convErr := convertThroughEncoding(sourceText, enc)
			if convErr != nil {
				return nil, fmt.Errorf("encoding-convert: source conversion failed: %w", convErr)
			}
			block.SetSourceText(converted)
		}

		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() && block.HasTarget(conf.TargetLocale) {
			targetText := block.TargetText(conf.TargetLocale)
			converted, convErr := convertThroughEncoding(targetText, enc)
			if convErr != nil {
				return nil, fmt.Errorf("encoding-convert: target conversion failed: %w", convErr)
			}
			block.SetTargetText(conf.TargetLocale, converted)
		}

		return part, nil
	}
	return t
}

// convertThroughEncoding encodes text to the given encoding and decodes back to UTF-8.
// This validates/normalizes the text through that encoding.
func convertThroughEncoding(text string, enc encoding.Encoding) (string, error) {
	// Encode UTF-8 → target encoding.
	encoded, err := enc.NewEncoder().String(text)
	if err != nil {
		return "", fmt.Errorf("encode: %w", err)
	}
	// Decode target encoding → UTF-8.
	decoded, err := enc.NewDecoder().String(encoded)
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	return decoded, nil
}
