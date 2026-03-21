package auth

import "encoding/json"

// marshalLanguages converts a slice of language tags to a JSON array string.
func marshalLanguages(langs []string) string {
	if len(langs) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(langs)
	return string(b)
}

// unmarshalLanguages converts a JSON array string to a slice of language tags.
// Returns nil for empty arrays or invalid JSON.
func unmarshalLanguages(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var langs []string
	if err := json.Unmarshal([]byte(s), &langs); err != nil {
		return nil
	}
	if len(langs) == 0 {
		return nil
	}
	return langs
}
