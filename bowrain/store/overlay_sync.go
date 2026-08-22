package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/model"
)

// Execer abstracts *sql.DB and *sql.Tx so the overlay-sync writers work
// against both transaction-scoped and pooled connections, exactly as Querier
// does for the readers. One writer serves the block round-trip (inside the
// store-blocks transaction) and the in-process blockstore adapter (on the
// pool), so a target reaches the same columns whichever door it came through.
type Execer = storage.Execer

// SyncBlockOverlays writes a block's targets and annotations into the
// kind-specific overlay tables (translations, annotations), keyed for
// access-pattern-specific indexes (#403 / #405).
//
// UPSERT semantics — partial maps only update the variants/kinds
// provided. Unspecified entries are left intact. This matches how
// editors and single-locale translators naturally operate.
//
// dialect: "pg" | "sqlite".
func SyncBlockOverlays(
	ctx context.Context,
	ex Execer,
	dialect string,
	projectID, stream, blockID string,
	targets map[model.VariantKey]*model.Target,
	annotations map[string]model.Payload,
	now time.Time,
) error {
	for key, target := range targets {
		if target == nil {
			continue
		}
		if err := UpsertBlockTarget(ctx, ex, dialect, projectID, stream, blockID, key, target, nil, now); err != nil {
			return err
		}
	}

	for key, ann := range annotations {
		if err := UpsertBlockAnnotation(ctx, ex, dialect, projectID, stream, blockID, key, ann, now); err != nil {
			return err
		}
	}
	return nil
}

// UpsertBlockAnnotation writes one (block, key) annotation row. It is the ONLY
// writer of the annotations table, because all three of a row's coordinates are
// read back by the block hydrator and a writer that spells any one of them
// differently files a row nothing joins to:
//
//   - block_id is the block's own `blocks.id` — what LoadBlockOverlays queries
//     by, and what block and item deletion clear.
//   - kind is the bare annotation key ("note", "quality.findings"), which is
//     what Block.SetAnno is handed on the way back.
//   - payload is the type-discriminated envelope {"type":…,"data":…}, which is
//     what model.DecodePayload reads.
//
// A caller holding bytes rather than a typed payload gets its type through
// model.DecodePayload first, so the envelope is written by one function and read
// by one function.
func UpsertBlockAnnotation(
	ctx context.Context,
	ex Execer,
	dialect string,
	projectID, stream, blockID, key string,
	ann model.Payload,
	now time.Time,
) error {
	if key == "" {
		return fmt.Errorf("upsert annotation block=%s: empty key", blockID)
	}
	body, err := serializeSingleAnnotation(ann)
	if err != nil {
		return fmt.Errorf("marshal annotation block=%s key=%s: %w", blockID, key, err)
	}
	if _, err := ex.ExecContext(ctx, sqlUpsertAnnotation(dialect),
		projectID, stream, blockID, key, body, now,
	); err != nil {
		return fmt.Errorf("upsert annotation block=%s key=%s: %w", blockID, key, err)
	}
	return nil
}

// UpsertBlockTarget writes one (block, variant) target row. It is the ONLY
// writer of the translations table, because the row's four content columns are
// four views of one thing and a writer that fills some of them makes the target
// invisible to every reader that consults the others:
//
//   - `locale` is the VariantKey text form ("fr-FR" or "fr-FR;tone=…").
//   - `target_json` is the full model.Target (runs + status + origin + score).
//     It is the truth: block hydration, the review queue, the decision ledger,
//     coverage and the context graph all read it.
//   - `text` is model.RunsText of the same runs — placeholder-stripped, so it
//     answers "what does this say" for history and search, never "what is the
//     content".
//   - `provider` is the producing engine off the same Origin.
//
// extra carries payload fields a writer keeps alongside the target that the
// Target model has no room for (a tool's config fingerprint, say). It belongs
// to whoever wrote the target, so a new target replaces it rather than
// inheriting the previous writer's; nil writes an empty object.
func UpsertBlockTarget(
	ctx context.Context,
	ex Execer,
	dialect string,
	projectID, stream, blockID string,
	key model.VariantKey,
	target *model.Target,
	extra []byte,
	now time.Time,
) error {
	if target == nil {
		return fmt.Errorf("upsert translation block=%s: nil target", blockID)
	}
	keyText, err := key.MarshalText()
	if err != nil {
		return fmt.Errorf("encode variant key for block %s: %w", blockID, err)
	}
	targetJSON, err := json.Marshal(target)
	if err != nil {
		return fmt.Errorf("marshal target for block %s variant %s: %w", blockID, keyText, err)
	}
	if len(extra) == 0 {
		extra = []byte("{}")
	}
	if _, err := ex.ExecContext(ctx, sqlUpsertTranslation(dialect),
		projectID, stream, blockID, string(keyText),
		model.RunsText(target.Runs), string(targetJSON), target.Origin.Engine, string(extra), now,
	); err != nil {
		return fmt.Errorf("upsert translation block=%s variant=%s: %w", blockID, keyText, err)
	}
	return nil
}

