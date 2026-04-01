package schema

import "encoding/json"

// ApplyConfig populates a struct from a map[string]any config via JSON
// round-trip. The struct's json tags (or camelCase field names) determine
// the mapping. Fields not present in the map retain their zero/default values.
func ApplyConfig(config map[string]any, dst any) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
