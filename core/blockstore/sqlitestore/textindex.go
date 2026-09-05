//go:build !wasm

package sqlitestore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
)

// trigramMin is the shortest needle the trigram tokenizer can index. A term of
// one or two characters — "AI", "UX", a CJK word — produces no trigram at all,
// so those searches take the LIKE path instead. That is a scan, and it is the
// honest cost of a two-letter term: no substring index can do better without
// indexing every prefix.
const trigramMin = 3

// candidateLimit caps how many rows the index hands back before the caller
// verifies them. It is deliberately generous: the caller filters to real
// matches, so a low cap would silently lose occurrences rather than save work.
const candidateLimit = 5000

// SearchBlockText implements blockstore.TextSearcher over the FTS5 trigram
// index on block_texts.
//
// The index narrows; it does not decide. A trigram match means "these three-
// character sequences all occur", which is necessary but not sufficient for the
// needle to be present, so every hit still carries its full text for the caller
// to verify. What the index guarantees is the other direction: no text
// containing the needle is missed.
func (k *cacheStore) SearchBlockText(ctx context.Context, needle string, opts blockstore.TextSearchOptions) ([]blockstore.TextHit, error) {
	if k.db == nil {
		return nil, blockstore.ErrClosed
	}
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return nil, nil
	}
	if err := k.ensureTextIndex(ctx); err != nil {
		return nil, err
	}
	lower := strings.ToLower(needle)

	var where []string
	var args []any
	if len([]rune(lower)) >= trigramMin {
		where = append(where, `bt.rowid IN (
			SELECT rowid FROM block_texts_trigram
			WHERE block_texts_trigram MATCH ? LIMIT ?
		)`)
		args = append(args, ftsPhrase(lower), candidateLimit)
	} else {
		where = append(where, `bt.text_lower LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(lower)+"%")
	}
	if opts.Collection != "" {
		where = append(where, `b.collection = ?`)
		args = append(args, opts.Collection)
	}
	if locales := opts.CanonicalLocales(); locales != nil {
		if len(locales) == 0 {
			return nil, nil
		}
		marks := make([]string, len(locales))
		for i, l := range locales {
			marks[i] = "?"
			args = append(args, l)
		}
		where = append(where, `bt.locale IN (`+strings.Join(marks, ",")+`)`)
	}

	q := `SELECT bt.block_hash, bt.locale, bt.text, b.collection, b.payload
		FROM block_texts bt JOIN blocks b ON b.hash = bt.block_hash
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY bt.block_hash, bt.locale`
	if opts.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, opts.Limit)
	}

	rows, err := k.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("blockstore: search block text: %w", err)
	}
	defer rows.Close()

	var hits []blockstore.TextHit
	for rows.Next() {
		var h blockstore.TextHit
		var payload []byte
		if err := rows.Scan(&h.Hash, &h.Locale, &h.Text, &h.Collection, &payload); err != nil {
			return nil, fmt.Errorf("blockstore: scan text hit: %w", err)
		}
		var b blockstore.Block
		if err := json.Unmarshal(payload, &b); err != nil {
			return nil, fmt.Errorf("blockstore: decode block: %w", err)
		}
		h.Block = &b
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("blockstore: iterate text hits: %w", err)
	}
	return hits, nil
}

// ftsPhrase wraps a needle as a single FTS5 phrase: double-quoted, with any
// internal quote doubled. Without it a needle containing a hyphen, a colon or a
// quote is read as query syntax and either matches the wrong thing or errors.
func ftsPhrase(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// escapeLike neutralizes the LIKE wildcards in a literal needle.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ensureTextIndex brings the text rows up to date with the blocks, and is the
// reason extraction pays nothing for this index.
//
// The obvious design — maintain the index inside PutBlock — was built and
// measured, and it cost too much to keep. Writing 2000 blocks went from 25ms to
// 192ms: the two extra statements per block roughly doubled it, and indexing a
// full sentence as three-character grams roughly doubled it again. Extraction
// writes every block in the project, so that is a tax on the operation kapi
// runs most, levied for a query it may never be asked.
//
// So PutBlock only marks the block: `text_indexed = 0`, one column value in a
// statement it was issuing anyway, which benchmarks as no change at all. The
// build happens here, before a search, over exactly the blocks that changed —
// so a second search after no writes builds nothing.
//
// Payloads are JSON, so no SQL can flatten them — the build has to come back
// through Go, which is also why it cannot be a trigger.
func (k *cacheStore) ensureTextIndex(ctx context.Context) error {
	var pending int
	err := k.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM blocks WHERE text_indexed = 0)`).Scan(&pending)
	if err != nil {
		return fmt.Errorf("blockstore: probe text index: %w", err)
	}
	if pending == 0 {
		return nil
	}

	sess, err := k.Begin(ctx)
	if err != nil {
		return err
	}
	defer sess.Close()
	cs, ok := sess.(*cacheSession)
	if !ok {
		return fmt.Errorf("blockstore: build text index: unexpected session type %T", sess)
	}

	rows, err := cs.q().QueryContext(ctx,
		`SELECT hash, payload FROM blocks WHERE text_indexed = 0`)
	if err != nil {
		return fmt.Errorf("blockstore: read unindexed blocks: %w", err)
	}
	type pendingBlock struct {
		hash  string
		block *blockstore.Block
	}
	var todo []pendingBlock
	for rows.Next() {
		var hash string
		var payload []byte
		if err := rows.Scan(&hash, &payload); err != nil {
			rows.Close()
			return fmt.Errorf("blockstore: scan unindexed block: %w", err)
		}
		var b blockstore.Block
		if err := json.Unmarshal(payload, &b); err != nil {
			rows.Close()
			return fmt.Errorf("blockstore: decode unindexed block: %w", err)
		}
		b.Hash = hash
		todo = append(todo, pendingBlock{hash: hash, block: &b})
	}
	closeErr := rows.Err()
	rows.Close()
	if closeErr != nil {
		return fmt.Errorf("blockstore: iterate unindexed blocks: %w", closeErr)
	}

	for _, p := range todo {
		if err := cs.putBlockTexts(p.block); err != nil {
			return err
		}
		if _, err := cs.q().ExecContext(ctx,
			`UPDATE blocks SET text_indexed = 1 WHERE hash = ?`, p.hash); err != nil {
			return fmt.Errorf("blockstore: mark block indexed: %w", err)
		}
	}
	return sess.Commit()
}
