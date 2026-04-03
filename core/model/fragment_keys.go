package model

import (
	"fmt"
	"strings"
)

// entityTypeLabel extracts the entity label from a type string.
// "entity:person" → "PERSON", "entity:organization" → "ORGANIZATION".
func entityTypeLabel(typeName string) string {
	return strings.ToUpper(strings.TrimPrefix(typeName, EntityPrefix))
}

// StructuralText returns the fragment text with span markers replaced by
// numbered placeholders like {1}, {/1}, {2}. This preserves inline code
// structure while abstracting away the actual markup.
func (f *Fragment) StructuralText() string {
	if len(f.Spans) == 0 {
		return f.Text()
	}

	var buf strings.Builder
	spanIdx := 0
	for _, r := range f.CodedText {
		if isMarker(r) && spanIdx < len(f.Spans) {
			span := f.Spans[spanIdx]
			switch span.SpanType {
			case SpanOpening:
				buf.WriteString(fmt.Sprintf("{%s}", span.ID))
			case SpanClosing:
				buf.WriteString(fmt.Sprintf("{/%s}", span.ID))
			case SpanPlaceholder:
				buf.WriteString(fmt.Sprintf("{%s/}", span.ID))
			}
			spanIdx++
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// GeneralizedText returns the fragment text with entity-type spans replaced
// by typed placeholders like {PERSON}, {PRODUCT} and structural spans as
// numbered placeholders. This enables maximum TM reuse — entities are
// interchangeable between segments with identical structure.
//
// Entity spans are identified by their Type field matching an EntityType value.
func (f *Fragment) GeneralizedText() string {
	if len(f.Spans) == 0 {
		return f.Text()
	}

	var buf strings.Builder
	spanIdx := 0
	for _, r := range f.CodedText {
		if isMarker(r) && spanIdx < len(f.Spans) {
			span := f.Spans[spanIdx]
			if isEntitySpan(span) {
				// Entity spans get typed placeholders.
				buf.WriteString(fmt.Sprintf("{%s}", entityTypeLabel(span.Type)))
			} else {
				// Structural spans get numbered placeholders.
				switch span.SpanType {
				case SpanOpening:
					buf.WriteString(fmt.Sprintf("{%s}", span.ID))
				case SpanClosing:
					buf.WriteString(fmt.Sprintf("{/%s}", span.ID))
				case SpanPlaceholder:
					buf.WriteString(fmt.Sprintf("{%s/}", span.ID))
				}
			}
			spanIdx++
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// isEntitySpan returns true if the span represents a named entity
// (person, product, organization, etc.) rather than structural markup.
func isEntitySpan(s *Span) bool {
	return IsEntityTypeString(s.Type)
}

// EntitySpans returns only the spans that represent named entities.
func (f *Fragment) EntitySpans() []*Span {
	var entities []*Span
	for _, s := range f.Spans {
		if isEntitySpan(s) {
			entities = append(entities, s)
		}
	}
	return entities
}

// EntityValues extracts the text values of entity spans from the coded text.
// Returns a map from span ID to the entity text value.
func (f *Fragment) EntityValues() map[string]string {
	values := make(map[string]string)
	if len(f.Spans) == 0 {
		return values
	}

	runes := []rune(f.CodedText)
	spanIdx := 0

	for i := range runes {
		if isMarker(runes[i]) && spanIdx < len(f.Spans) {
			span := f.Spans[spanIdx]
			if isEntitySpan(span) && span.SpanType == SpanPlaceholder {
				// The entity value is stored in the span's Data field.
				values[span.ID] = span.Data
			}
			spanIdx++
		}
	}
	return values
}
