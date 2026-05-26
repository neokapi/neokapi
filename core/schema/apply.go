package schema

import (
	"encoding/json"
	"reflect"
)

// ApplyConfig populates a struct from a map[string]any config via JSON
// round-trip. The struct's json tags (or camelCase field names) determine
// the mapping. Fields not present in the map retain their zero/default values.
//
// Non-serializable sidecar values (functions, channels) are ignored rather
// than failing the marshal: the CLI injects callbacks such as "onProgress"
// into the config map for tools that consume them, and tools that don't simply
// skip them here. Config structs never carry func/chan fields, so dropping
// such values can never lose real configuration.
func ApplyConfig(config map[string]any, dst any) error {
	data, err := json.Marshal(serializableConfig(config))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// serializableConfig returns config without entries whose values cannot be
// JSON-encoded (funcs, channels). It returns the original map untouched when
// there is nothing to strip, avoiding an allocation in the common case.
func serializableConfig(config map[string]any) map[string]any {
	filtered := config
	copied := false
	for k, v := range config {
		switch reflect.ValueOf(v).Kind() {
		case reflect.Func, reflect.Chan:
			if !copied {
				filtered = make(map[string]any, len(config))
				for k2, v2 := range config {
					filtered[k2] = v2
				}
				copied = true
			}
			delete(filtered, k)
		}
	}
	return filtered
}
