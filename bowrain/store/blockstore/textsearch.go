package blockstore

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/blockstore"
)

// pgStore is the adapter over a Postgres content store. It carries the one
// capability the SQLite shape does not: a candidate query that narrows a text
// search to the blocks that may hold the needle, so the search reads a handful
// of blocks rather than the project's whole corpus.
//
// The capability is a type, not a flag, because that is how core/blockstore
// asks for it: SearchText probes the Store for TextSearcher and scans when the
// probe fails. A SQLite-backed adapter therefore takes the scan, which is the
// right answer for it: SQLite's group_concat gives no ordering guarantee, and
// without document order a needle straddling two runs cannot be found at all.
type pgStore struct {
	*store
}

var _ blockstore.TextSearcher = (*pgStore)(nil)

// candidateLimit caps how many blocks the candidate query hands back before
// they are verified. It is deliberately generous: verification happens in Go
// over the same text the scan reads, so a low cap would silently lose
// occurrences rather than save work.
const candidateLimit = 5000

// SearchBlockText implements blockstore.TextSearcher over the stored runs.
//
// The query narrows; it does not decide. Every candidate is read back through
// the ordinary block-fetch path and verified against blockstore.BlockTexts —
// the one definition of "the text of a block" — so a hit carries the same text,
// under the same locale key, as the universal scan would report for it.
//
// What the query guarantees is the other direction: no block whose text
// contains the needle is missed. That rules out BlockQuery.Text, whose source
// side tests each run separately and so cannot see a needle straddling an
// inline-code boundary ("faithful **write-back**"), and whose target side is
// ORed with the source side, which would file a source-only match under a
// target locale.
func (s *pgStore) SearchBlockText(ctx context.Context, needle string, opts blockstore.TextSearchOptions) ([]blockstore.TextHit, error) {
	if needle == "" {
		return nil, nil
	}
	// Nil locales means every locale, so both sides are searched and neither
	// carries a locale list. A non-nil empty set asks for no locale at all, and
	// falls out of the three below as nothing to search.
	wantSource := opts.Locales == nil || slices.Contains(opts.Locales, blockstore.SourceLocale)
	anyTarget := opts.Locales == nil
	var wantTargets []string
	for _, l := range opts.Locales {
		if l != blockstore.SourceLocale {
			wantTargets = append(wantTargets, l)
		}
	}
	if !wantSource && !anyTarget && len(wantTargets) == 0 {
		return nil, nil
	}

	ids, collections, err := s.textCandidates(ctx, needle, opts.Collection, wantSource, anyTarget, wantTargets)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	rows, err := s.opts.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: s.opts.ProjectID,
		Stream:    s.opts.Stream,
		IDs:       ids,
	})
	if err != nil {
		return nil, fmt.Errorf("bowrain/blockstore: read text candidates: %w", err)
	}

	lowerNeedle := strings.ToLower(needle)
	var hits []blockstore.TextHit
	for _, sb := range rows {
		kb := toKBF(sb)
		if kb == nil {
			continue
		}
		texts := blockstore.BlockTexts(kb)
		// BlockTexts walks a map for the targets, so the order it returns them
		// in is not stable. Sorting by locale makes one corpus answer one query
		// the same way twice; the source locale is the empty string and sorts
		// to the front, where the scan puts it too.
		slices.SortFunc(texts, func(a, b blockstore.BlockText) int {
			return strings.Compare(a.Locale, b.Locale)
		})
		for _, bt := range texts {
			if opts.Locales != nil && !slices.Contains(opts.Locales, bt.Locale) {
				continue
			}
			if !strings.Contains(strings.ToLower(bt.Text), lowerNeedle) {
				continue
			}
			hits = append(hits, blockstore.TextHit{
				Hash:       kb.Hash,
				Collection: collections[sb.ID],
				Locale:     bt.Locale,
				Text:       bt.Text,
				Block:      kb,
			})
			if opts.Limit > 0 && len(hits) >= opts.Limit {
				return hits, nil
			}
		}
	}
	return hits, nil
}

