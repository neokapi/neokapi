package xliff2

import (
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// Codes of a block read from another format. A Markdown or HTML block that an
// extract writes into XLIFF holds inline codes whose native form is the other
// format's markup (`**`, `<a href="…">`). They are written as XLIFF codes, a
// <pc> or a <ph> with the markup in the unit's <originalData>, so a
// translator's tool shows each as a tag rather than as text to translate, and
// a merge reads each back as the code it was. Written as text, the markup
// would come back as text, and the merge would refuse every target of the
// block for the codes it lost.

// foreignData is one <data> entry of a unit's <originalData>.
type foreignData struct {
	id, text string
}

// foreignCodes indexes the codes the segments of a block read from another
// format hold, each referring to its native form in an <originalData> entry,
// and returns those entries in the order they were assigned. It returns false
// when the codes cannot be written as XLIFF codes: two codes of different
// kinds share an id, which XLIFF keeps unique in a unit, or a paired code has
// two native forms. The segments are then written as before, their markup as
// text.
func foreignCodes(segs ...[]seg) (codeIndex, []foreignData, bool) {
	ix := codeIndex{pc: map[string]CodeAttrs{}, sc: map[string]CodeAttrs{}, ec: map[string]CodeAttrs{}, ph: map[string]Inline{}, native: true}
	var data []foreignData
	dataID := map[string]string{}
	ref := func(text string) string {
		if text == "" {
			return ""
		}
		if id, ok := dataID[text]; ok {
			return id
		}
		id := "d" + strconv.Itoa(len(data)+1)
		dataID[text] = id
		data = append(data, foreignData{id: id, text: text})
		return id
	}
	kind := map[string]model.RunKind{}
	claim := func(id string, k model.RunKind) bool {
		if prev, ok := kind[id]; ok && prev != k {
			return false
		}
		kind[id] = k
		return true
	}
	var walk func(runs []model.Run) bool
	walk = func(runs []model.Run) bool {
		for _, r := range runs {
			switch {
			case r.PcOpen != nil:
				if !claim(r.PcOpen.ID, model.RunKindPcOpen) {
					return false
				}
				a := ix.pc[r.PcOpen.ID]
				d := ref(r.PcOpen.Data)
				if a.ID != "" && a.DataRefStart != d {
					return false
				}
				a.ID, a.DataRefStart = r.PcOpen.ID, d
				ix.pc[r.PcOpen.ID] = a
			case r.PcClose != nil:
				if !claim(r.PcClose.ID, model.RunKindPcOpen) {
					return false
				}
				a := ix.pc[r.PcClose.ID]
				d := ref(r.PcClose.Data)
				if a.DataRefEnd != "" && a.DataRefEnd != d {
					return false
				}
				a.ID, a.DataRefEnd = r.PcClose.ID, d
				ix.pc[r.PcClose.ID] = a
			case r.Ph != nil:
				if !claim(r.Ph.ID, model.RunKindPh) {
					return false
				}
				ix.ph[r.Ph.ID] = Inline{Ph: &Ph{ID: r.Ph.ID, DataRef: ref(r.Ph.Data), Equiv: r.Ph.Equiv}}
			case r.Plural != nil:
				for _, form := range r.Plural.Forms {
					if !walk(form) {
						return false
					}
				}
			case r.Select != nil:
				for _, c := range r.Select.Cases {
					if !walk(c) {
						return false
					}
				}
			}
		}
		return true
	}
	for _, list := range segs {
		for _, s := range list {
			if !walk(s.Runs) {
				return codeIndex{}, nil, false
			}
		}
	}
	return ix, data, true
}
