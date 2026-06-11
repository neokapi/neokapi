package store

import (
	"encoding/json"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/model"
)

// MaxChangesPerRequest is the maximum number of change entries returned per query.
const MaxChangesPerRequest = 1000

// MaxHistoryEntries is the default maximum number of history entries returned.
const MaxHistoryEntries = 100

// scanner is an alias for storage.Scanner, the interface shared by *sql.Row
// and *sql.Rows. Used by the scanX helper functions.
type scanner = storage.Scanner

// deserializeAnnotations converts a JSON string into a map of typed Annotations.
// The JSON format uses a type-discriminated wrapper: {"key": {"type": "...", "data": {...}}}.
// Used by overlay_sync.go's deserializeSingleAnnotation (#405).
func deserializeAnnotations(jsonStr string) map[string]model.Payload {
	result := make(map[string]model.Payload)
	if jsonStr == "" || jsonStr == "{}" || jsonStr == "null" {
		return result
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return result
	}

	for key, data := range raw {
		var wrapper struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			var ga model.GenericAnnotation
			if err := json.Unmarshal(data, &ga); err == nil {
				result[key] = &ga
			}
			continue
		}

		payload := wrapper.Data
		if payload == nil {
			payload = data
		}

		switch wrapper.Type {
		case "alt-translation":
			var ann model.AltTranslations
			if err := json.Unmarshal(payload, &ann); err == nil {
				result[key] = &ann
			}
		case "note":
			var ann model.Notes
			if err := json.Unmarshal(payload, &ann); err == nil {
				result[key] = &ann
			}
		case "entity":
			var ann model.EntityAnnotation
			if err := json.Unmarshal(payload, &ann); err == nil {
				result[key] = &ann
			}
		default:
			var ga model.GenericAnnotation
			if err := json.Unmarshal(payload, &ga); err == nil {
				if ga.Kind == "" {
					ga.Kind = wrapper.Type
				}
				result[key] = &ga
			}
		}
	}

	return result
}