// overlayChunk bounds one IN(...) list in the loaders below. Every id is one
// bind parameter, and a whole-project call — GetBlockStats deriving coverage,
// the review loop hydrating every block — must not gamble on the corpus being
// smaller than the driver's parameter budget (65,535 on Postgres, far less on
// SQLite). Third member of this family found at 74,916 blocks; the loaders
// chunk so no caller has to know.
func overlayChunk(dialect string) int {
	if dialect == "sqlite" {
		return 500
	}
	return 5000
}

// LoadBlockOverlays hydrates a block's Targets + Annotations from the
// kind-specific tables. Called by GetBlock(s) after the source row
// is fetched.
func LoadBlockOverlays(
	ctx context.Context,
	db Querier,
	dialect string,
	projectID, stream string,
	blockIDs []string,
) (map[string]map[model.VariantKey]*model.Target, map[string]map[string]model.Payload, error) {
	if len(blockIDs) == 0 {
		return nil, nil, nil
	}
	if size := overlayChunk(dialect); len(blockIDs) > size {
		targets := map[string]map[model.VariantKey]*model.Target{}
		annotations := map[string]map[string]model.Payload{}
		for start := 0; start < len(blockIDs); start += size {
			t, a, err := LoadBlockOverlays(ctx, db, dialect, projectID, stream, blockIDs[start:min(start+size, len(blockIDs))])
			if err != nil {
				return nil, nil, err
			}
			maps.Copy(targets, t)
			maps.Copy(annotations, a)
		}
		return targets, annotations, nil
	}

	targets := map[string]map[model.VariantKey]*model.Target{}
	rows, err := db.QueryContext(ctx, sqlListTranslationsByBlocks(dialect, len(blockIDs)),
		append([]any{projectID, stream}, anyStrings(blockIDs)...)...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load translations: %w", err)
	}
	for rows.Next() {
		var (
			bid, keyText, targetJSON string
		)
		if err := rows.Scan(&bid, &keyText, &targetJSON); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan translation: %w", err)
		}
		var key model.VariantKey
		if err := key.UnmarshalText([]byte(keyText)); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("decode variant key block=%s key=%s: %w", bid, keyText, err)
		}
		target := &model.Target{}
		if targetJSON != "" && targetJSON != "null" {
			if err := json.Unmarshal([]byte(targetJSON), target); err != nil {
				rows.Close()
				return nil, nil, fmt.Errorf("unmarshal target block=%s variant=%s: %w", bid, keyText, err)
			}
		}
		if targets[bid] == nil {
			targets[bid] = map[model.VariantKey]*model.Target{}
		}
		targets[bid][key] = target
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("translation rows: %w", err)
	}

	annotations := map[string]map[string]model.Payload{}
	rows, err = db.QueryContext(ctx, sqlListAnnotationsByBlocks(dialect, len(blockIDs)),
		append([]any{projectID, stream}, anyStrings(blockIDs)...)...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load annotations: %w", err)
	}
	for rows.Next() {
		var bid, kind, payload string
		if err := rows.Scan(&bid, &kind, &payload); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan annotation: %w", err)
		}
		ann, err := deserializeSingleAnnotation(kind, []byte(payload))
		if err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("deserialize annotation block=%s kind=%s: %w", bid, kind, err)
		}
		if annotations[bid] == nil {
			annotations[bid] = map[string]model.Payload{}
		}
		annotations[bid][kind] = ann
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("annotation rows: %w", err)
	}
	return targets, annotations, nil
}

