package check

import (
	"cmp"
	"slices"
)

// A DeclaredSpan is where one declared term matched in a text.
type DeclaredSpan struct {
	Start, End int
	// Index identifies the declaration that matched, in the caller's terms.
	Index int
}

// KeepLongestDeclared applies the longest-declared-match rule to the spans of
// several declared terms found in one text. A span that sits strictly inside
// another span is dropped: a project that declared "berth plan" has said those
// words are not a use of "berth". Spans that only overlap, and spans covering
// exactly the same bytes, are all kept, because neither term contains the other.
//
// The result is ordered by start, longest first at each start, then by Index.
// The input is not modified.
func KeepLongestDeclared(spans []DeclaredSpan) []DeclaredSpan {
	sorted := slices.Clone(spans)
	slices.SortFunc(sorted, func(a, b DeclaredSpan) int {
		if c := cmp.Compare(a.Start, b.Start); c != 0 {
			return c
		}
		if c := cmp.Compare(b.End, a.End); c != 0 {
			return c
		}
		return cmp.Compare(a.Index, b.Index)
	})
	if len(sorted) < 2 {
		return sorted
	}
	kept := sorted[:0]
	var cover DeclaredSpan
	covered := false
	for _, s := range sorted {
		if covered && cover.End >= s.End && (cover.Start != s.Start || cover.End != s.End) {
			continue
		}
		kept = append(kept, s)
		if !covered || s.End > cover.End {
			cover, covered = s, true
		}
	}
	return kept
}