// textCandidates returns the ids of the blocks whose text may hold the needle,
// and the collection each was stored under.
//
// Constant SQL fragments carrying $N tokens only; every value binds through
// args, including the collection name and the locale list.
func (s *pgStore) textCandidates(
	ctx context.Context,
	needle, collection string,
	wantSource, anyTarget bool,
	wantTargets []string,
) ([]string, map[string]string, error) {
	var args []any
	bind := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	project := bind(s.opts.ProjectID)
	stream := bind(s.opts.Stream)
	// One placeholder for the needle, referenced from every side of the OR.
	pin := bind(needle)

	var match []string
	if wantSource {
		src := pgRunArray("b.source_json::jsonb")
		match = append(match, pgTextMatch(src, pin))
	}
	if anyTarget || len(wantTargets) > 0 {
		tgt := pgRunArray("t.target_json->'runs'")
		where := []string{
			"t.project_id = b.project_id",
			"t.stream = " + stream,
			"t.block_id = b.id",
		}
		if !anyTarget {
			marks := make([]string, len(wantTargets))
			for i, l := range wantTargets {
				marks[i] = bind(l)
			}
			// The locale column holds the VariantKey text form, so a toned or
			// channelled target is filed under "nb;tone=formal". toKBF keys the
			// block's targets by the bare locale, and this must agree with it.
			where = append(where, "split_part(t.locale, ';', 1) IN ("+strings.Join(marks, ",")+")")
		}
		where = append(where, pgTextMatch(tgt, pin))
		match = append(match, "EXISTS (SELECT 1 FROM translations t WHERE "+strings.Join(where, " AND ")+")")
	}

	where := []string{
		"b.project_id = " + project,
		"(" + strings.Join(match, " OR ") + ")",
	}
	if collection != "" {
		where = append(where, "c.name = "+bind(collection))
	}

	// Blocks carry an item name, items carry the collection — the same two hops
	// the block iterator makes, done as a join so a hit can name its collection
	// without a second lookup. Both joins are LEFT: a block written without an
	// item still holds text worth finding, it just has no collection to report.
	q := `SELECT b.id, COALESCE(c.name, '')
		FROM blocks b
		LEFT JOIN items i ON i.project_id = b.project_id AND i.stream = ` + stream + ` AND i.name = b.item_name
		LEFT JOIN collections c ON c.project_id = b.project_id AND c.id = i.collection_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY b.id
		LIMIT ` + strconv.Itoa(candidateLimit)

	rows, err := s.opts.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("bowrain/blockstore: search block text: %w", err)
	}
	defer rows.Close()

	var ids []string
	collections := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, fmt.Errorf("bowrain/blockstore: scan text candidate: %w", err)
		}
		ids = append(ids, id)
		collections[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("bowrain/blockstore: iterate text candidates: %w", err)
	}
	return ids, collections, nil
}

// pgTextMatch is the candidate test for one run array: the needle is in the
// flattened text, or the array holds a construct the flattening cannot reach.
func pgTextMatch(arr, needle string) string {
	return fmt.Sprintf("(strpos(lower(%s), lower(%s)) > 0 OR %s)",
		pgFlatRuns(arr), needle, pgHasNestedRuns(arr))
}

// pgRunArray guards the expression as a jsonb array. A nil run slice marshals
// to JSON null, which jsonb_array_elements rejects, and a target with no runs
// at all leaves the key absent.
func pgRunArray(expr string) string {
	return `(CASE WHEN jsonb_typeof(` + expr + `) = 'array' THEN ` + expr + ` ELSE '[]'::jsonb END)`
}

// pgFlatRuns renders a run array's plain text, matching model.FlattenRuns for
// every run kind a flat expression can reach: text runs contribute their text,
// placeholders "{equiv}", subblock references "[equiv]", paired codes nothing.
// Text runs serialize flat — {"text":"literal"} — so the text is at ->>'text';
// every other kind nests under its own discriminator and yields NULL there.
//
// WITH ORDINALITY holds the runs in document order, and that is the whole point
// of aggregating rather than testing each run: "faithful write-back" written as
// text + bold + text lives in no single run, and a per-run test cannot find it.
func pgFlatRuns(arr string) string {
	return `(SELECT string_agg(CASE
			WHEN r->>'text' IS NOT NULL THEN r->>'text'
			WHEN r->'ph' IS NOT NULL THEN '{' || COALESCE(r->'ph'->>'equiv', '') || '}'
			WHEN r->'sub' IS NOT NULL THEN '[' || COALESCE(r->'sub'->>'equiv', '') || ']'
			ELSE '' END, '' ORDER BY ord)
		FROM jsonb_array_elements(` + arr + `) WITH ORDINALITY AS e(r, ord))`
}

// pgHasNestedRuns reports whether the array holds a plural or select run. Their
// branches sit a level below the flattening above, and flattening picks one
// branch by name, which is more than a single aggregate can express. A block
// holding one is therefore a candidate whatever the needle: this filter may
// over-return, and may not miss.
func pgHasNestedRuns(arr string) string {
	return `EXISTS (SELECT 1 FROM jsonb_array_elements(` + arr + `) AS n(r)
		WHERE r->'plural' IS NOT NULL OR r->'select' IS NOT NULL)`
}
