package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// SyncBlockOverlays writes a block's targets and annotations into the
// kind-specific overlay tables (#403 / #405). Replaces the former
// inline writes to blocks.targets_json / blocks.annotations.
//
// UPSERT semantics — partial maps only update the locales/kinds
// provided. Unspecified entries are left intact. This matches how
// editors and single-locale translators naturally operate.
//
// dialect: "pg" | "sqlite".
func SyncBlockOverlays(
	ctx context.Context,
	tx *sql.Tx,
	dialect string,
	projectID, stream, blockID string,
	targets map[model.LocaleID][]*model.Segment,
	annotations map[string]model.Annotation,
	now time.Time,
) error {
	for locale, segs := range targets {
		segJSON, err := json.Marshal(segs)
		if err != nil {
			return fmt.Errorf("marshal segments for block %s locale %s: %w", blockID, locale, err)
		}
		if _, err := tx.ExecContext(ctx, sqlUpsertTranslation(dialect),
			projectID, stream, blockID, string(locale),
			flattenSegmentsText(segs), string(segJSON), now,
		); err != nil {
			return fmt.Errorf("upsert translation block=%s locale=%s: %w", blockID, locale, err)
		}
	}

	for kind, ann := range annotations {
		body, err := serializeSingleAnnotation(ann)
		if err != nil {
			return fmt.Errorf("marshal annotation block=%s kind=%s: %w", blockID, kind, err)
		}
		if _, err := tx.ExecContext(ctx, sqlUpsertAnnotation(dialect),
			projectID, stream, blockID, kind, body, now,
		); err != nil {
			return fmt.Errorf("upsert annotation block=%s kind=%s: %w", blockID, kind, err)
		}
	}
	return nil
}

// loadBlockOverlays hydrates a block's Targets + Annotations from the
// kind-specific tables. Called by GetBlock(s) after the source row
// is fetched.
func LoadBlockOverlays(
	ctx context.Context,
	db querier,
	dialect string,
	projectID, stream string,
	blockIDs []string,
) (map[string]map[model.LocaleID][]*model.Segment, map[string]map[string]model.Annotation, error) {
	if len(blockIDs) == 0 {
		return nil, nil, nil
	}

	targets := map[string]map[model.LocaleID][]*model.Segment{}
	rows, err := db.QueryContext(ctx, sqlListTranslationsByBlocks(dialect, len(blockIDs)),
		append([]any{projectID, stream}, anyStrings(blockIDs)...)...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load translations: %w", err)
	}
	for rows.Next() {
		var (
			bid, locale, segJSON string
		)
		if err := rows.Scan(&bid, &locale, &segJSON); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan translation: %w", err)
		}
		var segs []*model.Segment
		if segJSON != "" && segJSON != "[]" && segJSON != "null" {
			if err := json.Unmarshal([]byte(segJSON), &segs); err != nil {
				rows.Close()
				return nil, nil, fmt.Errorf("unmarshal segments block=%s locale=%s: %w", bid, locale, err)
			}
		}
		if targets[bid] == nil {
			targets[bid] = map[model.LocaleID][]*model.Segment{}
		}
		targets[bid][model.LocaleID(locale)] = segs
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("translation rows: %w", err)
	}

	annotations := map[string]map[string]model.Annotation{}
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
			annotations[bid] = map[string]model.Annotation{}
		}
		annotations[bid][kind] = ann
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("annotation rows: %w", err)
	}
	return targets, annotations, nil
}

// loadBlockTargetLocales returns the set of locales each block has a
// translation for. Replaces the former extractTargetLocales path that
// parsed targets_json inline.
func LoadBlockTargetLocales(
	ctx context.Context,
	db querier,
	dialect string,
	projectID, stream string,
	blockIDs []string,
) (map[string][]string, error) {
	if len(blockIDs) == 0 {
		return nil, nil
	}
	out := map[string][]string{}
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

// querier abstracts *sql.DB and *sql.Tx so helpers work against both.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func anyStrings(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// flattenSegmentsText returns the concatenated text of a segment list
// for the `translations.text` column. Lossy — the full rich runs live
// in segments_json.
func flattenSegmentsText(segs []*model.Segment) string {
	if len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, s := range segs {
		if s == nil {
			continue
		}
		for _, r := range s.Runs {
			if r.Text != nil {
				b.WriteString(r.Text.Text)
			}
		}
	}
	return b.String()
}

// ─── SQL for overlay sync ───────────────────────────────────────

func sqlUpsertTranslation(dialect string) string {
	if dialect == "sqlite" {
		return `INSERT INTO translations (project_id, stream, block_id, locale, text, segments_json, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, stream, block_id, locale) DO UPDATE SET
				text = excluded.text,
				segments_json = excluded.segments_json,
				updated_at = excluded.updated_at`
	}
	return `INSERT INTO translations (project_id, stream, block_id, locale, text, segments_json, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (project_id, stream, block_id, locale) DO UPDATE SET
			text = EXCLUDED.text,
			segments_json = EXCLUDED.segments_json,
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
// (block_id, locale, segments_json) rows for a set of blocks in one
// project+stream. nblocks is the number of placeholder slots to emit.
func sqlListTranslationsByBlocks(dialect string, nblocks int) string {
	return `SELECT block_id, locale, segments_json FROM translations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND block_id IN (` + placeholderList(dialect, 3, nblocks) + `)`
}

func sqlListAnnotationsByBlocks(dialect string, nblocks int) string {
	return `SELECT block_id, kind, payload FROM annotations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND block_id IN (` + placeholderList(dialect, 3, nblocks) + `)`
}

func sqlListTranslationLocalesByBlocks(dialect string, nblocks int) string {
	return `SELECT block_id, locale FROM translations
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
	for i := 0; i < count; i++ {
		parts[i] = placeholder(dialect, startAt+i)
	}
	return strings.Join(parts, ", ")
}

// ─── Annotation (de)serialization ───────────────────────────────

// serializeSingleAnnotation emits one Annotation's wire bytes. Matches
// the type-discriminated wrapper `{"type":"…","data":{…}}` used by
// serializeAnnotations for cross-compat with existing round-trip code.
func serializeSingleAnnotation(ann model.Annotation) ([]byte, error) {
	return json.Marshal(map[string]any{
		"type": ann.AnnotationType(),
		"data": ann,
	})
}

// deserializeSingleAnnotation reverses serializeSingleAnnotation. Delegates
// to the existing map-based deserializer by wrapping the payload in a
// single-entry map under the caller-supplied kind, then picks the one
// annotation out.
func deserializeSingleAnnotation(kind string, payload []byte) (model.Annotation, error) {
	wrapped, err := json.Marshal(map[string]json.RawMessage{kind: payload})
	if err != nil {
		return nil, err
	}
	anns := deserializeAnnotations(string(wrapped))
	return anns[kind], nil
}
