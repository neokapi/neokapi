package screens

import (
	"sort"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/rulesview"
)

// DigestItemFrom shapes one digest item for a screen. The two shapes hold
// the same fields.
func DigestItemFrom(it rulesview.Item) DigestItem {
	return DigestItem{
		ID:            it.ID,
		Short:         it.Short,
		Status:        it.Status,
		Theme:         it.Theme,
		Sentence:      it.Sentence,
		Subject:       it.Subject,
		Quote:         it.Quote,
		Collection:    it.Collection,
		Standing:      it.Standing,
		Usage:         (*DigestUsage)(it.Usage),
		NoticedBy:     it.NoticedBy,
		At:            it.At,
		New:           it.New,
		Scope:         it.Scope,
		Keepable:      it.Keepable,
		Droppable:     it.Droppable,
		Widenable:     it.Widenable,
		WidenTo:       it.WidenTo,
		EstablishedAt: it.EstablishedAt,
		How:           it.How,
	}
}

// DigestItemsFrom shapes a list of digest items, keeping a nil list nil and
// an empty one empty.
func DigestItemsFrom(items []rulesview.Item) []DigestItem {
	if items == nil {
		return nil
	}
	out := make([]DigestItem, len(items))
	for i, it := range items {
		out[i] = DigestItemFrom(it)
	}
	return out
}

// DigestConflictFrom shapes one disagreement for a screen.
func DigestConflictFrom(c rulesview.Conflict) DigestConflict {
	return DigestConflict{Sides: DigestItemsFrom(c.Sides), ByEvidence: c.ByEvidence, Reason: c.Reason}
}

// MapSuggestions are the rows the Map lists as awaiting a decision, with
// their counts.
type MapSuggestions struct {
	Rows      []MapSuggestionDTO
	Conflicts int
	Suggested int
}

// MapSuggestionRows lists the items awaiting a decision: each disagreement
// under its first side, then each suggestion, in the order given. evidence
// holds each operation's evidence by its id, which the rows count blocks and
// files from.
func MapSuggestionRows(conflicts []DigestConflict, suggested []DigestItem, evidence map[string][]contextop.Evidence) MapSuggestions {
	out := MapSuggestions{Rows: []MapSuggestionDTO{}}
	row := func(group string, item DigestItem, conflict *DigestConflict) MapSuggestionDTO {
		blocks, files := EvidenceCounts(evidence[item.ID])
		return MapSuggestionDTO{Group: group, Item: item, Conflict: conflict, Blocks: blocks, Files: files}
	}
	for i := range conflicts {
		c := conflicts[i]
		if len(c.Sides) == 0 {
			continue
		}
		out.Rows = append(out.Rows, row("conflict", c.Sides[0], &c))
		out.Conflicts++
	}
	for _, item := range suggested {
		out.Rows = append(out.Rows, row("suggested", item, nil))
		out.Suggested++
	}
	return out
}

// DigestSuggestedItems are the suggestions of a digest's themes, in the order
// the digest lists them.
func DigestSuggestedItems(themes []rulesview.Theme) []DigestItem {
	var out []DigestItem
	for _, theme := range themes {
		for _, g := range theme.Groups {
			for _, item := range g.Items {
				out = append(out, DigestItemFrom(item))
			}
		}
	}
	return out
}

// EvidenceCounts counts the distinct blocks and files a list of evidence
// names. A block is a file's unit, or its quote where no unit is recorded; a
// web page counts as neither.
func EvidenceCounts(ev []contextop.Evidence) (blocks, files int) {
	seenFile := map[string]bool{}
	seenBlock := map[string]bool{}
	for _, e := range ev {
		if e.Path == "" {
			continue
		}
		if !seenFile[e.Path] {
			seenFile[e.Path] = true
			files++
		}
		at := e.Unit
		if at == "" {
			at = "quote:" + e.Quote
		}
		if key := e.Path + "\x00" + at; !seenBlock[key] && (e.Unit != "" || e.Quote != "") {
			seenBlock[key] = true
			blocks++
		}
	}
	return blocks, files
}

// SortMapFiles orders the Map's file rows the most findings first, then by
// path.
func SortMapFiles(rows []MapFileDTO) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Findings != rows[j].Findings {
			return rows[i].Findings > rows[j].Findings
		}
		return rows[i].Relative < rows[j].Relative
	})
}
