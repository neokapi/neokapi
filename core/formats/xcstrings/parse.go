package xcstrings

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// parseCatalog decodes an .xcstrings document into the catalog model,
// preserving the source order of keys, languages, and variation categories.
//
// Go's map decoding loses object key order, so this parser drives an
// encoding/json token stream directly. The token order it observes is the
// document order, which is what the *Order slices on the model record.
func parseCatalog(data []byte) (*catalog, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("xcstrings: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("xcstrings: expected object at top level, got %v", tok)
	}

	c := &catalog{
		entries: make(map[string]*entry),
		Version: "1.0",
	}

	for dec.More() {
		key, err := readKey(dec)
		if err != nil {
			return nil, err
		}
		switch key {
		case "sourceLanguage":
			s, err := readString(dec)
			if err != nil {
				return nil, fmt.Errorf("xcstrings: sourceLanguage: %w", err)
			}
			c.SourceLanguage = s
		case "version":
			s, err := readString(dec)
			if err != nil {
				return nil, fmt.Errorf("xcstrings: version: %w", err)
			}
			c.Version = s
		case "strings":
			if err := parseStrings(dec, c); err != nil {
				return nil, err
			}
		default:
			// Unknown top-level field — consume and ignore. Its raw bytes are
			// preserved via the token stream used by the writer, so dropping
			// it from the model is safe.
			if err := skipValue(dec); err != nil {
				return nil, fmt.Errorf("xcstrings: skip %q: %w", key, err)
			}
		}
	}
	// Consume the closing '}'.
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("xcstrings: %w", err)
	}
	return c, nil
}

func parseStrings(dec *json.Decoder, c *catalog) error {
	if err := expectDelim(dec, '{', "strings"); err != nil {
		return err
	}
	for dec.More() {
		key, err := readKey(dec)
		if err != nil {
			return err
		}
		e, err := parseEntry(dec)
		if err != nil {
			return fmt.Errorf("xcstrings: strings[%q]: %w", key, err)
		}
		if _, exists := c.entries[key]; !exists {
			c.keys = append(c.keys, key)
		}
		c.entries[key] = e
	}
	return consumeDelim(dec)
}

func parseEntry(dec *json.Decoder) (*entry, error) {
	if err := expectDelim(dec, '{', "entry"); err != nil {
		return nil, err
	}
	e := &entry{localizations: make(map[string]*localization)}
	for dec.More() {
		key, err := readKey(dec)
		if err != nil {
			return nil, err
		}
		switch key {
		case "comment":
			s, err := readString(dec)
			if err != nil {
				return nil, fmt.Errorf("comment: %w", err)
			}
			e.Comment = s
		case "extractionState":
			s, err := readString(dec)
			if err != nil {
				return nil, fmt.Errorf("extractionState: %w", err)
			}
			e.ExtractionState = s
		case "localizations":
			if err := parseLocalizations(dec, e); err != nil {
				return nil, err
			}
		default:
			if err := skipValue(dec); err != nil {
				return nil, fmt.Errorf("skip %q: %w", key, err)
			}
		}
	}
	return e, consumeDelim(dec)
}

func parseLocalizations(dec *json.Decoder, e *entry) error {
	if err := expectDelim(dec, '{', "localizations"); err != nil {
		return err
	}
	for dec.More() {
		lang, err := readKey(dec)
		if err != nil {
			return err
		}
		loc, err := parseLocalization(dec)
		if err != nil {
			return fmt.Errorf("localizations[%q]: %w", lang, err)
		}
		if _, exists := e.localizations[lang]; !exists {
			e.langOrder = append(e.langOrder, lang)
		}
		e.localizations[lang] = loc
	}
	return consumeDelim(dec)
}

func parseLocalization(dec *json.Decoder) (*localization, error) {
	if err := expectDelim(dec, '{', "localization"); err != nil {
		return nil, err
	}
	loc := &localization{}
	for dec.More() {
		key, err := readKey(dec)
		if err != nil {
			return nil, err
		}
		switch key {
		case "stringUnit":
			su, err := parseStringUnit(dec)
			if err != nil {
				return nil, fmt.Errorf("stringUnit: %w", err)
			}
			loc.StringUnit = su
		case "variations":
			v, err := parseVariations(dec)
			if err != nil {
				return nil, fmt.Errorf("variations: %w", err)
			}
			loc.Variations = v
		default:
			if err := skipValue(dec); err != nil {
				return nil, fmt.Errorf("skip %q: %w", key, err)
			}
		}
	}
	return loc, consumeDelim(dec)
}

func parseStringUnit(dec *json.Decoder) (*stringUnit, error) {
	if err := expectDelim(dec, '{', "stringUnit"); err != nil {
		return nil, err
	}
	su := &stringUnit{}
	for dec.More() {
		key, err := readKey(dec)
		if err != nil {
			return nil, err
		}
		switch key {
		case "state":
			s, err := readString(dec)
			if err != nil {
				return nil, fmt.Errorf("state: %w", err)
			}
			su.State = s
		case "value":
			s, err := readString(dec)
			if err != nil {
				return nil, fmt.Errorf("value: %w", err)
			}
			su.Value = s
		default:
			if err := skipValue(dec); err != nil {
				return nil, fmt.Errorf("skip %q: %w", key, err)
			}
		}
	}
	return su, consumeDelim(dec)
}

