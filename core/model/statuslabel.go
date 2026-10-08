package model

// StatusLabel is the word a person reads for a lifecycle status on a user
// surface (CLI output, MCP text, the apps, the docs). Content a person reviewed
// and let stand reads as "approved"; every other rung reads as its own name.
//
// The label is display only. The persisted value, the wire value, the JSON keys
// and the recipe keys (`established: 100`, `translate_after: established`)
// keep the status name, so this is the one place a surface turns a status into
// prose. Context rules keep "established": that word belongs to a rule in
// force and never passes through here.
func StatusLabel(status string) string {
	if status == string(TargetStatusEstablished) {
		return "approved"
	}
	return status
}

// Label is the display word for a translation status. See StatusLabel.
func (s TargetStatus) Label() string { return StatusLabel(string(s)) }

// Label is the display word for a source status. See StatusLabel.
func (s SourceStatus) Label() string { return StatusLabel(string(s)) }