// StoredTarget is one translations row as the store holds it: the variant it is
// filed under, the model.Target it carries, and the writer's residual payload.
type StoredTarget struct {
	// BlockID is the row's block key — the store's own `blocks.id`.
	BlockID string
	// Variant is the row's locale column, decoded.
	Variant model.VariantKey
	// Target is the row's target_json. Never nil on a loaded row.
	Target *model.Target
	// Extra is the row's metadata column: the payload fields the writer kept
	// alongside the target. An empty object when it kept none.
	Extra []byte
	// UpdatedAt is the row's write time in Unix seconds.
	UpdatedAt int64
}

// LoadBlockVariantTarget reads one (block, variant) target row, or (nil, nil)
// when the block has no target for that variant.
func LoadBlockVariantTarget(
	ctx context.Context,
	db Querier,
	dialect string,
	projectID, stream, blockID, variant string,
) (*StoredTarget, error) {
	rows, err := db.QueryContext(ctx, sqlSelectTargetRow(dialect), projectID, stream, blockID, variant)
	if err != nil {
		return nil, fmt.Errorf("load target block=%s variant=%s: %w", blockID, variant, err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("load target block=%s variant=%s: %w", blockID, variant, err)
		}
		return nil, nil
	}
	st, err := scanStoredTarget(rows, blockID)
	if err != nil {
		return nil, err
	}
	return st, rows.Err()
}

// LoadVariantTargets reads every target row for one variant in a project
// stream, ordered by block id so one corpus answers one query the same way
// twice.
func LoadVariantTargets(
	ctx context.Context,
	db Querier,
	dialect string,
	projectID, stream, variant string,
) ([]StoredTarget, error) {
	rows, err := db.QueryContext(ctx, sqlListTargetRowsByVariant(dialect), projectID, stream, variant)
	if err != nil {
		return nil, fmt.Errorf("list targets variant=%s: %w", variant, err)
	}
	defer rows.Close()
	var out []StoredTarget
	for rows.Next() {
		st, err := scanStoredTarget(rows, "")
		if err != nil {
			return nil, err
		}
		out = append(out, *st)
	}
	return out, rows.Err()
}

// scanStoredTarget reads one (block_id, locale, target_json, metadata,
// updated_at) row. blockID, when supplied, names a row the query already
// scoped to one block and is used in error messages.
func scanStoredTarget(rows *sql.Rows, blockID string) (*StoredTarget, error) {
	var (
		bid, keyText, targetJSON, extra, updatedAt string
	)
	if err := rows.Scan(&bid, &keyText, &targetJSON, &extra, &updatedAt); err != nil {
		return nil, fmt.Errorf("scan target block=%s: %w", blockID, err)
	}
	st := &StoredTarget{BlockID: bid, Target: &model.Target{}, UpdatedAt: ParseOverlayTimestamp(updatedAt)}
	if err := st.Variant.UnmarshalText([]byte(keyText)); err != nil {
		return nil, fmt.Errorf("decode variant key block=%s key=%s: %w", bid, keyText, err)
	}
	if targetJSON != "" && targetJSON != "null" {
		if err := json.Unmarshal([]byte(targetJSON), st.Target); err != nil {
			return nil, fmt.Errorf("unmarshal target block=%s variant=%s: %w", bid, keyText, err)
		}
	}
	st.Extra = []byte(extra)
	return st, nil
}

// StoredAnnotation is one annotations row as the store holds it: the block it
// is filed under, the annotation key it is filed as, and the payload decoded
// through model.DecodePayload.
type StoredAnnotation struct {
	// BlockID is the row's block key — the store's own `blocks.id`.
	BlockID string
	// Key is the row's kind column: the bare annotation key.
	Key string
	// Value is the row's payload, rehydrated. Never nil on a loaded row.
	Value model.Payload
	// UpdatedAt is the row's write time in Unix seconds.
	UpdatedAt int64
}

// LoadBlockAnnotation reads one (block, key) annotation row, or (nil, nil) when
// the block carries no annotation under that key.
func LoadBlockAnnotation(
	ctx context.Context,
	db Querier,
	dialect string,
	projectID, stream, blockID, key string,
) (*StoredAnnotation, error) {
	rows, err := db.QueryContext(ctx, sqlSelectAnnotationRow(dialect), projectID, stream, blockID, key)
	if err != nil {
		return nil, fmt.Errorf("load annotation block=%s key=%s: %w", blockID, key, err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("load annotation block=%s key=%s: %w", blockID, key, err)
		}
		return nil, nil
	}
	sa, err := scanStoredAnnotation(rows)
	if err != nil {
		return nil, err
	}
	return sa, rows.Err()
}

