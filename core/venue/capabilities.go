package venue

import "strings"

// ClientCapabilitiesHeader is the request header in which a client names
// what it sends that an older client does not, as a comma-separated list. A
// venue reads it to tell a client too old to send something from one that
// has not yet got it to send, and to name the remedy for each.
const ClientCapabilitiesHeader = "Kapi-Capabilities"

// CapabilityRevisions says the client sends, with each write to a record the
// venue keeps a revision of, the revision it last saw. A write from such a
// client that names none comes from a project that has not pulled the record.
const CapabilityRevisions = "revisions"

// HasCapability reports whether the header value names capability.
func HasCapability(header, capability string) bool {
	for part := range strings.SplitSeq(header, ",") {
		if strings.TrimSpace(part) == capability {
			return true
		}
	}
	return false
}
