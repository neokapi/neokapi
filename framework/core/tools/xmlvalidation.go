package tools

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// XML validation property keys.
const (
	PropXMLValid      = "xml-valid"
	PropXMLValidError = "xml-valid-error"
)

// XMLValidationConfig holds configuration for the XML validation tool.
type XMLValidationConfig struct {
	CheckSource bool           `schema:"description=Validate source text for XML well-formedness,default=true"` // Validate source text (default: true)
	CheckTarget bool           `schema:"description=Validate target text for XML well-formedness"` // Validate target text
	Locale      model.LocaleID `schema:"description=Target locale for validation,showIfSet=CheckTarget"` // Target locale for validation
	WrapRoot    bool           `schema:"description=Wrap text in a root element before validating,default=true"` // Wrap text in root element before validating
}

// ToolName returns the tool name this config applies to.
func (c *XMLValidationConfig) ToolName() string { return "xml-validation" }

// Reset restores default values.
func (c *XMLValidationConfig) Reset() {
	c.CheckSource = true
	c.CheckTarget = false
	c.Locale = ""
	c.WrapRoot = true
}

// Validate checks configuration validity.
func (c *XMLValidationConfig) Validate() error {
	if c.CheckTarget && c.Locale.IsEmpty() {
		return fmt.Errorf("xml-validation: Locale is required when CheckTarget is true")
	}
	return nil
}

// NewXMLValidationTool creates a tool that validates XML well-formedness
// of source and/or target text in blocks.
func NewXMLValidationTool(cfg *XMLValidationConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "xml-validation",
		ToolDescription: "Validates XML well-formedness of block text",
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

		conf := t.Cfg.(*XMLValidationConfig)
		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		valid := true
		var errMsg string

		if conf.CheckSource {
			if err := validateXML(block.SourceText(), conf.WrapRoot); err != nil {
				valid = false
				errMsg = fmt.Sprintf("source: %s", err.Error())
			}
		}

		if conf.CheckTarget && !conf.Locale.IsEmpty() && block.HasTarget(conf.Locale) {
			if err := validateXML(block.TargetText(conf.Locale), conf.WrapRoot); err != nil {
				valid = false
				if errMsg != "" {
					errMsg += "; "
				}
				errMsg += fmt.Sprintf("target: %s", err.Error())
			}
		}

		if valid {
			block.Properties[PropXMLValid] = "true"
		} else {
			block.Properties[PropXMLValid] = "false"
			block.Properties[PropXMLValidError] = errMsg
		}

		return part, nil
	}
	return t
}

// validateXML checks if the text is well-formed XML.
func validateXML(text string, wrapRoot bool) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	xmlText := text
	if wrapRoot {
		xmlText = "<root>" + text + "</root>"
	}
	decoder := xml.NewDecoder(strings.NewReader(xmlText))
	for {
		_, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				return nil
			}
			return err
		}
	}
}
