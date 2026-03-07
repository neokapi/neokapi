package config

// Envelope is a k8s-style versioned configuration wrapper.
// All gokapi config files (format configs, presets, flows, project configs)
// use this structure for schema versioning and cross-format transformation.
type Envelope struct {
	APIVersion string         `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind           `json:"kind"       yaml:"kind"`
	Metadata   Metadata       `json:"metadata"   yaml:"metadata"`
	Spec       map[string]any `json:"spec"       yaml:"spec"`
}

// Kind identifies the type of configuration in an envelope.
type Kind string

const (
	KindFormatConfig   Kind = "FormatConfig"
	KindFormatPreset   Kind = "FormatPreset"
	KindFlowDefinition Kind = "FlowDefinition"
	KindProjectConfig  Kind = "ProjectConfig"
)

// ValidKinds returns all valid Kind values.
func ValidKinds() []Kind {
	return []Kind{KindFormatConfig, KindFormatPreset, KindFlowDefinition, KindProjectConfig}
}

// IsValid reports whether the kind is a recognized value.
func (k Kind) IsValid() bool {
	switch k {
	case KindFormatConfig, KindFormatPreset, KindFlowDefinition, KindProjectConfig:
		return true
	}
	return false
}

// Metadata holds optional metadata for a configuration envelope.
type Metadata struct {
	Name        string            `json:"name"                  yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}
