package preset

// MergeConfig performs a 3-layer deep merge: defaults -> preset -> overrides.
// Each layer may be nil, which is treated as an empty map.
func MergeConfig(defaults, preset, overrides map[string]any) map[string]any {
	result := DeepMerge(defaults, preset)
	return DeepMerge(result, overrides)
}

// DeepMerge merges src into dst. Maps merge recursively; non-map values from
// src replace those in dst. Returns a new map (does not modify dst or src).
// Nil inputs are treated as empty maps.
func DeepMerge(dst, src map[string]any) map[string]any {
	out := DeepCopy(dst)
	if out == nil {
		out = make(map[string]any)
	}
	if src == nil {
		return out
	}
	for k, srcVal := range src {
		dstVal, exists := out[k]
		if !exists {
			out[k] = deepCopyValue(srcVal)
			continue
		}
		dstMap, dstOK := dstVal.(map[string]any)
		srcMap, srcOK := srcVal.(map[string]any)
		if dstOK && srcOK {
			out[k] = DeepMerge(dstMap, srcMap)
		} else {
			out[k] = deepCopyValue(srcVal)
		}
	}
	return out
}

// DeepCopy returns a deep copy of a map. Nested maps and slices are copied
// recursively. Returns nil if the input is nil.
func DeepCopy(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

// deepCopyValue copies a single value. Maps and slices are deep-copied;
// scalar values are returned as-is.
func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return DeepCopy(val)
	case []any:
		cp := make([]any, len(val))
		for i, elem := range val {
			cp[i] = deepCopyValue(elem)
		}
		return cp
	default:
		return v
	}
}
