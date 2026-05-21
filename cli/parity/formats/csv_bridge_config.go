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
// The upstream okf_table family (csv/tsv/fwc) reads its behaviour from
// the public fields of its Okapi `Parameters` object
// (`net.sf.okapi.filters.table.csv.Parameters` extending
// `…table.base.Parameters`). Those fields are populated ONLY by
// `Parameters.load(buffer)`, which runs inside `fromString(...)`. The
// bridge's per-key `params.setString("sourceColumns", "1")` path writes
// the StringParameters buffer but never re-runs `load()`, so the public
// field stays stale and the filter ignores the override (see #530 and
// the identical quirk documented in fixedwidth_bridge_config.go).
//
// To sidestep this, the translator renders the ENTIRE configuration as
// a single Okapi `#v1` ParametersString blob and ships it under the
// reserved `fprmContent` key. The bridge calls
// `filterParameters.fromString(fprmContent)` once, which clears the
// buffer and re-runs `load()` — syncing every field in one shot. All
// per-feature convergence forces collapse into that single payload.
//
// The blob always carries the neokapi-canonical CSV defaults (comma
// delimiter, double-quote qualifier, header on row 1 + data on row 2 +
// header not extracted, every column translatable, no trim); the spec
// config overrides individual fields on top. This matches the parity
// contract "same semantic config → same results": examples that depend
// on neokapi defaults MUST set explicit config (they do).
//
// Field/value mappings (verified against
// net.sf.okapi.filters.table.{base,csv}.Parameters):
//
//	separator (string)            → fieldDelimiter (string, verbatim)
//	textQualifier (string)        → textQualifier (string, verbatim)
//	hasHeader=true (bool)         → columnNamesLineNum=1, valuesStartLineNum=2,
//	                                sendHeaderMode=0 (SEND_HEADER_NONE)
//	hasHeader=false               → columnNamesLineNum=0, valuesStartLineNum=1,
//	                                sendHeaderMode=0
//	columnNamesRow (int)          → columnNamesLineNum (1-based, verbatim)
//	valuesStartRow (int)          → valuesStartLineNum (1-based, verbatim)
//	translatableColumns ([]int0)  → sourceColumns ("1,3" 1-based CSV) +
//	                                sendColumnsMode=1 (SEND_COLUMNS_LISTED)
//	keyColumns ([]int0)           → sourceIdColumns ("1,2" 1-based CSV)
//	commentColumns ([]int0)       → commentColumns ("2" 1-based CSV)
//	trimValues=true (bool)        → trimMode=2 (TRIM_ALL) + trimLeading/
//	                                trimTrailing=true
//	trimValues=false              → trimMode=0 (TRIM_NONE)
//
// targetColumns is forced empty: the upstream default ("2") would treat
// the second column as a localised target (multilingual mode) rather
// than translatable source, which neokapi has no notion of.
//
// The translator never mutates its input; it returns a fresh map.
func csvBridgeConfig(cfg map[string]any) (map[string]any, error) {
	// Defaults mirror neokapi's CSV semantics (see core/formats/csv/
	// config.go Reset()): comma delimiter, double-quote qualifier,
	// header present (row 1, data row 2, header not extracted), every
	// column translatable, no trim.
	p := csvParams{
		fieldDelimiter:     ",",
		textQualifier:      "\"",
		removeQualifiers:   true,
		escapingMode:       1, // ESCAPING_MODE_DUPLICATION
		columnNamesLineNum: 1, // header on row 1
		valuesStartLineNum: 2, // data on row 2
		sendHeaderMode:     0, // SEND_HEADER_NONE — header is skeleton, not a Block
		sendColumnsMode:    2, // SEND_COLUMNS_ALL — every cell is source
		trimMode:           0, // TRIM_NONE
		trimLeading:        false,
		trimTrailing:       false,
		preserveWS:         true,
		useCodeFinder:      false,
	}

	for key, val := range cfg {
		switch key {
		case "separator":
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("csvBridgeConfig: separator: expected string, got %T", val)
			}
			p.fieldDelimiter = s

		case "textQualifier":
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("csvBridgeConfig: textQualifier: expected string, got %T", val)
			}
			p.textQualifier = s

		case "hasHeader":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("csvBridgeConfig: hasHeader: expected bool, got %T", val)
			}
			if b {
				p.columnNamesLineNum = 1
				p.valuesStartLineNum = 2
			} else {
				p.columnNamesLineNum = 0
				p.valuesStartLineNum = 1
			}
			p.sendHeaderMode = 0

		case "columnNamesRow":
			n, err := asInt(val, "columnNamesRow")
			if err != nil {
				return nil, err
			}
			p.columnNamesLineNum = n

		case "valuesStartRow":
			n, err := asInt(val, "valuesStartRow")
			if err != nil {
				return nil, err
			}
			p.valuesStartLineNum = n

		case "translatableColumns":
			cols, err := intSliceToOneBasedCSV(val)
			if err != nil {
				return nil, fmt.Errorf("csvBridgeConfig: translatableColumns: %w", err)
			}
			p.sourceColumns = cols
			p.sendColumnsMode = 1 // SEND_COLUMNS_LISTED — only listed columns

		case "keyColumns":
			cols, err := intSliceToOneBasedCSV(val)
			if err != nil {
				return nil, fmt.Errorf("csvBridgeConfig: keyColumns: %w", err)
			}
			p.sourceIDColumns = cols

		case "commentColumns":
			cols, err := intSliceToOneBasedCSV(val)
			if err != nil {
				return nil, fmt.Errorf("csvBridgeConfig: commentColumns: %w", err)
			}
			p.commentColumns = cols

		case "trimValues":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("csvBridgeConfig: trimValues: expected bool, got %T", val)
			}
			if b {
				p.trimMode = 2 // TRIM_ALL — trim qualified + non-qualified
				p.trimLeading = true
				p.trimTrailing = true
			} else {
				p.trimMode = 0
				p.trimLeading = false
				p.trimTrailing = false
			}

		case "useCodeFinder":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("csvBridgeConfig: useCodeFinder: expected bool, got %T", val)
			}
			p.useCodeFinder = b

		default:
			return nil, fmt.Errorf("csvBridgeConfig: unknown spec key %q", key)
		}
	}

	return map[string]any{"fprmContent": p.toFprm()}, nil
}

