package xml

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
)

// RuleType defines an element or attribute processing rule.
type RuleType string

const (
	RuleTextUnit           RuleType = "TEXTUNIT"
	RuleInline             RuleType = "INLINE"
	RuleExclude            RuleType = "EXCLUDE"
	RuleInclude            RuleType = "INCLUDE"
	RulePreserveWhitespace RuleType = "PRESERVE_WHITESPACE"
	RuleAttributeTrans     RuleType = "ATTRIBUTE_TRANS"
	RuleAttributeID        RuleType = "ATTRIBUTE_ID"
	RuleAttributeWritable  RuleType = "ATTRIBUTE_WRITABLE"
)

// ConditionOp defines the operator for attribute-based conditions.
type ConditionOp string

const (
	ConditionEquals        ConditionOp = "EQUALS"
	ConditionMatches       ConditionOp = "MATCHES"
	ConditionNotEquals     ConditionOp = "NOT_EQUALS"
	ConditionExists        ConditionOp = "EXISTS"
	ConditionNotExists     ConditionOp = "NOT_EXISTS"
	ConditionStartsWith    ConditionOp = "STARTS_WITH"
	ConditionNotStartsWith ConditionOp = "NOT_STARTS_WITH"
	ConditionEndsWith      ConditionOp = "ENDS_WITH"
)

// Condition represents an attribute-based condition for element rules.
//
// When Parent is true, the condition is evaluated against the *parent*
// element's attributes rather than the element's own. This expresses
// XPath predicates that the upstream Okapi ITS rules place on an ancestor
// (e.g. the ResX selector //data[not(@type)]/value tests <data> while the
// translatable unit is the <value> child).
type Condition struct {
	Attribute string
	Op        ConditionOp
	Value     string
	Parent    bool
}

// Evaluate tests whether the condition holds for the given attribute map.
// Existence operators (EXISTS/NOT_EXISTS) reason about presence; the
// remaining string operators only fire when the attribute is present.
func (c *Condition) Evaluate(attrs map[string]string) bool {
	attrVal, exists := attrs[c.Attribute]
	switch c.Op {
	case ConditionExists:
		return exists
	case ConditionNotExists:
		return !exists
	case ConditionNotEquals:
		// not(@a='v') is true when absent or differing.
		return !exists || attrVal != c.Value
	case ConditionNotStartsWith:
		// not(starts-with(@a,'v')) is true when absent or not prefixed.
		return !exists || !strings.HasPrefix(attrVal, c.Value)
	}
	if !exists {
		return false
	}
	switch c.Op {
	case ConditionEquals:
		return attrVal == c.Value
	case ConditionMatches:
		matched, _ := regexp.MatchString("^"+c.Value+"$", attrVal)
		return matched
	case ConditionStartsWith:
		return strings.HasPrefix(attrVal, c.Value)
	case ConditionEndsWith:
		return strings.HasSuffix(attrVal, c.Value)
	default:
		return false
	}
}

// TranslatableAttrCondition represents conditions for conditional translatable attributes.
type TranslatableAttrCondition struct {
	// Conditions are OR'd: if any condition matches, the attribute is translatable.
	Conditions []Condition
}

// ElementRule defines processing rules for an XML element.
type ElementRule struct {
	// Name is the element name or a regex pattern (surrounded by single quotes).
	Name string

	// RuleTypes is the set of rules applied to this element.
	RuleTypes []RuleType

	// Condition is an optional attribute condition for this rule.
	Condition *Condition

	// Conditions is an optional set of conditions, all of which must hold
	// (AND) for the rule to apply. When both Condition and Conditions are
	// set, Condition is treated as one additional AND term. Conditions may
	// reference the parent element's attributes via Condition.Parent.
	Conditions []Condition

	// IDAttributes lists attribute names used as block IDs.
	IDAttributes []string

	// ParentIDAttr names an attribute on the *parent* element whose value
	// is used as this element's block id. Mirrors the upstream ITS
	// itsx:idValue="../@name" pointer (e.g. ResX <value> takes its id from
	// the enclosing <data>/@name).
	ParentIDAttr string

	// Namespace optionally scopes this rule to elements in a specific XML
	// namespace URI. Empty matches any namespace (the default). Non-empty
	// matches only elements whose namespace URI equals it — this mirrors
	// the upstream DocBook ITS rules, whose selectors are scoped to the
	// DocBook namespace (db:, http://docbook.org/ns/docbook), so an
	// unprefixed (no-namespace) element of the same local name does NOT
	// match the inline rule.
	Namespace string

	// ParentElement optionally scopes this rule to elements whose direct
	// parent has this local name. Mirrors XPath selectors that constrain
	// the parent axis (e.g. the ResX selector //data/value requires the
	// <value>'s parent to be <data>, not <resheader>).
	ParentElement string

	// TranslatableAttributes maps attribute names to optional conditions.
	TranslatableAttributes map[string]*TranslatableAttrCondition

	// isRegex is true if Name is a regex pattern (wrapped in single quotes).
	isRegex bool
	// compiled is the compiled regex pattern (only set if isRegex).
	compiled *regexp.Regexp
}

