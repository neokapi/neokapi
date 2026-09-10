package contextual

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

func decodeResponse(text string) (response, error) {
	if !utf8.ValidString(text) {
		return response{}, errors.New("response must be valid UTF-8")
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if err := scanValue(decoder, 0); err != nil {
		return response{}, fmt.Errorf("invalid response JSON: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return response{}, errors.New("response must contain exactly one JSON object")
	}
	object, err := exactObject(json.RawMessage(text), []string{"conflicts", "requirements", "suggestions"})
	if err != nil {
		return response{}, err
	}
	for _, name := range []string{"conflicts", "requirements", "suggestions"} {
		var items []json.RawMessage
		if err := json.Unmarshal(object[name], &items); err != nil {
			return response{}, fmt.Errorf("%s must be an array: %w", name, err)
		}
		if items == nil {
			return response{}, fmt.Errorf("%s must be an array, not null", name)
		}
		fields := []string{"candidate_quote", "source_ids", "rationale"}
		if name == "requirements" {
			fields = append(fields, "requirement_id", "status")
		}
		for _, item := range items {
			if _, err := exactObject(item, fields); err != nil {
				return response{}, fmt.Errorf("%s item: %w", name, err)
			}
		}
	}
	var parsed response
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return response{}, fmt.Errorf("decode response: %w", err)
	}
	return parsed, nil
}

func exactObject(data json.RawMessage, fields []string) (map[string]json.RawMessage, error) {
	object := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("expected object: %w", err)
	}
	if len(object) != len(fields) {
		return nil, errors.New("object has missing or unknown fields")
	}
	for _, field := range fields {
		value, ok := object[field]
		if !ok || string(value) == "null" {
			return nil, fmt.Errorf("field %q is required and cannot be null", field)
		}
	}
	return object, nil
}

// scanValue rejects duplicate keys before encoding/json can silently keep the
// last one. Token decoding also rejects malformed JSON and limits nesting here.
func scanValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("JSON nesting exceeds review protocol limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key must be a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = true
			if err := scanValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}
