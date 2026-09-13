package terms

import (
	"encoding/json"
	"fmt"
)

// FormsColumn encodes a term's forms for the forms column both SQL backends
// carry: a JSON array of strings, "[]" when the term declares none.
func FormsColumn(forms []string) string {
	if len(forms) == 0 {
		return "[]"
	}
	b, err := json.Marshal(forms)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// FormsFromColumn decodes the forms column. An empty column or an empty array
// yields nil, so a term with no forms reads back identically from every backend.
func FormsFromColumn(s string) ([]string, error) {
	if s == "" || s == "[]" {
		return nil, nil
	}
	var forms []string
	if err := json.Unmarshal([]byte(s), &forms); err != nil {
		return nil, fmt.Errorf("decode term forms: %w", err)
	}
	if len(forms) == 0 {
		return nil, nil
	}
	return forms, nil
}
