package memory

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
)

// Entity adaptation. A generalized match pairs the entities of a stored
// entry with the entities the lookup block locates, type by type and in the
// order they sit in the text, and adapts each stored entity whose value
// differs: the target says the block's entity where the entry said its own.
// An entity is adapted only where the target holds it exactly: as its
// placeholder run, or as the one occurrence of its value that stands as a
// whole word in the target's text. Where neither locates it, the match is
// returned without that adaptation, so no text that merely contains the
// value (Acmeville for Acme) is rewritten.

// ExtractEntityAnnotations returns the entities a block's source locates: the
// values of its entity overlay spans, which is where every producer records an
// entity, in the order the spans sit in the text. ComputeEntityAdaptations
// pairs them, type by type and in that order, with a stored entry's entities.
func ExtractEntityAnnotations(block *model.Block) []*model.EntityAnnotation {
	return entityAnnotations(block, func(model.Anchor) bool { return true })
}

// ExtractSegmentEntityAnnotations returns the entities ExtractEntityAnnotations
// returns that sit in the idx-th source segment (Block.SourceSegmentRuns): the
// ones a lookup of that segment pairs with a stored entry's.
func ExtractSegmentEntityAnnotations(block *model.Block, idx int) []*model.EntityAnnotation {
	if block == nil {
		return nil
	}
	seg := block.SourceSegmentation()
	if seg == nil {
		if idx == 0 {
			return ExtractEntityAnnotations(block)
		}
		return nil
	}
	if idx < 0 || idx >= len(seg.Spans) {
		return nil
	}
	within := seg.Spans[idx].Range
	runs := block.SourceRuns()
	start, end := canonicalPos(runs, within.Start), canonicalPos(runs, within.End)
	return entityAnnotations(block, func(a model.Anchor) bool {
		at := canonicalPos(runs, a.Start)
		if len(a.Path) > 0 && a.Path[0].Kind == model.StepIndex {
			// Inside a plural or select: the run that holds it.
			at = model.RunPos{Run: a.Path[0].Index}
		}
		return comparePos(start, at) <= 0 && comparePos(at, end) < 0
	})
}

// entityAnnotations returns the values of the block's source entity overlay
// spans that keep says to, in text order.
func entityAnnotations(block *model.Block, keep func(model.Anchor) bool) []*model.EntityAnnotation {
	if block == nil {
		return nil
	}
	overlay := block.OverlayOf(model.OverlayEntity)
	if overlay == nil {
		return nil
	}
	spans := slices.Clone(overlay.Spans)
	slices.SortStableFunc(spans, func(a, b model.Span) int { return slices.Compare(textOrder(a.Range), textOrder(b.Range)) })
	var entities []*model.EntityAnnotation
	for _, sp := range spans {
		if ea, ok := sp.Value.(*model.EntityAnnotation); ok && ea != nil && keep(sp.Range) {
			entities = append(entities, ea)
		}
	}
	return entities
}

// textOrder is where a span starts, as a key that sorts in text order: the
// run indexes of its path, then its start run and offset. A span inside a
// plural or select sorts at the run that holds the branch, after a span that
// starts before that run.
func textOrder(a model.Anchor) []int {
	key := make([]int, 0, len(a.Path)+2)
	for _, step := range a.Path {
		if step.Kind == model.StepIndex {
			key = append(key, step.Index)
		}
	}
	return append(key, a.Start.Run, a.Start.Offset)
}

// canonicalPos returns pos with a boundary at the end of a text run written
// as the start of the next run, so two spellings of one boundary compare
// equal.
func canonicalPos(runs []model.Run, pos model.RunPos) model.RunPos {
	if pos.Run >= 0 && pos.Run < len(runs) && runs[pos.Run].Text != nil && pos.Offset > 0 &&
		pos.Offset >= utf8.RuneCountInString(runs[pos.Run].Text.Text) {
		return model.RunPos{Run: pos.Run + 1}
	}
	return pos
}

func comparePos(a, b model.RunPos) int {
	if c := cmp.Compare(a.Run, b.Run); c != 0 {
		return c
	}
	return cmp.Compare(a.Offset, b.Offset)
}

