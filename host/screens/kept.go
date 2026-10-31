package screens

// KeptRuleDTO is one rule in force and who kept it, a line of Kept by on the
// Rules screen.
type KeptRuleDTO struct {
	// Kind is "name", "word", "tone" or "note".
	Kind string `json:"kind"`
	// Rule is the rule as a person reads it.
	Rule string `json:"rule"`
	// From is where the rule is held: terms, a pack, the workspace, the voice
	// profile. Empty for a rule the operation log alone holds.
	From string `json:"from,omitempty"`
	// Applies is where the rule holds.
	Applies string          `json:"applies"`
	KeptBy  ContextActorDTO `json:"kept_by"`
	// KeptAt is when the rule came into force, RFC 3339.
	KeptAt string `json:"kept_at"`
}
