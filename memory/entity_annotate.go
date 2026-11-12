package memory

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// EntityPattern is a text to mark as an entity wherever it occurs, and the
// entity type it is marked with.
type EntityPattern struct {
	Text          string `json:"text"`
	Type          string `json:"type"`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
}

// EntitySpan is an entity to place in a text: its range in code points of the
// text, its text and type, and the id its placeholder takes.
type EntitySpan struct {
	ID    string
	Text  string
	Type  string
	Start int
	End   int
}

// PlacedEntity is an entity PlaceEntities turned into a placeholder run, and
// the index of that run.
type PlacedEntity struct {
	Run  int
	Span EntitySpan
}

// ConceptResolver names the terms concept an entity's text matches in a
// locale. False when no concept matches.
type ConceptResolver func(ctx context.Context, text string, locale model.LocaleID) (string, bool)

// EntryStore is what entity annotation reads and writes entries through.
type EntryStore interface {
	GetEntry(ctx context.Context, id string) (Entry, bool, error)
	Add(ctx context.Context, entry Entry) error
}

// AnnotateResult counts what AnnotateEntries changed.
type AnnotateResult struct {
	EntriesUpdated int `json:"entries_updated"`
	EntitiesAdded  int `json:"entities_added"`
}

// AnnotateEntries marks each pattern as an entity in every variant of the
// entries ids name. Each occurrence becomes a placeholder run, and the
// entry's entity mappings are rebuilt from the variants, so one entity is
// paired across its translations. With resolve set, each entity is linked to
// the terms concept its text matches. An entry no pattern occurs in, or that
// is not found, is left as it is.
func AnnotateEntries(ctx context.Context, store EntryStore, ids []string, patterns []EntityPattern, resolve ConceptResolver) (AnnotateResult, error) {
	var res AnnotateResult
	for _, id := range ids {
		entry, found, err := store.GetEntry(ctx, id)
		if err != nil || !found {
			continue
		}
		added := 0
		variants := make(map[model.LocaleID][]model.Run, len(entry.Variants))
		for loc, runs := range entry.Variants {
			if len(runs) == 0 {
				continue
			}
			marked, n := MarkEntities(runs, patterns)
			variants[loc] = marked
			added += n
		}
		if added == 0 {
			continue
		}
		entry.Variants = variants
		entry.Entities = EntityMappingsFromVariants(entry.Variants)
		if resolve != nil {
			for i := range entry.Entities {
				LinkEntityConcept(ctx, &entry.Entities[i], resolve)
			}
		}
		entry.UpdatedAt = time.Now()
		if err := store.Add(ctx, entry); err != nil {
			return res, fmt.Errorf("update entry %q: %w", id, err)
		}
		res.EntriesUpdated++
		res.EntitiesAdded += added
	}
	return res, nil
}

// LinkEntryConcepts links the entities of the entries ids name to the terms
// concepts their text matches. An entity already linked is skipped unless
// force is set. It returns the number of entries whose links changed.
func LinkEntryConcepts(ctx context.Context, store EntryStore, ids []string, force bool, resolve ConceptResolver) (int, error) {
	updated := 0
	for _, id := range ids {
		entry, found, err := store.GetEntry(ctx, id)
		if err != nil || !found || len(entry.Entities) == 0 {
			continue
		}
		changed := false
		for i := range entry.Entities {
			if entry.Entities[i].ConceptID != "" && !force {
				continue
			}
			old := entry.Entities[i].ConceptID
			LinkEntityConcept(ctx, &entry.Entities[i], resolve)
			changed = changed || entry.Entities[i].ConceptID != old
		}
		if !changed {
			continue
		}
		entry.UpdatedAt = time.Now()
		if err := store.Add(ctx, entry); err != nil {
			return updated, fmt.Errorf("update entry %q: %w", id, err)
		}
		updated++
	}
	return updated, nil
}

// LinkEntityConcept sets an entity mapping's ConceptID to the concept the
// first of its values (in locale order) matches.
func LinkEntityConcept(ctx context.Context, em *EntityMapping, resolve ConceptResolver) {
	if resolve == nil {
		return
	}
	for _, loc := range slices.Sorted(maps.Keys(em.Values)) {
		text := em.Values[loc].Text
		if text == "" {
			continue
		}
		if id, ok := resolve(ctx, text, loc); ok {
			em.ConceptID = id
			return
		}
	}
}

