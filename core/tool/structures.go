package tool

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// A translator works on text: a string, or text with inline codes as
// placeholders. A plural or a select holds its words in branches that such
// text shows one of, so handing a translator the whole of a structured
// source and parsing the reply back keeps one branch and drops the structure.
// TranslateStructures gives the translator one sequence at a time instead.

// StandInType is the placeholder type of the code a plural or select stands
// as while the text around it is translated.
const StandInType = "kapi:structure"

// TranslateStructures translates runs that may hold plurals or selects
// without flattening any of them. translate translates one sequence that holds
// no plural or select, and is called for:
//
//   - the text around the structures, with each structure standing in it as a
//     placeholder of type StandInType, so the translation places it;
//   - each branch of each structure, nested ones included, in the order of
//     their keys.
//
// A sequence with no text to translate (only codes, whitespace or text marked
// not to translate) is kept as it is and not handed to translate. The result
// holds the source's structures with the same forms and cases, each holding
// its translation. When the translation of the text around them does not hold
// each stand-in exactly once, that text is kept untranslated, so no structure
// is lost or doubled.
func TranslateStructures(runs []model.Run, translate func(seq []model.Run) ([]model.Run, error)) ([]model.Run, error) {
	if !model.HasStructuredRuns(runs) {
		if !hasWords(runs) {
			return slices.Clone(runs), nil
		}
		return translate(runs)
	}

	taken := map[string]bool{}
	for _, r := range runs {
		if id := codeID(r); id != "" {
			taken[id] = true
		}
	}
	frame := make([]model.Run, 0, len(runs))
	structures := map[string]model.Run{}
	var ids []string
	for _, r := range runs {
		if r.Plural == nil && r.Select == nil {
			frame = append(frame, r)
			continue
		}
		translated, err := translateStructure(r, translate)
		if err != nil {
			return nil, err
		}
		id := standInID(taken, len(ids)+1)
		taken[id] = true
		ids = append(ids, id)
		structures[id] = translated
		frame = append(frame, model.PhR(model.PlaceholderRun{ID: id, Type: StandInType, Equiv: id, Disp: id}))
	}

	out := frame
	if hasWords(frame) {
		got, err := translate(frame)
		if err != nil {
			return nil, err
		}
		if holdsEachOnce(got, ids) {
			out = got
		}
	}
	result := make([]model.Run, 0, len(out))
	for _, r := range out {
		if r.Ph != nil && r.Ph.Type == StandInType {
			if s, ok := structures[r.Ph.ID]; ok {
				result = append(result, s)
				continue
			}
		}
		result = append(result, r)
	}
	return result, nil
}

// translateStructure translates each branch of a plural or select run.
func translateStructure(r model.Run, translate func([]model.Run) ([]model.Run, error)) (model.Run, error) {
	if r.Plural != nil {
		p := model.PluralRun{Pivot: r.Plural.Pivot, Forms: make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))}
		for _, form := range slices.Sorted(maps.Keys(r.Plural.Forms)) {
			branch, err := TranslateStructures(r.Plural.Forms[form], translate)
			if err != nil {
				return model.Run{}, err
			}
			p.Forms[form] = branch
		}
		return model.PluralR(p), nil
	}
	s := model.SelectRun{Pivot: r.Select.Pivot, Cases: make(map[string][]model.Run, len(r.Select.Cases))}
	for _, c := range slices.Sorted(maps.Keys(r.Select.Cases)) {
		branch, err := TranslateStructures(r.Select.Cases[c], translate)
		if err != nil {
			return model.Run{}, err
		}
		s.Cases[c] = branch
	}
	return model.SelectR(s), nil
}

// hasWords reports whether seq holds text a translator should see.
func hasWords(seq []model.Run) bool {
	return slices.ContainsFunc(seq, func(r model.Run) bool {
		return r.Text != nil && !r.Text.NoTranslate && strings.TrimSpace(r.Text.Text) != ""
	})
}

// codeID is the id an inline code run carries, "" for any other run.
func codeID(r model.Run) string {
	switch {
	case r.Ph != nil:
		return r.Ph.ID
	case r.PcOpen != nil:
		return r.PcOpen.ID
	case r.PcClose != nil:
		return r.PcClose.ID
	case r.Sub != nil:
		return r.Sub.ID
	}
	return ""
}

// standInID is an id for the n-th stand-in that no code of the sequence uses.
func standInID(taken map[string]bool, n int) string {
	id := "s" + strconv.Itoa(n)
	for taken[id] {
		id += "_"
	}
	return id
}

// holdsEachOnce reports whether seq holds a stand-in for each of ids exactly
// once, and no other.
func holdsEachOnce(seq []model.Run, ids []string) bool {
	seen := map[string]int{}
	for _, r := range seq {
		if r.Ph != nil && r.Ph.Type == StandInType {
			seen[r.Ph.ID]++
		}
	}
	if len(seen) != len(ids) {
		return false
	}
	for _, id := range ids {
		if seen[id] != 1 {
			return false
		}
	}
	return true
}
