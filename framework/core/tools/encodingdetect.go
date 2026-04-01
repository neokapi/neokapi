package tools

import (
	"fmt"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Encoding detection property keys.
const (
	PropEncodingDetected = "encoding-detected"
	PropEncodingIsUTF8   = "encoding-is-utf8"
	PropEncodingIsASCII  = "encoding-is-ascii"
)

// EncodingDetectConfig holds configuration for the encoding detection tool.
type EncodingDetectConfig struct{}

// ToolName returns the tool name this config applies to.
func (c *EncodingDetectConfig) ToolName() string { return "encoding-detect" }

// Reset restores default values.
func (c *EncodingDetectConfig) Reset() {}

// Validate checks configuration validity.
func (c *EncodingDetectConfig) Validate() error { return nil }

// EncodingDetectSchema returns the auto-generated schema for the encoding-detect tool.
func EncodingDetectSchema() *schema.ComponentSchema {
	return schema.FromStruct(&EncodingDetectConfig{}, schema.ToolMeta{
		ID:          "encoding-detect",
		Category:    schema.CategoryEnrich,
		DisplayName: "Encoding Detect",
		Description: "Detect character encoding of source files",
		Inputs:      []string{schema.PartTypeBlock},
	})
}

// NewEncodingDetectFromConfig creates an encoding-detect tool from a config map.
func NewEncodingDetectFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg EncodingDetectConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("encoding-detect config: %w", err)
	}
	return NewEncodingDetectTool(&cfg), nil
}

// NewEncodingDetectTool creates a tool that detects the encoding characteristics
// of source text in blocks and stores the results in properties.
func NewEncodingDetectTool(cfg *EncodingDetectConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "encoding-detect",
		ToolDescription: "Detects encoding characteristics of block text",
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

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		sourceText := block.SourceText()
		isASCII := isASCIIOnly(sourceText)
		isUTF8 := utf8.ValidString(sourceText)

		if isASCII {
			block.Properties[PropEncodingDetected] = "ascii"
		} else if isUTF8 {
			block.Properties[PropEncodingDetected] = "utf-8"
		} else {
			block.Properties[PropEncodingDetected] = "unknown"
		}

		if isUTF8 {
			block.Properties[PropEncodingIsUTF8] = "true"
		} else {
			block.Properties[PropEncodingIsUTF8] = "false"
		}

		if isASCII {
			block.Properties[PropEncodingIsASCII] = "true"
		} else {
			block.Properties[PropEncodingIsASCII] = "false"
		}

		return part, nil
	}
	return t
}

// isASCIIOnly returns true if all bytes in the string are in the ASCII range.
func isASCIIOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}
