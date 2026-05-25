package tools

import (
	"encoding/xml"
	"errors"
	"io"
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
	CheckSource bool           `schema:"title=Check Source,description=Validate source text for XML well-formedness,default=true"`          // Validate source text (default: true)
	CheckTarget bool           `schema:"title=Check Target,description=Validate target text for XML well-formedness"`                       // Validate target text
	Locale      model.LocaleID `schema:"title=Target Locale,description=Target locale for validation,showIfSet=CheckTarget"`                // Target locale for validation
	WrapRoot    bool           `schema:"title=Wrap in Root Element,description=Wrap text in a root element before validating,default=true"` // Wrap text in root element before validating
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
		return errors.New("xml-validation: locale is required when CheckTarget is true")
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
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*XMLValidationConfig)

		valid := true
		var errMsg string

		if conf.CheckSource {
			if err := validateXML(v.SourceText(), conf.WrapRoot); err != nil {
				valid = false
				errMsg = "source: " + err.Error()
			}
		}

		if conf.CheckTarget && !conf.Locale.IsEmpty() && v.HasTarget(conf.Locale) {
			if err := validateXML(v.TargetText(conf.Locale), conf.WrapRoot); err != nil {
				valid = false
				if errMsg != "" {
					errMsg += "; "
				}
				errMsg += "target: " + err.Error()
			}
		}

		if valid {
			v.SetProperty(PropXMLValid, "true")
		} else {
			v.SetProperty(PropXMLValid, "false")
			v.SetProperty(PropXMLValidError, errMsg)
		}

		return nil
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
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}
