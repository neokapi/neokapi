package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// SemanticHTML renders the Fragment as semantic HTML using the vocabulary
// registry to map span types to HTML elements. This is the projection used
// for MT APIs that handle HTML natively (DeepL, Google, Amazon).
func (f *Fragment) SemanticHTML(reg *VocabularyRegistry) string {
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
				buf.WriteString(reg.HTMLOpen(span.Type))
			case SpanClosing:
				buf.WriteString(reg.HTMLClose(span.Type))
			case SpanPlaceholder:
				buf.WriteString(reg.HTMLPlaceholder(span.Type))
			}
			spanIdx++
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// PlaceholderText renders markers as numbered XML placeholders.
// Opening: <x id="1"/>, Closing: <x id="/1"/>, Placeholder: <x id="1/"/>.
// No registry needed — purely structural.
func (f *Fragment) PlaceholderText() string {
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
				buf.WriteString(fmt.Sprintf(`<x id="%s"/>`, span.ID))
			case SpanClosing:
				buf.WriteString(fmt.Sprintf(`<x id="/%s"/>`, span.ID))
			case SpanPlaceholder:
				buf.WriteString(fmt.Sprintf(`<x id="%s/"/>`, span.ID))
			}
			spanIdx++
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// placeholderTagRe matches <x id="..." /> patterns from PlaceholderText output.
var placeholderTagRe = regexp.MustCompile(`<x\s+id="([^"]+)"\s*/>`)

// ParsePlaceholderText reconstructs a Fragment from a PlaceholderText response,
// matching <x id="N"/> back to source Spans by ID. Source spans provide Data
// and all metadata for the reconstructed Fragment.
func ParsePlaceholderText(text string, sourceSpans []*Span) *Fragment {
	// Build lookup from ID+type to source span.
	type spanKey struct {
		id       string
		spanType SpanType
	}
	lookup := make(map[spanKey]*Span)
	for _, s := range sourceSpans {
		lookup[spanKey{s.ID, s.SpanType}] = s
	}

	frag := &Fragment{}
	lastEnd := 0

	for _, loc := range placeholderTagRe.FindAllStringSubmatchIndex(text, -1) {
		// Append text before this match.
		if loc[0] > lastEnd {
			frag.CodedText += text[lastEnd:loc[0]]
		}

		idStr := text[loc[2]:loc[3]]
		var spanType SpanType
		var cleanID string

		if strings.HasPrefix(idStr, "/") {
			spanType = SpanClosing
			cleanID = strings.TrimPrefix(idStr, "/")
		} else if strings.HasSuffix(idStr, "/") {
			spanType = SpanPlaceholder
			cleanID = strings.TrimSuffix(idStr, "/")
		} else {
			spanType = SpanOpening
			cleanID = idStr
		}

		key := spanKey{cleanID, spanType}
		if srcSpan, ok := lookup[key]; ok {
			// Clone the source span to preserve all metadata.
			clone := *srcSpan
			frag.AppendSpan(&clone)
		} else {
			// Unknown placeholder — create a minimal span.
			frag.AppendSpan(&Span{
				SpanType: spanType,
				ID:       cleanID,
			})
		}

		lastEnd = loc[1]
	}

	// Append remaining text.
	if lastEnd < len(text) {
		frag.CodedText += text[lastEnd:]
	}

	return frag
}

// semanticHTMLTagRe matches opening, closing, and self-closing HTML tags.
var semanticHTMLTagRe = regexp.MustCompile(`<(/?)(\w+)([^>]*?)(/?)>`)

