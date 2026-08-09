package store_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
)

// blockQueryStore is the slice of ContentStore these cases exercise, so the
// same scenario runs against both engines: the SQL that answers a filtered
// page and the SQL that counts it must agree in each, and with each other.
type blockQueryStore interface {
	CreateProject(ctx context.Context, p *platstore.Project) error
	StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error
	GetBlocks(ctx context.Context, query platstore.BlockQuery) ([]*platstore.StoredBlock, error)
	CountBlocks(ctx context.Context, query platstore.BlockQuery) (platstore.BlockCounts, error)
}

// seedBlockQueryProject stores one block per bucket of the editor's status
// vocabulary, in two items, and returns the project id.
func seedBlockQueryProject(t *testing.T, s blockQueryStore) string {
	t.Helper()
	ctx := t.Context()
	proj := &platstore.Project{
		ID: "p-query", Name: "Query", DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"nb"},
	}
	require.NoError(t, s.CreateProject(ctx, proj))

	next := 0
	target := func(source, text string, status model.TargetStatus) *model.Block {
		next++
		b := model.NewBlock(fmt.Sprintf("tu%d", next), source)
		b.Translatable = true
		if text != "" {
			b.SetTargetText("nb", text)
			b.StampTargetProvenance("nb", status, model.Origin{Kind: model.OriginAI})
		}
		return b
	}

	draft := target("Hello world", "Hei verden", model.TargetStatusDraft)
	translated := target("Goodbye", "Ha det", model.TargetStatusTranslated)
	reviewed := target("Approved text", "Godkjent", model.TargetStatusReviewed)
	signedOff := target("Signed text", "Signert", model.TargetStatusSignedOff)
	untranslated := target("Nothing yet", "", model.TargetStatusNew)
	// Text with no committed rung is translated — the editor's fallback when
	// no legacy property and no machine provenance says otherwise.
	rungless := target("No rung", "Uten trinn", model.TargetStatusNew)
	// Machine provenance with no rung is a draft, by the same fallback.
	machine := target("Machine made", "Maskin", model.TargetStatusNew)
	machine.Properties = map[string]string{"translation-origin": "machine"}

	frozen := target("Frozen source", "", model.TargetStatusNew)
	frozen.Translatable = false

	require.NoError(t, s.StoreBlocksForItem(ctx, proj.ID, "main", "a.md",
		[]*model.Block{draft, translated, reviewed, signedOff}))
	require.NoError(t, s.StoreBlocksForItem(ctx, proj.ID, "main", "b.md",
		[]*model.Block{untranslated, rungless, machine, frozen}))
	return proj.ID
}

// sourcesOf names the matched blocks by their source text, the only stable
// coordinate across stores (both mint their own block ids).
func sourcesOf(blocks []*platstore.StoredBlock) []string {
	out := make([]string, 0, len(blocks))
	for _, sb := range blocks {
		out = append(out, sb.Block.SourceText())
	}
	return out
}

