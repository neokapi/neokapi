package connector

import "strings"

// urlSafeID reduces an arbitrary string (typically a URL) to an identifier
// that survives being a path segment without escaping: lowercase ASCII
// letters, digits, '.', '_' and '-'. Everything else — scheme colons,
// slashes, spaces — collapses to '-', with runs and edges trimmed. Connector
// ids appear in REST paths (/connectors/:id/...), so they must never require
// percent-encoding.
func urlSafeID(s string) string {
	var b strings.Builder
	lastDash := true // trim a leading dash
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
