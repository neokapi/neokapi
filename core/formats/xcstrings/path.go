package xcstrings

import (
	"regexp"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// printfSpecifierRe matches C/printf-style format specifiers used by Apple
// String Catalogs (e.g. %@, %d, %lld, %1$@, %.2f, %%). These are inline
// placeholders that must never be translated; the reader converts each match
// into a PlaceholderRun so tools treat them as opaque codes.
//
// Alternatives, matched in order:
//   - "%%"   the literal percent escape (matched first so it isn't split)
//   - "%arg" Apple's substitution-argument token used inside a
//     substitution's plural/device values to stand in for the substituted
//     value; treated as one opaque token rather than "%a" + "rg"
//   - the printf grammar: %, optional positional argument "<n>$", optional
//     flags, optional width, optional precision, optional length modifier,
//     conversion character.
var printfSpecifierRe = regexp.MustCompile(
	`%%|%arg|%(\d+\$)?[-+ #0]*\d*(\.\d+)?(hh|h|ll|l|q|L|z|j|t)?[@dDiuUxXoOfeEgGaAcCsSp]`,
)

// valueKind names the leaf-value location within an entry. It feeds the
// Block.Properties so the writer can splice the translated value back into
// the exact same location.
type valueKind string

const (
	// kindStringUnit is a plain localization stringUnit value.
	kindStringUnit valueKind = "stringUnit"
	// kindPlural is a plural-category value (top-level variation).
	kindPlural valueKind = "plural"
	// kindDevice is a device-class value (top-level variation).
	kindDevice valueKind = "device"
	// kindSubstitutionPlural is a plural value nested under a substitution.
	kindSubstitutionPlural valueKind = "subPlural"
	// kindSubstitutionDevice is a device value nested under a substitution.
	kindSubstitutionDevice valueKind = "subDevice"
)

// valueRef uniquely addresses one translatable leaf value inside the catalog.
// It is stored on each emitted Block (as properties) and consumed by the
// writer to locate the matching stringUnit.
type valueRef struct {
	Key      string    // entry key
	Lang     string    // BCP-47 localization language
	Kind     valueKind // location class
	Sub      string    // substitution argument name (for sub* kinds)
	Category string    // plural category or device class
}

// applyToBlockProps records the value reference on a Block's Properties so the
// writer can round-trip it. The entry key + lang are also exposed under
// human-friendly keys for tooling.
func (vr valueRef) applyToBlockProps(b *model.Block) {
	b.Properties["xcstrings.key"] = vr.Key
	b.Properties["xcstrings.lang"] = vr.Lang
	b.Properties["xcstrings.kind"] = string(vr.Kind)
	if vr.Sub != "" {
		b.Properties["xcstrings.sub"] = vr.Sub
	}
	if vr.Category != "" {
		b.Properties["xcstrings.category"] = vr.Category
	}
}

// valueRefFromBlock reconstructs a valueRef from a Block's Properties.
func valueRefFromBlock(b *model.Block) (valueRef, bool) {
	key, ok := b.Properties["xcstrings.key"]
	if !ok {
		return valueRef{}, false
	}
	return valueRef{
		Key:      key,
		Lang:     b.Properties["xcstrings.lang"],
		Kind:     valueKind(b.Properties["xcstrings.kind"]),
		Sub:      b.Properties["xcstrings.sub"],
		Category: b.Properties["xcstrings.category"],
	}, true
}

// blockName builds a human-readable, stable Block name for a value reference.
// The shape is: <key>[/<lang>][/<category>][#<sub>/<category>]. It is used for
// display and for deterministic ordering; round-trip relies on the structured
// properties, not on parsing this string.
func (vr valueRef) blockName() string {
	name := vr.Key + "/" + vr.Lang
	switch vr.Kind {
	case kindStringUnit:
		// no further qualifier
	case kindPlural, kindDevice:
		name += "/" + vr.Category
	case kindSubstitutionPlural, kindSubstitutionDevice:
		name += "/" + vr.Sub + "/" + vr.Category
	}
	return name
}

// runsFromValue converts a localization value string into a Run sequence,
// turning every printf specifier into an opaque PlaceholderRun and leaving
// the surrounding literal text as TextRuns. This protects placeholders from
// translation while preserving their exact bytes for round-trip.
func runsFromValue(value string) []model.Run {
	locs := printfSpecifierRe.FindAllStringIndex(value, -1)
	if len(locs) == 0 {
		return []model.Run{{Text: &model.TextRun{Text: value}}}
	}
	var runs []model.Run
	last := 0
	id := 0
	for _, loc := range locs {
		if loc[0] > last {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: value[last:loc[0]]}})
		}
		spec := value[loc[0]:loc[1]]
		id++
		runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
			ID:    "p" + strconv.Itoa(id),
			Type:  "placeholder",
			Data:  spec,
			Equiv: spec,
		}})
		last = loc[1]
	}
	if last < len(value) {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: value[last:]}})
	}
	return runs
}

// valueFromRuns renders a Run sequence back to a flat value string, emitting
// each placeholder's Data (the original printf specifier) verbatim.
func valueFromRuns(runs []model.Run) string {
	return model.RenderRunsWithData(runs)
}
