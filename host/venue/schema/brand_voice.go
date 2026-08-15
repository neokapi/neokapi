package schema

// BrandVoiceSpec holds brand voice profile bindings for a project (per
// Bowrain AD-015). Like AssetsSpec, this is project-level policy that only
// affects bowrain-connected workflows today, but is declared at the recipe
// level for clarity.
type BrandVoiceSpec struct {
	// Profile is the default brand voice profile ID for this project.
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`

	// Channel is the default channel override key for this project.
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`

	// Collections binds collection names to per-collection brand voice
	// settings, overriding the project-level Profile/Channel.
	Collections map[string]*BrandVoiceEntry `yaml:"collections,omitempty" json:"collections,omitempty"`
}

// Validate is a no-op today — present for symmetry with the other schema
// types and to give us a place to grow into.
func (b *BrandVoiceSpec) Validate() error {
	return nil
}

// BrandVoiceEntry is a per-scope brand voice binding.
type BrandVoiceEntry struct {
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`
}
