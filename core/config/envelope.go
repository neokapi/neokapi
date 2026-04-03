package config

import "strings"

// Envelope is a k8s-style versioned configuration wrapper.
// All neokapi config files (format configs, presets, flows, project configs)
// use this structure for schema versioning and cross-format transformation.
type Envelope struct {
	APIVersion string         `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind           `json:"kind"       yaml:"kind"`
	Metadata   Metadata       `json:"metadata"   yaml:"metadata"`
	Spec       map[string]any `json:"spec"       yaml:"spec"`
}

// Kind identifies the type of configuration in an envelope.
// Non-format kinds use fixed constants. Format kinds are generated per-format:
//   - Native: HtmlFormatConfig, JsonFormatConfig, etc.
//   - Okapi:  OkfHtmlFilterConfig, OkfJsonFilterConfig, etc.
type Kind string

const (
	KindFormatPreset   Kind = "FormatPreset"
	KindFlowDefinition Kind = "FlowDefinition"
	KindProjectConfig  Kind = "ProjectConfig"
)

// FormatConfigKind returns the Kind for a native format config.
// e.g., "html" → "HtmlFormatConfig", "json" → "JsonFormatConfig".
func FormatConfigKind(formatName string) Kind {
	return Kind(pascalCase(formatName) + "FormatConfig")
}

// OkapiFilterConfigKind returns the Kind for an Okapi filter config.
// e.g., "html" → "OkfHtmlFilterConfig", "json" → "OkfJsonFilterConfig".
func OkapiFilterConfigKind(formatName string) Kind {
	return Kind("Okf" + pascalCase(formatName) + "FilterConfig")
}

// IsFormatConfigKind reports whether the kind represents a format config
// (either native FormatConfig or Okapi FilterConfig).
func IsFormatConfigKind(k Kind) bool {
	s := string(k)
	return strings.HasSuffix(s, "FormatConfig") || strings.HasSuffix(s, "FilterConfig")
}

// IsValid reports whether the kind is a recognized value.
func (k Kind) IsValid() bool {
	switch k {
	case KindFormatPreset, KindFlowDefinition, KindProjectConfig:
		return true
	}
	return IsFormatConfigKind(k)
}

// Metadata holds optional metadata for a configuration envelope.
type Metadata struct {
	Name        string            `json:"name"                  yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

func pascalCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
