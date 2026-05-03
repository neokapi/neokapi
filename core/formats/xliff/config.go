package xliff

import (
	"fmt"

	"github.com/neokapi/neokapi/core/config"
)

// Config holds configuration for the XLIFF 1.2 format.
type Config struct {
	// Extraction settings

	// PreserveSpaceByDefault treats all content as xml:space="preserve" unless
	// explicitly overridden. Defaults to false.
	PreserveSpaceByDefault bool

	// IncludeExtensions includes XLIFF extension elements (non-standard namespaces)
	// in the extracted content. Defaults to true.
	IncludeExtensions bool

	// IncludeIts includes ITS (Internationalization Tag Set) markup. Defaults to true.
	IncludeIts bool

	// IgnoreInputSegmentation ignores <seg-source>/<mrk> segmentation from
	// the input XLIFF and treats the full source as a single segment. Defaults to false.
	IgnoreInputSegmentation bool

	// FallbackToID uses the trans-unit id attribute as the block name when no
	// resname attribute is present. Defaults to false.
	FallbackToID bool

	// ForceUniqueIds rewrites trans-unit IDs to enforce uniqueness across the
	// file. Defaults to false.
	ForceUniqueIds bool

	// States settings

	// UseTranslationTargetState updates the state attribute on <target> elements
	// when writing translated XLIFF. Defaults to true.
	UseTranslationTargetState bool

	// TargetStateValue is the state value to set on target elements when
	// UseTranslationTargetState is true. Defaults to "needs-translation".
	TargetStateValue string

	// EditAltTrans allows editing of <alt-trans> elements. Defaults to false.
	EditAltTrans bool

	// AddAltTrans allows addition of new <alt-trans> elements. Defaults to false.
	AddAltTrans bool

	// Output settings

	// AddTargetLanguage adds the target-language attribute to <file> elements
	// if not already present. Defaults to true.
	AddTargetLanguage bool

	// OverrideTargetLanguage overrides the target-language attribute in the
	// output document. Defaults to false.
	OverrideTargetLanguage bool

	// AllowEmptyTargets allows <target> elements with empty content to be
	// written. Defaults to false.
	AllowEmptyTargets bool

	// AlwaysAddTargets adds <target> elements for monolingual XLIFF (when no
	// target exists in the source). Defaults to false.
	AlwaysAddTargets bool

	// Inline codes settings

	// UseCodeFinder enables regex-based inline code detection. Defaults to false.
	UseCodeFinder bool

	// CodeFinderRules defines inline code patterns.
	CodeFinderRules []string

	// OkapiCompat enables opt-in writer behaviors that mimic okapi's
	// XLIFFWriter quirks for byte-equivalent parity comparison. Default
	// is the zero value (all flags off) — neokapi's xliff writer
	// otherwise follows the XLIFF 1.2 spec and intuitive defaults. See
	// OkapiCompatConfig for the full flag list and per-flag rationale.
	OkapiCompat OkapiCompatConfig
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "xliff" }

// ConfigKind returns the Kind for XLIFF format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("xliff") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		IncludeExtensions:         true,
		IncludeIts:                true,
		UseTranslationTargetState: true,
		TargetStateValue:          "needs-translation",
		AddTargetLanguage:         true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		// Extraction
		case "preserveSpaceByDefault":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("preserveSpaceByDefault: expected bool, got %T", val)
			}
			c.PreserveSpaceByDefault = b
		case "includeExtensions":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("includeExtensions: expected bool, got %T", val)
			}
			c.IncludeExtensions = b
		case "includeIts":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("includeIts: expected bool, got %T", val)
			}
			c.IncludeIts = b
		case "ignoreInputSegmentation":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("ignoreInputSegmentation: expected bool, got %T", val)
			}
			c.IgnoreInputSegmentation = b
		case "fallbackToID":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("fallbackToID: expected bool, got %T", val)
			}
			c.FallbackToID = b
		case "forceUniqueIds":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("forceUniqueIds: expected bool, got %T", val)
			}
			c.ForceUniqueIds = b

		// States
		case "useTranslationTargetState":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useTranslationTargetState: expected bool, got %T", val)
			}
			c.UseTranslationTargetState = b
		case "targetStateValue":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("targetStateValue: expected string, got %T", val)
			}
			c.TargetStateValue = s
		case "editAltTrans":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("editAltTrans: expected bool, got %T", val)
			}
			c.EditAltTrans = b
		case "addAltTrans":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("addAltTrans: expected bool, got %T", val)
			}
			c.AddAltTrans = b

		// Output
		case "addTargetLanguage":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("addTargetLanguage: expected bool, got %T", val)
			}
			c.AddTargetLanguage = b
		case "overrideTargetLanguage":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("overrideTargetLanguage: expected bool, got %T", val)
			}
			c.OverrideTargetLanguage = b
		case "allowEmptyTargets":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("allowEmptyTargets: expected bool, got %T", val)
			}
			c.AllowEmptyTargets = b
		case "alwaysAddTargets":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("alwaysAddTargets: expected bool, got %T", val)
			}
			c.AlwaysAddTargets = b

		// Inline codes
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

		case "okapiCompat":
			m, ok := val.(map[string]any)
			if !ok {
				return fmt.Errorf("okapiCompat: expected map, got %T", val)
			}
			if err := applyOkapiCompatMap(&c.OkapiCompat, m); err != nil {
				return fmt.Errorf("okapiCompat: %w", err)
			}

		default:
			return fmt.Errorf("xliff: unknown parameter: %s", key)
		}
	}
	return nil
}

// applyOkapiCompatMap fills an OkapiCompatConfig from a map. Each key
// maps to one bool field; unknown keys are an error so typos surface
// in tests instead of silently being ignored.
func applyOkapiCompatMap(o *OkapiCompatConfig, m map[string]any) error {
	for key, val := range m {
		b, ok := val.(bool)
		if !ok {
			return fmt.Errorf("%s: expected bool, got %T", key, val)
		}
		switch key {
		case "lowercaseLangSubtag":
			o.LowercaseLangSubtag = b
		case "unwrapSingleSegMrk":
			o.UnwrapSingleSegMrk = b
		case "stripTransUnitApprovedAttr":
			o.StripTransUnitApprovedAttr = b
		case "stripPhaseDateAttr":
			o.StripPhaseDateAttr = b
		case "stripCDataCREntities":
			o.StripCDataCREntities = b
		case "hoistAltTransNotes":
			o.HoistAltTransNotes = b
		case "escapeNonASCIIAsEntities":
			o.EscapeNonASCIIAsEntities = b
		case "simulateBrokenWindows1252Read":
			o.SimulateBrokenWindows1252Read = b
		case "reorderHeaderToolToEnd":
			o.ReorderHeaderToolToEnd = b
		default:
			return fmt.Errorf("unknown flag: %s", key)
		}
	}
	return nil
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
