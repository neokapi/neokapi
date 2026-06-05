package json

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format"
)

// Config holds configuration for the JSON format.
type Config struct {
	// ExtractAllPairs controls whether all string key-value pairs are extracted.
	// Defaults to true.
	ExtractAllPairs bool

	// Exceptions is a regex pattern for key names. When ExtractAllPairs is true,
	// matching keys are excluded. When false, matching keys are included.
	Exceptions string

	// ExtractIsolatedStrings controls whether standalone string values inside
	// arrays are extracted as translatable Blocks. Defaults to false.
	// (Named extractStandalone in Java, extractIsolatedStrings in fprm.)
	ExtractIsolatedStrings bool

	// UseKeyAsName uses the JSON key as the block name. Defaults to true.
	UseKeyAsName bool

	// UseFullKeyPath uses hierarchical paths (parent/child) as block names.
	UseFullKeyPath bool

	// UseLeadingSlashOnKeyPath prepends / to full key paths. Defaults to true.
	UseLeadingSlashOnKeyPath bool

	// EscapeForwardSlashes escapes / as \/ in JSON output. Defaults to true.
	EscapeForwardSlashes bool

	// Subfilters maps JSON key path patterns to format names for embedded content.
	Subfilters []format.SubfilterMapping

	// SubfilterFormat is the global subfilter format name (e.g., "html").
	// When set, all extracted string values are processed through this subfilter
	// unless SubfilterRules restricts it to specific keys.
	SubfilterFormat string

	// SubfilterRules is a regex pattern for key names that should be processed
	// by the subfilter. Only used when SubfilterFormat is set.
	SubfilterRules string

	// NoteRules is a regex pattern for key names whose values become notes
	// attached to the next translatable block.
	NoteRules string

	// IDRules is a regex pattern for key names whose values are used as
	// block names/IDs for the next translatable block.
	IDRules string

	// UseIDStack stacks IDs for nested structures, producing compound IDs.
	UseIDStack bool

	// GenericMetaRules is a regex pattern for key names whose values become
	// metadata annotations on the next translatable block.
	GenericMetaRules string

	// ExtractionRules is a regex pattern that limits which keys are extracted.
	// Only keys matching this pattern are extracted. Applied to key names or
	// full key paths depending on UseFullKeyPath.
	ExtractionRules string

	// MaxwidthRules is a regex pattern for key names whose numeric values
	// set the MAX_WIDTH property on the next translatable block.
	MaxwidthRules string

	// MaxwidthSizeUnit sets the SIZE_UNIT property when maxwidth is set.
	// Values: "pixel" (default) or "char".
	MaxwidthSizeUnit string

	// UseCodeFinder enables regex-based inline code detection.
	UseCodeFinder bool

	// CodeFinderRules defines inline code patterns.
	CodeFinderRules []string

	// compiled regex caches
	compiledExceptions      *regexp.Regexp
	compiledExtractionRules *regexp.Regexp
	compiledNoteRules       *regexp.Regexp
	compiledIDRules         *regexp.Regexp
	compiledGenericMeta     *regexp.Regexp
	compiledMaxwidth        *regexp.Regexp
	compiledSubfilterRules  *regexp.Regexp
	compiledCodeFinder      []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "json" }

// ConfigKind returns the Kind for JSON format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("json") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		ExtractAllPairs:          true,
		UseKeyAsName:             true,
		UseLeadingSlashOnKeyPath: true,
		EscapeForwardSlashes:     true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	for _, sf := range c.Subfilters {
		if sf.Pattern == "" {
			return errors.New("json: subfilter mapping has empty pattern")
		}
		if sf.Format == "" {
			return fmt.Errorf("json: subfilter mapping for %q has empty format", sf.Pattern)
		}
	}
	return nil
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractAllPairs":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractAllPairs: expected bool, got %T", val)
			}
			c.ExtractAllPairs = b
		case "exceptions":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("exceptions: expected string, got %T", val)
			}
			c.Exceptions = s
		case "extractIsolatedStrings", "extractArrayStrings":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("%s: expected bool, got %T", key, val)
			}
			c.ExtractIsolatedStrings = b
		case "useKeyAsName":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useKeyAsName: expected bool, got %T", val)
			}
			c.UseKeyAsName = b
		case "useFullKeyPath":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useFullKeyPath: expected bool, got %T", val)
			}
			c.UseFullKeyPath = b
		case "useLeadingSlashOnKeyPath":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useLeadingSlashOnKeyPath: expected bool, got %T", val)
			}
			c.UseLeadingSlashOnKeyPath = b
		case "escapeForwardSlashes":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("escapeForwardSlashes: expected bool, got %T", val)
			}
			c.EscapeForwardSlashes = b
		case "subfilters":
			sfs, err := parseSubfilterMappings(val)
			if err != nil {
				return fmt.Errorf("json: subfilters: %w", err)
			}
			c.Subfilters = sfs
		case "subfilter":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("subfilter: expected string, got %T", val)
			}
			c.SubfilterFormat = s
		case "subfilterRules":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("subfilterRules: expected string, got %T", val)
			}
			c.SubfilterRules = s
		case "noteRules":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("noteRules: expected string, got %T", val)
			}
			c.NoteRules = s
		case "idRules":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("idRules: expected string, got %T", val)
			}
			c.IDRules = s
		case "useIdStack":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useIdStack: expected bool, got %T", val)
			}
			c.UseIDStack = b
		case "genericMetaRules":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("genericMetaRules: expected string, got %T", val)
			}
			c.GenericMetaRules = s
		case "extractionRules":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("extractionRules: expected string, got %T", val)
			}
			c.ExtractionRules = s
		case "maxwidthRules":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("maxwidthRules: expected string, got %T", val)
			}
			c.MaxwidthRules = s
		case "maxwidthSizeUnit":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("maxwidthSizeUnit: expected string, got %T", val)
			}
			c.MaxwidthSizeUnit = s
		case "useCodeFinder":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useCodeFinder: expected bool, got %T", val)
			}
			c.UseCodeFinder = b
		case "codeFinderRules":
			rules, err := parseCodeFinderRules(val)
			if err != nil {
				return fmt.Errorf("codeFinderRules: %w", err)
			}
			c.CodeFinderRules = rules
		default:
			return fmt.Errorf("json: unknown parameter: %s", key)
		}
	}
	c.clearCompiledRegex()
	return nil
}

