package projector

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/kpz"
)

// KindCheckpoint records a checkpoint: the project's projections as of an
// operation, kept as a .kpz checkpoint (kpz.KindCheckpoint) in a blob the
// operation names. A rebuild loads the latest checkpoint that still stands and
// replays only what came after it.
const KindCheckpoint = "checkpoint.write"

// rulesTable is the member a checkpoint keeps the project's widened rules in.
// They live in the workspace database rather than the project's, so they
// travel beside the tables rather than as one of them.
const rulesTable = "workspace_rules"

// sequenceTable is the member a checkpoint keeps the numbering of its
// AUTOINCREMENT tables in, so a replay after it numbers rows as the writes did.
const sequenceTable = "sqlite_sequence"

// A checkpoint table whose rows run past inlineTableBytes is kept in parts of
// at most partBytes each (kpz.CheckpointPart), blobs of their own beside the
// checkpoint file. The checkpoint file then holds at most inlineTableBytes per
// table, and the block history, which grows with every pass, never makes one
// file larger than a blob may be (workspace.MaxBlobSize). A table that only
// grows is cut at the same rows each time, so successive checkpoints share its
// earlier parts and the workspace stores each once.
var (
	inlineTableBytes = 1 << 20
	partBytes        = 16 << 20
)

// checkpointPayload is what a checkpoint operation carries.
type checkpointPayload struct {
	Blob string `json:"blob"`
	// Blobs names the parts of the checkpoint's tables (kpz.CheckpointPart).
	Blobs   []string `json:"blobs,omitempty"`
	Through string   `json:"through"`
	// Seq is the local position the checkpoint was taken at. An operation
	// this log received later with an id before Through was merged in after
	// the checkpoint, which the checkpoint does not include, so it no longer
	// stands.
	Seq        int64 `json:"seq"`
	Operations int   `json:"operations"`
}

// CheckpointReport says what a checkpoint holds.
type CheckpointReport struct {
	// Through is the last operation the checkpoint includes.
	Through string `json:"through"`
	// Operations counts the operations it includes.
	Operations int `json:"operations"`
	// Bytes is the size of the checkpoint file.
	Bytes int `json:"bytes"`
	// Parts counts the blobs holding the rows of tables too large for the
	// checkpoint file.
	Parts int `json:"parts,omitempty"`
}

// storeParts moves the rows of every table too large to carry in a
// checkpoint file into parts, stored as blobs, and returns the tables left to
// carry and the parts in the order they load.
func (p *Projector) storeParts(ctx context.Context, tables []kpz.TableDoc) ([]kpz.TableDoc, []kpz.CheckpointPart, error) {
	var (
		inline []kpz.TableDoc
		parts  []kpz.CheckpointPart
	)
	for _, t := range tables {
		if len(t.Data) <= inlineTableBytes {
			inline = append(inline, t)
			continue
		}
		for _, chunk := range cutLines(t.Data, partBytes) {
			address, err := p.log.PutBlob(ctx, chunk)
			if err != nil {
				return nil, nil, fmt.Errorf("projector: store part of %s: %w", t.Table, err)
			}
			parts = append(parts, kpz.CheckpointPart{Table: t.Table, Blob: address})
		}
	}
	return inline, parts, nil
}

// cutLines cuts JSON Lines into runs of whole lines of at most limit bytes
// each. A line longer than limit is a run of its own.
func cutLines(data []byte, limit int) [][]byte {
	var out [][]byte
	for len(data) > 0 {
		end := len(data)
		if end > limit {
			end = bytes.LastIndexByte(data[:limit], '\n') + 1
			if end == 0 {
				end = bytes.IndexByte(data, '\n') + 1
				if end == 0 {
					end = len(data)
				}
			}
		}
		out = append(out, data[:end])
		data = data[end:]
	}
	return out
}

// partBlobs lists the blobs of a checkpoint's parts.
func partBlobs(parts []kpz.CheckpointPart) []string {
	var out []string
	for _, part := range parts {
		if !slices.Contains(out, part.Blob) {
			out = append(out, part.Blob)
		}
	}
	return out
}

