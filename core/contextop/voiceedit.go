package contextop

// VoiceEdit is what a whole-profile write did to a voice profile, carried by
// the edit operation that records it. It is what a reader drills into, and what
// dropping the edit restores.
type VoiceEdit struct {
	// Profile is the id of the voice profile that was written.
	Profile string `json:"profile"`
	// Changes are the fields that differ between the profile as it stood and
	// as it was written, in the order the authored document lists them.
	// Omitted lists the changes past the cap a record keeps.
	Changes []VoiceChange `json:"changes,omitempty"`
	Omitted int           `json:"omitted,omitempty"`
	// Previous is the profile as it stood before the write, as the YAML a
	// project authors. Empty when the write created the profile.
	Previous string `json:"previous,omitempty"`
	// Wrote fingerprints the profile as the write left it, so dropping the
	// edit restores Previous only while nothing has written the profile since.
	Wrote string `json:"wrote,omitempty"`
}

// VoiceChange is one field a voice-profile write changed. From is empty for a
// field the write added, and To for one it removed.
type VoiceChange struct {
	// Field is the field's path in the authored document: "tone.formality",
	// "constraints[no-passive].statement".
	Field string `json:"field"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}
