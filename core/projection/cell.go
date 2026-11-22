package projection

import "github.com/neokapi/neokapi/core/model"

// DisplayRuns returns the runs a serializer renders for a block: a single text
// run of the block's formatted display when the reader stamped one
// (model.PropCellDisplay), otherwise the runs with each placeholder in its
// display form (see displayPlaceholder).
//
// A spreadsheet value cell stores a number and shows it through its format: a
// date is a serial day count, a percentage a fraction. The source runs keep
// what the file stores, and the display travels beside them as a property.
// Every structural writer and the projection tree render the display through
// this function, so a table exported to any format reads the way the
// spreadsheet does. The display has no locale variant, so it wins over
// whichever side the caller chose.
//
// A character reference (`&amp;`, `&rsquo;`) kept as an inline code is, to any
// other serializer, the character, so it arrives here as text
// (model.CharacterReference).
func DisplayRuns(b *model.Block, runs []model.Run) []model.Run {
	if b != nil {
		if display, ok := b.Properties[model.PropCellDisplay]; ok {
			return []model.Run{{Text: &model.TextRun{Text: display}}}
		}
	}
	return displayPlaceholders(runs)
}

// displayPlaceholders returns runs with each placeholder in its display
// form (see displayPlaceholder), inside plurals and selects too. Runs
// holding no placeholder that changes are returned as they are.
func displayPlaceholders(runs []model.Run) []model.Run {
	if !hasDisplayPlaceholder(runs) {
		return runs
	}
	out := make([]model.Run, 0, len(runs))
	for _, r := range runs {
		switch r.Kind() {
		case model.RunKindPh:
			out = append(out, displayPlaceholder(r))
		case model.RunKindPlural:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, form := range r.Plural.Forms {
				forms[k] = displayPlaceholders(form)
			}
			out = append(out, model.Run{Plural: &model.PluralRun{Pivot: r.Plural.Pivot, Forms: forms}})
		case model.RunKindSelect:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for k, c := range r.Select.Cases {
				cases[k] = displayPlaceholders(c)
			}
			sel := *r.Select
			sel.Cases = cases
			out = append(out, model.Run{Select: &sel})
		default:
			out = append(out, r)
		}
	}
	return out
}

// displayPlaceholder returns the run a serializer renders for one
// placeholder: a character reference as a text run of its characters, a
// placeholder with a display form and no equivalent as a copy whose Equiv
// is that display form, and any other placeholder unchanged.
//
// Every inline sink renders a placeholder's Equiv. A Word footnote reference
// carries Disp "[1]" and no Equiv, so giving it its display form keeps the
// marker in the rendered sentence ("quarter[1], see"). A placeholder with
// neither renders nothing: its Data is markup in the source format and means
// nothing in another one.
func displayPlaceholder(r model.Run) model.Run {
	if chars, ok := model.CharacterReference(r.Ph); ok {
		return model.Run{Text: &model.TextRun{Text: chars}}
	}
	if showsDisp(r.Ph) {
		ph := *r.Ph
		ph.Equiv = ph.Disp
		return model.Run{Ph: &ph}
	}
	return r
}

// showsDisp reports whether a placeholder renders its display form: it
// has one and has no equivalent text of its own.
func showsDisp(ph *model.PlaceholderRun) bool {
	return ph.Equiv == "" && ph.Disp != ""
}

func hasDisplayPlaceholder(runs []model.Run) bool {
	for _, r := range runs {
		switch r.Kind() {
		case model.RunKindPh:
			if _, ok := model.CharacterReference(r.Ph); ok || showsDisp(r.Ph) {
				return true
			}
		case model.RunKindPlural:
			for _, form := range r.Plural.Forms {
				if hasDisplayPlaceholder(form) {
					return true
				}
			}
		case model.RunKindSelect:
			for _, c := range r.Select.Cases {
				if hasDisplayPlaceholder(c) {
					return true
				}
			}
		case model.RunKindText, model.RunKindPcOpen, model.RunKindPcClose, model.RunKindSub:
		}
	}
	return false
}