// Checkpoint writes the project's projections, as of every operation the log
// holds for it, into a checkpoint and records an operation naming it.
func (p *Projector) Checkpoint(ctx context.Context) (CheckpointReport, error) {
	var report CheckpointReport
	if p.log == nil {
		return report, errors.New("projector: this store has no log to checkpoint")
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	if err := p.catchUpLocked(ctx, nil); err != nil {
		return report, err
	}
	ops, err := p.log.Select(ctx, workspace.OpQuery{Project: p.key})
	if err != nil {
		return report, err
	}
	var seq int64
	for _, op := range ops {
		seq = max(seq, op.Seq)
		if projects(op.Kind) {
			report.Operations++
			report.Through = max(report.Through, op.ID)
		}
	}
	if report.Operations == 0 {
		return report, errors.New("projector: the log holds nothing to checkpoint")
	}

	tables, err := p.dumpTables(ctx)
	if err != nil {
		return report, err
	}
	tables, parts, err := p.storeParts(ctx, tables)
	if err != nil {
		return report, err
	}
	pkg := &kpz.Package{
		Kind:    kpz.KindCheckpoint,
		Created: time.Now().UTC().Format(time.RFC3339),
		Tables:  tables,
		Checkpoint: &kpz.CheckpointMark{
			Project: string(p.key), Through: report.Through, Operations: report.Operations, Parts: parts,
		},
	}
	data, err := pkg.Marshal()
	if err != nil {
		return report, fmt.Errorf("projector: write checkpoint: %w", err)
	}
	report.Bytes = len(data)
	report.Parts = len(parts)
	address, err := p.log.PutBlob(ctx, data)
	if err != nil {
		return report, fmt.Errorf("projector: store checkpoint: %w", err)
	}
	body, err := json.Marshal(checkpointPayload{
		Blob: address, Blobs: partBlobs(parts), Through: report.Through, Seq: seq, Operations: report.Operations,
	})
	if err != nil {
		return report, err
	}
	if _, err := p.log.Record(ctx, workspace.Op{Project: p.key, Kind: KindCheckpoint, Payload: body}); err != nil {
		return report, err
	}
	return report, nil
}

// latestCheckpoint finds the newest checkpoint that still stands: one no
// operation merged in after it precedes. ok is false when there is none.
func (p *Projector) latestCheckpoint(ctx context.Context, ops []workspace.Op) (checkpointPayload, *kpz.Package, bool) {
	for _, op := range slices.Backward(ops) {
		if op.Kind != KindCheckpoint {
			continue
		}
		var cp checkpointPayload
		if json.Unmarshal(op.Payload, &cp) != nil || cp.Blob == "" {
			continue
		}
		// A reset received after the checkpoint changes what the log before
		// it applies, so the stores it holds no longer follow from the log.
		stands := true
		for _, later := range ops {
			if later.Seq > cp.Seq && (later.Kind == workspace.OpContextReset || (projects(later.Kind) && later.ID <= cp.Through)) {
				stands = false
				break
			}
		}
		if !stands {
			continue
		}
		data, err := p.log.Blob(ctx, cp.Blob)
		if err != nil {
			continue
		}
		pkg, err := kpz.Unmarshal(data)
		if err != nil || pkg.Kind != kpz.KindCheckpoint || pkg.Checkpoint == nil ||
			pkg.Checkpoint.Project != string(p.key) {
			continue
		}
		return cp, pkg, true
	}
	return checkpointPayload{}, nil, false
}

// dumpTables reads every projection table, the numbering of the ones that
// count their rows, and the project's widened rules.
func (p *Projector) dumpTables(ctx context.Context) ([]kpz.TableDoc, error) {
	names, _, err := p.projectionTables(ctx)
	if err != nil {
		return nil, err
	}
	var out []kpz.TableDoc
	for _, name := range names {
		keep, err := rowidKept(ctx, p, name)
		if err != nil {
			return nil, err
		}
		query := `SELECT * FROM "` + name + `" ORDER BY rowid`
		if keep {
			query = `SELECT rowid AS "rowid", * FROM "` + name + `" ORDER BY rowid`
		}
		data, err := dumpRows(ctx, p, query)
		if err != nil {
			return nil, fmt.Errorf("projector: read %s: %w", name, err)
		}
		out = append(out, kpz.TableDoc{Table: name, Data: data})
	}
	if seqs, err := dumpRows(ctx, p, `SELECT name, seq FROM sqlite_sequence WHERE name IN (`+placeholders(len(names))+`)`, toAny(names)...); err == nil {
		out = append(out, kpz.TableDoc{Table: sequenceTable, Data: seqs})
	}
	rules, err := p.log.WidenedRules(ctx, "")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	for _, r := range rules {
		if r.Origin != p.key {
			continue
		}
		line, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return append(out, kpz.TableDoc{Table: rulesTable, Data: buf.Bytes()}), nil
}

// loadTables writes a checkpoint's tables into emptied projections.
func (p *Projector) loadTables(ctx context.Context, pkg *kpz.Package) error {
	raw := p.st.Raw
	var sequences []byte
	for _, t := range pkg.Tables {
		switch t.Table {
		case rulesTable:
			for line := range bytes.SplitSeq(bytes.TrimSpace(t.Data), []byte("\n")) {
				if len(line) == 0 {
					continue
				}
				var r workspace.Rule
				if err := json.Unmarshal(line, &r); err != nil {
					return fmt.Errorf("projector: read checkpoint rules: %w", err)
				}
				if err := p.log.WidenRule(ctx, r); err != nil {
					return err
				}
			}
			continue
		case sequenceTable:
			sequences = t.Data
			continue
		}
		if !projectionTable(t.Table) {
			return fmt.Errorf("projector: a checkpoint names %q, which is not a projection table", t.Table)
		}
		if err := loadRows(ctx, raw, t.Table, t.Data); err != nil {
			return fmt.Errorf("projector: load %s: %w", t.Table, err)
		}
	}
	for _, part := range pkg.Checkpoint.Parts {
		if !projectionTable(part.Table) {
			return fmt.Errorf("projector: a checkpoint names %q, which is not a projection table", part.Table)
		}
		data, err := p.log.Blob(ctx, part.Blob)
		if err != nil {
			return fmt.Errorf("projector: read part of %s: %w", part.Table, err)
		}
		if err := loadRows(ctx, raw, part.Table, data); err != nil {
			return fmt.Errorf("projector: load %s: %w", part.Table, err)
		}
	}
	if len(sequences) > 0 {
		for line := range bytes.SplitSeq(bytes.TrimSpace(sequences), []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var row map[string]json.RawMessage
			if err := json.Unmarshal(line, &row); err != nil {
				return err
			}
			name, err := cellValue(row["name"])
			if err != nil {
				return err
			}
			seq, err := cellValue(row["seq"])
			if err != nil {
				return err
			}
			if _, err := raw.ExecContext(ctx, `INSERT INTO sqlite_sequence (name, seq) VALUES (?, ?)`, name, seq); err != nil {
				return fmt.Errorf("projector: restore the numbering of %v: %w", name, err)
			}
		}
	}
	return nil
}

// rowidKept reports whether a table's rows are dumped with their rowid, which
// every table needs unless an INTEGER PRIMARY KEY column already carries it.
func rowidKept(ctx context.Context, p *Projector, table string) (bool, error) {
	rows, err := p.st.Raw.QueryContext(ctx, `SELECT type, pk FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	pks, integer := 0, false
	for rows.Next() {
		var typ string
		var pk int
		if err := rows.Scan(&typ, &pk); err != nil {
			return false, err
		}
		if pk > 0 {
			pks++
			integer = strings.EqualFold(typ, "INTEGER")
		}
	}
	return !(pks == 1 && integer), rows.Err()
}

// dumpRows renders a query's rows as JSON Lines, one object per row, each cell
// tagged with its SQLite type so it loads back as the same value.
func dumpRows(ctx context.Context, p *Projector, query string, args ...any) ([]byte, error) {
	rows, err := p.st.Raw.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = cell(vals[i])
		}
		line, err := json.Marshal(row)
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), rows.Err()
}

// cell tags a value with its type: {"i": 1}, {"f": 1.5}, {"s": "x"},
// {"b": "<base64>"}, or null.
func cell(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case int64:
		return map[string]int64{"i": x}
	case float64:
		return map[string]float64{"f": x}
	case string:
		return map[string]string{"s": x}
	case []byte:
		return map[string]string{"b": base64.StdEncoding.EncodeToString(x)}
	case bool:
		if x {
			return map[string]int64{"i": 1}
		}
		return map[string]int64{"i": 0}
	case time.Time:
		return map[string]string{"s": x.Format(time.RFC3339Nano)}
	}
	return map[string]string{"s": fmt.Sprint(v)}
}

// cellValue reads a tagged cell back.
func cellValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var tagged struct {
		I *int64   `json:"i"`
		F *float64 `json:"f"`
		S *string  `json:"s"`
		B *string  `json:"b"`
	}
	if err := json.Unmarshal(raw, &tagged); err != nil {
		return nil, err
	}
	switch {
	case tagged.I != nil:
		return *tagged.I, nil
	case tagged.F != nil:
		return *tagged.F, nil
	case tagged.S != nil:
		return *tagged.S, nil
	case tagged.B != nil:
		return base64.StdEncoding.DecodeString(*tagged.B)
	}
	return nil, nil
}

// loadRows inserts a table's dumped rows, in one transaction. Rows naming the
// same columns, which in a dump is every row, share one prepared statement.
func loadRows(ctx context.Context, raw *storage.DB, table string, data []byte) error {
	tx, err := raw.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmts := map[string]*sql.Stmt{}
	defer func() {
		for _, s := range stmts {
			_ = s.Close()
		}
	}()
	for line := range bytes.SplitSeq(bytes.TrimSpace(data), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var row map[string]json.RawMessage
		if err := json.Unmarshal(line, &row); err != nil {
			return err
		}
		cols := make([]string, 0, len(row))
		for c := range row {
			cols = append(cols, c)
		}
		sort.Strings(cols)
		args := make([]any, len(cols))
		quoted := make([]string, len(cols))
		for i, c := range cols {
			v, err := cellValue(row[c])
			if err != nil {
				return err
			}
			args[i] = v
			quoted[i] = `"` + strings.ReplaceAll(c, `"`, `""`) + `"`
		}
		names := strings.Join(quoted, ", ")
		stmt, ok := stmts[names]
		if !ok {
			if stmt, err = tx.PrepareContext(ctx, `INSERT INTO "`+table+`" (`+names+`) VALUES (`+placeholders(len(cols))+`)`); err != nil {
				return err
			}
			stmts[names] = stmt
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// placeholders renders n SQL parameters.
func placeholders(n int) string {
	if n == 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func toAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
