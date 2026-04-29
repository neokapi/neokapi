//go:build parity

package parity

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// StringifyParams converts a typed parameter map to the
// string-keyed/string-valued form the bridge's gRPC ProcessHeader
// accepts. It is the only place neokapi's typed-config view crosses
// over into the bridge's flat-string view; native filters keep using
// the typed map directly via DataFormatConfig.ApplyMap.
//
// Conversions:
//
//	bool        → "true" | "false"
//	int / *int  → decimal
//	float       → "%g" (no trailing zeros)
//	string      → as-is
//	nil         → omitted (falls back to bridge defaults)
//	other       → JSON-encoded (covers []string for codeFinderRules etc.)
//
// A nil or empty input returns nil so the harness can pass it through
// unchanged for specs that don't override defaults.
func StringifyParams(in map[string]any) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		s, ok := stringifyParamValue(v)
		if !ok {
			continue
		}
		out[k] = s
	}
	return out
}

func stringifyParamValue(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", false
	case bool:
		return strconv.FormatBool(x), true
	case string:
		return x, true
	case int:
		return strconv.Itoa(x), true
	case int32:
		return strconv.FormatInt(int64(x), 10), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32), true
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), true
	default:
		// Lists, nested objects: hand off to JSON. The bridge knows
		// how to interpret JSON-shaped values for slots like
		// codeFinderRules (see ParameterApplier.applyParameters).
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v), true
		}
		return string(b), true
	}
}
