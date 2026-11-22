package format

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	coreschema "github.com/neokapi/neokapi/core/schema"
)

// ApplyMapViaJSON applies configuration values from a map to a struct by
// marshalling the map to JSON and unmarshalling it into dst. Unknown keys
// are rejected (DisallowUnknownFields) and type mismatches produce errors,
// preserving the same contract as hand-written switch-based ApplyMap methods.
//
// For configs with complex parsing logic (custom subfilters, regex caches,
// rune conversions, etc.), keep the manual switch-based ApplyMap approach.
//
// A camelCase spelling of one of dst's snake_case json names is refused with
// the key to write instead, such as `vtt: maxCharsPerLine is now
// max_chars_per_line`.
func ApplyMapViaJSON(dst any, values map[string]any) error {
	if err := renamedJSONKeyError(dst, values); err != nil {
		return err
	}
	b, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshal config values: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// renamedJSONKeyError reports the first key of values, in sorted order, that
// is a camelCase spelling of one of dst's top-level json names.
func renamedJSONKeyError(dst any, values map[string]any) error {
	t := reflect.TypeOf(dst)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	names := make(map[string]bool)
	for f := range t.Fields() {
		if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name != "" && name != "-" {
			names[name] = true
		}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if snake, ok := coreschema.RenamedKey(k, func(n string) bool { return names[n] }); ok {
			if n, ok := dst.(interface{ FormatName() string }); ok {
				return fmt.Errorf("%s: %s is now %s", n.FormatName(), k, snake)
			}
			return fmt.Errorf("%s is now %s", k, snake)
		}
	}
	return nil
}