// Matches returns true if this rule matches the given element name
// (local name only, any namespace, any parent).
func (r *ElementRule) Matches(elemName string) bool {
	if r.isRegex {
		if r.compiled == nil {
			return false
		}
		return r.compiled.MatchString(elemName)
	}
	return r.Name == elemName
}

// matchesCtx reports whether the rule matches an element given its local
// name, namespace URI and parent local name, honoring the optional
// Namespace and ParentElement scoping. Empty Namespace / ParentElement
// match anything.
func (r *ElementRule) matchesCtx(local, nsURI, parentName string) bool {
	if !r.Matches(local) {
		return false
	}
	if r.Namespace != "" && r.Namespace != nsURI {
		return false
	}
	if r.ParentElement != "" && r.ParentElement != parentName {
		return false
	}
	return true
}

// HasRule returns true if the rule set includes the given rule type.
func (r *ElementRule) HasRule(rt RuleType) bool {
	for _, t := range r.RuleTypes {
		if t == rt {
			return true
		}
	}
	return false
}

// conditionsHold evaluates the rule's conditions against the element's
// own attributes (ownAttrs) and its parent's attributes (parentAttrs).
// All conditions must hold (AND). A nil parentAttrs is treated as an
// empty map so parent-targeted existence checks behave sensibly at the
// document root. Returns true when the rule carries no conditions.
func (r *ElementRule) conditionsHold(ownAttrs, parentAttrs map[string]string) bool {
	check := func(c *Condition) bool {
		if c.Parent {
			return c.Evaluate(parentAttrs)
		}
		return c.Evaluate(ownAttrs)
	}
	if r.Condition != nil && !check(r.Condition) {
		return false
	}
	for i := range r.Conditions {
		if !check(&r.Conditions[i]) {
			return false
		}
	}
	return true
}

// compileElementRules compiles regex names on element and attribute rules
// that were constructed programmatically (e.g. by the bundled presets in
// presets.go) rather than parsed from a config map. Names wrapped in
// single quotes are treated as anchored regular expressions, matching the
// behavior of parseElementRules. Safe to call multiple times.
func (c *Config) compileElementRules() {
	for _, r := range c.ElementRules {
		if r == nil || r.compiled != nil {
			continue
		}
		if len(r.Name) >= 2 && r.Name[0] == '\'' && r.Name[len(r.Name)-1] == '\'' {
			r.isRegex = true
			if compiled, err := regexp.Compile("^" + r.Name[1:len(r.Name)-1] + "$"); err == nil {
				r.compiled = compiled
			}
		}
	}
	for _, r := range c.AttributeRules {
		if r == nil || r.compiled != nil {
			continue
		}
		if len(r.Name) >= 2 && r.Name[0] == '\'' && r.Name[len(r.Name)-1] == '\'' {
			r.isRegex = true
			if compiled, err := regexp.Compile("^" + r.Name[1:len(r.Name)-1] + "$"); err == nil {
				r.compiled = compiled
			}
		}
	}
}

// AttributeRule defines processing rules for an XML attribute.
type AttributeRule struct {
	// Name is the attribute name or a regex pattern.
	Name string

	// RuleTypes is the set of rules for this attribute.
	RuleTypes []RuleType

	// AllElementsExcept limits this rule to all elements except these.
	AllElementsExcept []string

	// OnlyTheseElements limits this rule to only these elements.
	OnlyTheseElements []string

	// isRegex is true if Name is a regex pattern.
	isRegex  bool
	compiled *regexp.Regexp
}