// LoadAnnotationsByKey reads every annotation row filed under one key in a
// project stream, ordered by block id so one corpus answers one query the same
// way twice.
func LoadAnnotationsByKey(
	ctx context.Context,
	db Querier,
	dialect string,
	projectID, stream, key string,
) ([]StoredAnnotation, error) {
	rows, err := db.QueryContext(ctx, sqlListAnnotationRowsByKey(dialect), projectID, stream, key)
	if err != nil {
		return nil, fmt.Errorf("list annotations key=%s: %w", key, err)
	}
	defer rows.Close()
	var out []StoredAnnotation
	for rows.Next() {
		sa, err := scanStoredAnnotation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sa)
	}
	return out, rows.Err()
}

// scanStoredAnnotation reads one (block_id, kind, payload, updated_at) row and
// decodes the payload the way LoadBlockOverlays does, so a single-annotation
// read and block hydration cannot disagree about what a row holds.
func scanStoredAnnotation(rows *sql.Rows) (*StoredAnnotation, error) {
	var bid, key, payload, updatedAt string
	if err := rows.Scan(&bid, &key, &payload, &updatedAt); err != nil {
		return nil, fmt.Errorf("scan annotation: %w", err)
	}
	ann, err := deserializeSingleAnnotation(key, []byte(payload))
	if err != nil {
		return nil, fmt.Errorf("deserialize annotation block=%s key=%s: %w", bid, key, err)
	}
	return &StoredAnnotation{BlockID: bid, Key: key, Value: ann, UpdatedAt: ParseOverlayTimestamp(updatedAt)}, nil
}

