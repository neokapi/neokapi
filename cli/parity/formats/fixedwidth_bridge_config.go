//go:build parity

package formats

import (
	"fmt"
	"strconv"
	"strings"
)

// fixedwidthBridgeConfig translates a neokapi-keyed fixedwidth spec
// config map into the parameter shape the okapi-bridge daemon's
// FixedWidthColumnsFilter (okf_fixedwidthcolumns) expects.
//
// The two parameter surfaces differ structurally, not just in
// naming:
//
//   - Native takes `columns: [{name, start, width, translatable}]`
//     (a typed list of objects, 0-based offsets, per-column
//     translatability flag).
//   - Bridge takes `columnStartPositions` and `columnEndPositions`
//     as 1-based string-encoded position lists (start is 1-based
//     inclusive, end is 1-based exclusive — the column reaches up
//     to but not including end), `numColumns` as a count, and
//     `sourceColumns` as a 1-based comma-separated string of indices
//     that should be extracted as Blocks.
//
// We split the native list into the four bridge parameters per the
// neokapi-canonical list shape. `translatable: false` columns are
// excluded from `sourceColumns` and the bridge is set to
// `sendColumnsMode=1` ("listed columns only", aka SEND_COLUMNS_LISTED)
// so non-translatable columns are not extracted as Blocks.
//
// `hasHeader: true` is mapped to the bridge's three-way header
// scheme: `columnNamesLineNum=1` (header on row 1) +
// `valuesStartLineNum=2` (data on row 2) + `sendHeaderMode=0`
// (SEND_HEADER_NONE — header is Data, not a Block). `hasHeader:
// false` keeps `columnNamesLineNum=0` (no header) +
// `valuesStartLineNum=1` (data starts immediately).
//
// `trimValues: false` (the neokapi default) is forced onto the
// bridge as `trimMode=0` (TRIM_NONE) + `trimLeading=false` +
// `trimTrailing=false` to converge with native's "preserve cell
// padding verbatim" default; the bridge default (`trimMode=2`,
// `trimLeading=true`, `trimTrailing=true`) would otherwise strip
// pad whitespace.
//
// IMPORTANT: the upstream Okapi `FixedWidthColumnsFilter` reads
// `columnStartPositions` and `columnEndPositions` from its
// `Parameters` object's String fields, which are only populated
// when `params.fromString(...)` is called. The bridge's per-key
// `params.setString("columnStartPositions", "1,6")` path writes
// the value into the StringParameters buffer but does NOT trigger
// a re-load that syncs the field. To sidestep this Okapi quirk,
// the translator emits the entire configuration as a single
// `fprmContent` blob (the bridge then calls
// `filterParameters.fromString(fprmContent)` once, which DOES
// sync every field). All convergence forces collapse into one
// fprm payload that the upstream filter recognises.
//
// Spec examples that depend on default behaviour MUST set explicit
// `config:`; the translator does not synthesise convergence forces
// outside of the trim and header defaults documented above.
//
// The translator never mutates its input; it returns a fresh map.
func fixedwidthBridgeConfig(cfg map[string]any) (map[string]any, error) {
	// Defaults that mirror neokapi semantics: no header, no trim,
	// explicit per-column extraction.
	p := fwcParams{
		columnNamesLineNum: 0, // no header
		valuesStartLineNum: 1, // data starts row 1
		sendHeaderMode:     0, // SEND_HEADER_NONE
		detectColumnsMode:  0, // DETECT_COLUMNS_NONE — use explicit positions
		sendColumnsMode:    1, // SEND_COLUMNS_LISTED — only sourceColumns
		trimMode:           0, // TRIM_NONE
		trimLeading:        false,
		trimTrailing:       false,
		preserveWS:         true,
		useCodeFinder:      false,
		unescapeSource:     true,
	}

	for key, val := range cfg {
		switch key {
		case "columns":
			cols, err := parseSpecColumns(val)
			if err != nil {
				return nil, fmt.Errorf("fixedwidthBridgeConfig: columns: %w", err)
			}
			starts, ends, sources := splitColumns(cols)
			p.columnStartPositions = starts
			p.columnEndPositions = ends
			p.numColumns = len(cols)
			p.sourceColumns = sources

		case "hasHeader":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("fixedwidthBridgeConfig: hasHeader: expected bool, got %T", val)
			}
			if b {
				p.columnNamesLineNum = 1
				p.valuesStartLineNum = 2
			} else {
				p.columnNamesLineNum = 0
				p.valuesStartLineNum = 1
			}

		case "trimValues":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("fixedwidthBridgeConfig: trimValues: expected bool, got %T", val)
			}
			if b {
				p.trimMode = 1 // TRIM_NONQUALIFIED_ONLY
				p.trimLeading = true
				p.trimTrailing = true
			} else {
				p.trimMode = 0
				p.trimLeading = false
				p.trimTrailing = false
			}

		default:
			return nil, fmt.Errorf("fixedwidthBridgeConfig: unknown spec key %q", key)
		}
	}

	return map[string]any{"fprmContent": p.toFprm()}, nil
}