func parseVariations(dec *json.Decoder) (*variations, error) {
	if err := expectDelim(dec, '{', "variations"); err != nil {
		return nil, err
	}
	v := &variations{}
	for dec.More() {
		key, err := readKey(dec)
		if err != nil {
			return nil, err
		}
		switch key {
		case "plural":
			cats, order, err := parseCategoryMap(dec)
			if err != nil {
				return nil, fmt.Errorf("plural: %w", err)
			}
			v.plural = cats
			v.pluralOrder = order
		case "device":
			cats, order, err := parseCategoryMap(dec)
			if err != nil {
				return nil, fmt.Errorf("device: %w", err)
			}
			v.device = cats
			v.deviceOrder = order
		case "substitutions":
			subs, order, err := parseSubstitutions(dec)
			if err != nil {
				return nil, fmt.Errorf("substitutions: %w", err)
			}
			v.substitutions = subs
			v.substitutionOrder = order
		default:
			if err := skipValue(dec); err != nil {
				return nil, fmt.Errorf("skip %q: %w", key, err)
			}
		}
	}
	return v, consumeDelim(dec)
}

// parseSubstitutions parses a { "<argName>" : { argNum, formatSpecifier,
// variations } } map.
func parseSubstitutions(dec *json.Decoder) (map[string]*substitution, []string, error) {
	if err := expectDelim(dec, '{', "substitutions"); err != nil {
		return nil, nil, err
	}
	out := make(map[string]*substitution)
	var order []string
	for dec.More() {
		name, err := readKey(dec)
		if err != nil {
			return nil, nil, err
		}
		sub, err := parseSubstitution(dec)
		if err != nil {
			return nil, nil, fmt.Errorf("substitutions[%q]: %w", name, err)
		}
		if _, exists := out[name]; !exists {
			order = append(order, name)
		}
		out[name] = sub
	}
	return out, order, consumeDelim(dec)
}

func parseSubstitution(dec *json.Decoder) (*substitution, error) {
	if err := expectDelim(dec, '{', "substitution"); err != nil {
		return nil, err
	}
	sub := &substitution{}
	for dec.More() {
		key, err := readKey(dec)
		if err != nil {
			return nil, err
		}
		switch key {
		case "argNum":
			tok, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("argNum: %w", err)
			}
			num, ok := tok.(json.Number)
			if !ok {
				return nil, fmt.Errorf("argNum: expected number, got %v", tok)
			}
			n, err := num.Int64()
			if err != nil {
				return nil, fmt.Errorf("argNum: %w", err)
			}
			sub.ArgNum = int(n)
			sub.HasArgNum = true
		case "formatSpecifier":
			s, err := readString(dec)
			if err != nil {
				return nil, fmt.Errorf("formatSpecifier: %w", err)
			}
			sub.FormatSpecifier = s
		case "variations":
			v, err := parseVariations(dec)
			if err != nil {
				return nil, fmt.Errorf("variations: %w", err)
			}
			sub.vars = v
		default:
			if err := skipValue(dec); err != nil {
				return nil, fmt.Errorf("skip %q: %w", key, err)
			}
		}
	}
	return sub, consumeDelim(dec)
}

// parseCategoryMap parses a { "<category>" : { "stringUnit" : {...} } } map,
// returning the category→stringUnit mapping and the source order of
// categories.
func parseCategoryMap(dec *json.Decoder) (map[string]*stringUnit, []string, error) {
	if err := expectDelim(dec, '{', "category map"); err != nil {
		return nil, nil, err
	}
	out := make(map[string]*stringUnit)
	var order []string
	for dec.More() {
		cat, err := readKey(dec)
		if err != nil {
			return nil, nil, err
		}
		// Each category is itself { "stringUnit" : { state, value } }.
		if err := expectDelim(dec, '{', "category"); err != nil {
			return nil, nil, err
		}
		var su *stringUnit
		for dec.More() {
			inner, err := readKey(dec)
			if err != nil {
				return nil, nil, err
			}
			switch inner {
			case "stringUnit":
				s, err := parseStringUnit(dec)
				if err != nil {
					return nil, nil, fmt.Errorf("category[%q].stringUnit: %w", cat, err)
				}
				su = s
			default:
				if err := skipValue(dec); err != nil {
					return nil, nil, fmt.Errorf("skip %q: %w", inner, err)
				}
			}
		}
		if err := consumeDelim(dec); err != nil {
			return nil, nil, err
		}
		if _, exists := out[cat]; !exists {
			order = append(order, cat)
		}
		out[cat] = su
	}
	return out, order, consumeDelim(dec)
}

// --- token-stream helpers ---

func readKey(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", fmt.Errorf("xcstrings: read key: %w", err)
	}
	s, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("xcstrings: expected object key, got %v", tok)
	}
	return s, nil
}

func readString(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	s, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %v", tok)
	}
	return s, nil
}

func expectDelim(dec *json.Decoder, want rune, what string) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("xcstrings: %s: %w", what, err)
	}
	d, ok := tok.(json.Delim)
	if !ok || rune(d) != want {
		return fmt.Errorf("xcstrings: %s: expected %q, got %v", what, string(want), tok)
	}
	return nil
}

// consumeDelim consumes a single closing delimiter token (} or ]).
func consumeDelim(dec *json.Decoder) error {
	_, err := dec.Token()
	if err != nil {
		return fmt.Errorf("xcstrings: close delimiter: %w", err)
	}
	return nil
}

// skipValue consumes and discards the next JSON value (scalar, object, or
// array), keeping the token stream balanced.
func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return nil // scalar consumed
	}
	switch d {
	case '{', '[':
		depth := 1
		for depth > 0 {
			t, err := dec.Token()
			if err != nil {
				return err
			}
			if dd, ok := t.(json.Delim); ok {
				switch dd {
				case '{', '[':
					depth++
				case '}', ']':
					depth--
				}
			}
		}
		return nil
	default:
		return nil
	}
}
