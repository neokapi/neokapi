package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// The edition writes a push carries beside its decisions.
//
// A project keeps how each translation came to be in its block history: the
// source a run made it from, and who wrote it. A push sends the last write of
// each translation (venue.EditionWrite), and the venue keeps the facts it
// grades by where it keeps the same facts about its own drafts:
//
//   - the basis, on the unit's ledger row, where nobody has decided the unit:
//     the row a run of the venue's own writes for a draft it made, so a
//     translation a run on the checkout made is graded stale against the
//     source the same way (TallyDecisionBasis). A decision's basis is the
//     decision's, and a write never replaces it.
//   - the draft mark, for a translation a tool made from a recorded source, so
//     the venue's own runs count the unit as drafted against that source.
//   - the author of a translation the pusher wrote by hand: in
//     edition_writers, whether or not the venue holds the translation, which
//     separation of duties reads when an approval of it arrives in a later
//     push (EditionWriters); and on the block history row this push wrote for
//     a translation the venue holds, which the venue's own review surfaces
//     read (LastTargetAuthors).
//
// A write about another translation than the one the venue holds describes
// nothing the venue has, and leaves its basis and draft mark alone.
//
// The venue's view of an item is read once per item for all its writes: the
// blocks, the translations and the ledger rows, each in one query, so a push
// that carries the writes of a whole pass costs a few queries per item rather
// than several per write.

// writeLookupChunk bounds the units one lookup names, well inside the
// statement's parameter limit.
const writeLookupChunk = 1000

// RecordEditionWrites implements platstore.PushApplier.
func (a pushApply) RecordEditionWrites(ctx context.Context, projectID, stream, author string, writes []venue.EditionWrite) (int, error) {
	if len(writes) == 0 {
		return 0, nil
	}
	if err := a.hold(ctx, projectID, stream); err != nil {
		return 0, err
	}
	return recordEditionWritesTx(ctx, a.tx, projectID, stream, author, a.correlation, writes)
}

// recordEditionWritesTx records writes on tx. correlation names the block
// history rows the push wrote, which a translation the pusher wrote by hand is
// attributed on.
func recordEditionWritesTx(ctx context.Context, tx Runner, projectID, stream, author, correlation string, writes []venue.EditionWrite) (int, error) {
	stream = storeutil.DefaultStream(stream)
	byItem := map[string][]venue.EditionWrite{}
	var items []string
	for _, w := range writes {
		if w.ItemName == "" || w.Unit == "" || w.Variant == "" || w.Revision == "" {
			continue
		}
		if key, err := model.ParseEditionKey(w.Variant); err != nil || key.Locale == "" {
			continue
		}
		if _, seen := byItem[w.ItemName]; !seen {
			items = append(items, w.ItemName)
		}
		byItem[w.ItemName] = append(byItem[w.ItemName], w)
	}

	rec := writeRecorder{
		tx: tx, projectID: projectID, stream: stream, author: author, correlation: correlation,
		updated: time.Now().UTC().Format(time.RFC3339),
	}
	recorded := 0
	for _, item := range items {
		n, err := rec.item(ctx, item, byItem[item])
		recorded += n
		if err != nil {
			return recorded, err
		}
	}
	return recorded, nil
}

// writeRecorder records one push's edition writes, an item at a time.
type writeRecorder struct {
	tx                  Runner
	projectID, stream   string
	author, correlation string
	updated             string
}

// basisRecord is a unit's ledger record of the source its translation was
// made from, as an edition write gives it: a basis and nothing else.
type basisRecord struct {
	unit, variant, targetHash, contentHash, governing string
}

// draftMark is the source a tool's write says it drafted a unit's translation
// from.
type draftMark struct {
	unit, variant, basis string
}

// unitRecord is the part of a unit's ledger row a write is judged against.
type unitRecord struct {
	reviewState, targetHash, contentHash, draftBasis string
}

// unitVariant names one translation of a unit within an item.
type unitVariant struct{ unit, variant string }