// fwcParams is the in-translator view of the bridge filter's
// Parameters object. Mirrors the field names used by
// `net.sf.okapi.filters.table.fwc.Parameters`.
type fwcParams struct {
	columnNamesLineNum   int
	valuesStartLineNum   int
	sendHeaderMode       int
	detectColumnsMode    int
	numColumns           int
	sendColumnsMode      int
	trimMode             int
	trimLeading          bool
	trimTrailing         bool
	preserveWS           bool
	useCodeFinder        bool
	unescapeSource       bool
	columnStartPositions string
	columnEndPositions   string
	sourceColumns        string
}

// toFprm renders the params as an Okapi fprm v1 string. The order
// roughly mirrors the upstream okf_table_fwc.fprm preset for
// readability.
func (p fwcParams) toFprm() string {
	var b strings.Builder
	b.WriteString("#v1\n")
	fmt.Fprintf(&b, "unescapeSource.b=%t\n", p.unescapeSource)
	fmt.Fprintf(&b, "trimLeading.b=%t\n", p.trimLeading)
	fmt.Fprintf(&b, "trimTrailing.b=%t\n", p.trimTrailing)
	fmt.Fprintf(&b, "preserveWS.b=%t\n", p.preserveWS)
	fmt.Fprintf(&b, "useCodeFinder.b=%t\n", p.useCodeFinder)
	fmt.Fprintf(&b, "columnNamesLineNum.i=%d\n", p.columnNamesLineNum)
	fmt.Fprintf(&b, "valuesStartLineNum.i=%d\n", p.valuesStartLineNum)
	fmt.Fprintf(&b, "detectColumnsMode.i=%d\n", p.detectColumnsMode)
	fmt.Fprintf(&b, "numColumns.i=%d\n", p.numColumns)
	fmt.Fprintf(&b, "sendHeaderMode.i=%d\n", p.sendHeaderMode)
	fmt.Fprintf(&b, "trimMode.i=%d\n", p.trimMode)
	fmt.Fprintf(&b, "sendColumnsMode.i=%d\n", p.sendColumnsMode)
	fmt.Fprintf(&b, "sourceColumns=%s\n", p.sourceColumns)
	fmt.Fprintf(&b, "columnStartPositions=%s\n", p.columnStartPositions)
	fmt.Fprintf(&b, "columnEndPositions=%s\n", p.columnEndPositions)
	b.WriteString("parametersClass=net.sf.okapi.filters.table.fwc.Parameters\n")
	return b.String()
}

// fwcColumn is the in-translator view of one column entry from the
// spec's `columns:` list.
type fwcColumn struct {
	name         string
	start        int  // 0-based rune offset (native shape)
	width        int  // column width in runes
	translatable bool // true → contributes to sourceColumns
}

// parseSpecColumns converts the YAML `columns:` value into a typed
// slice. The YAML decoder hands us either []any (each element a
// map[string]any) when the list is inline or []map[string]any when
// the list is anchor-merged; we accept both shapes.
func parseSpecColumns(v any) ([]fwcColumn, error) {
	var raw []any
	switch x := v.(type) {
	case []any:
		raw = x
	case []map[string]any:
		raw = make([]any, len(x))
		for i, m := range x {
			raw[i] = m
		}
	default:
		return nil, fmt.Errorf("expected list, got %T", v)
	}
	out := make([]fwcColumn, 0, len(raw))
	for i, elem := range raw {
		m, ok := elem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("column %d: expected map, got %T", i, elem)
		}
		var col fwcColumn
		if name, ok := m["name"].(string); ok {
			col.name = name
		} else {
			return nil, fmt.Errorf("column %d: name is required", i)
		}
		start, err := asColInt(m["start"], "start")
		if err != nil {
			return nil, fmt.Errorf("column %d: %w", i, err)
		}
		col.start = start
		width, err := asColInt(m["width"], "width")
		if err != nil {
			return nil, fmt.Errorf("column %d: %w", i, err)
		}
		col.width = width
		if t, ok := m["translatable"].(bool); ok {
			col.translatable = t
		}
		out = append(out, col)
	}
	return out, nil
}

// splitColumns turns a list of columns into the bridge's three flat
// strings: 1-based column start positions (inclusive), 1-based
// exclusive column end positions ("first position past the column"),
// and a 1-based comma-separated list of indices for columns marked
// translatable.
//
// Bridge convention (verified empirically against
// FixedWidthColumnsFilter): `substring(start_pos - 1, end_pos - 1)`,
// so a 0-based start `s` and width `w` map to start_pos = s+1 and
// end_pos = s+w+1.
func splitColumns(cols []fwcColumn) (starts, ends, sources string) {
	startParts := make([]string, len(cols))
	endParts := make([]string, len(cols))
	var sourceParts []string
	for i, c := range cols {
		startParts[i] = strconv.Itoa(c.start + 1)
		endParts[i] = strconv.Itoa(c.start + c.width + 1)
		if c.translatable {
			sourceParts = append(sourceParts, strconv.Itoa(i+1))
		}
	}
	starts = strings.Join(startParts, ",")
	ends = strings.Join(endParts, ",")
	sources = strings.Join(sourceParts, ",")
	return starts, ends, sources
}

// asColInt accepts the YAML decoder's int / int64 / float64 shapes.
func asColInt(v any, label string) (int, error) {
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
