package projection

import "github.com/neokapi/neokapi/core/model"

// DisplayRuns returns the runs a serializer renders for a block: a single text
// run of the block's formatted display when the reader stamped one
// (model.PropCellDisplay), otherwise runs unchanged.
//
// A spreadsheet value cell stores a number and shows the number through its
// format: a date is a serial day count, a percentage a fraction, a price a
// bare decimal. The source runs keep what the file stores, so the round-trip
// and the block's identity are untouched, and the display travels beside them
// as a property. Every structural writer and the projection tree render the
// display through this one function, so a table exported to any format reads
// the way the spreadsheet does. The display has no locale variant, so it wins
// over whichever side the caller chose.
func DisplayRuns(b *model.Block, runs []model.Run) []model.Run {
	if b == nil {
		return runs
	}
	display, ok := b.Properties[model.PropCellDisplay]
	if !ok {
		return runs
	}
	return []model.Run{{Text: &model.TextRun{Text: display}}}
}
