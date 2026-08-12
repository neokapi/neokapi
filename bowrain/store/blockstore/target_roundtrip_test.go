package blockstore_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	corestore "github.com/neokapi/neokapi/bowrain/store"
	bwblockstore "github.com/neokapi/neokapi/bowrain/store/blockstore"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// A target written through the block store's overlay API and a target written
// through the block round-trip are the same target: one row, one set of
// columns, one key. These fixtures drive the whole verb on both backends — a
// server-side pass commits a target, then every reader that derives state from
// it is asked what it sees, and the reverse leg asks the overlay API what the
// editor's write looks like.
//
// Postgres is the deployed shape and SQLite is what a self-hosted instance and
// the fast lane run; the pair runs on both because the divergence this pins was
// two SQL statements, and a statement can drift on one dialect alone.

// backend is one store the adapter sits on.
type backend struct {
	name      string
	content   platstore.ContentStore
	db        bwblockstore.DB
	dialect   bwblockstore.Dialect
	projectID string
}

func (b backend) blocks(t *testing.T) blockstore.Store {
	t.Helper()
	bs, err := bwblockstore.New(bwblockstore.Options{
		ContentStore: b.content,
		DB:           b.db,
		Dialect:      b.dialect,
		ProjectID:    b.projectID,
		Stream:       "main",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = bs.Close() })
	return bs
}

func (b backend) dialectName() string {
	if b.dialect == bwblockstore.SQLiteDialect {
		return "sqlite"
	}
	return "pg"
}

// newSQLiteBackend stands up a throwaway SQLite content store with one project.
func newSQLiteBackend(t *testing.T) backend {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "roundtrip.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.DB().Close() })
	projectID := "proj-roundtrip"
	require.NoError(t, cs.CreateProject(context.Background(), &platstore.Project{
		ID:                    projectID,
		Name:                  "Roundtrip",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr", "de"},
	}))
	return backend{name: "sqlite", content: cs, db: cs.DB(), dialect: bwblockstore.SQLiteDialect, projectID: projectID}
}

// newPostgresBackend stands up a container-backed Postgres content store with
// one project. Skips in -short unless a ready instance is supplied.
func newPostgresBackend(t *testing.T) backend {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := corestore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	p := &platstore.Project{
		Name:                  "Roundtrip",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr", "de"},
	}
	require.NoError(t, cs.CreateProject(context.Background(), p))
	return backend{name: "postgres", content: cs, db: cs.SQLDB(), dialect: bwblockstore.PostgresDialect, projectID: p.ID}
}

// eachBackend runs fn against every backend the adapter supports.
func eachBackend(t *testing.T, fn func(t *testing.T, b backend)) {
	t.Helper()
	for _, mk := range []struct {
		name string
		make func(*testing.T) backend
	}{
		{"sqlite", newSQLiteBackend},
		{"postgres", newPostgresBackend},
	} {
		t.Run(mk.name, func(t *testing.T) { fn(t, mk.make(t)) })
	}
}

// seedItemBlock stores one block inside a named item, the shape a real project
// holds: the item names the document, the block's own id is the durable unit
// identity a decision addresses it by.
func seedItemBlock(t *testing.T, b backend, item, unit, text string) *platstore.StoredBlock {
	t.Helper()
	ctx := context.Background()
	mb := model.NewRunsBlock(unit, []model.Run{{Text: &model.TextRun{Text: text}}})
	mb.Translatable = true
	require.NoError(t, b.content.StoreBlocksForItem(ctx, b.projectID, "main", item, []*model.Block{mb}))
	rows, err := b.content.GetBlocks(ctx, platstore.BlockQuery{ProjectID: b.projectID, Stream: "main", ItemName: item})
	require.NoError(t, err)
	for _, sb := range rows {
		if sb.SourceID == unit || sb.ID == unit {
			return sb
		}
	}
	t.Fatalf("seeded block %q not found in item %q", unit, item)
	return nil
}

