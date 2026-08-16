package arb

import (
	"strconv"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/icu"
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
			end, ok := icu.MatchBrace(value, i)
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

// appendQuoted consumes the ICU apostrophe span starting at s[*i] and appends
// its raw bytes (apostrophes included) to dst, advancing *i past the span. The
// bytes are copied verbatim so the message value round-trips exactly; the only
// thing that matters here is that braces inside a quoted span do not affect
// construct scanning. The span's extent is ICU's own (core/icu.SkipQuoted).
func appendQuoted(dst []byte, s string, i *int) []byte {
	start := *i
	icu.SkipQuoted(s, i)
	return append(dst, s[start:*i]...)
}

func appendRune(dst []byte, r rune) []byte {
	return utf8.AppendRune(dst, r)
}