func runBlockQueryCases(t *testing.T, s blockQueryStore) {
	t.Helper()
	pid := seedBlockQueryProject(t, s)
	ctx := t.Context()

	t.Run("status buckets partition the translatable blocks", func(t *testing.T) {
		for bucket, want := range map[string][]string{
			platstore.BlockStatusNotStarted: {"Nothing yet"},
			platstore.BlockStatusDraft:      {"Hello world", "Machine made"},
			platstore.BlockStatusTranslated: {"Goodbye", "No rung"},
			platstore.BlockStatusReviewed:   {"Approved text", "Signed text"},
		} {
			tr := true
			got, err := s.GetBlocks(ctx, platstore.BlockQuery{
				ProjectID: pid, Stream: "main", TargetLocale: "nb", Status: bucket, Translatable: &tr,
			})
			require.NoError(t, err)
			assert.ElementsMatch(t, want, sourcesOf(got), "bucket %q", bucket)
		}
	})

	t.Run("counts are the histogram, not a filtered page", func(t *testing.T) {
		counts, err := s.CountBlocks(ctx, platstore.BlockQuery{
			ProjectID: pid, Stream: "main", TargetLocale: "nb",
		})
		require.NoError(t, err)
		assert.Equal(t, platstore.BlockCounts{
			Total: 8, Translatable: 7,
			NotStarted: 1, Draft: 2, Translated: 2, Reviewed: 2,
		}, counts)
		assert.Equal(t, counts.Translatable,
			counts.NotStarted+counts.Draft+counts.Translated+counts.Reviewed,
			"the buckets must partition the translatable blocks")
	})

	t.Run("counts respect the item filter", func(t *testing.T) {
		counts, err := s.CountBlocks(ctx, platstore.BlockQuery{
			ProjectID: pid, Stream: "main", ItemName: "a.md", TargetLocale: "nb",
		})
		require.NoError(t, err)
		assert.Equal(t, platstore.BlockCounts{
			Total: 4, Translatable: 4, Draft: 1, Translated: 1, Reviewed: 2,
		}, counts)
	})

	t.Run("counts of an empty match are zeros, not an error", func(t *testing.T) {
		counts, err := s.CountBlocks(ctx, platstore.BlockQuery{
			ProjectID: pid, Stream: "main", ItemName: "absent.md", TargetLocale: "nb",
		})
		require.NoError(t, err)
		assert.Equal(t, platstore.BlockCounts{}, counts)
	})

	t.Run("text search covers source and target, case-insensitively", func(t *testing.T) {
		cases := map[string][]string{
			"WORLD":   {"Hello world"}, // source side
			"hei":     {"Hello world"}, // target side, folded
			"godkjen": {"Approved text"},
			"nothing": {"Nothing yet"},
		}
		for q, want := range cases {
			got, err := s.GetBlocks(ctx, platstore.BlockQuery{
				ProjectID: pid, Stream: "main", TargetLocale: "nb", Text: q,
			})
			require.NoError(t, err)
			assert.ElementsMatch(t, want, sourcesOf(got), "q=%q", q)
		}
	})

	t.Run("text search matches run text, never JSON punctuation", func(t *testing.T) {
		got, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: pid, Stream: "main", TargetLocale: "nb", Text: `{"text"`,
		})
		require.NoError(t, err)
		assert.Empty(t, sourcesOf(got))
	})

	t.Run("counts follow the text filter", func(t *testing.T) {
		counts, err := s.CountBlocks(ctx, platstore.BlockQuery{
			ProjectID: pid, Stream: "main", TargetLocale: "nb", Text: "text",
		})
		require.NoError(t, err)
		assert.Equal(t, platstore.BlockCounts{Total: 2, Translatable: 2, Reviewed: 2}, counts,
			"only 'Approved text' and 'Signed text' carry the substring")
	})

	t.Run("a status filter without a locale is inert", func(t *testing.T) {
		got, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: pid, Stream: "main", Status: platstore.BlockStatusReviewed,
		})
		require.NoError(t, err)
		assert.Len(t, got, 8, "no locale means no per-locale row to test")
	})

	t.Run("paging holds the filtered order", func(t *testing.T) {
		all, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: pid, Stream: "main", TargetLocale: "nb", Status: platstore.BlockStatusReviewed,
		})
		require.NoError(t, err)
		require.Len(t, all, 2)
		page, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: pid, Stream: "main", TargetLocale: "nb", Status: platstore.BlockStatusReviewed,
			Limit: 1, Offset: 1,
		})
		require.NoError(t, err)
		require.Len(t, page, 1)
		assert.Equal(t, all[1].Block.SourceText(), page[0].Block.SourceText())
	})
}

// The editor filters and counts blocks server-side: one query answers the
// progress bar the surfaces used to derive by downloading every block.
func TestBlockQueryFilters_SQLite(t *testing.T) {
	s, err := sqlitestore.NewSQLiteStore(t.TempDir() + "/store.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	runBlockQueryCases(t, s)
}

func TestBlockQueryFilters_Postgres(t *testing.T) {
	db := pgtest.NewTestDB(t)
	s, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	runBlockQueryCases(t, s)
}
