// Package schemaversion parses the "MAJOR.MINOR" schemaVersion string every
// neokapi serialization format's manifest carries, and answers the one
// question every format's reader asks of it: is this an unknown MAJOR (reject)
// or an unknown MINOR within a known major (accept)?
//
// Each format keeps its own SchemaVersion constant, its own Kind, and its own
// error wording; only the parsing and comparison rule is shared here.
package schemaversion

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
