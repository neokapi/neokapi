package androidxml

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// printfSpecifierRe matches the printf-style format arguments Android string
// resources use (e.g. %s, %d, %1$s, %2$d, %.2f, %%). Each match becomes an
// opaque PlaceholderRun so tools (pseudo-translation, AI, MT) treat the argument
// as code and never reorder or translate its inner characters.
//
// Alternatives, matched in order:
//   - "%%"  the literal percent escape (matched first so it isn't split)
//   - the printf grammar: %, optional positional argument "<n>$", optional
//     flags, optional width, optional precision, optional length modifier,
//     conversion character.
var printfSpecifierRe = regexp.MustCompile(
	`%%|%(\d+\$)?[-+ #0,(]*\d*(\.\d+)?(hh|h|ll|l|L|z|j|t)?[bBhHsScCdoxXeEfgGaAn%]`,
)

// resourceRefRe matches an Android resource reference (@string/foo, @android:string/ok,
// ?attr/colorPrimary) that spans the entire (trimmed) value. A value that is just
// a reference is not translatable text and is left untouched (see reader.go).
var resourceRefRe = regexp.MustCompile(`^[@?](\+?[a-zA-Z0-9_.]+:)?[a-zA-Z0-9_.]+/[a-zA-Z0-9_.]+$`)

// isResourceReference reports whether a decoded value is a bare Android
// resource/attribute reference and therefore not translatable text.
func isResourceReference(decoded string) bool {
	return resourceRefRe.MatchString(strings.TrimSpace(decoded))
}

// buildRuns converts the inner token span of a value element (a <string> or an
// <item>) into a Run sequence. It walks the lossless tokens so that inline
// xliff:g markup and CDATA are protected as opaque codes carrying their exact
// source bytes, and so the writer can reconstruct the value verbatim. Plain
// character data is entity-decoded and scanned for printf placeholders.
//
// innerToks is the slice of tokens strictly between the value element's start
// and end tags (it does not include those tags themselves).
func buildRuns(innerToks []token) []model.Run {
	var runs []model.Run
	phID := 0
	pcID := 0

	// nextPh / nextPc allocate stable inline-code ids.
	nextPh := func() string { phID++; return "p" + strconv.Itoa(phID) }
	nextPc := func() string { pcID++; return "g" + strconv.Itoa(pcID) }

	for i := 0; i < len(innerToks); i++ {
		t := innerToks[i]
		switch t.kind {
		case tokText:
			appendTextRuns(&runs, decodeEntities(t.raw), nextPh)
		case tokCDATA:
			// CDATA (often embedded HTML) is preserved verbatim as one opaque
			// placeholder so its bytes survive untouched.
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID:    nextPh(),
				Type:  "cdata",
				Data:  t.raw,
				Equiv: t.raw,
			}})
		case tokStartTag:
			// An inline element (xliff:g, b, i, u, a, …). xliff:g marks a
			// do-not-translate span; HTML inline tags are styling. Both are
			// protected as paired codes so the translatable text between them
			// stays editable while the markup itself is opaque.
			end := matchEndInner(innerToks, i, t.name)
			if end < 0 {
				// Unbalanced — treat the start tag as a standalone placeholder.
				runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
					ID:    nextPh(),
					Type:  "code",
					Data:  t.raw,
					Equiv: t.raw,
				}})
				continue
			}
			id := nextPc()
			runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{
				ID:    id,
				Type:  pairedType(t.name),
				Data:  t.raw,
				Equiv: t.raw,
			}})
			// Recurse into the inner span (tokens strictly between start/end).
			inner := buildRunsWithCounters(innerToks[i+1:end], &phID, &pcID)
			runs = append(runs, inner...)
			runs = append(runs, model.Run{PcClose: &model.PcCloseRun{
				ID:   id,
				Type: pairedType(t.name),
				Data: innerToks[end].raw,
			}})
			i = end
		case tokSelfClose:
			// A self-closing inline element (e.g. <br/>) → standalone placeholder.
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID:    nextPh(),
				Type:  "code",
				Data:  t.raw,
				Equiv: t.raw,
			}})
		case tokComment:
			// A comment inside a value is non-content; keep it verbatim as code.
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID:    nextPh(),
				Type:  "comment",
				Data:  t.raw,
				Equiv: t.raw,
			}})
		}
	}

	if len(runs) == 0 {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: ""}})
	}
	return runs
}

