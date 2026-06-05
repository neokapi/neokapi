package html

import (
	"fmt"
	"regexp"

	"github.com/neokapi/neokapi/core/config"
)

// ElementRule defines how an element should be treated during extraction.
type ElementRule struct {
	RuleTypes              []string       // TEXTUNIT, INLINE, EXCLUDE, INCLUDE, ATTRIBUTES_ONLY, PRESERVE_WHITESPACE, GROUP
	Conditions             []string       // [attr, operator, value] triplet
	IDAttributes           []string       // attributes used as block name/ID
	TranslatableAttributes map[string]any // attr name → condition or []condition
	WritableAttributes     []string       // writable localizable attributes
	ReadOnlyAttributes     []string       // read-only localizable attributes
}

// AttributeRule defines how an attribute should be treated during extraction.
type AttributeRule struct {
	RuleTypes         []string // ATTRIBUTE_TRANS, ATTRIBUTE_ID, ATTRIBUTE_WRITABLE, ATTRIBUTE_READONLY
	AllElementsExcept []string // apply to all elements except these
	OnlyTheseElements []string // apply only to these elements
	Conditions        []string // [attr, operator, value] triplet
}

// Config holds configuration for the HTML format.
type Config struct {
	// PreserveWhitespace preserves significant whitespace in text nodes.
	PreserveWhitespace bool

	// Elements maps element names to their extraction rules.
	Elements map[string]*ElementRule

	// Attributes maps attribute names to their extraction rules.
	Attributes map[string]*AttributeRule

	// UseCodeFinder enables regex-based inline code detection.
	UseCodeFinder bool

	// CodeFinderRules are regex patterns that match inline codes.
	CodeFinderRules []string

	// compiledCodeFinder caches compiled regex patterns.
	compiledCodeFinder []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "html" }

// ConfigKind returns the Kind for HTML format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("html") }

// Reset restores default values.
func (c *Config) Reset() {
	c.PreserveWhitespace = false
	c.Elements = nil
	c.Attributes = nil
	c.UseCodeFinder = false
	c.CodeFinderRules = nil
	c.compiledCodeFinder = nil
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
// Uses the same hierarchical structure as the okf_html bridge config:
// parser settings are nested under "parser", element/attribute rules and
// inline code settings are top-level.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "parser":
			m, ok := val.(map[string]any)
			if !ok {
				return fmt.Errorf("parser: expected map[string]any, got %T", val)
			}
			if err := c.applyParserSettings(m); err != nil {
				return err
			}
		case "useCodeFinder":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useCodeFinder: expected bool, got %T", val)
			}
			c.UseCodeFinder = b
		case "codeFinderRules":
			rules, ok := val.([]string)
			if !ok {
				return fmt.Errorf("codeFinderRules: expected []string, got %T", val)
			}
			c.CodeFinderRules = rules
		case "elements":
			m, ok := val.(map[string]any)
			if !ok {
				return fmt.Errorf("elements: expected map[string]any, got %T", val)
			}
			if err := c.applyElementRules(m); err != nil {
				return err
			}
		case "attributes":
			m, ok := val.(map[string]any)
			if !ok {
				return fmt.Errorf("attributes: expected map[string]any, got %T", val)
			}
			if err := c.applyAttributeRules(m); err != nil {
				return err
			}
		default:
			// Ignore unknown parameters for forward compatibility
		}
	}
	return nil
}

func (c *Config) applyParserSettings(m map[string]any) error {
	for key, val := range m {
		switch key {
		case "preserveWhitespace":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("parser.preserveWhitespace: expected bool, got %T", val)
			}
			c.PreserveWhitespace = b
		default:
			// Ignore unknown parser settings for forward compatibility
		}
	}
	return nil
}

func (c *Config) applyElementRules(m map[string]any) error {
	if c.Elements == nil {
		c.Elements = make(map[string]*ElementRule)
	}
	for name, ruleVal := range m {
		ruleMap, ok := ruleVal.(map[string]any)
		if !ok {
			continue
		}
		rule := &ElementRule{}
		if rt, ok := ruleMap["ruleTypes"].([]string); ok {
			rule.RuleTypes = rt
		}
		if cond, ok := ruleMap["conditions"].([]string); ok {
			rule.Conditions = cond
		}
		if ids, ok := ruleMap["idAttributes"].([]string); ok {
			rule.IDAttributes = ids
		}
		if ta, ok := ruleMap["translatableAttributes"]; ok {
			switch v := ta.(type) {
			case []string:
				rule.TranslatableAttributes = make(map[string]any)
				for _, a := range v {
					rule.TranslatableAttributes[a] = nil
				}
			case map[string]any:
				rule.TranslatableAttributes = v
			}
		}
		if wa, ok := ruleMap["writableLocalizableAttributes"].([]string); ok {
			rule.WritableAttributes = wa
		}
		if ra, ok := ruleMap["readOnlyLocalizableAttributes"].([]string); ok {
			rule.ReadOnlyAttributes = ra
		}
		c.Elements[name] = rule
	}
	return nil
}

func (c *Config) applyAttributeRules(m map[string]any) error {
	if c.Attributes == nil {
		c.Attributes = make(map[string]*AttributeRule)
	}
	for name, ruleVal := range m {
		ruleMap, ok := ruleVal.(map[string]any)
		if !ok {
			continue
		}
		rule := &AttributeRule{}
		if rt, ok := ruleMap["ruleTypes"].([]string); ok {
			rule.RuleTypes = rt
		}
		if exc, ok := ruleMap["allElementsExcept"].([]string); ok {
			rule.AllElementsExcept = exc
		}
		if only, ok := ruleMap["onlyTheseElements"].([]string); ok {
			rule.OnlyTheseElements = only
		}
		if cond, ok := ruleMap["conditions"].([]string); ok {
			rule.Conditions = cond
		}
		c.Attributes[name] = rule
	}
	return nil
}

// CodeFinderPatterns returns compiled regex patterns for code finder.
func (c *Config) CodeFinderPatterns() []*regexp.Regexp {
	if c.compiledCodeFinder != nil {
		return c.compiledCodeFinder
	}
	if !c.UseCodeFinder || len(c.CodeFinderRules) == 0 {
		return nil
	}
	for _, pattern := range c.CodeFinderRules {
		re, err := regexp.Compile(pattern)
		if err == nil {
			c.compiledCodeFinder = append(c.compiledCodeFinder, re)
		}
	}
	return c.compiledCodeFinder
}
