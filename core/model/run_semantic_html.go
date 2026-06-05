package model

import (
	"regexp"
	"strconv"
	"strings"
)

// semanticHTMLTagRe matches opening, closing, and self-closing HTML tags.
var semanticHTMLTagRe = regexp.MustCompile(`<(/?)(\w+)([^>]*?)(/?)>`)

// tagNameRe extracts a tag name from an HTML opening / placeholder snippet.
var tagNameRe = regexp.MustCompile(`<(\w+)`)

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

// buildHTMLToTypeMap creates a reverse map from HTML tag names to
// semantic types using the vocabulary registry.
func buildHTMLToTypeMap(reg *VocabularyRegistry) map[string]string {
	if reg == nil {
		return make(map[string]string)
	}
	allTypes := reg.AllTypes()
	m := make(map[string]string, len(allTypes))
	for _, typeName := range allTypes {
		info := reg.Lookup(typeName)
		if info == nil {
			continue
		}
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

func extractTagName(html string) string {
	m := tagNameRe.FindStringSubmatch(html)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

// RunsSemanticHTML renders a Run sequence as semantic HTML using the
// vocabulary registry to map run types to HTML elements. This is the
// projection used for MT APIs that handle HTML natively (DeepL,
// Google, Amazon).
//
// PcOpen runs become `<tag>` openers, PcClose runs become `</tag>`
// closers, and Ph runs become self-closing placeholder elements. Sub
// runs and plural/select runs render their flattened text fallback so
// the wire shape is always well-formed.
func RunsSemanticHTML(runs []Run, reg *VocabularyRegistry) string {
	if len(runs) == 0 {
		return ""
	}
	var buf strings.Builder
	appendRunsSemanticHTML(&buf, runs, reg)
	return buf.String()
}

func appendRunsSemanticHTML(buf *strings.Builder, runs []Run, reg *VocabularyRegistry) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			buf.WriteString(r.Text.Text)
		case r.PcOpen != nil:
			buf.WriteString(reg.HTMLOpen(r.PcOpen.Type))
		case r.PcClose != nil:
			buf.WriteString(reg.HTMLClose(r.PcClose.Type))
		case r.Ph != nil:
			buf.WriteString(reg.HTMLPlaceholder(r.Ph.Type))
		case r.Sub != nil:
			buf.WriteString(r.Sub.Equiv)
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[PluralOther]; ok {
				appendRunsSemanticHTML(buf, form, reg)
				continue
			}
			for _, form := range r.Plural.Forms {
				appendRunsSemanticHTML(buf, form, reg)
				break
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				appendRunsSemanticHTML(buf, form, reg)
				continue
			}
			for _, form := range r.Select.Cases {
				appendRunsSemanticHTML(buf, form, reg)
				break
			}
		}
	}
}

