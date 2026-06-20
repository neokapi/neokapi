package arb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// parseCatalog decodes an .arb document into the catalog model, preserving the
// source order of resource keys.
//
// Go's map decoding loses object key order, so this parser drives an
// encoding/json token stream directly. The token order it observes is the
// document order. ARB is a flat object, so a single pass suffices: message
// keys, "@<id>" attribute objects, and "@@<global>" metadata are distinguished
// by their key shape.
func parseCatalog(data []byte) (*catalog, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("arb: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("arb: expected object at top level, got %v", tok)
	}

	c := &catalog{resources: make(map[string]*resource)}

	// descriptions collects the "description" of each "@<id>" attributes object,
	// applied to its sibling resource after the full pass (the attributes object
	// may precede or follow its resource).
	descriptions := make(map[string]string)
	// placeholderHints collects the per-placeholder example/description hints of
	// each "@<id>" attributes object, applied alongside descriptions after the
	// full pass.
	placeholderHints := make(map[string][]placeholderHint)

	for dec.More() {
		key, err := readKey(dec)
		if err != nil {
			return nil, err
		}
		switch {
		case key == "@@locale":
			s, err := readString(dec)
			if err != nil {
				return nil, fmt.Errorf("arb: @@locale: %w", err)
			}
			c.locale = s
		case strings.HasPrefix(key, "@@"):
			// File-global metadata other than locale — preserved verbatim by
			// the writer; not modeled beyond skipping its value here.
			if err := skipValue(dec); err != nil {
				return nil, fmt.Errorf("arb: skip %q: %w", key, err)
			}
		case strings.HasPrefix(key, "@"):
			// Resource attributes ("@<id>"). Capture the description for the
			// sibling resource's translator note; everything else is preserved
			// verbatim by the writer.
			id := key[1:]
			attrs, err := parseAttributes(dec)
			if err != nil {
				return nil, fmt.Errorf("arb: %q: %w", key, err)
			}
			if attrs.description != "" {
				descriptions[id] = attrs.description
			}
			if len(attrs.placeholders) > 0 {
				placeholderHints[id] = attrs.placeholders
			}
		default:
			// A translatable resource. ARB message values are always strings.
			val, err := readString(dec)
			if err != nil {
				return nil, fmt.Errorf("arb: resource %q: %w", key, err)
			}
			if _, exists := c.resources[key]; !exists {
				c.keyOrder = append(c.keyOrder, key)
			}
			c.resources[key] = &resource{id: key, value: val}
		}
	}
	// Consume the closing '}'.
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("arb: %w", err)
	}

	for id, desc := range descriptions {
		if r, ok := c.resources[id]; ok {
			r.description = desc
		}
	}
	for id, hints := range placeholderHints {
		if r, ok := c.resources[id]; ok {
			r.placeholders = hints
		}
	}
	return c, nil
}

// attributes is the human-facing context captured from an "@<id>" attributes
// object. Everything else (type, format, optionalParameters, …) is left as
// structure, preserved byte-faithfully by the writer.
type attributes struct {
	// description is the resource-level "description" field (empty when absent).
	description string
	// placeholders are the per-placeholder example/description hints, in
	// document order (empty when there is no "placeholders" object).
	placeholders []placeholderHint
}

// parseAttributes consumes an "@<id>" attributes object, returning its
// "description" field and per-placeholder example/description hints. The rest of
// the object (type, format, etc.) is skipped — it is preserved byte-faithfully
// by the writer.
func parseAttributes(dec *json.Decoder) (attributes, error) {
	var attrs attributes
	tok, err := dec.Token()
	if err != nil {
		return attrs, err
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		// Some files use a non-object attributes value (rare); skip whatever
		// remains so the stream stays balanced.
		return attrs, skipAfterToken(dec, tok)
	}
	for dec.More() {
		field, err := readKey(dec)
		if err != nil {
			return attrs, err
		}
		switch field {
		case "description":
			s, err := readStringOrSkip(dec)
			if err != nil {
				return attrs, fmt.Errorf("description: %w", err)
			}
			attrs.description = s
		case "placeholders":
			hints, err := parsePlaceholders(dec)
			if err != nil {
				return attrs, fmt.Errorf("placeholders: %w", err)
			}
			attrs.placeholders = hints
		default:
			if err := skipValue(dec); err != nil {
				return attrs, fmt.Errorf("skip %q: %w", field, err)
			}
		}
	}
	// Consume the closing '}'.
	if _, err := dec.Token(); err != nil {
		return attrs, err
	}
	return attrs, nil
}

// parsePlaceholders consumes a "placeholders" object — a map of placeholder
// name to its definition object — returning the example/description hint of each
// placeholder in document order. Placeholders without an example or description
// still appear in the slice with empty hint fields; the reader drops the empty
// ones. A non-object "placeholders" value (invalid ARB) is skipped to keep the
// token stream balanced.
func parsePlaceholders(dec *json.Decoder) ([]placeholderHint, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return nil, skipAfterToken(dec, tok)
	}
	var hints []placeholderHint
	for dec.More() {
		name, err := readKey(dec)
		if err != nil {
			return nil, err
		}
		hint, err := parsePlaceholder(dec)
		if err != nil {
			return nil, fmt.Errorf("placeholder %q: %w", name, err)
		}
		hint.name = name
		hints = append(hints, hint)
	}
	// Consume the closing '}'.
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return hints, nil
}

// parsePlaceholder consumes a single placeholder definition object, capturing
// its "example" and "description" fields. All other fields (type, format,
// optionalParameters, isCustomDateFormat, …) are skipped — they are structure,
// preserved byte-faithfully by the writer. A non-object definition (invalid ARB)
// is skipped to keep the token stream balanced.
func parsePlaceholder(dec *json.Decoder) (placeholderHint, error) {
	var hint placeholderHint
	tok, err := dec.Token()
	if err != nil {
		return hint, err
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return hint, skipAfterToken(dec, tok)
	}
	for dec.More() {
		field, err := readKey(dec)
		if err != nil {
			return hint, err
		}
		switch field {
		case "example":
			s, err := readStringOrSkip(dec)
			if err != nil {
				return hint, fmt.Errorf("example: %w", err)
			}
			hint.example = s
		case "description":
			s, err := readStringOrSkip(dec)
			if err != nil {
				return hint, fmt.Errorf("description: %w", err)
			}
			hint.description = s
		default:
			if err := skipValue(dec); err != nil {
				return hint, fmt.Errorf("skip %q: %w", field, err)
			}
		}
	}
	// Consume the closing '}'.
	if _, err := dec.Token(); err != nil {
		return hint, err
	}
	return hint, nil
}

// --- token-stream helpers ---

func readKey(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", fmt.Errorf("arb: read key: %w", err)
	}
	s, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("arb: expected object key, got %v", tok)
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

// readStringOrSkip reads the next value: when it is a string it is returned,
// otherwise the value is skipped (keeping the token stream balanced) and "" is
// returned. This keeps note extraction tolerant of unusual but valid JSON shapes
// in attribute objects — previously these values were skipped wholesale, so
// erroring here would regress files that round-tripped before.
func readStringOrSkip(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	if s, ok := tok.(string); ok {
		return s, nil
	}
	return "", skipAfterToken(dec, tok)
}

// skipValue consumes and discards the next JSON value (scalar, object, or
// array), keeping the token stream balanced.
func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	return skipAfterToken(dec, tok)
}

// skipAfterToken finishes skipping a value whose first token has already been
// read. For a scalar this is a no-op; for an object/array it consumes the
// balanced remainder.
func skipAfterToken(dec *json.Decoder, tok json.Token) error {
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
