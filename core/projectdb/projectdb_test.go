package projectdb_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// ledgerTables are the per-subsystem migration ledgers that must coexist in the
// merged file. Their names are persisted state, not internal identifiers: each
// one is written into every database the subsystem ever created, so a rename
// makes Migrate replay every migration against a populated schema. This list
// asserts they came along unchanged.
var ledgerTables = []string{
	"projectdb_migrations", // store metadata (this package)
	"sievepen_migrations",  // content memory
	"termbase_migrations",  // terms store
	"cache_migrations",     // block cache
	"state",                // unit working set
}

func newLayout(t *testing.T) project.Layout {
	t.Helper()
	root := t.TempDir()
	return project.Layout{
		Root:       root,
		RecipePath: filepath.Join(root, project.RecipeFileName),
		StateDir:   filepath.Join(root, project.StateDirName),
	}
}

func openStore(t *testing.T, layout project.Layout) *projectdb.DB {
	t.Helper()
	db, err := projectdb.Open(t.Context(), layout)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func tableExists(t *testing.T, db *projectdb.DB, name string) bool {
	t.Helper()
	var n int
	require.NoError(t, db.Raw().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n))
	return n == 1
}

func appliedMigrations(t *testing.T, db *projectdb.DB, ledger string) int {
	t.Helper()
	var n int
	require.NoError(t, db.Raw().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+ledger).Scan(&n))
	return n
}

// One open must leave every subsystem migrated. The whole point of the merged
// store is that no caller sequences four opens in the right order.
func TestOpen_AllSubsystemsMigrateInOneFile(t *testing.T) {
	layout := newLayout(t)
	db := openStore(t, layout)

	assert.FileExists(t, layout.StorePath(), "the store sits at the top of the state dir")
	assert.Equal(t, layout.StorePath(), db.Path())

	for _, ledger := range ledgerTables {
		assert.True(t, tableExists(t, db, ledger), "ledger %s is present", ledger)
		assert.Positive(t, appliedMigrations(t, db, ledger), "ledger %s recorded its migrations", ledger)
	}

	// The subsystems' own tables, not just their bookkeeping.
	for _, table := range []string{"store_meta", "tm_entries", "tb_concepts", "blocks", "overlays", "unit_state", "document"} {
		assert.True(t, tableExists(t, db, table), "table %s is present", table)
	}

	require.NotNil(t, db.Memory())
	require.NotNil(t, db.Terms())
	require.NotNil(t, db.Blocks())
	require.NotNil(t, db.BlocksAutocommit())
	require.NotNil(t, db.Work())
	require.NotNil(t, db.Raw())
}

// Reopening applies nothing further and loses nothing. A ledger that replayed
// would be the symptom of a renamed bookkeeping table.
func TestOpen_ReopenIsIdempotent(t *testing.T) {
	layout := newLayout(t)

	first, err := projectdb.Open(t.Context(), layout)
	require.NoError(t, err)

	before := map[string]int{}
	for _, ledger := range ledgerTables {
		before[ledger] = appliedMigrations(t, first, ledger)
	}
	require.NoError(t, first.Memory().Add(t.Context(), entry("e-persist", "Hello", "Hei")))
	require.NoError(t, first.Close())

	second := openStore(t, layout)
	for _, ledger := range ledgerTables {
		assert.Equal(t, before[ledger], appliedMigrations(t, second, ledger),
			"reopening replayed migrations in %s", ledger)
	}

	got, found, err := second.Memory().GetEntry(t.Context(), "e-persist")
	require.NoError(t, err)
	require.True(t, found, "content written before the reopen survived it")
	assert.Equal(t, "Hei", got.VariantText("nb"))
}

func TestOpen_RejectsLayoutWithoutStateDir(t *testing.T) {
	_, err := projectdb.Open(t.Context(), project.Layout{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state directory")
}

func entry(id, en, nb string) memory.Entry {
	return memory.Entry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR(en)},
			"nb": {model.TextR(nb)},
		},
		HintSrcLang: "en",
	}
}

