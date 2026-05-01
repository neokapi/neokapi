//go:build parity

package formats

import (
	"fmt"
	"strconv"
	"strings"
)

// csvBridgeConfig translates a neokapi-keyed csv spec config map into
// the parameter shape the okapi-bridge daemon's CommaSeparatedValuesFilter
// (okf_commaseparatedvalues) expects.
//
// One responsibility: rename/transform per-key. e.g. separator →
// fieldDelimiter, hasHeader true → columnNamesLineNum=1 +
// valuesStartLineNum=2 + sendHeaderMode=0, translatableColumns
// ([0,1] 0-based) → sourceColumns ("1,2" 1-based string).
//
// Spec examples that depend on default behaviour MUST set explicit
// config:; the translator does not synthesise convergence forces. This
// matches the parity contract "same semantic config → same results"
// — implicit defaults are permitted to differ between native and
// bridge; explicit config must converge.
//
// The translator never mutates its input; it returns a fresh map.
func csvBridgeConfig(cfg map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(cfg))

	for key, val := range cfg {
		switch key {
		case "separator":
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("csvBridgeConfig: separator: expected string, got %T", val)
			}
			out["fieldDelimiter"] = s

		case "textQualifier":
			out["textQualifier"] = val

		case "hasHeader":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("csvBridgeConfig: hasHeader: expected bool, got %T", val)
			}
			if b {
				// Header present → row 1 is header, data starts row 2,
				// header skipped from extraction (neokapi semantics).
				out["columnNamesLineNum"] = 1
				out["valuesStartLineNum"] = 2
				out["sendHeaderMode"] = 0
			} else {
				// No header → no column names row, data starts row 1.
				out["columnNamesLineNum"] = 0
				out["valuesStartLineNum"] = 1
				out["sendHeaderMode"] = 0
			}

		case "columnNamesRow":
			n, err := asInt(val, "columnNamesRow")
			if err != nil {
				return nil, err
			}
			out["columnNamesLineNum"] = n

		case "valuesStartRow":
			n, err := asInt(val, "valuesStartRow")
			if err != nil {
				return nil, err
			}
			// neokapi spec uses 1-based row number directly; bridge
			// uses 1-based valuesStartLineNum. Pass through.
			out["valuesStartLineNum"] = n

		case "translatableColumns":
			cols, err := intSliceToOneBasedCSV(val)
			if err != nil {
				return nil, fmt.Errorf("csvBridgeConfig: translatableColumns: %w", err)
			}
			out["sourceColumns"] = cols
			out["sendColumnsMode"] = 1 // listed columns only

		case "keyColumns":
			cols, err := intSliceToOneBasedCSV(val)
			if err != nil {
				return nil, fmt.Errorf("csvBridgeConfig: keyColumns: %w", err)
			}
			out["sourceIdColumns"] = cols

		case "commentColumns":
			cols, err := intSliceToOneBasedCSV(val)
			if err != nil {
				return nil, fmt.Errorf("csvBridgeConfig: commentColumns: %w", err)
			}
			out["commentColumns"] = cols

		case "trimValues":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("csvBridgeConfig: trimValues: expected bool, got %T", val)
			}
			if b {
				out["trimMode"] = 1
				out["trimLeading"] = true
				out["trimTrailing"] = true
			} else {
				out["trimMode"] = 0
				out["trimLeading"] = false
				out["trimTrailing"] = false
			}

		case "useCodeFinder", "codeFinderRules":
			out[key] = val // bridge uses the same key names

		default:
			return nil, fmt.Errorf("csvBridgeConfig: unknown spec key %q", key)
		}
	}

	return out, nil
}

// intSliceToOneBasedCSV converts a 0-based int column list (from the
// spec) into a 1-based comma-separated string (Okapi's
// sourceColumns / sourceIdColumns / commentColumns shape).
func intSliceToOneBasedCSV(v any) (string, error) {
	var ints []int
	switch x := v.(type) {
	case []int:
		ints = x
	case []any:
		ints = make([]int, len(x))
		for i, item := range x {
			n, err := asInt(item, "column index")
			if err != nil {
				return "", err
			}
			ints[i] = n
		}
	default:
		return "", fmt.Errorf("expected int slice, got %T", v)
	}
	parts := make([]string, len(ints))
	for i, n := range ints {
		parts[i] = strconv.Itoa(n + 1)
	}
	return strings.Join(parts, ","), nil
}

func asInt(v any, label string) (int, error) {
	switch x := v.(type) {
	case int:
		return x, nil
	case int64:
		return int(x), nil
	case float64:
		return int(x), nil
	default:
		return 0, fmt.Errorf("%s: expected int, got %T", label, v)
	}
}
