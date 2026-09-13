package check

import (
	"cmp"
	"slices"
)

// Warning is a problem in the configuration a check ran under, as opposed to
// the content it checked: a key a voice profile carries that the profile model
// does not define, or an advisory note from validating the profile.
//
// A Report lists warnings apart from its findings, and Decide never reads them.
// A run with warnings has the summary, score, gate and verdict it would have
// without them, so a warning names something to fix and settles nothing.
type Warning struct {
	// Code is the stable id a program branches on, such as "voice.unknown_key".
	Code string `json:"code"`
	// Message is the sentence a person reads.
	Message string `json:"message"`
	// Source is where the configuration was loaded from: a profile file's path,
	// or "pack:<name>" or "store:<name>" for a profile with no file.
	Source string `json:"source"`
	// Key is the dotted path of the key the warning is about, when it is about
	// one.
	Key string `json:"key,omitempty"`
}

// MergeWarnings joins warning lists into one list, sorted by source, key and
// code, holding each distinct warning once. It returns nil when there are none.
//
// A profile that governs many files is resolved for each of them, and a run
// still reports its warnings once.
func MergeWarnings(lists ...[]Warning) []Warning {
	var out []Warning
	seen := map[Warning]bool{}
	for _, list := range lists {
		for _, w := range list {
			if seen[w] {
				continue
			}
			seen[w] = true
			out = append(out, w)
		}
	}
	slices.SortFunc(out, func(a, b Warning) int {
		return cmp.Or(
			cmp.Compare(a.Source, b.Source),
			cmp.Compare(a.Key, b.Key),
			cmp.Compare(a.Code, b.Code),
			cmp.Compare(a.Message, b.Message),
		)
	})
	return out
}