// TestOverlayWrite_EveryReaderSeesTheTarget drives the forward leg: a pass
// commits a `targets/<locale>` overlay through the Store API, and each reader
// that derives project state from a stored target is asked what it holds.
//
// Before the write and read agreed on one row, every assertion below failed:
// the write landed in columns the readers do not consult, under a key they do
// not join on, so the block came back untranslated and the absence read as
// ordinary pending work.
func TestOverlayWrite_EveryReaderSeesTheTarget(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		block := seedItemBlock(t, b, "greetings.json", "tu1", "Hello")

		sess, err := b.blocks(t).Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind:      "targets/fr",
			BlockHash: block.ContentHash,
			Payload:   []byte(`{"runs":[{"text":"Bonjour"}],"status":"translated","origin":{"kind":"mt","engine":"demo"}}`),
		}))
		require.NoError(t, sess.Commit())

		t.Run("editor fetch", func(t *testing.T) {
			rows, err := b.content.GetBlocks(ctx, platstore.BlockQuery{
				ProjectID: b.projectID, Stream: "main", IDs: []string{block.ID},
			})
			require.NoError(t, err)
			require.Len(t, rows, 1)
			target := rows[0].Block.Target("fr")
			require.NotNil(t, target, "block came back with no fr target")
			require.Equal(t, "Bonjour", model.RunsText(target.Runs))
			require.Equal(t, model.TargetStatusTranslated, target.Status)
			require.Equal(t, "mt", target.Origin.Kind)
		})

		t.Run("coverage derivation", func(t *testing.T) {
			locales, err := corestore.LoadBlockTargetLocales(ctx, b.db, b.dialectName(),
				b.projectID, "main", []string{block.ID})
			require.NoError(t, err)
			require.Equal(t, []string{"fr"}, locales[block.ID])
		})

		t.Run("ship state", func(t *testing.T) {
			states, err := corestore.LoadBlockTargetStates(ctx, b.db, b.dialectName(),
				b.projectID, "main", []string{block.ID})
			require.NoError(t, err)
			require.Equal(t,
				[]corestore.TargetLocaleState{{Locale: "fr", Status: model.TargetStatusTranslated}},
				states[block.ID])
		})

		t.Run("review queue", func(t *testing.T) {
			refs, total, err := b.content.ListPendingReview(ctx, platstore.PendingReviewQuery{
				ProjectID: b.projectID, Stream: "main", Locales: []string{"fr"},
			})
			require.NoError(t, err)
			require.Equal(t, 1, total)
			require.Len(t, refs, 1)
			require.Equal(t, block.ID, refs[0].BlockID)
		})

		t.Run("decision basis", func(t *testing.T) {
			decisions, ok := b.content.(platstore.DecisionStore)
			require.True(t, ok)
			n, err := decisions.UpsertUnitDecisions(ctx, b.projectID, "main", []platstore.UnitDecision{{
				ItemName:    "greetings.json",
				Unit:        block.SourceID,
				Variant:     "fr",
				Status:      string(model.TargetStatusReviewed),
				TargetHash:  state.TargetHash("Bonjour"),
				ReviewState: "approved",
			}})
			require.NoError(t, err)
			require.Equal(t, 1, n)

			// The ledger blesses the translation it names, and the projection
			// lands on the stored target — which it can only find if the
			// decision and the write address one row.
			states, err := corestore.LoadBlockTargetStates(ctx, b.db, b.dialectName(),
				b.projectID, "main", []string{block.ID})
			require.NoError(t, err)
			require.Equal(t,
				[]corestore.TargetLocaleState{{Locale: "fr", Status: model.TargetStatusReviewed}},
				states[block.ID])
		})
	})
}

// TestEditorWrite_OverlayReadSeesTheTarget drives the return leg: the editor
// (and every server path that writes a block) commits a target through the
// block round-trip, and the overlay API — the sync/pull side and any flow the
// server runs in-process — reads it back with its runs and lifecycle intact.
//
// The flattened text alone is not the target: it drops inline codes, so a leg
// that read only that column shipped a target with its markup stripped.
func TestEditorWrite_OverlayReadSeesTheTarget(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		block := seedItemBlock(t, b, "greetings.json", "tu1", "Hello there")

		edited := model.NewRunsBlock(block.ID, []model.Run{{Text: &model.TextRun{Text: "Hello there"}}})
		edited.Translatable = true
		edited.SetTargetRuns("de", []model.Run{
			{Text: &model.TextRun{Text: "Hallo "}},
			{Ph: &model.PlaceholderRun{ID: "p1", Equiv: "{name}"}},
		})
		edited.StampTargetProvenance("de", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
		require.NoError(t, b.content.StoreBlocksForItem(ctx, b.projectID, "main", "greetings.json", []*model.Block{edited}))

		sess, err := b.blocks(t).Begin(ctx)
		require.NoError(t, err)
		defer sess.Close()

		got, err := sess.GetOverlay("targets/de", block.ContentHash)
		require.NoError(t, err, "the editor's target is invisible to the overlay read")

		var target model.Target
		require.NoError(t, json.Unmarshal(got.Payload, &target))
		require.Len(t, target.Runs, 2, "the target came back without its placeholder")
		require.Equal(t, "Hallo ", target.Runs[0].Text.Text)
		require.NotNil(t, target.Runs[1].Ph)
		require.Equal(t, "{name}", target.Runs[1].Ph.Equiv)
		require.Equal(t, model.TargetStatusReviewed, target.Status)
		require.Equal(t, model.OriginHuman, target.Origin.Kind)

		// And the listing agrees with the single read, under the key the block
		// iterator reports.
		var listed []blockstore.Overlay
		for o, err := range sess.ListOverlays("targets/de") {
			require.NoError(t, err)
			listed = append(listed, o)
		}
		require.Len(t, listed, 1)
		require.Equal(t, block.ContentHash, listed[0].BlockHash)
		require.JSONEq(t, string(got.Payload), string(listed[0].Payload))
	})
}