// ParseSemanticHTML reconstructs a Fragment from an HTML response, matching
// tags back to source Spans by position. Restores Data from source.
func ParseSemanticHTML(html string, sourceSpans []*Span, reg *VocabularyRegistry) *Fragment {
	// Build a list of source spans indexed by position for sequential matching.
	openingSpans := make([]*Span, 0)
	closingSpans := make([]*Span, 0)
	placeholderSpans := make([]*Span, 0)

	for _, s := range sourceSpans {
		switch s.SpanType {
		case SpanOpening:
			openingSpans = append(openingSpans, s)
		case SpanClosing:
			closingSpans = append(closingSpans, s)
		case SpanPlaceholder:
			placeholderSpans = append(placeholderSpans, s)
		}
	}

	// Build reverse map: HTML tag → semantic type for detection.
	htmlToType := buildHTMLToTypeMap(reg)

	frag := &Fragment{}
	openIdx, closeIdx, phIdx := 0, 0, 0
	lastEnd := 0

	for _, loc := range semanticHTMLTagRe.FindAllStringSubmatchIndex(html, -1) {
		// Append text before this match.
		if loc[0] > lastEnd {
			frag.CodedText += html[lastEnd:loc[0]]
		}

		isClosing := html[loc[2]:loc[3]] == "/"
		tagName := html[loc[4]:loc[5]]
		isSelfClosing := loc[6] < loc[7] && html[loc[6]:loc[7]] == "/" ||
			isSelfClosingTag(tagName)

		if isClosing {
			// Closing tag.
			if closeIdx < len(closingSpans) {
				clone := *closingSpans[closeIdx]
				frag.AppendSpan(&clone)
				closeIdx++
			} else {
				frag.AppendSpan(&Span{
					SpanType: SpanClosing,
					Type:     htmlToType[tagName],
					Data:     html[loc[0]:loc[1]],
				})
			}
		} else if isSelfClosing {
			// Self-closing / placeholder tag.
			if phIdx < len(placeholderSpans) {
				clone := *placeholderSpans[phIdx]
				frag.AppendSpan(&clone)
				phIdx++
			} else {
				frag.AppendSpan(&Span{
					SpanType: SpanPlaceholder,
					Type:     htmlToType[tagName],
					Data:     html[loc[0]:loc[1]],
				})
			}
		} else {
			// Opening tag.
			if openIdx < len(openingSpans) {
				clone := *openingSpans[openIdx]
				frag.AppendSpan(&clone)
				openIdx++
			} else {
				frag.AppendSpan(&Span{
					SpanType: SpanOpening,
					Type:     htmlToType[tagName],
					Data:     html[loc[0]:loc[1]],
				})
			}
		}

		lastEnd = loc[1]
	}

	// Append remaining text.
	if lastEnd < len(html) {
		frag.CodedText += html[lastEnd:]
	}

	// Assign sequential IDs if not already set.
	assignSequentialIDs(frag)

	return frag
}

// isSelfClosingTag returns true for HTML void elements.
func isSelfClosingTag(tag string) bool {
	switch strings.ToLower(tag) {
	case "br", "hr", "img", "input", "meta", "link", "area", "base",
		"col", "embed", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

// buildHTMLToTypeMap creates a reverse map from HTML tag names to semantic types.
func buildHTMLToTypeMap(reg *VocabularyRegistry) map[string]string {
	m := make(map[string]string)
	if reg == nil {
		return m
	}
	for _, typeName := range reg.AllTypes() {
		info := reg.Lookup(typeName)
		if info == nil {
			continue
		}
		// Extract tag name from HTML open or placeholder.
		if info.HTML.Open != "" {
			if tag := extractTagName(info.HTML.Open); tag != "" {
				m[tag] = typeName
			}
		}
		if info.HTML.Placeholder != "" {
			if tag := extractTagName(info.HTML.Placeholder); tag != "" {
				m[tag] = typeName
			}
		}
	}
	return m
}

var tagNameRe = regexp.MustCompile(`<(\w+)`)

func extractTagName(html string) string {
	m := tagNameRe.FindStringSubmatch(html)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

// assignSequentialIDs assigns sequential numeric IDs to spans in a Fragment.
// Opening and closing spans that pair share the same ID.
func assignSequentialIDs(frag *Fragment) {
	nextID := 1
	// Track IDs already assigned via source spans.
	hasIDs := true
	for _, s := range frag.Spans {
		if s.ID == "" {
			hasIDs = false
			break
		}
	}
	if hasIDs {
		return
	}

	// Stack for pairing opening/closing spans.
	type stackEntry struct {
		spanIdx int
		id      string
	}
	var stack []stackEntry

	for i, s := range frag.Spans {
		switch s.SpanType {
		case SpanOpening:
			id := strconv.Itoa(nextID)
			nextID++
			s.ID = id
			stack = append(stack, stackEntry{i, id})
		case SpanClosing:
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				s.ID = top.id
			} else {
				s.ID = strconv.Itoa(nextID)
				nextID++
			}
		case SpanPlaceholder:
			s.ID = strconv.Itoa(nextID)
			nextID++
		}
	}
}
