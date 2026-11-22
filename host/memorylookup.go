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
// origin. The result sits where its first match sits, so the order stays the
// store's ranking, and carries the score and match type of its strongest
// match.
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
		if seen && stronger(m, e) {
			e.Score, e.MatchType = m.Score, string(m.MatchType)
		}
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

// stronger reports whether m is a stronger match than the result e holds: a
// higher score, or the same score with a stronger match type.
func stronger(m memory.Match, e *output.MemoryLookupEntry) bool {
	if m.Score != e.Score {
		return m.Score > e.Score
	}
	return memory.MatchTypePriority(m.MatchType) < memory.MatchTypePriority(memory.MatchType(e.MatchType))
}
