package schema

// AssetsSpec controls project-wide media asset sync behavior (per Bowrain
// AD-007). Currently only meaningful for bowrain-connected projects, but
// declared at the project level so the policy is recipe-owned, not
// server-owned.
type AssetsSpec struct {
	// Enabled is the master toggle for asset sync. Defaults to true when nil.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`

	// Exclude is a list of filename glob patterns that are skipped.
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`

	// MaxSize is the global per-asset size limit (e.g. "100MB").
	MaxSize string `yaml:"max_size,omitempty" json:"max_size,omitempty"`
}

// IsEnabled reports whether asset sync is enabled. Defaults to true when
// the spec or its Enabled field is unset.
func (a *AssetsSpec) IsEnabled() bool {
	if a == nil || a.Enabled == nil {
		return true
	}
	return *a.Enabled
}

// Validate is a no-op today — present for symmetry with the other schema
// types and to give us a place to grow into.
func (a *AssetsSpec) Validate() error {
	return nil
}
