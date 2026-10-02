package projection

import "github.com/neokapi/neokapi/core/model"

// DisplayRuns returns the runs a serializer renders for a block: a single text
// run of the block's formatted display when the reader stamped one
// (model.PropCellDisplay), otherwise runs with every character reference
// given as the characters it stands for.
//
// A spreadsheet value cell stores a number and shows the number through its
// format: a date is a serial day count, a percentage a fraction, a price a
// bare decimal. The source runs keep what the file stores, so the round-trip
// and the block's identity are untouched, and the display travels beside them
// as a property. Every structural writer and the projection tree render the
// display through this one function, so a table exported to any format reads
// the way the spreadsheet does. The display has no locale variant, so it wins
// over whichever side the caller chose.
//
// A reader that keeps a character reference (`&amp;`, `&rsquo;`) as an inline
// code keeps the source's spelling for its own writer. To any other serializer
// it is the character, written in that serializer's own spelling, so it
// arrives here as text (model.CharacterReference).
func DisplayRuns(b *model.Block, runs []model.Run) []model.Run {
	if b != nil {
		if display, ok := b.Properties[model.PropCellDisplay]; ok {
			return []model.Run{{Text: &model.TextRun{Text: display}}}
		}
	}
	return characterReferencesAsText(runs)
}

// characterReferencesAsText returns runs with each character reference
// replaced by a text run of its characters, inside plurals and selects too.
// Runs holding none are returned as they are.
func characterReferencesAsText(runs []model.Run) []model.Run {
	if !hasCharacterReference(runs) {
		return runs
	}
	out := make([]model.Run, 0, len(runs))
	for _, r := range runs {
		switch {
		case r.Ph != nil:
			if chars, ok := model.CharacterReference(r.Ph); ok {
				out = append(out, model.Run{Text: &model.TextRun{Text: chars}})
				continue
			}
		case r.Plural != nil:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, form := range r.Plural.Forms {
				forms[k] = characterReferencesAsText(form)
			}
			out = append(out, model.Run{Plural: &model.PluralRun{Pivot: r.Plural.Pivot, Forms: forms}})
			continue
		case r.Select != nil:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for k, c := range r.Select.Cases {
				cases[k] = characterReferencesAsText(c)
			}
			sel := *r.Select
			sel.Cases = cases
			out = append(out, model.Run{Select: &sel})
			continue
		}
		out = append(out, r)
	}
	return out
}

func hasCharacterReference(runs []model.Run) bool {
	for _, r := range runs {
		switch {
		case r.Ph != nil:
			if _, ok := model.CharacterReference(r.Ph); ok {
				return true
			}
		case r.Plural != nil:
			for _, form := range r.Plural.Forms {
				if hasCharacterReference(form) {
					return true
				}
			}
		case r.Select != nil:
			for _, c := range r.Select.Cases {
				if hasCharacterReference(c) {
					return true
				}
			}
		}
	}
	return false
}
