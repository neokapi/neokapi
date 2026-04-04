package format

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ApplyMapViaJSON applies configuration values from a map to a struct by
// marshalling the map to JSON and unmarshalling it into dst. Unknown keys
// are rejected (DisallowUnknownFields) and type mismatches produce errors,
// preserving the same contract as hand-written switch-based ApplyMap methods.
//
// For configs with complex parsing logic (custom subfilters, regex caches,
// rune conversions, etc.), keep the manual switch-based ApplyMap approach.
func ApplyMapViaJSON(dst any, values map[string]any) error {
	b, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshal config values: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
