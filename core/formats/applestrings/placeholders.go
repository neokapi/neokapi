package applestrings

import (
	"regexp"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// placeholderRe matches the format specifiers used by Apple localization
// strings. These are inline placeholders that must never be translated; the
// reader converts each match into a PlaceholderRun so tools treat them as
// opaque codes and the writer re-emits the exact original bytes.
//
// Alternatives, matched in order:
//   - "%%"     the literal percent escape (matched first so it is not split)
//   - "%#@…@"  the .stringsdict variable token used inside an
//     NSStringLocalizedFormatKey (e.g. %#@count@). It stands in for a whole
//     sub-dictionary of plural values; treated as one opaque token.
//   - the printf grammar: %, optional positional argument "<n>$", optional
//     flags, optional width, optional precision, optional length modifier,
//     conversion character. Apple's repertoire adds @ (object), and the
//     length modifiers used by NSString format strings (lld, etc.).
var placeholderRe = regexp.MustCompile(
	`%%|%#@[^@]+@|%(\d+\$)?[-+ #0]*\d*(\.\d+)?(hh|h|ll|l|q|L|z|j|t)?[@dDiuUxXoOfeEgGaAcCsSp]`,
)

// runsFromValue converts a localization value string into a Run sequence,
// turning every format specifier into an opaque PlaceholderRun and leaving the
// surrounding literal text as TextRuns. This protects placeholders from
// translation while preserving their exact bytes for round-trip. When protect
// is false the entire value is emitted as a single TextRun (placeholders are
// not lifted), which keeps the round-trip byte-faithful regardless.
func runsFromValue(value string, protect bool) []model.Run {
	if !protect {
		return []model.Run{{Text: &model.TextRun{Text: value}}}
	}
	locs := placeholderRe.FindAllStringIndex(value, -1)
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
// each placeholder's Data (the original format specifier) verbatim.
func valueFromRuns(runs []model.Run) string {
	return model.RenderRunsWithData(runs)
}