// Matches returns true if this attribute rule matches the given attribute name.
func (r *AttributeRule) Matches(attrName string) bool {
	if r.isRegex {
		if r.compiled == nil {
			return false
		}
		return r.compiled.MatchString(attrName)
	}
	return r.Name == attrName
}

// AppliesToElement returns true if this rule applies to the given element.
func (r *AttributeRule) AppliesToElement(elemName string) bool {
	if len(r.OnlyTheseElements) > 0 {
		for _, e := range r.OnlyTheseElements {
			if e == elemName {
				return true
			}
		}
		return false
	}
	if len(r.AllElementsExcept) > 0 {
		for _, e := range r.AllElementsExcept {
			if e == elemName {
				return false
			}
		}
		return true
	}
	return true
}

// Config holds configuration for the XML format.
type Config struct {
	// TranslatableElements lists element names whose text content is translatable.
	// If empty, all text content is considered translatable.
	TranslatableElements []string

	// TranslatableAttributes lists attribute names that are translatable.
	TranslatableAttributes []string

	// Subfilters maps XML element path patterns to format names for embedded
	// content. When an element's text content path matches a pattern, it is
	// parsed by the named format reader instead of being emitted as a plain
	// text block. Patterns use dot-separated element paths with glob support.
	Subfilters []format.SubfilterMapping

	// PreserveWhitespace controls global whitespace handling.
	// When false (default), whitespace is collapsed in text content.
	// When true, whitespace is preserved as-is.
	PreserveWhitespace bool

	// ExcludeByDefault inverts the default extraction: all elements are
	// excluded unless explicitly included by an element rule with INCLUDE.
	ExcludeByDefault bool

	// InlineElements lists element names treated as inline (spans within text).
	InlineElements []string

	// ExcludedElements lists element names whose content is excluded.
	ExcludedElements []string

	// ElementRules holds element-specific processing rules.
	ElementRules []*ElementRule

	// AttributeRules holds attribute-specific processing rules.
	AttributeRules []*AttributeRule

	// PreserveWhitespaceElements lists elements that preserve whitespace.
	PreserveWhitespaceElements []string

	// GroupElements lists elements that produce group/layer boundaries.
	GroupElements []string

	// BlockTypeMap maps element names to block type strings.
	BlockTypeMap map[string]string

	// IDAttributeNames lists attribute names used to extract block IDs.
	IDAttributeNames []string

	// UseCodeFinder enables regex-based inline code detection.
	UseCodeFinder bool

	// CodeFinderRules are regex patterns that match inline codes.
	CodeFinderRules []string

	// compiledCodeFinder caches compiled regex patterns.
	compiledCodeFinder []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "xml" }