// item records the writes of one item.
func (r *writeRecorder) item(ctx context.Context, item string, writes []venue.EditionWrite) (int, error) {
	units := make([]string, 0, len(writes))
	seen := map[string]bool{}
	for _, w := range writes {
		if !seen[w.Unit] {
			seen[w.Unit] = true
			units = append(units, w.Unit)
		}
	}
	writers, err := r.writers(ctx, item, writes)
	if err != nil {
		return 0, err
	}
	blockIDs, err := r.blocks(ctx, item, units)
	if err != nil {
		return 0, err
	}
	targets, err := r.translations(ctx, blockIDs)
	if err != nil {
		return 0, err
	}
	records, err := r.records(ctx, item, units)
	if err != nil {
		return 0, err
	}

	var bases []basisRecord
	var drafts []draftMark
	recorded := 0
	for _, w := range writes {
		blockID, ok := blockIDs[w.Unit]
		if !ok {
			continue // a unit the venue does not hold
		}
		key, _ := model.ParseEditionKey(w.Variant)

		// The translation the venue holds, when it holds one.
		held, targetHash := false, ""
		if targetJSON, ok := targets[[2]string{blockID, w.Variant}]; ok {
			var tgt model.Target
			if uerr := json.Unmarshal([]byte(targetJSON), &tgt); uerr == nil {
				if model.RunsRevision(key.Canonical(), tgt.Runs) != w.Revision {
					continue // the write describes another translation
				}
				held, targetHash = true, state.TargetHash(model.RunsText(tgt.Runs))
			}
		}

		prev, haveRecord := records[unitVariant{w.Unit, w.Variant}]
		decided := haveRecord && prev.reviewState != ""
		basis := ""
		if w.HasBasis() {
			basis = w.Basis
		}
		if !decided && (basis != "" || w.KnowsNoBasis()) &&
			(!haveRecord || prev.contentHash != basis || prev.targetHash != targetHash) {
			bases = append(bases, basisRecord{
				unit: w.Unit, variant: w.Variant, targetHash: targetHash, contentHash: basis,
				governing: w.GoverningFingerprint,
			})
		}
		if w.Produced() && (!haveRecord || prev.draftBasis != w.Basis) {
			drafts = append(drafts, draftMark{unit: w.Unit, variant: w.Variant, basis: w.Basis})
		}
		if author := writers[unitVariant{w.Unit, w.Variant}]; held && author != "" && r.correlation != "" {
			if _, err := r.tx.ExecContext(ctx,
				`UPDATE block_history SET author=$1
				 WHERE id = (SELECT MAX(id) FROM block_history
				   WHERE project_id=$2 AND stream=$3 AND block_id=$4 AND locale=$5
				     AND change_type IN `+targetContentChangeTypes+`
				     AND correlation_id=$6)
				   AND author=''`,
				author, r.projectID, r.stream, blockID, w.Variant, r.correlation); err != nil {
				return recorded, fmt.Errorf("attribute the translation of %s/%s: %w", w.Unit, w.Variant, err)
			}
		}
		recorded++
	}
	// The records first: a unit nobody had a record of gets its row here, and
	// the draft marks land on rows and create none. A statement may name a
	// row once, so the last write of a translation answers for it.
	if err := r.recordBases(ctx, item, lastOf(bases, func(b basisRecord) unitVariant { return unitVariant{b.unit, b.variant} })); err != nil {
		return recorded, err
	}
	return recorded, r.markDrafts(ctx, item, lastOf(drafts, func(d draftMark) unitVariant { return unitVariant{d.unit, d.variant} }))
}

// lastOf keeps the last of the values that share a key, in the order of their
// first appearance.
func lastOf[T any](in []T, key func(T) unitVariant) []T {
	at := make(map[unitVariant]int, len(in))
	out := make([]T, 0, len(in))
	for _, v := range in {
		k := key(v)
		if i, seen := at[k]; seen {
			out[i] = v
			continue
		}
		at[k] = len(out)
		out = append(out, v)
	}
	return out
}

