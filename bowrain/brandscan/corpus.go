package brandscan

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// SourceKind identifies where a piece of brand material came from.
type SourceKind string

// Source kinds, in corpus precedence order: pasted text is the most
// deliberate signal, then linked pages, then uploaded files, then
// repository-harvested content.
const (
	SourcePaste SourceKind = "paste"
	SourceURL   SourceKind = "url"
	SourceFile  SourceKind = "file"
	SourceRepo  SourceKind = "repo"
)

// Source is one ingested piece of brand material, already reduced to plain
// text (by FetchURL, ExtractFile, or verbatim paste).
type Source struct {
	Kind  SourceKind
	Label string // e.g. the URL, filename, or a user-facing name
	Text  string
}

// CorpusSource records one source's contribution to an assembled Corpus.
type CorpusSource struct {
	Kind  SourceKind
	Label string
	Runes int // runes of this source's text included (after any truncation)
}

// Corpus is the merged, source-tagged brand text handed to the profiling
// model.
type Corpus struct {
	Text      string         // tagged sections, one per included source
	RuneCount int            // total runes in Text
	Truncated bool           // true when the budget cut text or dropped sources
	Sources   []CorpusSource // included sources, in corpus order
}

// kindRank orders sources deterministically by kind precedence.
func kindRank(k SourceKind) int {
	switch k {
	case SourcePaste:
		return 0
	case SourceURL:
		return 1
	case SourceFile:
		return 2
	case SourceRepo:
		return 3
	}
	return 4
}

// sourceHeader renders the tag line that precedes a source's text.
func sourceHeader(s Source) string {
	label := strings.TrimSpace(s.Label)
	if label == "" {
		label = string(s.Kind)
	}
	return fmt.Sprintf("=== source: %s (%s) ===", label, s.Kind)
}

// AssembleCorpus merges sources into one source-tagged, budget-capped corpus.
//
// Sources with empty (whitespace-only) text are dropped. The rest are ordered
// deterministically: by kind precedence (paste, url, file, repo), then label,
// then original position for equal keys. Each source contributes a header
// line ("=== source: <label> (<kind>) ===") followed by its trimmed text;
// sections are separated by a blank line.
//
// maxRunes caps the total rune count of the corpus (headers included);
// maxRunes <= 0 means no cap. Truncation always happens on a rune boundary —
// never mid-rune — and a source whose header no longer fits is dropped
// entirely rather than emitted without content.
func AssembleCorpus(sources []Source, maxRunes int) Corpus {
	ordered := make([]Source, 0, len(sources))
	for _, s := range sources {
		if strings.TrimSpace(s.Text) == "" {
			continue
		}
		ordered = append(ordered, s)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		ri, rj := kindRank(ordered[i].Kind), kindRank(ordered[j].Kind)
		if ri != rj {
			return ri < rj
		}
		return ordered[i].Label < ordered[j].Label
	})

	var sb strings.Builder
	var included []CorpusSource
	total := 0
	truncated := false

	for _, s := range ordered {
		header := sourceHeader(s)
		text := strings.TrimSpace(s.Text)

		// Fixed overhead for this section: separator (when not first),
		// header line, and the newline between header and text.
		overhead := utf8.RuneCountInString(header) + 1 // header + "\n"
		separator := ""
		if sb.Len() > 0 {
			separator = "\n\n"
			overhead += 2
		}

		textRunes := utf8.RuneCountInString(text)
		if maxRunes > 0 {
			remaining := maxRunes - total
			if remaining <= overhead {
				// Not even room for a header plus one rune of content:
				// drop this and all following sources.
				truncated = true
				break
			}
			if budget := remaining - overhead; textRunes > budget {
				text = truncateRunes(text, budget)
				textRunes = budget
				truncated = true
			}
		}

		sb.WriteString(separator)
		sb.WriteString(header)
		sb.WriteString("\n")
		sb.WriteString(text)
		total += overhead + textRunes
		included = append(included, CorpusSource{Kind: s.Kind, Label: s.Label, Runes: textRunes})
	}

	return Corpus{
		Text:      sb.String(),
		RuneCount: total,
		Truncated: truncated,
		Sources:   included,
	}
}

// truncateRunes returns the longest prefix of s holding at most max runes,
// cutting only on rune boundaries.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == max {
			return s[:i]
		}
		count++
	}
	return s
}