// ComputeEntityAdaptations computes how to adapt entity values from a stored
// content-memory entry's target variant to match the current source content.
// sourceLocale is the locale of currentEntities; targetLocale is the variant
// whose entity values should be rewritten. The stored entities are paired
// with currentEntities type by type in the order they sit in the entry's
// source variant, whatever order the store keeps them in. An adaptation the
// target variant cannot locate exactly (LocateEntity) is left out.
func ComputeEntityAdaptations(entry Entry, sourceLocale, targetLocale model.LocaleID, currentEntities []*model.EntityAnnotation) []EntityAdaptation {
	if len(entry.Entities) == 0 || len(currentEntities) == 0 {
		return nil
	}
	typeQueues := make(map[model.EntityType][]*model.EntityAnnotation)
	for _, ea := range currentEntities {
		typeQueues[ea.Type] = append(typeQueues[ea.Type], ea)
	}
	target := entry.Variant(targetLocale)
	typeIdx := make(map[model.EntityType]int)
	var adaptations []EntityAdaptation
	for _, em := range entitiesInTextOrder(entry, sourceLocale) {
		queue := typeQueues[em.Type]
		idx := typeIdx[em.Type]
		if idx >= len(queue) {
			continue
		}
		current := queue[idx]
		typeIdx[em.Type] = idx + 1
		tv, ok := em.Value(targetLocale)
		if !ok {
			continue
		}
		if tv.Text == current.Text {
			continue
		}
		a := EntityAdaptation{
			PlaceholderID: em.PlaceholderID,
			Type:          em.Type,
			StoredValue:   tv.Text,
			CurrentValue:  current.Text,
			TargetPos:     model.TextRange{Start: tv.Start, End: tv.End},
		}
		if LocateEntity(target, a) == EntityNotLocated {
			continue
		}
		adaptations = append(adaptations, a)
	}
	return adaptations
}

// entitiesInTextOrder returns the entry's entities in the order they sit in
// its source variant: by the first placeholder run with the entity's id, else
// by the first whole-word occurrence of its source value; an entity found in
// neither way follows, ordered by id with its number read as a number (e2
// before e10).
func entitiesInTextOrder(entry Entry, sourceLocale model.LocaleID) []EntityMapping {
	source := entry.Variant(sourceLocale)
	type placed struct {
		em  EntityMapping
		pos []int
	}
	out := make([]placed, 0, len(entry.Entities))
	for _, em := range entry.Entities {
		var pos []int
		if sv, ok := em.Value(sourceLocale); ok {
			pos = entityPosition(source, em.PlaceholderID, sv.Text)
		}
		out = append(out, placed{em: em, pos: pos})
	}
	slices.SortStableFunc(out, func(a, b placed) int {
		switch {
		case a.pos != nil && b.pos != nil:
			return slices.Compare(a.pos, b.pos)
		case a.pos != nil:
			return -1
		case b.pos != nil:
			return 1
		}
		return compareIDs(a.em.PlaceholderID, b.em.PlaceholderID)
	})
	ems := make([]EntityMapping, len(out))
	for i, p := range out {
		ems[i] = p.em
	}
	return ems
}

// entityPosition returns where runs hold the entity, as the ordinal of the run
// in document order and a byte offset into it, or nil.
func entityPosition(runs []model.Run, placeholderID, value string) []int {
	var at, token []int
	ordinal := 0
	walkRuns(runs, func(r model.Run) {
		switch {
		case at != nil:
		case r.Ph != nil && placeholderID != "" && r.Ph.ID == placeholderID:
			at = []int{ordinal, 0}
		case r.Text != nil && token == nil:
			if offs := wholeWordOffsets(r.Text.Text, value); len(offs) > 0 {
				token = []int{ordinal, offs[0]}
			}
		}
		ordinal++
	})
	if at != nil {
		return at
	}
	return token
}

// compareIDs orders placeholder ids by their text before a trailing number,
// then by that number.
func compareIDs(a, b string) int {
	pa, na := splitTrailingNumber(a)
	pb, nb := splitTrailingNumber(b)
	if c := cmp.Compare(pa, pb); c != 0 {
		return c
	}
	if c := cmp.Compare(na, nb); c != 0 {
		return c
	}
	return cmp.Compare(a, b)
}

func splitTrailingNumber(s string) (string, int) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil {
		return s, -1
	}
	return s[:i], n
}

// EntityLocation is where a target holds an adaptation's stored entity.
type EntityLocation int

const (
	// EntityNotLocated: the target holds neither the entity's placeholder
	// run nor exactly one whole-word occurrence of its value.
	EntityNotLocated EntityLocation = iota
	// EntityAtPlaceholder: the target holds the entity as placeholder runs
	// with its id.
	EntityAtPlaceholder
	// EntityAtWord: the target's text holds the entity's value once, as a
	// whole word.
	EntityAtWord
)

// LocateEntity returns where target holds the entity a adapts: its
// placeholder runs (by PlaceholderID), or else the one occurrence of
// StoredValue in the target's text that is not part of a longer word.
func LocateEntity(target []model.Run, a EntityAdaptation) EntityLocation {
	placeholders, words := 0, 0
	walkRuns(target, func(r model.Run) {
		switch {
		case r.Ph != nil && a.PlaceholderID != "" && r.Ph.ID == a.PlaceholderID:
			placeholders++
		case r.Text != nil:
			words += len(wholeWordOffsets(r.Text.Text, a.StoredValue))
		}
	})
	switch {
	case placeholders > 0:
		return EntityAtPlaceholder
	case words == 1:
		return EntityAtWord
	}
	return EntityNotLocated
}

