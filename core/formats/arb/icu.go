package arb

import (
	"strconv"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
)

// This file converts an ARB message value (ICU MessageFormat text) into a Run
// sequence that protects ICU syntax from translation, and renders such a Run
// sequence back to the exact original value.
//
// ARB messages use ICU MessageFormat: simple placeholders like {name}, and
// structured constructs like {count, plural, …}, {gender, select, …}, and
// selectordinal. The whole point of protection is that translators (and AI/MT
// tools) must never alter the ICU syntax — the {…} braces, argument names,
// plural/select keywords, and the # number sign carry program semantics.
//
// Rather than expose plural/select branches as translatable structure (which
// would let tooling rewrite keywords and break the message), we treat every
// top-level ICU construct as ONE opaque PlaceholderRun whose Data is its exact
// source text. Literal text outside any construct becomes TextRun content and
// remains translatable. RenderRunsWithData reproduces the value byte-for-byte
// because each placeholder re-emits its captured Data, so round-trips through
// the writer's value-substitution path are exact.
//
// ICU single-quote escaping is honoured while scanning so that a quoted '{' or
// '}' inside literal text does not open or close a construct. Quotes are kept
// in the surrounding literal text (they are part of the message, not syntax we
// strip), preserving the value exactly.

// runsFromValue splits an ICU MessageFormat message into Runs: literal text as
// TextRuns and each balanced top-level {…} construct as an opaque
// PlaceholderRun carrying its exact source bytes. A simple {name} reference and
// a full {count, plural, …} block are both treated as single placeholders —
// the distinction does not matter for protection, only that the syntax is kept
// intact.
func runsFromValue(value string) []model.Run {
	var runs []model.Run
	var lit []byte
	id := 0

	flushLit := func() {
		if len(lit) > 0 {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: string(lit)}})
			lit = lit[:0]
		}
	}

	i := 0
	n := len(value)
	for i < n {
		ch := value[i]
		switch ch {
		case '\'':
			// ICU quoting. Copy the quoted span verbatim into literal text so a
			// quoted brace does not toggle construct scanning. The quote
			// characters themselves are part of the literal message text.
			lit = appendQuoted(lit, value, &i)
		case '{':
			// Start of an ICU construct. Find its matching close brace,
			// honouring nested braces and quoting.
			end, ok := matchBrace(value, i)
			if !ok {
				// Unbalanced brace — treat the rest as literal text so the value
				// still round-trips exactly.
				lit = append(lit, value[i:]...)
				i = n
				continue
			}
			flushLit()
			id++
			data := value[i : end+1]
			runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
				ID:    "p" + strconv.Itoa(id),
				Type:  "icu",
				Data:  data,
				Equiv: data,
				Disp:  data,
			}})
			i = end + 1
		default:
			r, size := utf8.DecodeRuneInString(value[i:])
			lit = appendRune(lit, r)
			i += size
		}
	}
	flushLit()

	if len(runs) == 0 {
		// No content at all (empty message) — emit a single empty TextRun so the
		// block still has a source segment.
		return []model.Run{{Text: &model.TextRun{Text: value}}}
	}
	return runs
}

// valueFromRuns renders a Run sequence back to a flat ARB message value,
// emitting each placeholder's captured Data (the exact ICU source) verbatim.
func valueFromRuns(runs []model.Run) string {
	return model.RenderRunsWithData(runs)
}

// matchBrace returns the index of the '}' that closes the '{' at start,
// honouring nested braces and ICU single-quote escaping. ok is false when no
// matching brace is found.
func matchBrace(s string, start int) (int, bool) {
	depth := 0
	i := start
	n := len(s)
	for i < n {
		ch := s[i]
		switch ch {
		case '\'':
			// Skip a quoted span without counting any braces inside it.
			skipQuoted(s, &i)
			continue
		case '{':
			depth++
			i++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
			i++
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
	}
	return 0, false
}

// appendQuoted consumes an ICU-quoted span starting at the single quote at
// s[*i] and appends its raw bytes (including the surrounding quotes) to dst,
// advancing *i past the span. ICU quoting rules:
//
//   - a doubled quote is a literal apostrophe;
//   - a quote followed by other characters opens a quoted literal run that
//     ends at the next lone quote (a doubled quote inside is an escaped
//     apostrophe and does not end the run);
//   - a lone quote at end of string is a literal apostrophe.
//
// The bytes are copied verbatim (quotes included) so the message value
// round-trips exactly; we only need to ensure braces inside a quote do not
// affect construct scanning.
func appendQuoted(dst []byte, s string, i *int) []byte {
	n := len(s)
	// Copy the opening quote.
	dst = append(dst, '\'')
	*i++ // past opening quote
	if *i >= n {
		return dst // lone trailing quote
	}
	if s[*i] == '\'' {
		// A doubled quote is an escaped apostrophe; copy it and return.
		dst = append(dst, '\'')
		*i++
		return dst
	}
	// Quoted literal run: copy until the next lone single quote.
	for *i < n {
		if s[*i] == '\'' {
			dst = append(dst, '\'')
			*i++
			if *i < n && s[*i] == '\'' {
				dst = append(dst, '\'')
				*i++
				continue
			}
			return dst
		}
		r, size := utf8.DecodeRuneInString(s[*i:])
		dst = appendRune(dst, r)
		*i += size
	}
	return dst
}

// skipQuoted advances *i past an ICU-quoted span starting at s[*i], applying
// the same quoting rules as appendQuoted but discarding the bytes.
func skipQuoted(s string, i *int) {
	n := len(s)
	*i++ // past opening quote
	if *i >= n {
		return
	}
	if s[*i] == '\'' {
		*i++
		return
	}
	for *i < n {
		if s[*i] == '\'' {
			*i++
			if *i < n && s[*i] == '\'' {
				*i++
				continue
			}
			return
		}
		_, size := utf8.DecodeRuneInString(s[*i:])
		*i += size
	}
}

func appendRune(dst []byte, r rune) []byte {
	return utf8.AppendRune(dst, r)
}
