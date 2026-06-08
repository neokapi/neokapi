package store

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
)

// newBlockID generates a short random block ID.
func newBlockID() string { return id.New() }

// MaxChangesPerRequest is the maximum number of change entries returned per query.
const MaxChangesPerRequest = 1000

// MaxHistoryEntries is the default maximum number of history entries returned.
const MaxHistoryEntries = 100

// defaultStream returns "main" when stream is empty.
func defaultStream(stream string) string {
	if stream == "" {
		return "main"
	}
	return stream
}

func joinLocales(locales []model.LocaleID) string {
	parts := make([]string, len(locales))
	for i, l := range locales {
		parts[i] = string(l)
	}
	return strings.Join(parts, ",")
}

func splitLocales(s string) []model.LocaleID {
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

// scanner is an alias for storage.Scanner, the interface shared by *sql.Row
// and *sql.Rows. Used by the scanX helper functions.
type scanner = storage.Scanner

// deserializeAnnotations converts a JSON string into a map of typed Annotations.
// The JSON format uses a type-discriminated wrapper: {"key": {"type": "...", "data": {...}}}.
// Used by overlay_sync.go's deserializeSingleAnnotation (#405).
func deserializeAnnotations(jsonStr string) map[string]any {
	result := make(map[string]any)
	if jsonStr == "" || jsonStr == "{}" || jsonStr == "null" {
		return result
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return result
	}

	for key, data := range raw {
		var wrapper struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			var ga model.GenericAnnotation
			if err := json.Unmarshal(data, &ga); err == nil {
				result[key] = &ga
			}
			continue
		}

		payload := wrapper.Data
		if payload == nil {
			payload = data
		}

		switch wrapper.Type {
		case "alt-translation":
			var ann model.AltTranslation
			if err := json.Unmarshal(payload, &ann); err == nil {
				result[key] = &ann
			}
		case "note":
			var ann model.NoteAnnotation
			if err := json.Unmarshal(payload, &ann); err == nil {
				result[key] = &ann
			}
		case "entity":
			var ann model.EntityAnnotation
			if err := json.Unmarshal(payload, &ann); err == nil {
				result[key] = &ann
			}
		default:
			var ga model.GenericAnnotation
			if err := json.Unmarshal(payload, &ga); err == nil {
				if ga.Kind == "" {
					ga.Kind = wrapper.Type
				}
				result[key] = &ga
			}
		}
	}

	return result
}

// countWordsFromSourceJSON counts words from the serialized source runs JSON.
// source_json now holds the block's flat []model.Run sequence directly. Text
// runs serialize as `{"text":"..."}` per Framework AD-002, so we decode the
// text key as a bare string.
func countWordsFromSourceJSON(sourceJSON string) int {
	var runs []struct {
		Text *string `json:"text,omitempty"`
	}
	if err := json.Unmarshal([]byte(sourceJSON), &runs); err != nil {
		return 0
	}
	count := 0
	for _, r := range runs {
		if r.Text != nil {
			count += countWords(*r.Text)
		}
	}
	return count
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

// isMarker checks if a rune is a Unicode Private Use Area marker.
func isMarker(r rune) bool {
	return r >= 0xE000 && r <= 0xF8FF
}
