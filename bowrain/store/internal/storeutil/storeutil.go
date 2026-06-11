// Package storeutil holds helpers shared by the PostgreSQL ContentStore
// (package store) and the SQLite ContentStore (package sqlitestore), so the
// two backends cannot drift on stream defaulting, locale serialization, word
// counting, or block-ID generation.
package storeutil

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
)

// NewBlockID generates a short random block ID.
func NewBlockID() string { return id.New() }

// DefaultStream returns "main" when stream is empty.
func DefaultStream(stream string) string {
	if stream == "" {
		return "main"
	}
	return stream
}

// JoinLocales serializes locales as a comma-separated column value.
func JoinLocales(locales []model.LocaleID) string {
	parts := make([]string, len(locales))
	for i, l := range locales {
		parts[i] = string(l)
	}
	return strings.Join(parts, ",")
}

// SplitLocales parses a comma-separated column value into locale IDs.
func SplitLocales(s string) []model.LocaleID {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	locales := make([]model.LocaleID, len(parts))
	for i, p := range parts {
		locales[i] = model.LocaleID(strings.TrimSpace(p))
	}
	return locales
}

// CountWordsFromSourceJSON counts words from the serialized source runs JSON.
// source_json holds the block's flat []model.Run sequence directly. Text runs
// serialize as `{"text":"..."}` per Framework AD-002, so we decode the text
// key as a bare string. Other run kinds contribute nothing. Unicode space
// boundaries define word breaks.
func CountWordsFromSourceJSON(sourceJSON string) int {
	var runs []struct {
		Text *string `json:"text,omitempty"`
	}
	if err := json.Unmarshal([]byte(sourceJSON), &runs); err != nil {
		return 0
	}
	count := 0
	for _, r := range runs {
		if r.Text != nil {
			count += CountWords(*r.Text)
		}
	}
	return count
}

// CountWords counts whitespace-delimited words, skipping PUA marker runes.
func CountWords(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) || IsMarker(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

// IsMarker checks if a rune is a Unicode Private Use Area marker
// used by the content model for inline span encoding.
func IsMarker(r rune) bool {
	return r >= 0xE000 && r <= 0xF8FF
}