// csvParams is the in-translator view of the bridge filter's CSV
// Parameters object. Mirrors the public-field names of
// net.sf.okapi.filters.table.csv.Parameters (and its base classes).
type csvParams struct {
	fieldDelimiter     string
	textQualifier      string
	removeQualifiers   bool
	escapingMode       int
	columnNamesLineNum int
	valuesStartLineNum int
	sendHeaderMode     int
	sendColumnsMode    int
	trimMode           int
	trimLeading        bool
	trimTrailing       bool
	preserveWS         bool
	useCodeFinder      bool
	sourceColumns      string
	sourceIDColumns    string
	commentColumns     string
}

// toFprm renders the params as an Okapi ParametersString `#v1` blob.
// targetColumns is forced empty so the upstream filter does not enter
// multilingual (source/target) mode.
func (p csvParams) toFprm() string {
	var b strings.Builder
	b.WriteString("#v1\n")
	// plaintext base + table base booleans / ints.
	fmt.Fprintf(&b, "preserveWS.b=%t\n", p.preserveWS)
	fmt.Fprintf(&b, "useCodeFinder.b=%t\n", p.useCodeFinder)
	fmt.Fprintf(&b, "trimLeading.b=%t\n", p.trimLeading)
	fmt.Fprintf(&b, "trimTrailing.b=%t\n", p.trimTrailing)
	fmt.Fprintf(&b, "columnNamesLineNum.i=%d\n", p.columnNamesLineNum)
	fmt.Fprintf(&b, "valuesStartLineNum.i=%d\n", p.valuesStartLineNum)
	fmt.Fprintf(&b, "sendHeaderMode.i=%d\n", p.sendHeaderMode)
	fmt.Fprintf(&b, "sendColumnsMode.i=%d\n", p.sendColumnsMode)
	fmt.Fprintf(&b, "trimMode.i=%d\n", p.trimMode)
	fmt.Fprintf(&b, "sourceColumns=%s\n", p.sourceColumns)
	fmt.Fprintf(&b, "sourceIdColumns=%s\n", p.sourceIDColumns)
	fmt.Fprintf(&b, "commentColumns=%s\n", p.commentColumns)
	b.WriteString("targetColumns=\n")
	// csv subclass fields.
	fmt.Fprintf(&b, "fieldDelimiter=%s\n", p.fieldDelimiter)
	fmt.Fprintf(&b, "textQualifier=%s\n", p.textQualifier)
	fmt.Fprintf(&b, "removeQualifiers.b=%t\n", p.removeQualifiers)
	fmt.Fprintf(&b, "escapingMode.i=%d\n", p.escapingMode)
	b.WriteString("parametersClass=net.sf.okapi.filters.table.csv.Parameters\n")
	return b.String()
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