// Each subsystem must behave through the shared pool exactly as it does through
// its own file — the migrations are the same, so the round trips must be too.
func TestSubsystems_RoundTripThroughSharedPool(t *testing.T) {
	db := openStore(t, newLayout(t))
	ctx := t.Context()

	t.Run("memory", func(t *testing.T) {
		require.NoError(t, db.Memory().Add(ctx, entry("e1", "Save changes", "Lagre endringer")))

		got, found, err := db.Memory().GetEntry(ctx, "e1")
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "Save changes", got.VariantText("en"))
		assert.Equal(t, "Lagre endringer", got.VariantText("nb"))

		matches, err := db.Memory().LookupText(ctx, "Save changes", "en", "nb", memory.LookupOptions{})
		require.NoError(t, err)
		require.NotEmpty(t, matches, "the exact-match tier resolves through the shared pool")
		assert.True(t, matches[0].MatchType.IsExact())
	})

	t.Run("terms", func(t *testing.T) {
		require.NoError(t, db.Terms().AddConcept(ctx, terms.Concept{
			ID:         "c1",
			Definition: "A saved revision of a document",
			Terms: []terms.Term{
				{Text: "revision", Locale: "en", Status: model.TermPreferred},
				{Text: "revisjon", Locale: "nb", Status: model.TermPreferred},
			},
		}))

		got, found, err := db.Terms().GetConcept(ctx, "c1")
		require.NoError(t, err)
		require.True(t, found)
		require.NotNil(t, got.PreferredTerm("nb"))
		assert.Equal(t, "revisjon", got.PreferredTerm("nb").Text)

		found2, total, err := db.Terms().Search(ctx, "revision", "en", "nb", 0, 10)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, found2, 1)
		assert.Equal(t, "c1", found2[0].ID)
	})

	t.Run("blocks", func(t *testing.T) {
		sess, err := db.Blocks().Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, sess.PutBlock("docs", &blockstore.Block{
			ID:           "b1",
			Hash:         "h-b1",
			Translatable: true,
			Source:       []model.Run{model.TextR("Save changes")},
		}))
		require.NoError(t, sess.Commit())

		read, err := db.Blocks().Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = read.Rollback() }()
		got, err := read.GetBlock("h-b1")
		require.NoError(t, err)
		assert.Equal(t, "b1", got.ID)
		assert.Equal(t, "Save changes", model.FlattenRuns(got.Source))
	})

	t.Run("blocks autocommit", func(t *testing.T) {
		sess, err := db.BlocksAutocommit().Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind: "targets", BlockHash: "h-b1", Payload: []byte(`{"nb":"Lagre endringer"}`),
		}))
		// No Commit: an autocommit session's writes are durable when they return.
		other, err := db.Blocks().Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = other.Rollback() }()
		got, err := other.GetOverlay("targets", "h-b1")
		require.NoError(t, err)
		assert.JSONEq(t, `{"nb":"Lagre endringer"}`, string(got.Payload))
	})

	t.Run("work", func(t *testing.T) {
		require.NoError(t, db.Work().Put(ctx, unit("u1", "d-intro", "Save changes")))

		pending, err := db.Work().Pending(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, pending)

		require.NoError(t, db.Work().Commit(ctx))

		committed, err := state.ReadCommitted(db.Layout().DecisionsDir())
		require.NoError(t, err)
		require.Len(t, committed, 1, "Commit wrote the committed shard")
		assert.Equal(t, "u1", committed[0].Unit)

		pending, err = db.Work().Pending(ctx)
		require.NoError(t, err)
		assert.Zero(t, pending)
	})
}

func unit(id, scope, text string) state.UnitState {
	return state.UnitState{
		Unit:        id,
		Variant:     model.Variant("nb"),
		Scope:       scope,
		ContentHash: model.ComputeContentHash(text),
		Decision:    state.Decision{ReviewState: "approved"},
	}
}