// MarkEntities finds each pattern in the text of runs and returns a run
// sequence with a placeholder run at each occurrence, and the number of
// occurrences marked. Where two occurrences overlap, the earlier one is kept.
// The result is built from the runs' text, so an inline code in runs is left
// out of it.
//
// Each pattern names one entity in every variant, so a placeholder takes its
// id from the pattern and the occurrence (e1 for the first match of the first
// pattern, e1.2 for its second), whatever order a translation puts the
// entities in. EntityMappingsFromVariants pairs the variants' values by that
// id.
func MarkEntities(runs []model.Run, patterns []EntityPattern) ([]model.Run, int) {
	text := model.FlattenRuns(runs)
	type hit struct {
		span    EntitySpan
		pattern int
	}
	chars := []rune(text)
	var hits []hit
	for pi, p := range patterns {
		n := len([]rune(p.Text))
		for _, pos := range patternOccurrences(text, p.Text, p.CaseSensitive) {
			hits = append(hits, hit{
				span:    EntitySpan{Text: string(chars[pos : pos+n]), Type: p.Type, Start: pos, End: pos + n},
				pattern: pi,
			})
		}
	}
	slices.SortFunc(hits, func(a, b hit) int { return cmp.Compare(a.span.Start, b.span.Start) })
	var spans []EntitySpan
	seen := map[int]int{}
	end := 0
	for _, h := range hits {
		if h.span.Start < end {
			continue
		}
		end = h.span.End
		seen[h.pattern]++
		h.span.ID = fmt.Sprintf("e%d", h.pattern+1)
		if n := seen[h.pattern]; n > 1 {
			h.span.ID = fmt.Sprintf("e%d.%d", h.pattern+1, n)
		}
		spans = append(spans, h.span)
	}
	out, _ := PlaceEntities(text, spans)
	return out, len(spans)
}

// patternOccurrences returns the code-point positions of the occurrences of
// pattern in text that do not overlap.
func patternOccurrences(text, pattern string, caseSensitive bool) []int {
	if pattern == "" {
		return nil
	}
	if !caseSensitive {
		text, pattern = strings.ToLower(text), strings.ToLower(pattern)
	}
	chars := []rune(text)
	want := []rune(pattern)
	var positions []int
	for i := 0; i <= len(chars)-len(want); {
		if slices.Equal(chars[i:i+len(want)], want) {
			positions = append(positions, i)
			i += len(want)
			continue
		}
		i++
	}
	return positions
}

// PlaceEntities builds a run sequence from text with each span, sorted by
// start, as a placeholder run carrying the span's id, type and text; the text
// around the spans becomes text runs. A span that overlaps an earlier one or
// falls outside the text is left out. It returns each span it placed with the
// index of its run, in text order.
func PlaceEntities(text string, spans []EntitySpan) ([]model.Run, []PlacedEntity) {
	if len(spans) == 0 {
		if text == "" {
			return nil, nil
		}
		return []model.Run{{Text: &model.TextRun{Text: text}}}, nil
	}
	chars := []rune(text)
	var runs []model.Run
	var placed []PlacedEntity
	pos := 0
	appendText := func(s string) {
		if s != "" {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: s}})
		}
	}
	for _, sp := range spans {
		if sp.Start < pos || sp.Start >= len(chars) || sp.End > len(chars) {
			continue
		}
		appendText(string(chars[pos:sp.Start]))
		placed = append(placed, PlacedEntity{Run: len(runs), Span: sp})
		runs = append(runs, model.Run{Ph: &model.PlaceholderRun{ID: sp.ID, Type: sp.Type, Data: sp.Text}})
		pos = sp.End
	}
	appendText(string(chars[pos:]))
	return runs, placed
}

// EntityMappingsFromVariants reads the entity placeholders of every variant
// and returns one mapping per placeholder id and type, with each locale's
// value taken from that variant's placeholder.
func EntityMappingsFromVariants(variants map[model.LocaleID][]model.Run) []EntityMapping {
	if len(variants) == 0 {
		return nil
	}
	type key struct{ id, typ string }
	byKey := map[key]*EntityMapping{}
	var order []key
	for _, loc := range slices.Sorted(maps.Keys(variants)) {
		for _, r := range variants[loc] {
			if r.Ph == nil || !model.IsEntityTypeString(r.Ph.Type) {
				continue
			}
			k := key{id: r.Ph.ID, typ: r.Ph.Type}
			em, ok := byKey[k]
			if !ok {
				em = &EntityMapping{PlaceholderID: r.Ph.ID, Type: model.EntityType(r.Ph.Type), Values: map[model.LocaleID]EntityValue{}}
				byKey[k] = em
				order = append(order, k)
			}
			em.Values[loc] = EntityValue{Text: r.Ph.Data}
		}
	}
	out := make([]EntityMapping, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}