// AdaptEntities returns target with each adaptation substituted where the
// target holds its entity (LocateEntity): a placeholder run with its id takes
// the current value as its data (and as its display or equivalent text where
// those said the stored value), and the one whole-word occurrence of the
// stored value in the text becomes the current value. Every adaptation is
// located in target as given, so one substitution never feeds another, and an
// adaptation the target does not locate is skipped. Runs are copied where
// they change; target is not mutated.
func AdaptEntities(target []model.Run, adaptations []EntityAdaptation) []model.Run {
	if len(target) == 0 || len(adaptations) == 0 {
		return target
	}
	byID := map[string]EntityAdaptation{}
	var words []EntityAdaptation
	for _, a := range adaptations {
		switch LocateEntity(target, a) {
		case EntityAtPlaceholder:
			if _, dup := byID[a.PlaceholderID]; !dup {
				byID[a.PlaceholderID] = a
			}
		case EntityAtWord:
			words = append(words, a)
		}
	}
	if len(byID) == 0 && len(words) == 0 {
		return target
	}
	return mapRuns(target, func(r model.Run) model.Run {
		switch {
		case r.Ph != nil:
			a, ok := byID[r.Ph.ID]
			if !ok {
				return r
			}
			ph := *r.Ph
			ph.Data = a.CurrentValue
			if ph.Disp == a.StoredValue {
				ph.Disp = a.CurrentValue
			}
			if ph.Equiv == a.StoredValue {
				ph.Equiv = a.CurrentValue
			}
			return model.Run{Ph: &ph}
		case r.Text != nil:
			text, changed := substituteWords(r.Text.Text, words)
			if !changed {
				return r
			}
			t := *r.Text
			t.Text = text
			return model.Run{Text: &t}
		}
		return r
	})
}

// substituteWords replaces, in s, each whole-word occurrence of an
// adaptation's stored value with its current value, every occurrence located
// in s as given. Occurrences that overlap an earlier one are left as they are.
func substituteWords(s string, words []EntityAdaptation) (string, bool) {
	type sub struct {
		start, end int
		with       string
	}
	var subs []sub
	for _, a := range words {
		for _, off := range wholeWordOffsets(s, a.StoredValue) {
			subs = append(subs, sub{start: off, end: off + len(a.StoredValue), with: a.CurrentValue})
		}
	}
	if len(subs) == 0 {
		return s, false
	}
	slices.SortStableFunc(subs, func(a, b sub) int { return cmp.Compare(a.start, b.start) })
	var b strings.Builder
	prev := 0
	for _, x := range subs {
		if x.start < prev {
			continue
		}
		b.WriteString(s[prev:x.start])
		b.WriteString(x.with)
		prev = x.end
	}
	b.WriteString(s[prev:])
	return b.String(), true
}

// wholeWordOffsets returns the byte offsets in s where value occurs with no
// letter, digit or mark joined to either end, so it is not part of a longer
// word.
func wholeWordOffsets(s, value string) []int {
	if value == "" {
		return nil
	}
	var out []int
	for from := 0; from <= len(s)-len(value); {
		i := strings.Index(s[from:], value)
		if i < 0 {
			break
		}
		start := from + i
		end := start + len(value)
		before, _ := utf8.DecodeLastRuneInString(s[:start])
		after, _ := utf8.DecodeRuneInString(s[end:])
		first, _ := utf8.DecodeRuneInString(value)
		last, _ := utf8.DecodeLastRuneInString(value)
		if !(start > 0 && wordRune(before) && wordRune(first)) && !(end < len(s) && wordRune(after) && wordRune(last)) {
			out = append(out, start)
		}
		_, size := utf8.DecodeRuneInString(s[start:])
		from = start + size
	}
	return out
}

func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_'
}

// walkRuns calls f for every run of runs and of the branches of its plurals
// and selects, in document order, branches in the order of their keys.
func walkRuns(runs []model.Run, f func(model.Run)) {
	for _, r := range runs {
		f(r)
		switch {
		case r.Plural != nil:
			for _, k := range slices.Sorted(maps.Keys(r.Plural.Forms)) {
				walkRuns(r.Plural.Forms[k], f)
			}
		case r.Select != nil:
			for _, k := range slices.Sorted(maps.Keys(r.Select.Cases)) {
				walkRuns(r.Select.Cases[k], f)
			}
		}
	}
}

// mapRuns returns runs with f applied to every run, branches included,
// copying each sequence and structured run it rebuilds.
func mapRuns(runs []model.Run, f func(model.Run) model.Run) []model.Run {
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		switch {
		case r.Plural != nil:
			p := *r.Plural
			p.Forms = make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, form := range r.Plural.Forms {
				p.Forms[k] = mapRuns(form, f)
			}
			out[i] = model.Run{Plural: &p}
		case r.Select != nil:
			s := *r.Select
			s.Cases = make(map[string][]model.Run, len(r.Select.Cases))
			for k, c := range r.Select.Cases {
				s.Cases[k] = mapRuns(c, f)
			}
			out[i] = model.Run{Select: &s}
		default:
			out[i] = f(r)
		}
	}
	return out
}
