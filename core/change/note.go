package change

import "strings"

// A person who declares themselves with KAPI_ACTOR=person inside an agent
// host's shell is recorded as a person, and the agent host is recorded beside
// the note as Actor.AgentHost (state.Decision.AgentHost,
// venue.UnitDecision.AgentHost). The note stays what the person typed.
//
// Released clients folded the host into the note instead, in exactly two
// forms: the phrase alone when the person wrote no note, and
// "<note> (<phrase>)" otherwise. Records written that way are read, never
// rewritten: SplitNote recovers the note and the host, and LegacyNote
// rebuilds the old form for the shared decision identity.

// legacyHosts are the agent hosts released clients wrote into a note: the
// names in host's agent host table when the phrase was retired. A host added
// to that table since never wrote the phrase.
var legacyHosts = []string{"claude-code", "codex"}

// legacyPhrase is the provenance a released client wrote for host.
func legacyPhrase(host string) string {
	return "recorded as a person in a " + host + " shell"
}

// SplitNote reads a note a released client wrote: the note the person typed,
// and the agent host the client folded into it, empty when there was none.
// Only the two forms a client wrote are read, the phrase alone or
// " (<phrase>)" at the very end, and only for a host a client wrote. Any
// other note comes back exactly as it is, and the person's text is never
// trimmed.
func SplitNote(note string) (text, host string) {
	for _, h := range legacyHosts {
		phrase := legacyPhrase(h)
		if note == phrase {
			return "", h
		}
		if suffix := " (" + phrase + ")"; strings.HasSuffix(note, suffix) && len(note) > len(suffix) {
			return strings.TrimSuffix(note, suffix), h
		}
	}
	return note, ""
}

// LegacyNote is the note a released client would have written for text
// recorded by a person in host's shell. Only the shared decision identity
// uses it, so a record a released client wrote and the same record held as
// note and host are one record to both ends. Nothing stores it.
func LegacyNote(text, host string) string {
	if host == "" {
		return text
	}
	if text == "" {
		return legacyPhrase(host)
	}
	return text + " (" + legacyPhrase(host) + ")"
}