func (c *Config) clearCompiledRegex() {
	c.compiledExceptions = nil
	c.compiledExtractionRules = nil
	c.compiledNoteRules = nil
	c.compiledIDRules = nil
	c.compiledGenericMeta = nil
	c.compiledMaxwidth = nil
	c.compiledSubfilterRules = nil
	c.compiledCodeFinder = nil
}

func (c *Config) getExceptions() *regexp.Regexp {
	if c.compiledExceptions == nil && c.Exceptions != "" {
		c.compiledExceptions = regexp.MustCompile(c.Exceptions)
	}
	return c.compiledExceptions
}

func (c *Config) getExtractionRules() *regexp.Regexp {
	if c.compiledExtractionRules == nil && c.ExtractionRules != "" {
		c.compiledExtractionRules = regexp.MustCompile(c.ExtractionRules)
	}
	return c.compiledExtractionRules
}

func (c *Config) getNoteRules() *regexp.Regexp {
	if c.compiledNoteRules == nil && c.NoteRules != "" {
		c.compiledNoteRules = regexp.MustCompile(c.NoteRules)
	}
	return c.compiledNoteRules
}

func (c *Config) getIDRules() *regexp.Regexp {
	if c.compiledIDRules == nil && c.IDRules != "" {
		c.compiledIDRules = regexp.MustCompile(c.IDRules)
	}
	return c.compiledIDRules
}

func (c *Config) getGenericMetaRules() *regexp.Regexp {
	if c.compiledGenericMeta == nil && c.GenericMetaRules != "" {
		c.compiledGenericMeta = regexp.MustCompile(c.GenericMetaRules)
	}
	return c.compiledGenericMeta
}

func (c *Config) getMaxwidthRules() *regexp.Regexp {
	if c.compiledMaxwidth == nil && c.MaxwidthRules != "" {
		c.compiledMaxwidth = regexp.MustCompile(c.MaxwidthRules)
	}
	return c.compiledMaxwidth
}

func (c *Config) getSubfilterRules() *regexp.Regexp {
	if c.compiledSubfilterRules == nil && c.SubfilterRules != "" {
		c.compiledSubfilterRules = regexp.MustCompile(c.SubfilterRules)
	}
	return c.compiledSubfilterRules
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

// shouldExtract decides whether a key should be extracted based on config rules.
func (c *Config) shouldExtract(keyName, fullPath string) bool {
	matchTarget := keyName
	if c.UseFullKeyPath {
		matchTarget = fullPath
	}

	// If extractionRules is set, only extract keys matching it
	if re := c.getExtractionRules(); re != nil {
		return re.MatchString(matchTarget)
	}

	// Default: extractAllPairs with exceptions
	if c.ExtractAllPairs {
		if re := c.getExceptions(); re != nil {
			return !re.MatchString(matchTarget)
		}
		return true
	}

	// extractAllPairs=false: only extract if key matches exceptions
	if re := c.getExceptions(); re != nil {
		return re.MatchString(matchTarget)
	}
	return false
}

// isNote returns true if the key matches noteRules.
func (c *Config) isNote(keyName, fullPath string) bool {
	re := c.getNoteRules()
	if re == nil {
		return false
	}
	if c.UseFullKeyPath {
		return re.MatchString(fullPath)
	}
	return re.MatchString(keyName)
}

// isID returns true if the key matches idRules.
func (c *Config) isID(keyName, fullPath string) bool {
	re := c.getIDRules()
	if re == nil {
		return false
	}
	if c.UseFullKeyPath {
		return re.MatchString(fullPath)
	}
	return re.MatchString(keyName)
}

// isGenericMeta returns true if the key matches genericMetaRules.
func (c *Config) isGenericMeta(keyName, fullPath string) bool {
	re := c.getGenericMetaRules()
	if re == nil {
		return false
	}
	if c.UseFullKeyPath {
		return re.MatchString(fullPath)
	}
	return re.MatchString(keyName)
}

// isMaxwidth returns true if the key matches maxwidthRules.
func (c *Config) isMaxwidth(keyName, fullPath string) bool {
	re := c.getMaxwidthRules()
	if re == nil {
		return false
	}
	if c.UseFullKeyPath {
		return re.MatchString(fullPath)
	}
	return re.MatchString(keyName)
}

// shouldSubfilter returns true if the key should be processed by the subfilter.
func (c *Config) shouldSubfilter(keyName, fullPath string) bool {
	if c.SubfilterFormat == "" {
		return false
	}
	re := c.getSubfilterRules()
	if re == nil {
		return true // no rules = apply to all
	}
	if c.UseFullKeyPath {
		return re.MatchString(fullPath)
	}
	return re.MatchString(keyName)
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

// parseCodeFinderRules parses code finder rules from bridge-style map or string slice.
func parseCodeFinderRules(val any) ([]string, error) {
	// Handle direct string slice
	if rules, ok := val.([]string); ok {
		return rules, nil
	}
	// Handle bridge-style map with count + rule0, rule1, etc.
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
