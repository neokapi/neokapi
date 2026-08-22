package schema

// VoiceSpec holds voice profile bindings for a project (per
// Bowrain AD-015). Like AssetsSpec, this is project-level policy that only
// affects bowrain-connected workflows today, but is declared at the recipe
// level for clarity.
type VoiceSpec struct {
	// Profile is the default voice profile ID for this project.
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`

	// Channel is the default channel override key for this project.
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`

	// Collections binds collection names to per-collection voice
	// settings, overriding the project-level Profile/Channel.
	Collections map[string]*VoiceEntry `yaml:"collections,omitempty" json:"collections,omitempty"`
}

// Validate is a no-op today — present for symmetry with the other schema
// types and to give us a place to grow into.
func (b *VoiceSpec) Validate() error {
	return nil
}

// VoiceEntry is a per-scope voice binding.
type VoiceEntry struct {
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`
}