// basisChunk and draftChunk bound the rows one statement writes, inside the
// statement's parameter limit.
const (
	basisChunk = 500
	draftChunk = 1000
)

// recordBases writes the basis records of an item's units, a few hundred to a
// statement. A record carries a basis and nothing else, so it never replaces
// a decision: a unit decided since the push read its record keeps the
// decision whole, and an undecided record keeps its rung, note and assignee.
// A basis decides nothing, so it is not filed in the block history as a
// decision.
func (r *writeRecorder) recordBases(ctx context.Context, item string, bases []basisRecord) error {
	if len(bases) == 0 {
		return nil
	}
	itemID, err := resolveItemIDPg(ctx, r.tx, r.projectID, r.stream, item)
	if err != nil {
		return err
	}
	for start := 0; start < len(bases); start += basisChunk {
		chunk := bases[start:min(start+basisChunk, len(bases))]
		args := []any{r.projectID, r.stream, itemID, item, r.updated}
		values := make([]string, len(chunk))
		for i, b := range chunk {
			n := len(args)
			values[i] = fmt.Sprintf("($1,$2,$3,$4,$%d,$%d,$%d,$%d,$%d,$5,NOW())", n+1, n+2, n+3, n+4, n+5)
			args = append(args, b.unit, b.variant, b.targetHash, b.contentHash, b.governing)
		}
		if _, err := r.tx.ExecContext(ctx,
			`INSERT INTO unit_decisions
				(project_id, stream, item_id, item_name, unit, variant, target_hash, content_hash,
				 governing_fingerprint, updated, updated_at)
			 VALUES `+strings.Join(values, ",")+`
			 ON CONFLICT (project_id, stream, item_id, unit, variant) DO UPDATE SET
				target_hash=EXCLUDED.target_hash, content_hash=EXCLUDED.content_hash,
				governing_fingerprint=EXCLUDED.governing_fingerprint,
				updated=EXCLUDED.updated, updated_at=EXCLUDED.updated_at
			 WHERE unit_decisions.review_state = ''`,
			args...); err != nil {
			return fmt.Errorf("record the bases of %s: %w", item, err)
		}
	}
	return nil
}

// markDrafts stamps the draft basis of an item's units a tool's write
// drafted, a thousand to a statement, on the rows that hold another.
func (r *writeRecorder) markDrafts(ctx context.Context, item string, drafts []draftMark) error {
	if len(drafts) == 0 {
		return nil
	}
	for start := 0; start < len(drafts); start += draftChunk {
		chunk := drafts[start:min(start+draftChunk, len(drafts))]
		args := []any{r.projectID, r.stream, item}
		values := make([]string, len(chunk))
		for i, d := range chunk {
			n := len(args)
			values[i] = fmt.Sprintf("($%d::text,$%d::text,$%d::text)", n+1, n+2, n+3)
			args = append(args, d.unit, d.variant, d.basis)
		}
		if _, err := r.tx.ExecContext(ctx,
			`UPDATE unit_decisions d SET draft_basis=v.basis, updated_at=NOW()
			 FROM (VALUES `+strings.Join(values, ",")+`) AS v(unit, variant, basis)
			 WHERE d.project_id=$1 AND d.stream=$2
			   AND d.item_id = (SELECT id FROM items WHERE project_id=$1 AND stream=$2 AND name=$3)
			   AND d.unit=v.unit AND d.variant=v.variant AND d.draft_basis <> v.basis`,
			args...); err != nil {
			return fmt.Errorf("mark the drafts of %s: %w", item, err)
		}
	}
	return nil
}

