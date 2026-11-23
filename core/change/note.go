package change

import (
	"regexp"
	"strings"
)

// A person who declares themselves with KAPI_ACTOR=person inside an agent
// host's shell is recorded as a person, and the agent host is recorded beside
// the note as Actor.AgentHost (state.Decision.AgentHost,
// venue.UnitDecision.AgentHost). The note stays what the person typed.
//
// Released clients folded the host into the note instead, as "<note>
// (recorded as a person in a <host> shell)". Records written that way are
// read, never rewritten: SplitNote recovers the note and the host, and
// LegacyNote rebuilds the old form for the shared decision identity.

// legacyHostNote matches the provenance released clients appended to a note.
var legacyHostNote = regexp.MustCompile(`\s*\(?recorded as a person in an? (\S+) shell\)?\s*$`)

// SplitNote reads a note a released client wrote: the note the person typed,
// and the agent host the client folded into it, empty when there was none.
// A note without the phrase comes back as it is.
func SplitNote(note string) (text, host string) {
	m := legacyHostNote.FindStringSubmatchIndex(note)
	if m == nil {
		return note, ""
	}
	return strings.TrimRight(note[:m[0]], " ;"), note[m[2]:m[3]]
}

// LegacyNote is the note a released client would have written for text
// recorded by a person in host's shell. Only the shared decision identity
// uses it, so a record a released client wrote and the same record held as
// note and host are one record to both ends. Nothing stores it.
func LegacyNote(text, host string) string {
	if host == "" {
		return text
	}
	said := "recorded as a person in a " + host + " shell"
	if text == "" {
		return said
	}
	return text + " (" + said + ")"
}