// Reset restores default values.
func (c *Config) Reset() {
	c.TranslatableElements = nil
	c.TranslatableAttributes = nil
	c.Subfilters = nil
	c.PreserveWhitespace = false
	c.ExcludeByDefault = false
	c.InlineElements = nil
	c.ExcludedElements = nil
	c.ElementRules = nil
	c.AttributeRules = nil
	c.PreserveWhitespaceElements = nil
	c.GroupElements = nil
	c.BlockTypeMap = nil
	c.IDAttributeNames = nil
	c.UseCodeFinder = false
	c.CodeFinderRules = nil
	c.compiledCodeFinder = nil
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	for _, sf := range c.Subfilters {
		if sf.Pattern == "" {
			return errors.New("xml: subfilter mapping has empty pattern")
		}
		if sf.Format == "" {
			return fmt.Errorf("xml: subfilter mapping for %q has empty format", sf.Pattern)
		}
	}
	return nil
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "translatableElements":
			arr, err := toStringSlice(val, key)
			if err != nil {
				return err
			}
			c.TranslatableElements = arr
		case "translatableAttributes":
			arr, err := toStringSlice(val, key)
			if err != nil {
				return err
			}
			c.TranslatableAttributes = arr
		case "subfilters":
			sfs, err := parseSubfilterMappings(val)
			if err != nil {
				return fmt.Errorf("xml: subfilters: %w", err)
			}
			c.Subfilters = sfs
		case "preserveWhitespace":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("preserveWhitespace: expected bool, got %T", val)
			}
			c.PreserveWhitespace = b
		case "excludeByDefault":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("excludeByDefault: expected bool, got %T", val)
			}
			c.ExcludeByDefault = b
		case "inlineElements":
			arr, err := toStringSlice(val, key)
			if err != nil {
				return err
			}
			c.InlineElements = arr
		case "excludedElements":
			arr, err := toStringSlice(val, key)
			if err != nil {
				return err
			}
			c.ExcludedElements = arr
		case "preserveWhitespaceElements":
			arr, err := toStringSlice(val, key)
			if err != nil {
				return err
			}
			c.PreserveWhitespaceElements = arr
		case "groupElements":
			arr, err := toStringSlice(val, key)
			if err != nil {
				return err
			}
			c.GroupElements = arr
		case "blockTypeMap":
			m, ok := val.(map[string]any)
			if !ok {
				return fmt.Errorf("blockTypeMap: expected map, got %T", val)
			}
			c.BlockTypeMap = make(map[string]string)
			for k, v := range m {
				s, ok := v.(string)
				if !ok {
					return fmt.Errorf("blockTypeMap[%s]: expected string, got %T", k, v)
				}
				c.BlockTypeMap[k] = s
			}
		case "idAttributes":
			arr, err := toStringSlice(val, key)
			if err != nil {
				return err
			}
			c.IDAttributeNames = arr
		case "elements":
			rules, err := parseElementRules(val)
			if err != nil {
				return fmt.Errorf("xml: elements: %w", err)
			}
			c.ElementRules = rules
		case "attributes":
			rules, err := parseAttributeRules(val)
			if err != nil {
				return fmt.Errorf("xml: attributes: %w", err)
			}
			c.AttributeRules = rules
		case "useCodeFinder":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useCodeFinder: expected bool, got %T", val)
			}
			c.UseCodeFinder = b
			c.compiledCodeFinder = nil
		case "codeFinderRules":
			rules, err := parseCodeFinderRules(val)
			if err != nil {
				return fmt.Errorf("codeFinderRules: %w", err)
			}
			c.CodeFinderRules = rules
			c.compiledCodeFinder = nil
		case "parser":
			m, ok := val.(map[string]any)
			if !ok {
				return fmt.Errorf("parser: expected map, got %T", val)
			}
			for pk, pv := range m {
				switch pk {
				case "preserveWhitespace":
					b, ok := pv.(bool)
					if !ok {
						return fmt.Errorf("parser.preserveWhitespace: expected bool, got %T", pv)
					}
					c.PreserveWhitespace = b
				case "assumeWellformed":
					// Ignored — Go XML parser always handles this
				default:
					// Silently ignore unknown parser params
				}
			}
		default:
			// Silently ignore unknown parameters for forward compat
		}
	}
	return nil
}

func toStringSlice(val any, key string) ([]string, error) {
	arr, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected []any, got %T", key, val)
	}
	strs := make([]string, 0, len(arr))
	for i, elem := range arr {
		s, ok := elem.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: expected string, got %T", key, i, elem)
		}
		strs = append(strs, s)
	}
	return strs, nil
}

// parseSubfilterMappings parses subfilter config from a generic map value.
func parseSubfilterMappings(val any) ([]format.SubfilterMapping, error) {
	arr, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", val)
	}
	var result []format.SubfilterMapping
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object, got %T", item)
		}
		pattern, _ := m["pattern"].(string)
		formatName, _ := m["format"].(string)
		if pattern == "" || formatName == "" {
			return nil, errors.New("subfilter mapping requires 'pattern' and 'format'")
		}
		result = append(result, format.SubfilterMapping{Pattern: pattern, Format: formatName})
	}
	return result, nil
}