// buildRunsWithCounters is buildRuns sharing inline-code id counters with a
// parent invocation, so ids stay unique across nested xliff:g spans.
func buildRunsWithCounters(innerToks []token, phID, pcID *int) []model.Run {
	var runs []model.Run
	nextPh := func() string { *phID++; return "p" + strconv.Itoa(*phID) }
	nextPc := func() string { *pcID++; return "g" + strconv.Itoa(*pcID) }

	for i := 0; i < len(innerToks); i++ {
		t := innerToks[i]
		switch t.kind {
		case tokText:
			appendTextRuns(&runs, decodeEntities(t.raw), nextPh)
		case tokCDATA:
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID: nextPh(), Type: "cdata", Data: t.raw, Equiv: t.raw,
			}})
		case tokStartTag:
			end := matchEndInner(innerToks, i, t.name)
			if end < 0 {
				runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
					ID: nextPh(), Type: "code", Data: t.raw, Equiv: t.raw,
				}})
				continue
			}
			id := nextPc()
			runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{
				ID: id, Type: pairedType(t.name), Data: t.raw, Equiv: t.raw,
			}})
			runs = append(runs, buildRunsWithCounters(innerToks[i+1:end], phID, pcID)...)
			runs = append(runs, model.Run{PcClose: &model.PcCloseRun{
				ID: id, Type: pairedType(t.name), Data: innerToks[end].raw,
			}})
			i = end
		case tokSelfClose:
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID: nextPh(), Type: "code", Data: t.raw, Equiv: t.raw,
			}})
		case tokComment:
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID: nextPh(), Type: "comment", Data: t.raw, Equiv: t.raw,
			}})
		}
	}
	return runs
}

// pairedType classifies a paired inline element so downstream tooling can tell a
// do-not-translate xliff:g span from ordinary inline styling.
func pairedType(name string) string {
	if name == "xliff:g" {
		return "ph"
	}
	return "fmt"
}

// appendTextRuns splits decoded character data into TextRuns and PlaceholderRuns,
// lifting every printf specifier (%s, %1$d, %%) into an opaque Ph code so it
// survives translation untouched.
func appendTextRuns(runs *[]model.Run, text string, nextPh func() string) {
	if text == "" {
		return
	}
	locs := printfSpecifierRe.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		*runs = append(*runs, model.Run{Text: &model.TextRun{Text: text}})
		return
	}
	last := 0
	for _, loc := range locs {
		if loc[0] > last {
			*runs = append(*runs, model.Run{Text: &model.TextRun{Text: text[last:loc[0]]}})
		}
		spec := text[loc[0]:loc[1]]
		*runs = append(*runs, model.Run{Ph: &model.PlaceholderRun{
			ID:    nextPh(),
			Type:  "placeholder",
			Data:  spec,
			Equiv: spec,
		}})
		last = loc[1]
	}
	if last < len(text) {
		*runs = append(*runs, model.Run{Text: &model.TextRun{Text: text[last:]}})
	}
}

// matchEndInner finds the matching end tag for the start tag at innerToks[start]
// within an inner token span, honouring nesting. Returns -1 if unbalanced.
func matchEndInner(innerToks []token, start int, name string) int {
	if innerToks[start].kind == tokSelfClose {
		return start
	}
	depth := 0
	for i := start; i < len(innerToks); i++ {
		t := innerToks[i]
		switch {
		case t.kind == tokStartTag && t.name == name:
			depth++
		case t.kind == tokEndTag && t.name == name:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
