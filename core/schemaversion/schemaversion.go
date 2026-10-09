// Package schemaversion parses the "MAJOR.MINOR" schemaVersion string every
// neokapi serialization format's manifest carries, and answers the one
// question every format's reader asks of it: is this an unknown MAJOR (reject)
// or an unknown MINOR within a known major (accept)?
//
// Each format keeps its own SchemaVersion constant and its own Kind; the
// parsing rule, the comparison rule and the refusal are shared here, so every
// format accepts and rejects the same inputs and reports a refusal the same
// way.
package schemaversion

import (
	"fmt"
	"strings"
)

// Error is the refusal a format reports for a schemaVersion it cannot read:
// one it cannot parse, or one whose MAJOR this build does not speak.
type Error struct {
	// Format names the serialization format, as its error prefix spells it
	// ("kbf", "kmb", "ktb", "kpz").
	Format string
	// Found is the schemaVersion string the file carried.
	Found string
	// Major is the parsed MAJOR of Found. Zero when Invalid is set.
	Major int
	// Invalid is set when Found is not a "MAJOR.MINOR" string at all.
	Invalid bool
	// Supported lists the versions this build reads, one per readable major.
	Supported []string
}

func (e *Error) Error() string {
	if e.Invalid {
		return fmt.Sprintf("%s: invalid schemaVersion %q", e.Format, e.Found)
	}
	return fmt.Sprintf("%s: unsupported major schemaVersion %d (this build reads %s)",
		e.Format, e.Major, joinVersions(e.Supported))
}

// Check applies the contract to found: it returns the parsed MAJOR and nil
// when found is a well-formed "MAJOR.MINOR" whose MAJOR equals the major of
// one of the supported versions, whatever its MINOR, and a *Error otherwise.
// supported is the list of versions the caller's build reads, newest last.
func Check(format, found string, supported ...string) (major int, err error) {
	major, ok := Major(found)
	if !ok {
		return 0, &Error{Format: format, Found: found, Invalid: true, Supported: supported}
	}
	for _, s := range supported {
		if want, ok := Major(s); ok && want == major {
			return major, nil
		}
	}
	return major, &Error{Format: format, Found: found, Major: major, Supported: supported}
}

// Split parses v as "MAJOR.MINOR" into its two non-negative integer
// components. Both sides must be present and made only of ASCII digits: an
// empty string, a missing dot, a leading or trailing dot (an empty major or
// minor segment), or a non-digit rune on either side all report ok=false.
//
// A leading or trailing dot is deliberately rejected rather than treated as a
// zero segment: ".5" and "5." are not valid MAJOR.MINOR strings.
func Split(v string) (major, minor int, ok bool) {
	dot := -1
	for i := range v {
		if v[i] == '.' {
			dot = i
			break
		}
	}
	if dot <= 0 || dot == len(v)-1 {
		return 0, 0, false
	}
	major, ok = parseDigits(v[:dot])
	if !ok {
		return 0, 0, false
	}
	minor, ok = parseDigits(v[dot+1:])
	if !ok {
		return 0, 0, false
	}
	return major, minor, true
}

// Major reports only the MAJOR component of v, for callers that never need
// the minor.
func Major(v string) (int, bool) {
	major, _, ok := Split(v)
	return major, ok
}

func parseDigits(s string) (int, bool) {
	n := 0
	for i := range s {
		r := s[i]
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// joinVersions spells a supported list for a message: "1.0", "1.0 and 2.0",
// "1.0, 2.0 and 3.0".
func joinVersions(vs []string) string {
	switch len(vs) {
	case 0:
		return "nothing"
	case 1:
		return vs[0]
	default:
		return strings.Join(vs[:len(vs)-1], ", ") + " and " + vs[len(vs)-1]
	}
}