// parseElementRules parses element rules from config.
func parseElementRules(val any) ([]*ElementRule, error) {
	m, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map, got %T", val)
	}
	var rules []*ElementRule
	for name, v := range m {
		ruleMap, ok := v.(map[string]any)
		if !ok {
			continue
		}
		rule := &ElementRule{Name: name}

		// Check for regex pattern
		if len(name) >= 2 && name[0] == '\'' && name[len(name)-1] == '\'' {
			rule.isRegex = true
			pattern := name[1 : len(name)-1]
			compiled, err := regexp.Compile("^" + pattern + "$")
			if err != nil {
				return nil, fmt.Errorf("invalid regex %q: %w", pattern, err)
			}
			rule.compiled = compiled
		}

		// Parse ruleTypes
		if rt, ok := ruleMap["ruleTypes"]; ok {
			arr, ok := rt.([]string)
			if ok {
				for _, s := range arr {
					rule.RuleTypes = append(rule.RuleTypes, RuleType(s))
				}
			} else if anyArr, ok := rt.([]any); ok {
				for _, a := range anyArr {
					if s, ok := a.(string); ok {
						rule.RuleTypes = append(rule.RuleTypes, RuleType(s))
					}
				}
			}
		}

		// Parse conditions
		if cond, ok := ruleMap["conditions"]; ok {
			c, err := parseCondition(cond)
			if err == nil && c != nil {
				rule.Condition = c
			}
		}

		// Parse idAttributes
		if idAttrs, ok := ruleMap["idAttributes"]; ok {
			arr, ok := idAttrs.([]string)
			if ok {
				rule.IDAttributes = arr
			} else if anyArr, ok := idAttrs.([]any); ok {
				for _, a := range anyArr {
					if s, ok := a.(string); ok {
						rule.IDAttributes = append(rule.IDAttributes, s)
					}
				}
			}
		}

		// Parse translatableAttributes
		if ta, ok := ruleMap["translatableAttributes"]; ok {
			taMap, ok := ta.(map[string]any)
			if ok {
				rule.TranslatableAttributes = make(map[string]*TranslatableAttrCondition)
				for attrName, condVal := range taMap {
					tac := &TranslatableAttrCondition{}
					switch cv := condVal.(type) {
					case []string:
						if len(cv) == 3 {
							tac.Conditions = []Condition{{
								Attribute: cv[0],
								Op:        ConditionOp(cv[1]),
								Value:     cv[2],
							}}
						}
					case []any:
						conditions := parseTranslatableAttrConditions(cv)
						tac.Conditions = conditions
					}
					rule.TranslatableAttributes[attrName] = tac
				}
			}
		}

		rules = append(rules, rule)
	}
	return rules, nil
}

func parseTranslatableAttrConditions(arr []any) []Condition {
	var conditions []Condition
	// Could be a single condition [attr, op, val] or array of conditions [[attr, op, val], ...]
	if len(arr) == 3 {
		if _, ok := arr[0].(string); ok {
			// Single condition
			attr, _ := arr[0].(string)
			op, _ := arr[1].(string)
			val, _ := arr[2].(string)
			conditions = append(conditions, Condition{
				Attribute: attr,
				Op:        ConditionOp(op),
				Value:     val,
			})
			return conditions
		}
	}
	// Multiple conditions (OR'd)
	for _, item := range arr {
		switch cv := item.(type) {
		case []string:
			if len(cv) == 3 {
				conditions = append(conditions, Condition{
					Attribute: cv[0],
					Op:        ConditionOp(cv[1]),
					Value:     cv[2],
				})
			}
		case []any:
			if len(cv) == 3 {
				attr, _ := cv[0].(string)
				op, _ := cv[1].(string)
				val, _ := cv[2].(string)
				if attr != "" && op != "" {
					conditions = append(conditions, Condition{
						Attribute: attr,
						Op:        ConditionOp(op),
						Value:     val,
					})
				}
			}
		}
	}
	return conditions
}

func parseCondition(val any) (*Condition, error) {
	switch v := val.(type) {
	case []string:
		if len(v) == 3 {
			return &Condition{
				Attribute: v[0],
				Op:        ConditionOp(v[1]),
				Value:     v[2],
			}, nil
		}
	case []any:
		if len(v) == 3 {
			attr, _ := v[0].(string)
			op, _ := v[1].(string)
			value, _ := v[2].(string)
			if attr != "" && op != "" {
				return &Condition{
					Attribute: attr,
					Op:        ConditionOp(op),
					Value:     value,
				}, nil
			}
		}
	}
	return nil, nil
}

