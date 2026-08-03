package csv

import (
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// How a CSV block is named — its structural address in the table.
//
// A row has no position in a tree, so the only address a CSV can offer is the
// one the file itself carries: the value of its key column. A row NUMBER is not
// an address. Insert a row at the top and every name below it shifts, which is
// precisely the signal core/reconcile grades as "this is a different block".
//
// Three cases, in order of preference:
//
//   - The recipe names key columns (`columns.key` / `keyColumns`). Their values
//     joined with "." address the row, and `<key>/<column>` addresses the cell.
//     A cell's column is named by its header, which is structure the author
//     controls; without a header row it falls back to `colN`, the position the
//     format itself is defined by.
//   - No key column is configured, but the header row labels one — `id`, `key`,
//     `product_id` — and that column's values are unique and non-empty across
//     the data rows. Labelling a column `id` is the author saying it identifies
//     a row, so it is used the same way. Inference decides the NAME only: unlike
//     a configured key column, an inferred one is still extracted as content and
//     still forms no part of any block ID.
//   - Neither. Then the file genuinely has no row address and the name falls
//     back to `<column>/row<N>`: positional, and honest about being positional.
//     core/reconcile compensates, matching on content when context has shifted.
//
// Only Block.Name is derived this way. Block.ID is the skeleton join the writer
// matches on and is left exactly as it was.

// keyHeaderBases are the header labels that declare a column to be a row's
// identifier rather than its content. A header qualifies when it equals one of
// these (case-insensitively) or ends with one as its final `_`/`-`/`.`/space
// separated word — `product_id`, `message key`, `Ref`.
//
// The list is deliberately short and excludes labels that are as often content
// as identity (`name`, `term`, `title`): a wrong guess here does not fail
// loudly, it quietly addresses rows by their own text, which is the
// content-derived naming core/model/structural.go rules out.
var keyHeaderBases = []string{"id", "key", "identifier", "slug", "uid", "uuid", "msgid", "code", "ref"}

// isKeyHeader reports whether a header label declares its column to be a key.
func isKeyHeader(header string) bool {
	h := strings.ToLower(strings.TrimSpace(header))
	if h == "" {
		return false
	}
	for _, base := range keyHeaderBases {
		if h == base {
			return true
		}
		if strings.HasSuffix(h, base) {
			switch h[len(h)-len(base)-1] {
			case '_', '-', '.', ' ':
				return true
			}
		}
	}
	return false
}

// nameKeyColumns returns the columns that address a row for naming purposes:
// the configured key columns when the recipe names them, otherwise an inferred
// one, otherwise nil (no row address — names fall back to the row ordinal).
func (r *Reader) nameKeyColumns(headers []string, records [][]string, startRow int) []int {
	if len(r.cfg.KeyColumns) > 0 {
		return r.cfg.KeyColumns
	}
	if col := r.inferKeyColumn(headers, records, startRow); col >= 0 {
		return []int{col}
	}
	return nil
}

// inferKeyColumn returns the leftmost column whose header labels it a key and
// whose values are unique and non-empty over every data row, or -1 when the
// file offers none. Columns the recipe has claimed as content — translatable,
// comment, or a bilingual source/target column — are never inferred as keys.
func (r *Reader) inferKeyColumn(headers []string, records [][]string, startRow int) int {
	if len(headers) == 0 || startRow >= len(records) {
		return -1
	}
	for colIdx, header := range headers {
		if !isKeyHeader(header) {
			continue
		}
		if len(r.cfg.TranslatableColumns) > 0 && slices.Contains(r.cfg.TranslatableColumns, colIdx) {
			continue
		}
		if r.isCommentColumn(colIdx) {
			continue
		}
		if r.cfg.BilingualMode() && (colIdx == r.cfg.SourceColumn || colIdx == r.cfg.TargetColumn) {
			continue
		}
		if columnIsUnique(records, startRow, colIdx) {
			return colIdx
		}
	}
	return -1
}

// columnIsUnique reports whether every data row carries a distinct, non-empty
// value in colIdx. A column with a repeat or a hole does not identify a row, so
// it is not a key however it is labelled.
func columnIsUnique(records [][]string, startRow, colIdx int) bool {
	seen := make(map[string]bool, len(records)-startRow)
	for rowIdx := startRow; rowIdx < len(records); rowIdx++ {
		row := records[rowIdx]
		if colIdx >= len(row) {
			return false
		}
		v := strings.TrimSpace(row[colIdx])
		if v == "" || seen[v] {
			return false
		}
		seen[v] = true
	}
	return len(seen) > 0
}

// rowAddress joins one row's key-column values into its address, or "" when the
// table has no key columns.
func rowAddress(row []string, keyCols []int) string {
	if len(keyCols) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keyCols))
	for _, kc := range keyCols {
		if kc < len(row) {
			parts = append(parts, strings.TrimSpace(row[kc]))
		}
	}
	return strings.Trim(strings.Join(parts, "."), ".")
}

// cellName names one cell: `<row address>/<column>` when the row has an
// address, `<column>/row<N>` when it does not.
func (r *Reader) cellName(rowAddr, colName string, rowNum int) string {
	if rowAddr != "" {
		return r.names.Name(model.StructuralPath(rowAddr, colName))
	}
	return r.names.Name(model.StructuralPath(colName, "row"+strconv.Itoa(rowNum)))
}

// rowName names a whole row, for bilingual tables where the row IS the unit.
func (r *Reader) rowName(rowAddr string, rowNum int) string {
	if rowAddr != "" {
		return r.names.Name(model.StructuralPath(rowAddr))
	}
	return r.names.Name("row" + strconv.Itoa(rowNum))
}
