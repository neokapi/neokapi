package schema

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// SnakeCase spells a Go or camelCase identifier in snake_case, the spelling
// of every tool and format config key: FillTargetThreshold and
// fillTargetThreshold both become fill_target_threshold. An acronym stays one
// word, including its plural: TextUnitIDs becomes text_unit_ids and
// TranslateURLs becomes translate_urls. A snake_case name is returned
// unchanged.
func SnakeCase(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			switch {
			case unicode.IsLower(prev) || unicode.IsDigit(prev):
				b.WriteByte('_')
			case unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) && !pluralAcronym(runes, i):
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// pluralAcronym reports whether the upper-case rune at i ends an acronym
// followed by its plural s, as the D in IDs or the L in URLs.
func pluralAcronym(runes []rune, i int) bool {
	if runes[i+1] != 's' {
		return false
	}
	return i+2 == len(runes) || unicode.IsUpper(runes[i+2])
}

// SnakeCaseKeys returns a copy of m with each key spelled in snake_case. The
// values are kept as they are.
func SnakeCaseKeys(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[SnakeCase(k)] = v
	}
	return out
}

// IsSnakeCase reports whether key holds no upper-case letter, the one rule a
// tool or format config key keeps.
func IsSnakeCase(key string) bool {
	return !strings.ContainsFunc(key, unicode.IsUpper)
}

// renamedKeys holds the camelCase keys whose replacement is not their
// mechanical snake_case spelling: search-replace's regEx became regex, not
// reg_ex.
var renamedKeys = map[string]string{
	"regEx": "regex",
}

// ReplacementKey returns the key that replaces a camelCase key: the entry in
// renamedKeys when it has one, and its snake_case spelling otherwise.
func ReplacementKey(key string) string {
	if k, ok := renamedKeys[key]; ok {
		return k
	}
	return SnakeCase(key)
}

// RenamedKey returns the snake_case key that replaced key, and true, when key
// is a camelCase spelling of a name known accepts.
func RenamedKey(key string, known func(string) bool) (string, bool) {
	if IsSnakeCase(key) {
		return "", false
	}
	snake := ReplacementKey(key)
	if snake == key || !known(snake) {
		return "", false
	}
	return snake, true
}

// RenamedKeyError reports the first key in config, in sorted order, that is a
// camelCase spelling of one of the schema's snake_case properties, naming the
// key to write instead. at is the key path of the config map, such as
// `flows.translate.steps[1].config`. It descends into the nested objects and
// lists of objects the schema describes.
func RenamedKeyError(props map[string]PropertySchema, config map[string]any, at string) error {
	if len(props) == 0 || len(config) == 0 {
		return nil
	}
	keys := make([]string, 0, len(config))
	for k := range config {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if snake, ok := RenamedKey(k, func(n string) bool { _, ok := props[n]; return ok }); ok {
			return fmt.Errorf("%s: %s is now %s", at, k, snake)
		}
		prop, ok := props[k]
		if !ok {
			continue
		}
		switch v := config[k].(type) {
		case map[string]any:
			if err := RenamedKeyError(prop.Properties, v, at+"."+k); err != nil {
				return err
			}
		case []any:
			if prop.Items == nil {
				continue
			}
			for i, item := range v {
				if m, ok := item.(map[string]any); ok {
					if err := RenamedKeyError(prop.Items.Properties, m, fmt.Sprintf("%s.%s[%d]", at, k, i)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