// parseAttributeRules parses attribute rules from config.
func parseAttributeRules(val any) ([]*AttributeRule, error) {
	m, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map, got %T", val)
	}
	var rules []*AttributeRule
	for name, v := range m {
		ruleMap, ok := v.(map[string]any)
		if !ok {
			continue
		}
		rule := &AttributeRule{Name: name}

		// Check for regex pattern
		if len(name) >= 2 && name[0] == '\'' && name[len(name)-1] == '\'' {
			rule.isRegex = true
			pattern := name[1 : len(name)-1]
			compiled, err := regexp.Compile("^" + pattern + "$")
			if err != nil {
				return nil, fmt.Errorf("invalid regex %q: %w", pattern, err)
			}
			rule.compiled = compiled
		}

		// Parse ruleTypes
		if rt, ok := ruleMap["ruleTypes"]; ok {
			arr, ok := rt.([]string)
			if ok {
				for _, s := range arr {
					rule.RuleTypes = append(rule.RuleTypes, RuleType(s))
				}
			} else if anyArr, ok := rt.([]any); ok {
				for _, a := range anyArr {
					if s, ok := a.(string); ok {
						rule.RuleTypes = append(rule.RuleTypes, RuleType(s))
					}
				}
			}
		}

		// Parse allElementsExcept
		if exc, ok := ruleMap["allElementsExcept"]; ok {
			arr, ok := exc.([]string)
			if ok {
				rule.AllElementsExcept = arr
			} else if anyArr, ok := exc.([]any); ok {
				for _, a := range anyArr {
					if s, ok := a.(string); ok {
						rule.AllElementsExcept = append(rule.AllElementsExcept, s)
					}
				}
			}
		}

		// Parse onlyTheseElements
		if only, ok := ruleMap["onlyTheseElements"]; ok {
			arr, ok := only.([]string)
			if ok {
				rule.OnlyTheseElements = arr
			} else if anyArr, ok := only.([]any); ok {
				for _, a := range anyArr {
					if s, ok := a.(string); ok {
						rule.OnlyTheseElements = append(rule.OnlyTheseElements, s)
					}
				}
			}
		}

		rules = append(rules, rule)
	}
	return rules, nil
}

// isInlineElementNS checks whether the given element is configured as
// inline, with the element's namespace URI and parent local name
// available for namespace-/parent-scoped rules.
func (c *Config) isInlineElementNS(name, nsURI, parentName string) bool {
	for _, e := range c.InlineElements {
		if e == name {
			return true
		}
	}
	// Check element rules
	for _, r := range c.ElementRules {
		if r.matchesCtx(name, nsURI, parentName) && r.HasRule(RuleInline) {
			return true
		}
	}
	return false
}

// elementCtx carries the contextual identity of an element needed to
// evaluate namespace-/parent-scoped rules and parent-targeted conditions.
type elementCtx struct {
	local       string
	nsURI       string
	attrs       map[string]string
	parentName  string
	parentAttrs map[string]string
}

// isExcludedElementCtx checks whether the given element is excluded, with
// the surrounding context (namespace URI, parent name and parent
// attributes) available for namespace-/parent-scoped rules and
// parent-targeted conditions.
func (c *Config) isExcludedElementCtx(ctx elementCtx) bool {
	for _, e := range c.ExcludedElements {
		if e == ctx.local {
			return true
		}
	}
	for _, r := range c.ElementRules {
		if r.matchesCtx(ctx.local, ctx.nsURI, ctx.parentName) && r.HasRule(RuleExclude) {
			if r.Condition != nil || len(r.Conditions) > 0 {
				if r.conditionsHold(ctx.attrs, ctx.parentAttrs) {
					return true
				}
				continue
			}
			return true
		}
	}
	return false
}

// isIncludedElementCtx checks whether the given element is explicitly
// included, with the surrounding context available for namespace-/
// parent-scoped rules and parent-targeted conditions.
func (c *Config) isIncludedElementCtx(ctx elementCtx) bool {
	for _, r := range c.ElementRules {
		if r.matchesCtx(ctx.local, ctx.nsURI, ctx.parentName) && r.HasRule(RuleInclude) {
			if r.Condition != nil || len(r.Conditions) > 0 {
				if r.conditionsHold(ctx.attrs, ctx.parentAttrs) {
					return true
				}
				continue
			}
			return true
		}
	}
	return false
}

// shouldPreserveWhitespace checks whether the given element preserves whitespace.
func (c *Config) shouldPreserveWhitespace(name string) bool {
	if c.PreserveWhitespace {
		return true
	}
	for _, e := range c.PreserveWhitespaceElements {
		if e == name {
			return true
		}
	}
	for _, r := range c.ElementRules {
		if r.Matches(name) && r.HasRule(RulePreserveWhitespace) {
			return true
		}
	}
	return false
}

