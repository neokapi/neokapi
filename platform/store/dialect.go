package store

import (
	"encoding/json"
	"strconv"
	"unicode"
)

// Dialect identifies the SQL database backend.
type Dialect int

const (
	DialectSQLite Dialect = iota
	DialectPostgres
)

// Rebind converts ?-placeholder SQL to $N-placeholder SQL for PostgreSQL.
// For SQLite, it returns the query unchanged.
func Rebind(dialect Dialect, query string) string {
	if dialect == DialectSQLite {
		return query
	}

	var out []byte
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			out = append(out, '$')
			out = append(out, []byte(strconv.Itoa(n))...)
			n++
		} else {
			out = append(out, query[i])
		}
	}
	return string(out)
}

// countWordsFromSourceJSON counts words from serialized source segments JSON
// without fully deserializing Span objects. It extracts only the CodedText
// fields and counts words using Unicode space boundaries.
func countWordsFromSourceJSON(sourceJSON string) int {
	// Lightweight: unmarshal only the fields we need.
	var segments []struct {
		Content struct {
			CodedText string `json:"CodedText"`
		} `json:"Content"`
	}
	if err := json.Unmarshal([]byte(sourceJSON), &segments); err != nil {
		return 0
	}
	count := 0
	for _, seg := range segments {
		count += countWords(seg.Content.CodedText)
	}
	return count
}

// extractTargetLocales returns locale keys from a targets_json string
// (which is a JSON object mapping locale → segments) without deserializing
// the segment values. Only locales with non-empty segment arrays are included.
func extractTargetLocales(targetsJSON string) []string {
	// Parse just the top-level object to get keys + check for non-empty arrays.
	var targets map[string]json.RawMessage
	if err := json.Unmarshal([]byte(targetsJSON), &targets); err != nil {
		return nil
	}
	var locales []string
	for locale, raw := range targets {
		// Skip empty arrays: "[]" or null.
		if len(raw) > 2 { // longer than "[]"
			locales = append(locales, locale)
		}
	}
	return locales
}

// countWords counts whitespace-delimited words, skipping PUA marker runes.
func countWords(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) || isMarker(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

// isMarker checks if a rune is a Unicode Private Use Area marker
// used by the content model for inline span encoding.
func isMarker(r rune) bool {
	return r >= 0xE000 && r <= 0xF8FF
}
