// Package redaction replaces sensitive spans in content with protected
// placeholders before processing, and restores the originals afterwards.
//
// The defining guarantee is locality: the original sensitive value never
// leaves the local machine. Detection runs either fully offline (literal
// terms and regular expressions, the default) or via an injected provider;
// the replacement rewrites translatable runs into protected
// model.PlaceholderRun tokens whose Type carries only a coarse category
// (e.g. "redaction:person"). The secret↔token mapping lives only in a
// [Vault] — an in-process map for single-run flows, or a gitignored sidecar
// file for the extract → external translation → merge roundtrip — and is
// never serialized into the content handed to a tool, prompt, or external
// service.
//
// The package is framework-native and has no dependency on AI providers.
// Provider-backed detection is supplied from outside as a [Detector].
package redaction

import (
	"context"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// CategoryPrefix namespaces redaction placeholder types, mirroring the
// "entity:" convention in core/model/entity.go. A redacted person becomes a
// model.PlaceholderRun with Type "redaction:person".
const CategoryPrefix = "redaction:"

// Recommended categories. Categories are free-form strings — these are the
// set surfaced in defaults and documentation, not a closed enum.
const (
	CategoryPerson   = "person"
	CategoryRole     = "role"
	CategoryProduct  = "product"
	CategoryOrg      = "org"
	CategoryLocation = "location"
	CategoryCustom   = "custom"
)

// PlaceholderType returns the model.PlaceholderRun Type for a category,
// e.g. "person" → "redaction:person".
func PlaceholderType(category string) string { return CategoryPrefix + category }

// CategoryOf returns the bare category for a redaction placeholder Type and
// whether the Type belongs to redaction at all. "redaction:person" →
// ("person", true); "fmt:bold" → ("", false).
func CategoryOf(phType string) (string, bool) {
	if rest, ok := strings.CutPrefix(phType, CategoryPrefix); ok {
		return rest, true
	}
	return "", false
}

// Match is a detected sensitive span in a text string. Start and End are
// byte offsets into the scanned text, half-open [Start, End).
type Match struct {
	Start    int
	End      int
	Category string
	Original string // the matched text; equals text[Start:End]
}

// Detector finds sensitive spans in text. Implementations must be safe for
// concurrent use. Detect returns matches with byte offsets into text; the
// slice need not be sorted or non-overlapping — callers normalize via
// [NormalizeMatches].
type Detector interface {
	Detect(ctx context.Context, text string, locale model.LocaleID) ([]Match, error)
	// Name identifies the detector for diagnostics, e.g. "rules" or "ai".
	Name() string
}

// Detectors runs several detectors over the same text and merges their
// results into one normalized, non-overlapping match list.
type Detectors []Detector

// Name reports the composite detector names joined with "+".
func (ds Detectors) Name() string {
	names := make([]string, 0, len(ds))
	for _, d := range ds {
		names = append(names, d.Name())
	}
	return strings.Join(names, "+")
}

// Detect runs every detector and returns the normalized union of matches.
// A failure in any detector aborts and returns the error.
func (ds Detectors) Detect(ctx context.Context, text string, locale model.LocaleID) ([]Match, error) {
	var all []Match
	for _, d := range ds {
		if d == nil {
			continue
		}
		ms, err := d.Detect(ctx, text, locale)
		if err != nil {
			return nil, err
		}
		all = append(all, ms...)
	}
	return NormalizeMatches(all), nil
}

// NormalizeMatches sorts matches by start offset and drops overlaps. When
// two matches overlap, the one starting earlier wins; on an equal start, the
// longer span wins. Zero-width matches are discarded.
func NormalizeMatches(matches []Match) []Match {
	if len(matches) == 0 {
		return nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Start != matches[j].Start {
			return matches[i].Start < matches[j].Start
		}
		// Longer span first so it wins the overlap check below.
		return matches[i].End > matches[j].End
	})
	out := make([]Match, 0, len(matches))
	prevEnd := -1
	for _, m := range matches {
		if m.End <= m.Start {
			continue
		}
		if m.Start < prevEnd {
			continue // overlaps a kept match
		}
		out = append(out, m)
		prevEnd = m.End
	}
	return out
}
