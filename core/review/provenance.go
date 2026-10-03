package review

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// ProvenanceOf reads where the current target came from and who decided on
// it: the decision in force from the unit's state record, and the target's
// origin. The format's own provenance wins over the record's, because it
// describes the bytes on disk while the record describes what was last written
// through kapi. A nil record leaves the decision empty.
func ProvenanceOf(b *model.Block, loc model.LocaleID, unit *state.UnitState) Provenance {
	var p Provenance
	if unit != nil {
		if unit.Origin.Kind != "" {
			o := unit.Origin
			p.Origin = &o
		}
		p.ReviewState = unit.Decision.ReviewState
		p.By = unit.Decision.By
		p.At = unit.Decision.At
		p.Note = unit.Decision.Note
		// The rung the decision landed the unit on: a translation's target rung,
		// or the authoring rung for source wording reviewed in its own language.
		p.Status = string(unit.Status)
		if p.Status == "" {
			p.Status = string(unit.SourceStatus)
		}
	}
	if b != nil {
		if t, ok := targetOf(b, loc); ok && t.Origin.Kind != "" {
			o := t.Origin
			p.Origin = &o
		}
	}
	return p
}

// targetOf returns the target edition b holds for loc. The edition the block
// was read in is never a target, so loc names a target in the source language
// only when the block holds one.
func targetOf(b *model.Block, loc model.LocaleID) (model.Edition, bool) {
	key := model.Variant(loc)
	if b.IsSourceEdition(key) {
		return model.Edition{}, false
	}
	return b.Edition(key)
}
