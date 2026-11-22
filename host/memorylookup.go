package host

import (
	"slices"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/memory"
)

// MemoryLookupEntries turns content-memory matches into lookup results. The
// store can hold one translation as several entries, one per origin (a
// materialized merge and an absorbed record, say). Matches with the same
// source and target text become one result that names every entry and every
// origin. The result keeps the position and the best score of its first
// match, so the order stays the store's ranking.
func MemoryLookupEntries(matches []memory.Match, sourceLocale, targetLocale model.LocaleID) []output.MemoryLookupEntry {
	entries := make([]output.MemoryLookupEntry, 0, len(matches))
	at := map[[2]string]int{}
	for _, m := range matches {
		source := m.Entry.VariantText(sourceLocale)
		target := m.Entry.VariantText(targetLocale)
		key := [2]string{source, target}
		i, seen := at[key]
		if !seen {
			i = len(entries)
			at[key] = i
			entries = append(entries, output.MemoryLookupEntry{
				Source:    source,
				Target:    target,
				Score:     m.Score,
				MatchType: string(m.MatchType),
				EntryID:   m.Entry.ID,
			})
		}
		e := &entries[i]
		e.Score = max(e.Score, m.Score)
		if !slices.Contains(e.EntryIDs, m.Entry.ID) {
			e.EntryIDs = append(e.EntryIDs, m.Entry.ID)
		}
		for _, o := range m.Entry.Origins {
			if o.Source != "" && !slices.Contains(e.Origins, o.Source) {
				e.Origins = append(e.Origins, o.Source)
			}
		}
	}
	for i := range entries {
		slices.Sort(entries[i].Origins)
	}
	return entries
}
