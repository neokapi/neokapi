package model

import (
	"regexp"
	"strings"
)

// RunsPlaceholderText renders a Run sequence as a flat string with
// each inline-code run replaced by a numbered XML placeholder tag.
// The shape is LLM-friendly: the model sees plain text with opaque
// <x id="…"/> tokens it must preserve verbatim, and callers parse
// the response back into Runs via ParseRunsPlaceholderText.
//
// Tag shapes:
//   - PcOpen: <x id="ID"/>
//   - PcClose: <x id="/ID"/>
//   - Ph: <x id="ID/"/>
//   - Sub: <x id="sub:ID"/>
//
// Plural/select runs are rendered by concatenating their 'other'
// form (or the first form present), so LLMs see flat text — the
// structured construct is reassembled from the source when the
// response is parsed.
func RunsPlaceholderText(runs []Run) string {
	if len(runs) == 0 {
		return ""
	}
	var buf strings.Builder
	appendRunsPlaceholder(&buf, runs)
	return buf.String()
}

func appendRunsPlaceholder(buf *strings.Builder, runs []Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			buf.WriteString(r.Text.Text)
		case r.Ph != nil:
			buf.WriteString(`<x id="`)
			buf.WriteString(r.Ph.ID)
			buf.WriteString(`/"/>`)
		case r.PcOpen != nil:
			buf.WriteString(`<x id="`)
			buf.WriteString(r.PcOpen.ID)
			buf.WriteString(`"/>`)
		case r.PcClose != nil:
			buf.WriteString(`<x id="/`)
			buf.WriteString(r.PcClose.ID)
			buf.WriteString(`"/>`)
		case r.Sub != nil:
			buf.WriteString(`<x id="sub:`)
			buf.WriteString(r.Sub.ID)
			buf.WriteString(`"/>`)
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[PluralOther]; ok {
				appendRunsPlaceholder(buf, form)
				continue
			}
			for _, form := range r.Plural.Forms {
				appendRunsPlaceholder(buf, form)
				break
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				appendRunsPlaceholder(buf, form)
				continue
			}
			for _, form := range r.Select.Cases {
				appendRunsPlaceholder(buf, form)
				break
			}
		}
	}
}

// runPlaceholderTagRe matches the <x id="…"/> tags emitted by
// RunsPlaceholderText (and reused by any LLM response that preserves
// them verbatim).
var runPlaceholderTagRe = regexp.MustCompile(`<x\s+id="([^"]+)"\s*/>`)

// runLookupKey keys sourceRuns entries for ParseRunsPlaceholderText.
type runLookupKey struct {
	id   string
	kind RunKind
}

// ParseRunsPlaceholderText reconstructs a Run sequence from an
// LLM response that contains <x id="…"/> placeholders, matching each
// tag back to the corresponding source Run by id. Source metadata
// (type, subType, data, equiv, disp, constraints) is copied from
// the matching source run so the target preserves the full Run
// structure.
//
// Text segments between placeholders become TextRuns. An unknown
// placeholder (no source match) produces a minimal Ph run so the
// output stays well-formed.
func ParseRunsPlaceholderText(text string, sourceRuns []Run) []Run {
	lookup := make(map[runLookupKey]Run, len(sourceRuns))
	indexRunsForLookup(lookup, sourceRuns)

	var out []Run
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

	for _, loc := range runPlaceholderTagRe.FindAllStringSubmatchIndex(text, -1) {
		if loc[0] > lastEnd {
			appendText(text[lastEnd:loc[0]])
		}
		raw := text[loc[2]:loc[3]]
		out = append(out, runFromPlaceholderID(raw, lookup))
		lastEnd = loc[1]
	}
	if lastEnd < len(text) {
		appendText(text[lastEnd:])
	}
	return out
}

func indexRunsForLookup(lookup map[runLookupKey]Run, runs []Run) {
	for _, r := range runs {
		switch {
		case r.Ph != nil:
			lookup[runLookupKey{r.Ph.ID, RunKindPh}] = r
		case r.PcOpen != nil:
			lookup[runLookupKey{r.PcOpen.ID, RunKindPcOpen}] = r
		case r.PcClose != nil:
			lookup[runLookupKey{r.PcClose.ID, RunKindPcClose}] = r
		case r.Sub != nil:
			lookup[runLookupKey{r.Sub.ID, RunKindSub}] = r
		case r.Plural != nil:
			for _, form := range r.Plural.Forms {
				indexRunsForLookup(lookup, form)
			}
		case r.Select != nil:
			for _, form := range r.Select.Cases {
				indexRunsForLookup(lookup, form)
			}
		}
	}
}

// runFromPlaceholderID decodes a placeholder tag's id field back
// into a Run by consulting the source-side lookup.
func runFromPlaceholderID(raw string, lookup map[runLookupKey]Run) Run {
	switch {
	case strings.HasPrefix(raw, "/"):
		id := strings.TrimPrefix(raw, "/")
		if r, ok := lookup[runLookupKey{id, RunKindPcClose}]; ok {
			return r
		}
		return Run{PcClose: &PcCloseRun{ID: id}}
	case strings.HasSuffix(raw, "/"):
		id := strings.TrimSuffix(raw, "/")
		if r, ok := lookup[runLookupKey{id, RunKindPh}]; ok {
			return r
		}
		return Run{Ph: &PlaceholderRun{ID: id}}
	case strings.HasPrefix(raw, "sub:"):
		id := strings.TrimPrefix(raw, "sub:")
		if r, ok := lookup[runLookupKey{id, RunKindSub}]; ok {
			return r
		}
		return Run{Sub: &SubRun{ID: id}}
	default:
		if r, ok := lookup[runLookupKey{raw, RunKindPcOpen}]; ok {
			return r
		}
		return Run{PcOpen: &PcOpenRun{ID: raw}}
	}
}