// getBlockType returns the block type for the given element name.
func (c *Config) getBlockType(name string) string {
	if c.BlockTypeMap != nil {
		if t, ok := c.BlockTypeMap[name]; ok {
			return t
		}
	}
	return ""
}

// getIDAttribute returns the ID value from the given attributes, based on config.
func (c *Config) getIDAttribute(elemName string, attrs map[string]string) string {
	// Check element rules first
	for _, r := range c.ElementRules {
		if r.Matches(elemName) && len(r.IDAttributes) > 0 {
			for _, idAttr := range r.IDAttributes {
				if v, ok := attrs[idAttr]; ok {
					return v
				}
			}
		}
	}
	// Then check global ID attribute names
	for _, idAttr := range c.IDAttributeNames {
		if v, ok := attrs[idAttr]; ok {
			return v
		}
	}
	// Fallback: check for "id" attribute
	if v, ok := attrs["id"]; ok {
		return v
	}
	return ""
}

// parentIDAttr returns the parent-attribute name that supplies the block
// id for the given element, from the first matching element rule that
// declares ParentIDAttr (empty when none). Namespace and parent-element
// scoping on the rule are honored.
func (c *Config) parentIDAttr(local, nsURI, parentName string) string {
	for _, r := range c.ElementRules {
		if r.matchesCtx(local, nsURI, parentName) && r.ParentIDAttr != "" {
			return r.ParentIDAttr
		}
	}
	return ""
}

// isTranslatableAttribute checks whether the attribute is translatable for the given element.
func (c *Config) isTranslatableAttribute(elemName, attrName string, allAttrs map[string]string) bool {
	// Check element-level translatable attributes
	for _, r := range c.ElementRules {
		if r.Matches(elemName) && r.TranslatableAttributes != nil {
			if tac, ok := r.TranslatableAttributes[attrName]; ok {
				if len(tac.Conditions) == 0 {
					return true
				}
				// OR logic: any condition matching means translatable
				for _, cond := range tac.Conditions {
					if cond.Evaluate(allAttrs) {
						return true
					}
				}
				return false
			}
		}
	}
	// Check attribute rules for ATTRIBUTE_TRANS
	for _, r := range c.AttributeRules {
		if r.Matches(attrName) && r.HasAttrRule(RuleAttributeTrans) {
			if r.AppliesToElement(elemName) {
				return true
			}
		}
	}
	// Check simple list
	for _, a := range c.TranslatableAttributes {
		if a == attrName {
			return true
		}
	}
	return false
}

// getWritableAttributes returns writable attribute values for the given element.
func (c *Config) getWritableAttributes(elemName string, attrs map[string]string) map[string]string {
	result := make(map[string]string)
	for _, r := range c.AttributeRules {
		if r.HasAttrRule(RuleAttributeWritable) && !r.isRegex {
			if r.AppliesToElement(elemName) {
				if val, ok := attrs[r.Name]; ok {
					result[r.Name] = val
				}
			}
		}
	}
	return result
}

// HasAttrRule returns true if the attribute rule set includes the given rule type.
func (r *AttributeRule) HasAttrRule(rt RuleType) bool {
	for _, t := range r.RuleTypes {
		if t == rt {
			return true
		}
	}
	return false
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

// parseCodeFinderRules parses code finder rules from bridge-style map or string slice.
func parseCodeFinderRules(val any) ([]string, error) {
	if rules, ok := val.([]string); ok {
		return rules, nil
	}
	if m, ok := val.(map[string]any); ok {
		count := 0
		if c, ok := m["count"]; ok {
			switch v := c.(type) {
			case int:
				count = v
			case float64:
				count = int(v)
			}
		}
		var rules []string
		for i := range count {
			key := fmt.Sprintf("rule%d", i)
			if rule, ok := m[key].(string); ok {
				rules = append(rules, rule)
			}
		}
		return rules, nil
	}
	return nil, fmt.Errorf("expected []string or map, got %T", val)
}

// CollapseWhitespace replaces sequences of whitespace with a single space
// and trims leading/trailing whitespace.
func CollapseWhitespace(s string) string {
	var buf strings.Builder
	inSpace := false
	started := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if started {
				inSpace = true
			}
		} else {
			if inSpace {
				buf.WriteByte(' ')
				inSpace = false
			}
			buf.WriteRune(r)
			started = true
		}
	}
	return buf.String()
}