// ParseOverlayTimestamp reads an overlay row's updated_at, which arrives as the
// SQLite `datetime('now')` TEXT form ("2006-01-02 15:04:05") or the Postgres
// TIMESTAMPTZ that pgx renders over the database/sql surface. Returns Unix
// seconds, or 0 when the column was empty or unparseable.
func ParseOverlayTimestamp(s string) int64 {
	if s == "" {
		return 0
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// LoadBlockTargetLocales returns the set of locales each block has a
// translation for, read from the translations overlay table.
func LoadBlockTargetLocales(
	ctx context.Context,
	db Querier,
	dialect string,
	projectID, stream string,
	blockIDs []string,
) (map[string][]string, error) {
	if len(blockIDs) == 0 {
		return nil, nil
	}
	out := map[string][]string{}
	if size := overlayChunk(dialect); len(blockIDs) > size {
		for start := 0; start < len(blockIDs); start += size {
			part, err := LoadBlockTargetLocales(ctx, db, dialect, projectID, stream, blockIDs[start:min(start+size, len(blockIDs))])
			if err != nil {
				return nil, err
			}
			maps.Copy(out, part)
		}
		return out, nil
	}
	rows, err := db.QueryContext(ctx, sqlListTranslationLocalesByBlocks(dialect, len(blockIDs)),
		append([]any{projectID, stream}, anyStrings(blockIDs)...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("load locales: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var bid, locale string
		if err := rows.Scan(&bid, &locale); err != nil {
			return nil, fmt.Errorf("scan locale: %w", err)
		}
		out[bid] = append(out[bid], locale)
	}
	return out, rows.Err()
}

// TargetLocaleState is one (locale, status) pair for a block's stored target,
// as read from the translations overlay table. Locale is the raw VariantKey
// text form ("fr-FR" or "fr-FR;tone=…"); Status is the stored
// model.TargetStatus text ("" when the target has no committed status).
type TargetLocaleState struct {
	Locale string
	Status model.TargetStatus
}

// LoadBlockTargetStates returns, per block, the (locale, status) pairs of its
// stored targets. The status is extracted from target_json in SQL so the run
// payloads never leave the database — the same lightweight-projection contract
// GetBlockStats holds for source_json.
func LoadBlockTargetStates(
	ctx context.Context,
	db Querier,
	dialect string,
	projectID, stream string,
	blockIDs []string,
) (map[string][]TargetLocaleState, error) {
	if len(blockIDs) == 0 {
		return nil, nil
	}
	out := map[string][]TargetLocaleState{}
	if size := overlayChunk(dialect); len(blockIDs) > size {
		for start := 0; start < len(blockIDs); start += size {
			part, err := LoadBlockTargetStates(ctx, db, dialect, projectID, stream, blockIDs[start:min(start+size, len(blockIDs))])
			if err != nil {
				return nil, err
			}
			maps.Copy(out, part)
		}
		return out, nil
	}
	rows, err := db.QueryContext(ctx, sqlListTranslationStatesByBlocks(dialect, len(blockIDs)),
		append([]any{projectID, stream}, anyStrings(blockIDs)...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("load target states: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var bid, locale, status string
		if err := rows.Scan(&bid, &locale, &status); err != nil {
			return nil, fmt.Errorf("scan target state: %w", err)
		}
		out[bid] = append(out[bid], TargetLocaleState{Locale: locale, Status: model.TargetStatus(status)})
	}
	return out, rows.Err()
}

// SplitTargetStates projects a block's target states onto the two BlockStatRow
// slices: every target locale, and the subset whose status carries a review
// decision (reviewed or above on the model.TargetStatus ladder).
func SplitTargetStates(states []TargetLocaleState) (locales, approved []string) {
	for _, st := range states {
		locales = append(locales, st.Locale)
		if st.Status.Rank() >= model.TargetStatusReviewed.Rank() {
			approved = append(approved, st.Locale)
		}
	}
	return locales, approved
}

// Querier abstracts *sql.DB and *sql.Tx so the overlay-sync helpers work
// against both transaction-scoped and pooled connections. Reads only, and
// deliberately the narrowest thing those helpers need: a caller holding nothing
// more than a query method can still use them.
type Querier = storage.Querier

// Runner is everything a *sql.DB and a *sql.Tx both do.
//
// It is what lets one body serve two callers: a verb invoked on its own, which
// opens a transaction of its own, and the same verb invoked as one step of a
// larger transition that has to land whole. Without it every write owns its
// transaction, which is exactly why a push used to apply in pieces — one per
// chunk, one per item, and more again after the loop.
type Runner = storage.Runner

// VariantKeyText renders a VariantKey to its canonical text form for use as a
// change-log identifier and the translations.locale column ("fr-FR" for the
// locale-only common case).
func VariantKeyText(key model.VariantKey) string {
	b, err := key.MarshalText()
	if err != nil {
		return string(key.Locale)
	}
	return string(b)
}

func anyStrings(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// ─── SQL for overlay sync ───────────────────────────────────────

func sqlUpsertTranslation(dialect string) string {
	if dialect == "sqlite" {
		return `INSERT INTO translations (project_id, stream, block_id, locale, text, target_json, provider, metadata, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, stream, block_id, locale) DO UPDATE SET
				text = excluded.text,
				target_json = excluded.target_json,
				provider = excluded.provider,
				metadata = excluded.metadata,
				updated_at = excluded.updated_at`
	}
	return `INSERT INTO translations (project_id, stream, block_id, locale, text, target_json, provider, metadata, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (project_id, stream, block_id, locale) DO UPDATE SET
			text = EXCLUDED.text,
			target_json = EXCLUDED.target_json,
			provider = EXCLUDED.provider,
			metadata = EXCLUDED.metadata,
			updated_at = EXCLUDED.updated_at`
}

func sqlUpsertAnnotation(dialect string) string {
	if dialect == "sqlite" {
		return `INSERT INTO annotations (project_id, stream, block_id, kind, payload, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, stream, block_id, kind) DO UPDATE SET
				payload = excluded.payload,
				updated_at = excluded.updated_at`
	}
	return `INSERT INTO annotations (project_id, stream, block_id, kind, payload, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, stream, block_id, kind) DO UPDATE SET
			payload = EXCLUDED.payload,
			updated_at = EXCLUDED.updated_at`
}

// sqlListTranslationsByBlocks returns a SELECT that pulls all
// (block_id, locale, target_json) rows for a set of blocks in one
// project+stream. nblocks is the number of placeholder slots to emit.
func sqlListTranslationsByBlocks(dialect string, nblocks int) string {
	return `SELECT block_id, locale, target_json FROM translations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND block_id IN (` + placeholderList(dialect, 3, nblocks) + `)`
}

// sqlSelectTargetRow and sqlListTargetRowsByVariant read the whole target row —
// the same column set, so a single-block read and a whole-variant listing
// cannot disagree about what a target is.
func sqlSelectTargetRow(dialect string) string {
	return `SELECT block_id, locale, target_json, metadata, updated_at FROM translations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND block_id = ` + placeholder(dialect, 3) + ` AND locale = ` + placeholder(dialect, 4)
}

func sqlListTargetRowsByVariant(dialect string) string {
	return `SELECT block_id, locale, target_json, metadata, updated_at FROM translations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND locale = ` + placeholder(dialect, 3) + ` ORDER BY block_id`
}

func sqlListAnnotationsByBlocks(dialect string, nblocks int) string {
	return `SELECT block_id, kind, payload FROM annotations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND block_id IN (` + placeholderList(dialect, 3, nblocks) + `)`
}

// sqlSelectAnnotationRow and sqlListAnnotationRowsByKey read the same column
// set, so a single-annotation read and a whole-key listing cannot disagree
// about what an annotation is.
func sqlSelectAnnotationRow(dialect string) string {
	return `SELECT block_id, kind, payload, updated_at FROM annotations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND block_id = ` + placeholder(dialect, 3) + ` AND kind = ` + placeholder(dialect, 4)
}

func sqlListAnnotationRowsByKey(dialect string) string {
	return `SELECT block_id, kind, payload, updated_at FROM annotations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND kind = ` + placeholder(dialect, 3) + ` ORDER BY block_id`
}

func sqlListTranslationLocalesByBlocks(dialect string, nblocks int) string {
	return `SELECT block_id, locale FROM translations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND block_id IN (` + placeholderList(dialect, 3, nblocks) + `)`
}

// sqlListTranslationStatesByBlocks pulls (block_id, locale, status) with the
// status extracted from the target_json payload in SQL: jsonb ->> on Postgres,
// json_extract on SQLite. COALESCE keeps a missing status as the empty string.
func sqlListTranslationStatesByBlocks(dialect string, nblocks int) string {
	statusExpr := `COALESCE(target_json->>'status', '')`
	if dialect == "sqlite" {
		statusExpr = `COALESCE(json_extract(target_json, '$.status'), '')`
	}
	return `SELECT block_id, locale, ` + statusExpr + ` FROM translations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND block_id IN (` + placeholderList(dialect, 3, nblocks) + `)`
}

func placeholder(dialect string, n int) string {
	if dialect == "sqlite" {
		return "?"
	}
	return fmt.Sprintf("$%d", n)
}

func placeholderList(dialect string, startAt, count int) string {
	if count == 0 {
		return "NULL" // never matches, avoids syntax error
	}
	parts := make([]string, count)
	for i := range count {
		parts[i] = placeholder(dialect, startAt+i)
	}
	return strings.Join(parts, ", ")
}

// ─── Annotation (de)serialization ───────────────────────────────

// annotationEnvelope is the annotations column's payload shape: the payload's
// own type name alongside its body, so an interface-typed annotation can be
// rehydrated as the concrete type its writer held.
type annotationEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// serializeSingleAnnotation emits one annotation's stored bytes.
func serializeSingleAnnotation(ann model.Payload) ([]byte, error) {
	data, err := json.Marshal(ann)
	if err != nil {
		return nil, err
	}
	return json.Marshal(annotationEnvelope{Type: model.PayloadTypeName(ann), Data: data})
}

// deserializeSingleAnnotation reverses serializeSingleAnnotation through
// model.DecodePayload — the one decode every reader of a stand-off payload
// uses. The row's kind names the annotation key; the envelope's type names the
// payload, and falls back to the key for a row whose writer left it empty.
func deserializeSingleAnnotation(key string, payload []byte) (model.Payload, error) {
	var env annotationEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return model.DecodePayload(key, payload), nil
	}
	typeName := env.Type
	if typeName == "" {
		typeName = key
	}
	body := env.Data
	if len(body) == 0 {
		body = payload
	}
	return model.DecodePayload(typeName, body), nil
}
