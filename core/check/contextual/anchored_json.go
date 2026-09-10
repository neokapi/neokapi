package contextual

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

func decodeAnchoredResponse(text string) (anchoredResponse, error) {
	if !utf8.ValidString(text) {
		return anchoredResponse{}, errors.New("response must be valid UTF-8")
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if err := scanValue(decoder, 0); err != nil {
		return anchoredResponse{}, fmt.Errorf("invalid response JSON: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return anchoredResponse{}, errors.New("response must contain exactly one JSON object")
	}
	object, err := exactObject(json.RawMessage(text), []string{"conflicts", "requirements", "suggestions"})
	if err != nil {
		return anchoredResponse{}, err
	}
	for _, name := range []string{"conflicts", "requirements", "suggestions"} {
		var items []json.RawMessage
		if err := json.Unmarshal(object[name], &items); err != nil {
			return anchoredResponse{}, fmt.Errorf("%s must be an array: %w", name, err)
		}
		if items == nil {
			return anchoredResponse{}, fmt.Errorf("%s must be an array, not null", name)
		}
		fields := []string{"candidate_ids", "source_ids", "rationale"}
		switch name {
		case "conflicts":
			fields = append(fields, "candidate_claim", "source_claim")
		case "requirements":
			fields = append(fields, "requirement_id", "status")
		}
		for _, item := range items {
			if _, err := exactObject(item, fields); err != nil {
				return anchoredResponse{}, fmt.Errorf("%s item: %w", name, err)
			}
		}
	}
	var parsed anchoredResponse
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return anchoredResponse{}, fmt.Errorf("decode anchored response: %w", err)
	}
	return parsed, nil
}
