package csv

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/config"
)

// okapiCSVTransformer converts OkfCommaseparatedvaluesFilterConfig specs
// (from Okapi's okf_commaseparatedvalues / okf_table filter parameters)
// to the native CSV format config.
//
// Okapi table filter parameters include:
//   - fieldDelimiter (string) -> separator
//   - columnNamesLineNum (int) -> columnNamesRow
//   - valuesStartLineNum (int) -> valuesStartRow
//   - sendColumnsV (bool) -> hasHeader (inverted logic)
//   - sourceColumns (comma-separated 1-based) -> translatableColumns (0-based)
//   - sourceIdColumns (comma-separated 1-based) -> keyColumns (0-based)
//   - commentColumns (comma-separated 1-based) -> commentColumns (0-based)
//   - trimMode (string) -> trimValues
//
// Parameters specific to the Java bridge with no native equivalent are dropped:
//   - textQualifier, removeQualifiers, addQualifiers, escapingMode
//   - targetColumns, targetLanguages, parametersClass
//   - useCodeFinder, codeFinderRules, sendColumnsV
type okapiCSVTransformer struct{}

func (okapiCSVTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, val := range spec {
		switch key {
		// Direct mappings
		case "fieldDelimiter":
			s, ok := val.(string)
			if !ok {
				continue
			}
			// Okapi uses escape sequences for special delimiters
			switch s {
			case "\\t":
				result["separator"] = "\t"
			case "\\s":
				result["separator"] = " "
			default:
				if len(s) > 0 {
					result["separator"] = string([]rune(s)[0:1])
				}
			}

		case "columnNamesLineNum":
			if n, err := toInt(val); err == nil {
				result["columnNamesRow"] = n
				if n > 0 {
					result["hasHeader"] = true
				}
			}

		case "valuesStartLineNum":
			if n, err := toInt(val); err == nil {
				result["valuesStartRow"] = n
			}

		case "sourceColumns":
			cols, err := parseOkapiColumns(val)
			if err == nil && len(cols) > 0 {
				result["translatableColumns"] = intsToAny(cols)
			}

		case "sourceIdColumns":
			cols, err := parseOkapiColumns(val)
			if err == nil && len(cols) > 0 {
				result["keyColumns"] = intsToAny(cols)
			}

		case "commentColumns":
			cols, err := parseOkapiColumns(val)
			if err == nil && len(cols) > 0 {
				result["commentColumns"] = intsToAny(cols)
			}

		case "trimMode":
			s, ok := val.(string)
			if ok && s != "NONE" && s != "" {
				result["trimValues"] = true
			}

		// Okapi-only params — drop silently
		case "textQualifier", "removeQualifiers", "addQualifiers", "escapingMode",
			"targetColumns", "targetLanguages", "parametersClass",
			"useCodeFinder", "codeFinderRules", "sendColumnsV",
			"recordIdColumns", "bom", "subfilter":
			continue

		default:
			// Drop unknown params silently for forward compatibility
			continue
		}
	}

	return result, nil
}

// parseOkapiColumns parses Okapi's 1-based column specification (comma-separated
// string or number) into 0-based column indices.
func parseOkapiColumns(val any) ([]int, error) {
	switch v := val.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		parts := strings.Split(v, ",")
		var cols []int
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			n := 0
			_, err := fmt.Sscanf(p, "%d", &n)
			if err != nil {
				return nil, fmt.Errorf("invalid column number: %q", p)
			}
			cols = append(cols, n-1) // convert 1-based to 0-based
		}
		return cols, nil
	case float64:
		return []int{int(v) - 1}, nil
	case int:
		return []int{v - 1}, nil
	default:
		return nil, fmt.Errorf("expected string or number, got %T", val)
	}
}

func toInt(val any) (int, error) {
	switch v := val.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("expected number, got %T", val)
	}
}

func intsToAny(ints []int) []any {
	result := make([]any, len(ints))
	for i, n := range ints {
		result[i] = n
	}
	return result
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("commaseparatedvalues"), config.FormatConfigKind("csv"),
		okapiCSVTransformer{},
	)
	// Also register for "table" since Okapi uses okf_table as the meta-filter
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("table"), config.FormatConfigKind("csv"),
		okapiCSVTransformer{},
	)
}
