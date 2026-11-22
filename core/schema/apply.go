package schema

import (
	"encoding/json"
	"reflect"
)

// ApplyConfig populates a struct from a map[string]any config. A key is the
// field's config name as the schema shows it (fieldName): its json tag, or
// the field name in snake_case. Fields not present in the map retain their
// zero/default values, and a key that names no field is ignored.
//
// The map is re-keyed onto the names encoding/json decodes before the JSON
// round trip, so an untagged field takes its snake_case key and only that
// key: encoding/json alone would also match FillTarget or filltarget, and
// would never match fill_target.
//
// Non-serializable sidecar values (functions, channels) are dropped rather
// than failing the marshal: the CLI injects callbacks such as "onProgress"
// into the config map for tools that consume them, and tools that don't simply
// skip them here. Config structs never carry func/chan fields, so dropping
// such values can never lose real configuration.
func ApplyConfig(config map[string]any, dst any) error {
	t := reflect.TypeOf(dst)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	var payload any = serializableConfig(config)
	if t != nil && t.Kind() == reflect.Struct {
		payload = rekeyStruct(t, serializableConfig(config))
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

var jsonUnmarshaler = reflect.TypeFor[json.Unmarshaler]()

// rekeyStruct returns the entries of m that name a field of t, keyed by the
// name encoding/json decodes that field under, with nested struct values
// re-keyed the same way.
func rekeyStruct(t reflect.Type, m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for f := range t.Fields() {
			if f.Anonymous {
				if _, tagged := jsonTagName(f); !tagged {
					ft := f.Type
					if ft.Kind() == reflect.Pointer {
						ft = ft.Elem()
					}
					if ft.Kind() == reflect.Struct {
						walk(ft)
						continue
					}
				}
			}
			if !f.IsExported() || f.Tag.Get("json") == "-" {
				continue
			}
			v, ok := m[fieldName(f)]
			if !ok {
				continue
			}
			decode := f.Name
			if name, tagged := jsonTagName(f); tagged {
				decode = name
			}
			out[decode] = rekeyValue(f.Type, v)
		}
	}
	walk(t)
	return out
}

// rekeyValue re-keys a nested config value for a field of type t: a struct
// from a map, and each element of a slice or array of structs. A type that
// decodes itself (json.Unmarshaler), a map field and a scalar pass unchanged.
func rekeyValue(t reflect.Type, v any) any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Implements(jsonUnmarshaler) || reflect.PointerTo(t).Implements(jsonUnmarshaler) {
		return v
	}
	switch t.Kind() {
	case reflect.Struct:
		if m, ok := v.(map[string]any); ok {
			return rekeyStruct(t, m)
		}
	case reflect.Slice, reflect.Array:
		items, ok := v.([]any)
		if !ok {
			return v
		}
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = rekeyValue(t.Elem(), item)
		}
		return out
	}
	return v
}

// serializableConfig returns config without entries whose values cannot be
// JSON-encoded (funcs, channels).
func serializableConfig(config map[string]any) map[string]any {
	filtered := make(map[string]any, len(config))
	for k, v := range config {
		switch reflect.ValueOf(v).Kind() {
		case reflect.Func, reflect.Chan:
			continue
		}
		filtered[k] = v
	}
	return filtered
}
