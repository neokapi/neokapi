package storage

import "strings"

// FTSWordTokenizerPlaceholder stands in a migration's SQL for the FTS5 word
// tokenizer, which differs between builds (FTSWordTokenizer: icu with cgo,
// unicode61 without). The runner puts the build's tokenizer in its place when
// it executes the migration (ExecutableSQL), and a migration's identity is
// taken over the placeholder, so one migration has one identity in every
// build.
const FTSWordTokenizerPlaceholder = "{{fts_word_tokenizer}}"

// ExecutableSQL is the SQL a runner executes for m: its text with the build's
// FTS5 word tokenizer in place of FTSWordTokenizerPlaceholder.
func ExecutableSQL(m Migration) string {
	return strings.ReplaceAll(m.SQL, FTSWordTokenizerPlaceholder, FTSWordTokenizer)
}

// NormalizeSQL is SQL as a database executes it rather than as it is typed:
// comments removed and every run of whitespace outside a literal collapsed to
// one space. Text inside a string literal ('…', E'…'), a quoted identifier
// ("…") or a dollar-quoted body ($tag$…$tag$) is kept exactly, comment
// markers included, because the database keeps it. A migration's identity is
// taken over this form, so rewording a comment inside a shipped migration
// does not change what it is.
func NormalizeSQL(sql string) string {
	var b strings.Builder
	space := false
	emit := func(s string) {
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteString(s)
	}
	for i := 0; i < len(sql); {
		c := sql[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			space = true
			i++
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			space = true
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			i = skipBlockComment(sql, i)
			space = true
		case c == '\'':
			escapes := i > 0 && (sql[i-1] == 'E' || sql[i-1] == 'e') && (i < 2 || !isIdentByte(sql[i-2]))
			end := quotedEnd(sql, i, '\'', escapes)
			emit(sql[i:end])
			i = end
		case c == '"':
			end := quotedEnd(sql, i, '"', false)
			emit(sql[i:end])
			i = end
		case c == '$':
			if end, ok := dollarQuotedEnd(sql, i); ok {
				emit(sql[i:end])
				i = end
				continue
			}
			emit(string(c))
			i++
		default:
			j := i
			for j < len(sql) && !strings.ContainsRune(" \t\n\r\f\v'\"$", rune(sql[j])) &&
				!(sql[j] == '-' && j+1 < len(sql) && sql[j+1] == '-') &&
				!(sql[j] == '/' && j+1 < len(sql) && sql[j+1] == '*') {
				j++
			}
			if j == i {
				j = i + 1
			}
			emit(sql[i:j])
			i = j
		}
	}
	return b.String()
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// quotedEnd is the index just past a literal opened by quote at start. A
// doubled quote stands for itself; with escapes, a backslash escapes the next
// byte. An unterminated literal runs to the end.
func quotedEnd(sql string, start int, quote byte, escapes bool) int {
	for i := start + 1; i < len(sql); i++ {
		switch {
		case escapes && sql[i] == '\\':
			i++
		case sql[i] == quote:
			if i+1 < len(sql) && sql[i+1] == quote {
				i++
				continue
			}
			return i + 1
		}
	}
	return len(sql)
}

// dollarQuotedEnd is the index just past a dollar-quoted body opened at start
// ($$ or $tag$), and whether start opens one. A tag starts with a letter or
// an underscore, so a positional parameter such as $1 opens nothing.
func dollarQuotedEnd(sql string, start int) (int, bool) {
	j := start + 1
	if j < len(sql) && (sql[j] == '_' || sql[j] >= 'a' && sql[j] <= 'z' || sql[j] >= 'A' && sql[j] <= 'Z') {
		for j < len(sql) && isIdentByte(sql[j]) {
			j++
		}
	}
	if j >= len(sql) || sql[j] != '$' {
		return 0, false
	}
	tag := sql[start : j+1]
	if k := strings.Index(sql[j+1:], tag); k >= 0 {
		return j + 1 + k + len(tag), true
	}
	return len(sql), true
}

// skipBlockComment is the index just past a block comment opened at start.
// PostgreSQL nests block comments, so a nested opener needs its own closer.
func skipBlockComment(sql string, start int) int {
	depth := 0
	for i := start; i < len(sql)-1; i++ {
		switch {
		case sql[i] == '/' && sql[i+1] == '*':
			depth++
			i++
		case sql[i] == '*' && sql[i+1] == '/':
			depth--
			i++
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(sql)
}
