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
			desc, err := parseAttributes(dec)
			if err != nil {
				return nil, fmt.Errorf("arb: %q: %w", key, err)
			}
			if desc != "" {
				descriptions[id] = desc
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
	return c, nil
}

// parseAttributes consumes an "@<id>" attributes object, returning its
// "description" field (empty when absent). The rest of the object (placeholders,
// type, etc.) is skipped — it is preserved byte-faithfully by the writer.
func parseAttributes(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		// Some files use a non-object attributes value (rare); skip whatever
		// remains so the stream stays balanced.
		return "", skipAfterToken(dec, tok)
	}
	var description string
	for dec.More() {
		field, err := readKey(dec)
		if err != nil {
			return "", err
		}
		if field == "description" {
			s, err := readString(dec)
			if err != nil {
				return "", fmt.Errorf("description: %w", err)
			}
			description = s
			continue
		}
		if err := skipValue(dec); err != nil {
			return "", fmt.Errorf("skip %q: %w", field, err)
		}
	}
	// Consume the closing '}'.
	if _, err := dec.Token(); err != nil {
		return "", err
	}
	return description, nil
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
