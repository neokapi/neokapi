package model

import (
	"strings"
)

// RunsStructuralText renders a Run sequence with inline-code runs
// replaced by numbered placeholders like {1}, {/1}, {2/}. Pure
// structural projection — abstract away the actual markup so two
// segments with the same shape match. Used as a TM secondary key.
//
// Plural / Select runs render their 'other' branch (or the first
// branch present); Sub runs render as `[id]`.
func RunsStructuralText(runs []Run) string {
	var buf strings.Builder
	appendRunsStructural(&buf, runs)
	return buf.String()
}

func appendRunsStructural(buf *strings.Builder, runs []Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			buf.WriteString(r.Text.Text)
		case r.PcOpen != nil:
			buf.WriteByte('{')
			buf.WriteString(r.PcOpen.ID)
			buf.WriteByte('}')
		case r.PcClose != nil:
			buf.WriteString("{/")
			buf.WriteString(r.PcClose.ID)
			buf.WriteByte('}')
		case r.Ph != nil:
			buf.WriteByte('{')
			buf.WriteString(r.Ph.ID)
			buf.WriteString("/}")
		case r.Sub != nil:
			buf.WriteByte('[')
			buf.WriteString(r.Sub.ID)
			buf.WriteByte(']')
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[PluralOther]; ok {
				appendRunsStructural(buf, form)
				continue
			}
			for _, form := range r.Plural.Forms {
				appendRunsStructural(buf, form)
				break
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				appendRunsStructural(buf, form)
				continue
			}
			for _, form := range r.Select.Cases {
				appendRunsStructural(buf, form)
				break
			}
		}
	}
}

// RunsGeneralizedText renders a Run sequence with entity-typed Ph runs
// replaced by typed placeholders like {PERSON}, {PRODUCT} and other
// inline-code runs replaced by numbered placeholders. This enables
// maximum TM reuse — entities are interchangeable between segments
// with identical structure.
//
// Entity Ph runs are identified by their Type field matching an
// EntityType value. All other inline-code runs follow
// RunsStructuralText.
func RunsGeneralizedText(runs []Run) string {
	var buf strings.Builder
	appendRunsGeneralized(&buf, runs)
	return buf.String()
}

func appendRunsGeneralized(buf *strings.Builder, runs []Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			buf.WriteString(r.Text.Text)
		case r.PcOpen != nil:
			buf.WriteByte('{')
			buf.WriteString(r.PcOpen.ID)
			buf.WriteByte('}')
		case r.PcClose != nil:
			buf.WriteString("{/")
			buf.WriteString(r.PcClose.ID)
			buf.WriteByte('}')
		case r.Ph != nil:
			if IsEntityTypeString(r.Ph.Type) {
				buf.WriteByte('{')
				buf.WriteString(entityTypeLabel(r.Ph.Type))
				buf.WriteByte('}')
			} else {
				buf.WriteByte('{')
				buf.WriteString(r.Ph.ID)
				buf.WriteString("/}")
			}
		case r.Sub != nil:
			buf.WriteByte('[')
			buf.WriteString(r.Sub.ID)
			buf.WriteByte(']')
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[PluralOther]; ok {
				appendRunsGeneralized(buf, form)
				continue
			}
			for _, form := range r.Plural.Forms {
				appendRunsGeneralized(buf, form)
				break
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				appendRunsGeneralized(buf, form)
				continue
			}
			for _, form := range r.Select.Cases {
				appendRunsGeneralized(buf, form)
				break
			}
		}
	}
}

// RunsEntityValues extracts the text values of entity Ph runs from a
// Run sequence. Returns a map from run id to the entity text value.
func RunsEntityValues(runs []Run) map[string]string {
	values := make(map[string]string)
	for _, r := range runs {
		if r.Ph == nil || !IsEntityTypeString(r.Ph.Type) {
			continue
		}
		values[r.Ph.ID] = r.Ph.Data
	}
	return values
}

// RunsPlainText returns the textual projection of a Run sequence
// (TextRun content concatenated, inline-code runs dropped). Suitable
// for plain-text TM matching keys.
func RunsPlainText(runs []Run) string {
	var buf strings.Builder
	for _, r := range runs {
		if r.Text != nil {
			buf.WriteString(r.Text.Text)
		}
	}
	return buf.String()
}