// Presence is a row question. A fresh store has every table and no content, so
// a file-existence signal would have reported all three as present.
func TestPresence_FalseOnFreshStoreTrueAfterWrites(t *testing.T) {
	db := openStore(t, newLayout(t))
	ctx := t.Context()

	probes := []struct {
		name string
		has  func(context.Context) (bool, error)
		fill func(*testing.T)
	}{
		{
			name: "memory",
			has:  db.HasMemory,
			fill: func(t *testing.T) {
				require.NoError(t, db.Memory().Add(ctx, entry("p1", "One", "En")))
			},
		},
		{
			name: "terms",
			has:  db.HasTerms,
			fill: func(t *testing.T) {
				require.NoError(t, db.Terms().AddConcept(ctx, terms.Concept{
					ID:    "p-c1",
					Terms: []terms.Term{{Text: "one", Locale: "en", Status: model.TermPreferred}},
				}))
			},
		},
		{
			name: "blocks",
			has:  db.HasBlocks,
			fill: func(t *testing.T) {
				sess, err := db.Blocks().Begin(ctx)
				require.NoError(t, err)
				require.NoError(t, sess.PutBlock("docs", &blockstore.Block{Hash: "p-h1", Source: []model.Run{model.TextR("One")}}))
				require.NoError(t, sess.Commit())
			},
		},
	}

	for _, p := range probes {
		t.Run(p.name, func(t *testing.T) {
			has, err := p.has(ctx)
			require.NoError(t, err)
			assert.False(t, has, "a fresh store holds no %s content", p.name)

			p.fill(t)

			has, err = p.has(ctx)
			require.NoError(t, err)
			assert.True(t, has, "%s reports present once written", p.name)
		})
	}
}

// The reason the four files became one: a decision and the wording it blesses
// are written by one transaction, and a rollback takes both or neither.
func TestRaw_CrossSubsystemTransaction(t *testing.T) {
	db := openStore(t, newLayout(t))
	ctx := t.Context()

	t.Run("commit takes both", func(t *testing.T) {
		tx, err := db.Raw().BeginTx(ctx, nil)
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx,
			`INSERT INTO tm_entries (id, project_id, hint_src_lang, created_at, updated_at, has_codes)
			 VALUES (?, '', 'en', datetime('now'), datetime('now'), 0)`, "x1")
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
INSERT INTO unit_state (unit, variant, scope, content_hash, context_hash, payload, staged)
VALUES (?, 'nb', 'd-intro', '', '', ?, 1)`, "x1", `{"unit":"x1","variant":"nb"}`)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		hasMem, err := db.HasMemory(ctx)
		require.NoError(t, err)
		assert.True(t, hasMem)
		pending, err := db.Work().Pending(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, pending)
	})

	t.Run("rollback takes neither", func(t *testing.T) {
		tx, err := db.Raw().BeginTx(ctx, nil)
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx,
			`INSERT INTO tm_entries (id, project_id, hint_src_lang, created_at, updated_at, has_codes)
			 VALUES (?, '', 'en', datetime('now'), datetime('now'), 0)`, "x2")
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
INSERT INTO unit_state (unit, variant, scope, content_hash, context_hash, payload, staged)
VALUES (?, 'nb', 'd-intro', '', '', ?, 1)`, "x2", `{"unit":"x2","variant":"nb"}`)
		require.NoError(t, err)
		require.NoError(t, tx.Rollback())

		_, found, err := db.Memory().GetEntry(ctx, "x2")
		require.NoError(t, err)
		assert.False(t, found, "the rolled-back entry is not in the content memory")
		_, ok := db.Work().Get(ctx, state.Key{Unit: "x2", Variant: model.Variant("nb")})
		assert.False(t, ok, "nor is the decision in the working set")
	})
}

// Close releases the pool once, for all four subsystems, and tolerates a second
// call.
func TestClose_IsIdempotent(t *testing.T) {
	layout := newLayout(t)
	db, err := projectdb.Open(t.Context(), layout)
	require.NoError(t, err)

	require.NoError(t, db.Close())
	require.NoError(t, db.Close())
	assert.Nil(t, db.Raw())

	// The file is intact and reopens.
	again := openStore(t, layout)
	assert.NotNil(t, again.Raw())
}

// The block cache is two tables, and a flow run writes only the second: it
// persists the overlays it produced without caching the blocks it parsed. So
// "has this been extracted?" and "is there anything here worth carrying?" are
// different questions, and pack must ask the second — under the four-file
// layout a run-only project still had a blocks.db on disk and packed fine.
func TestHasBlockCache_TrueForOverlaysAlone(t *testing.T) {
	db := openStore(t, newLayout(t))
	ctx := t.Context()

	sess, err := db.Blocks().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{
		Kind: "targets/nb", BlockHash: "p-h2", Payload: []byte(`{"text":"Hei"}`),
	}))
	require.NoError(t, sess.Commit())

	blocks, err := db.HasBlocks(ctx)
	require.NoError(t, err)
	assert.False(t, blocks, "overlays alone must not read as extracted blocks")

	cache, err := db.HasBlockCache(ctx)
	require.NoError(t, err)
	assert.True(t, cache, "overlays alone are still content the block cache holds")
}