// writers keeps who wrote each of the item's translations by hand
// (edition_writers) and returns, for each write by hand, its author: the
// pusher, or for a revision an earlier push recorded, that push's author, so
// a checkout that pulled a colleague's record and sends it again is not
// taken for its author. A write of another revision by anybody else ends the
// earlier author's row.
func (r *writeRecorder) writers(ctx context.Context, item string, writes []venue.EditionWrite) (map[unitVariant]string, error) {
	rows, err := r.tx.QueryContext(ctx,
		`SELECT unit, variant, revision, author FROM edition_writers
		 WHERE project_id=$1 AND stream=$2 AND item_name=$3`,
		r.projectID, r.stream, item)
	if err != nil {
		return nil, fmt.Errorf("read the writers of %s: %w", item, err)
	}
	type keptWriter struct {
		at unitVariant
		w  platstore.EditionWriter
	}
	kept, err := storage.ScanRows(rows, func(sc storage.Scanner) (keptWriter, error) {
		var k keptWriter
		err := sc.Scan(&k.at.unit, &k.at.variant, &k.w.Revision, &k.w.Author)
		return k, err
	})
	if err != nil {
		return nil, fmt.Errorf("read the writers of %s: %w", item, err)
	}
	known := make(map[unitVariant]platstore.EditionWriter, len(kept))
	for _, k := range kept {
		known[k.at] = k.w
	}

	out := map[unitVariant]string{}
	for _, w := range writes {
		at := unitVariant{w.Unit, w.Variant}
		prev, had := known[at]
		switch {
		case had && prev.Revision == w.Revision:
			if w.ByHand() {
				out[at] = prev.Author
			}
		case w.ByHand():
			if r.author == "" {
				continue
			}
			if _, err := r.tx.ExecContext(ctx,
				`INSERT INTO edition_writers (project_id, stream, item_name, unit, variant, block, revision, author, origin, written_at)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())
				 ON CONFLICT (project_id, stream, item_name, unit, variant) DO UPDATE SET
				   block=EXCLUDED.block, revision=EXCLUDED.revision, author=EXCLUDED.author,
				   origin=EXCLUDED.origin, written_at=EXCLUDED.written_at`,
				r.projectID, r.stream, item, w.Unit, w.Variant, w.Block, w.Revision, r.author, w.Origin); err != nil {
				return nil, fmt.Errorf("record who wrote %s/%s: %w", w.Unit, w.Variant, err)
			}
			known[at] = platstore.EditionWriter{Revision: w.Revision, Author: r.author}
			out[at] = r.author
		case had:
			if _, err := r.tx.ExecContext(ctx,
				`DELETE FROM edition_writers
				 WHERE project_id=$1 AND stream=$2 AND item_name=$3 AND unit=$4 AND variant=$5`,
				r.projectID, r.stream, item, w.Unit, w.Variant); err != nil {
				return nil, fmt.Errorf("forget who wrote %s/%s: %w", w.Unit, w.Variant, err)
			}
			delete(known, at)
		}
	}
	return out, nil
}

// blocks maps each of the units the venue holds in item to its row.
func (r *writeRecorder) blocks(ctx context.Context, item string, units []string) (map[string]string, error) {
	out := map[string]string{}
	for chunk := range chunked(units, writeLookupChunk) {
		args := []any{r.projectID, r.stream, item}
		args = append(args, anyStrings(chunk)...)
		rows, err := r.tx.QueryContext(ctx,
			`SELECT source_id, id FROM blocks
			 WHERE project_id=$1 AND stream=$2 AND item_name=$3 AND source_id IN (`+placeholderList("pg", 4, len(chunk))+`)`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("resolve the blocks of %s: %w", item, err)
		}
		pairs, err := storage.ScanRows(rows, func(sc storage.Scanner) ([2]string, error) {
			var p [2]string
			err := sc.Scan(&p[0], &p[1])
			return p, err
		})
		if err != nil {
			return nil, fmt.Errorf("resolve the blocks of %s: %w", item, err)
		}
		for _, p := range pairs {
			out[p[0]] = p[1]
		}
	}
	return out, nil
}

