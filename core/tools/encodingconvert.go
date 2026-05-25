package tools

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"
)

// PropEncodingTarget is the block property key for the target encoding.
const PropEncodingTarget = "encoding-target"

// EncodingConvertConfig holds configuration for the encoding conversion tool.
type EncodingConvertConfig struct {
	TargetEncoding string         `json:"targetEncoding,omitempty" schema:"title=Target Encoding,description=Target encoding name (e.g. utf-8 or iso-8859-1 or shift-jis)"` // Target encoding name (e.g., "utf-8", "iso-8859-1", "shift-jis")
	ApplySource    bool           `json:"applySource,omitempty"    schema:"title=Apply to Source,description=Apply encoding conversion to source text"`                     // Apply to source (default: false)
	ApplyTarget    bool           `json:"applyTarget,omitempty"    schema:"title=Apply to Target,description=Apply encoding conversion to target text,default=true"`        // Apply to target (default: true)
	TargetLocale   model.LocaleID `json:"targetLocale,omitempty"   schema:"title=Target Locale,description=Target locale for processing,showIfSet=ApplyTarget"`             // Required when ApplyTarget is true

	// Unescape options control how escape sequences in input are decoded.
	UnescapeNCR  bool `json:"unescapeNCR,omitempty"  schema:"title=Unescape Numeric Character References,description=Unescape numeric character references (e.g. &#xE1;) when reading input,default=true"`
	UnescapeCER  bool `json:"unescapeCER,omitempty"  schema:"title=Unescape Character Entity References,description=Unescape HTML character entity references (e.g. &aacute;) when reading input,default=true"`
	UnescapeJava bool `json:"unescapeJava,omitempty" schema:"title=Unescape Java-style Notation,description=Unescape Java-style \\\\uXXXX escape sequences when reading input,default=true"`

	// Escape options control how unmappable characters are handled in output.
	EscapeAll         bool `json:"escapeAll,omitempty"         schema:"title=Escape All Extended Characters,description=Escape all extended (non-ASCII) characters in output"`
	ReportUnsupported bool `json:"reportUnsupported,omitempty" schema:"title=Report Unsupported Characters,description=Report characters not supported by the target encoding,default=true"`
}

// ToolName returns the tool name this config applies to.
func (c *EncodingConvertConfig) ToolName() string { return "encoding-convert" }

// Reset restores default values.
func (c *EncodingConvertConfig) Reset() {
	c.TargetEncoding = ""
	c.ApplySource = false
	c.ApplyTarget = true
	c.TargetLocale = ""
	c.UnescapeNCR = true
	c.UnescapeCER = true
	c.UnescapeJava = true
	c.EscapeAll = false
	c.ReportUnsupported = true
}

// Validate checks configuration validity.
func (c *EncodingConvertConfig) Validate() error {
	if c.TargetEncoding == "" {
		return errors.New("encoding-convert: TargetEncoding is required")
	}
	if _, err := ianaindex.IANA.Encoding(c.TargetEncoding); err != nil {
		return fmt.Errorf("encoding-convert: unsupported encoding %q: %w", c.TargetEncoding, err)
	}
	if c.ApplyTarget && c.TargetLocale.IsEmpty() {
		return errors.New("encoding-convert: TargetLocale required when ApplyTarget is true")
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
	// Transform: encoding-convert may rewrite source and/or target text.
	t.Transform = func(v tool.SourceView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*EncodingConvertConfig)

		enc, err := ianaindex.IANA.Encoding(conf.TargetEncoding)
		if err != nil {
			return fmt.Errorf("encoding-convert: unsupported encoding %q: %w", conf.TargetEncoding, err)
		}

		v.SetProperty(PropEncodingTarget, conf.TargetEncoding)

		if conf.ApplySource {
			sourceText := unescapeText(v.SourceText(), conf)
			converted, convErr := convertThroughEncoding(sourceText, enc)
			if convErr != nil {
				return fmt.Errorf("encoding-convert: source conversion failed: %w", convErr)
			}
			v.SetSourceText(converted)
		}

		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() && v.HasTarget(conf.TargetLocale) {
			targetText := unescapeText(v.TargetText(conf.TargetLocale), conf)
			converted, convErr := convertThroughEncoding(targetText, enc)
			if convErr != nil {
				return fmt.Errorf("encoding-convert: target conversion failed: %w", convErr)
			}
			v.SetTargetText(conf.TargetLocale, converted)
		}

		return nil
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

// ncrPattern matches numeric character references like &#xE1; or &#225;.
var ncrPattern = regexp.MustCompile(`&#x([0-9a-fA-F]+);|&#(\d+);`)

// javaEscapePattern matches Java-style \uXXXX escape sequences.
var javaEscapePattern = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)

// unescapeText applies the configured unescape operations to text before encoding conversion.
func unescapeText(text string, conf *EncodingConvertConfig) string {
	if conf.UnescapeNCR {
		text = ncrPattern.ReplaceAllStringFunc(text, func(match string) string {
			subs := ncrPattern.FindStringSubmatch(match)
			var cp int64
			if subs[1] != "" {
				cp, _ = strconv.ParseInt(subs[1], 16, 32)
			} else {
				cp, _ = strconv.ParseInt(subs[2], 10, 32)
			}
			if cp > 0 {
				return string(rune(cp))
			}
			return match
		})
	}
	if conf.UnescapeCER {
		text = html.UnescapeString(text)
	}
	if conf.UnescapeJava {
		text = javaEscapePattern.ReplaceAllStringFunc(text, func(match string) string {
			hex := strings.TrimPrefix(match, `\u`)
			cp, _ := strconv.ParseInt(hex, 16, 32)
			if cp > 0 {
				return string(rune(cp))
			}
			return match
		})
	}
	return text
}
