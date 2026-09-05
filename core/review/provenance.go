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
		if t := b.Target(loc); t != nil && t.Origin.Kind != "" {
			o := t.Origin
			p.Origin = &o
		}
	}
	return p
}