// ParseRunsSemanticHTML reconstructs a Run sequence from an HTML
// response, matching tags back to source runs by position. Source-run
// metadata (id, type, subType, data, equiv, disp, constraints) is
// copied from the matching source run so the target preserves the
// full Run structure.
//
// Tags are paired against the source by traversal order: opening tags
// consume PcOpen source runs in order, closing tags consume PcClose
// source runs in order, self-closing tags consume Ph source runs in
// order. When the target contains more codes of a kind than the
// source, the surplus runs receive synthetic ids assigned via
// assignSequentialRunIDs so the result is still well-formed.
func ParseRunsSemanticHTML(html string, sourceRuns []Run, reg *VocabularyRegistry) []Run {
	var pcOpens, pcCloses, phs []Run
	indexRunsForHTMLLookup(&pcOpens, &pcCloses, &phs, sourceRuns)

	htmlToType := buildHTMLToTypeMap(reg)

	var out []Run
	openIdx, closeIdx, phIdx := 0, 0, 0
	lastEnd := 0
	appendText := func(s string) {
		if s == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].Text != nil {
			out[n-1].Text.Text += s
			return
		}
		out = append(out, Run{Text: &TextRun{Text: s}})
	}

	for _, loc := range semanticHTMLTagRe.FindAllStringSubmatchIndex(html, -1) {
		if loc[0] > lastEnd {
			appendText(html[lastEnd:loc[0]])
		}

		isClosing := html[loc[2]:loc[3]] == "/"
		tagName := html[loc[4]:loc[5]]
		hasSlash := loc[6] < loc[7] && html[loc[6]:loc[7]] == "/"
		isSelfClosing := hasSlash || isSelfClosingTag(tagName)

		switch {
		case isClosing:
			if closeIdx < len(pcCloses) {
				out = append(out, pcCloses[closeIdx])
				closeIdx++
			} else {
				out = append(out, Run{PcClose: &PcCloseRun{
					Type: htmlToType[tagName],
					Data: html[loc[0]:loc[1]],
				}})
			}
		case isSelfClosing:
			if phIdx < len(phs) {
				out = append(out, phs[phIdx])
				phIdx++
			} else {
				out = append(out, Run{Ph: &PlaceholderRun{
					Type: htmlToType[tagName],
					Data: html[loc[0]:loc[1]],
				}})
			}
		default:
			if openIdx < len(pcOpens) {
				out = append(out, pcOpens[openIdx])
				openIdx++
			} else {
				out = append(out, Run{PcOpen: &PcOpenRun{
					Type: htmlToType[tagName],
					Data: html[loc[0]:loc[1]],
				}})
			}
		}

		lastEnd = loc[1]
	}

	if lastEnd < len(html) {
		appendText(html[lastEnd:])
	}

	assignSequentialRunIDs(out)

	return out
}

func indexRunsForHTMLLookup(pcOpens, pcCloses, phs *[]Run, runs []Run) {
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			*pcOpens = append(*pcOpens, r)
		case r.PcClose != nil:
			*pcCloses = append(*pcCloses, r)
		case r.Ph != nil:
			*phs = append(*phs, r)
		case r.Plural != nil:
			for _, form := range r.Plural.Forms {
				indexRunsForHTMLLookup(pcOpens, pcCloses, phs, form)
			}
		case r.Select != nil:
			for _, form := range r.Select.Cases {
				indexRunsForHTMLLookup(pcOpens, pcCloses, phs, form)
			}
		}
	}
}

// assignSequentialRunIDs walks a Run sequence and assigns sequential
// numeric IDs to runs that lack one. Opening / closing pairs share the
// id of the matching opener (LIFO). Used by ParseRunsSemanticHTML when
// the target HTML produced extra inline codes that have no source
// match — the synthetic ids keep the wire shape well-formed.
func assignSequentialRunIDs(runs []Run) {
	nextID := 1
	allHaveIDs := true
	for _, r := range runs {
		if r.RunID() == "" && (r.Ph != nil || r.PcOpen != nil || r.PcClose != nil) {
			allHaveIDs = false
			break
		}
	}
	if allHaveIDs {
		return
	}

	type stackEntry struct {
		idx int
		id  string
	}
	var stack []stackEntry

	for i := range runs {
		switch {
		case runs[i].PcOpen != nil:
			id := runs[i].PcOpen.ID
			if id == "" {
				id = strconv.Itoa(nextID)
				nextID++
				runs[i].PcOpen.ID = id
			}
			stack = append(stack, stackEntry{i, id})
		case runs[i].PcClose != nil:
			if runs[i].PcClose.ID != "" {
				continue
			}
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				runs[i].PcClose.ID = top.id
			} else {
				runs[i].PcClose.ID = strconv.Itoa(nextID)
				nextID++
			}
		case runs[i].Ph != nil:
			if runs[i].Ph.ID == "" {
				runs[i].Ph.ID = strconv.Itoa(nextID)
				nextID++
			}
		}
	}
}