// translations maps each translation the blocks hold, by row and locale, to
// its target_json.
func (r *writeRecorder) translations(ctx context.Context, blockIDs map[string]string) (map[[2]string]string, error) {
	ids := make([]string, 0, len(blockIDs))
	for _, id := range blockIDs {
		ids = append(ids, id)
	}
	out := map[[2]string]string{}
	for chunk := range chunked(ids, writeLookupChunk) {
		args := []any{r.projectID, r.stream}
		args = append(args, anyStrings(chunk)...)
		rows, err := r.tx.QueryContext(ctx,
			`SELECT block_id, locale, target_json FROM translations
			 WHERE project_id=$1 AND stream=$2 AND block_id IN (`+placeholderList("pg", 3, len(chunk))+`)`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("read the translations a push's writes describe: %w", err)
		}
		held, err := storage.ScanRows(rows, func(sc storage.Scanner) ([3]string, error) {
			var t [3]string
			err := sc.Scan(&t[0], &t[1], &t[2])
			return t, err
		})
		if err != nil {
			return nil, fmt.Errorf("read the translations a push's writes describe: %w", err)
		}
		for _, t := range held {
			out[[2]string{t[0], t[1]}] = t[2]
		}
	}
	return out, nil
}

// records maps each ledger row the units hold in item to what a write is
// judged against.
func (r *writeRecorder) records(ctx context.Context, item string, units []string) (map[unitVariant]unitRecord, error) {
	out := map[unitVariant]unitRecord{}
	for chunk := range chunked(units, writeLookupChunk) {
		args := []any{r.projectID, r.stream, item}
		args = append(args, anyStrings(chunk)...)
		rows, err := r.tx.QueryContext(ctx,
			`SELECT unit, variant, review_state, target_hash, content_hash, draft_basis FROM unit_decisions
			 WHERE project_id=$1 AND stream=$2
			   AND item_id = (SELECT id FROM items WHERE project_id=$1 AND stream=$2 AND name=$3)
			   AND unit IN (`+placeholderList("pg", 4, len(chunk))+`)`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("read the records of %s: %w", item, err)
		}
		type keptRecord struct {
			at  unitVariant
			rec unitRecord
		}
		held, err := storage.ScanRows(rows, func(sc storage.Scanner) (keptRecord, error) {
			var k keptRecord
			err := sc.Scan(&k.at.unit, &k.at.variant, &k.rec.reviewState, &k.rec.targetHash, &k.rec.contentHash, &k.rec.draftBasis)
			return k, err
		})
		if err != nil {
			return nil, fmt.Errorf("read the records of %s: %w", item, err)
		}
		for _, k := range held {
			out[k.at] = k.rec
		}
	}
	return out, nil
}

// chunked yields s in pieces of at most n.
func chunked(s []string, n int) func(func([]string) bool) {
	return func(yield func([]string) bool) {
		for start := 0; start < len(s); start += n {
			if !yield(s[start:min(start+n, len(s))]) {
				return
			}
		}
	}
}

// EditionWriters implements platstore.EditionWriterStore.
func (s *PostgresStore) EditionWriters(ctx context.Context, projectID, stream string, items []string) ([]platstore.EditionWriter, error) {
	if len(items) == 0 {
		return nil, nil
	}
	var out []platstore.EditionWriter
	for chunk := range chunked(items, writeLookupChunk) {
		args := []any{projectID, storeutil.DefaultStream(stream)}
		args = append(args, anyStrings(chunk)...)
		rows, err := s.db.QueryContext(ctx,
			`SELECT item_name, unit, block, variant, revision, author FROM edition_writers
			 WHERE project_id=$1 AND stream=$2 AND item_name IN (`+placeholderList("pg", 3, len(chunk))+`)
			 ORDER BY item_name, unit, variant`,
			args...)
		if err != nil {
			return nil, fmt.Errorf("read edition writers: %w", err)
		}
		writers, err := storage.ScanRows(rows, func(sc storage.Scanner) (platstore.EditionWriter, error) {
			var w platstore.EditionWriter
			err := sc.Scan(&w.ItemName, &w.Unit, &w.Block, &w.Variant, &w.Revision, &w.Author)
			return w, err
		})
		if err != nil {
			return nil, fmt.Errorf("read edition writers: %w", err)
		}
		out = append(out, writers...)
	}
	return out, nil
}

var _ platstore.EditionWriterStore = (*PostgresStore)(nil)
