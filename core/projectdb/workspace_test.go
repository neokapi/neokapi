package projectdb_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/terms"
)

// openWorkspaceProject opens a checkout's store against a workspace, the way
// the host layer does.
func openWorkspaceProject(t *testing.T, ws *workspace.Workspace, root string, key workspace.ProjectKey) *projectdb.DB {
	t.Helper()
	ctx := t.Context()
	contextDB, err := ws.Context(ctx, key)
	require.NoError(t, err)
	db, err := projectdb.Open(ctx, project.LayoutAt(root), projectdb.WithWorkspace(projectdb.Stores{
		Context: contextDB,
		Graph:   ws.Registry(),
	}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// checkout scaffolds a project directory.
func checkout(t *testing.T, base, name string) string {
	t.Helper()
	root := filepath.Join(base, name)
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	return root
}

func TestEmbeddedLayoutKeepsBothPoolsInTheCheckout(t *testing.T) {
	root := checkout(t, t.TempDir(), "solo")
	db, err := projectdb.Open(t.Context(), project.LayoutAt(root))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	assert.Equal(t, db.Path(), db.ContextPath(),
		"with no workspace the context tables sit beside the projection")
	assert.Same(t, db.Projection(), db.Raw(), "and the two pools are one handle")
	assert.Same(t, db.Projection(), db.Graph(), "with the graph in it")
	assert.Equal(t, filepath.Join(root, project.StateDirName, project.WorkDirName, project.StoreFileName), db.Path())
}

func TestTwoCheckoutsShareTheContextStoreAndKeepTheirOwnProjection(t *testing.T) {
	base := t.TempDir()
	ws, err := workspace.OpenLocal(t.Context(), filepath.Join(base, "workspaces", "default"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })

	ctx := t.Context()
	first := openWorkspaceProject(t, ws, checkout(t, base, "clone-a"), "prj_shared")
	second := openWorkspaceProject(t, ws, checkout(t, base, "clone-b"), "prj_shared")

	assert.Equal(t, first.ContextPath(), second.ContextPath(), "one project, one context store")
	assert.NotEqual(t, first.Path(), second.Path(), "each checkout keeps its own projection")
	assert.NotSame(t, first.Projection(), first.Raw(), "the two pools are two handles")

	// Authored context written in one checkout is in force in the other.
	require.NoError(t, first.Terms().AddConcept(ctx, terms.Concept{
		ID:         "c1",
		Definition: "the project's own record of what it has said before",
		Terms:      []terms.Term{{Text: "content memory", Locale: "en", Status: model.TermPreferred}},
	}))
	has, err := second.HasTerms(ctx)
	require.NoError(t, err)
	assert.True(t, has, "the second checkout reads what the first authored")

	// What a checkout derived stays in that checkout.
	writeBlock(t, first, "docs", "k1_aaa", "The content memory holds what has been said.")
	firstHas, err := first.HasBlocks(ctx)
	require.NoError(t, err)
	assert.True(t, firstHas)
	secondHas, err := second.HasBlocks(ctx)
	require.NoError(t, err)
	assert.False(t, secondHas, "the second checkout's projection is its own")
}

func TestTwoProjectsGetTheirOwnContextStore(t *testing.T) {
	base := t.TempDir()
	ctx := t.Context()
	ws, err := workspace.OpenLocal(ctx, filepath.Join(base, "workspaces", "default"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })

	docs := openWorkspaceProject(t, ws, checkout(t, base, "docs"), "prj_docs")
	app := openWorkspaceProject(t, ws, checkout(t, base, "app"), "prj_app")
	assert.NotEqual(t, docs.ContextPath(), app.ContextPath())

	require.NoError(t, docs.Terms().AddConcept(ctx, terms.Concept{
		ID: "c1", Terms: []terms.Term{{Text: "voice profile", Locale: "en", Status: model.TermPreferred}},
	}))
	has, err := app.HasTerms(ctx)
	require.NoError(t, err)
	assert.False(t, has, "what one project learned is scoped to it")
}

func TestJoinAnswersTermToBlocksAcrossTheTwoFiles(t *testing.T) {
	base := t.TempDir()
	ctx := t.Context()
	ws, err := workspace.OpenLocal(ctx, filepath.Join(base, "workspaces", "default"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })

	db := openWorkspaceProject(t, ws, checkout(t, base, "docs"), "prj_join")
	require.NoError(t, db.Terms().AddConcept(ctx, terms.Concept{
		ID:         "c-memory",
		Definition: "the store of wording a project has approved",
		Terms:      []terms.Term{{Text: "content memory", Locale: "en", Status: model.TermPreferred}},
	}))
	writeBlock(t, db, "docs", "k1_uses", "Every run reads the content memory first.")
	writeBlock(t, db, "docs", "k1_other", "The recipe names the collections a project holds.")

	// The searchable text of a block is reconciled before a search rather than
	// maintained on every write, so the join follows a search the way a term
	// occurrence listing does.
	_, err = blockstore.SearchText(ctx, db.BlocksAutocommit(), "content memory", blockstore.TextSearchOptions{})
	require.NoError(t, err)

	var hashes []string
	require.NoError(t, db.Join(ctx, func(ctx context.Context, conn *sql.Conn) error {
		rows, err := conn.QueryContext(ctx, `
			SELECT DISTINCT bt.block_hash
			FROM block_texts bt
			JOIN `+projectdb.ContextSchema+`.tb_terms t
			  ON instr(bt.text_lower, t.text_lower) > 0
			WHERE t.concept_id = ?
			ORDER BY bt.block_hash`, "c-memory")
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var h string
			if err := rows.Scan(&h); err != nil {
				return err
			}
			hashes = append(hashes, h)
		}
		return rows.Err()
	}))
	assert.Equal(t, []string{"k1_uses"}, hashes,
		"the term-to-blocks join reaches both files in one query")
}

func TestJoinLeavesTheConnectionUsable(t *testing.T) {
	base := t.TempDir()
	ctx := t.Context()
	ws, err := workspace.OpenLocal(ctx, filepath.Join(base, "workspaces", "default"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	db := openWorkspaceProject(t, ws, checkout(t, base, "docs"), "prj_reuse")

	for range 3 {
		require.NoError(t, db.Join(ctx, func(ctx context.Context, conn *sql.Conn) error {
			var n int
			return conn.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM `+projectdb.ContextSchema+`.tb_concepts`).Scan(&n)
		}), "a joined connection is detached and unsealed on the way out")
	}

	// The pool still writes: the connection the join borrowed went back clean.
	writeBlock(t, db, "docs", "k1_after", "A write after a join still lands.")
	has, err := db.HasBlocks(ctx)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestJoinRefusesAWrite(t *testing.T) {
	base := t.TempDir()
	ctx := t.Context()
	ws, err := workspace.OpenLocal(ctx, filepath.Join(base, "workspaces", "default"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	db := openWorkspaceProject(t, ws, checkout(t, base, "docs"), "prj_sealed")

	err = db.Join(ctx, func(ctx context.Context, conn *sql.Conn) error {
		_, execErr := conn.ExecContext(ctx, `INSERT INTO store_meta (key, value) VALUES ('x', 'y')`)
		return execErr
	})
	require.Error(t, err, "a join reads; a write through it is refused")
}

func TestAdoptionCarriesStagedDecisions(t *testing.T) {
	base := t.TempDir()
	ctx := t.Context()
	root := checkout(t, base, "adopted")

	// A project living in the embedded layout, with a decision staged and not
	// yet committed to `.kapi/state/`.
	embedded, err := projectdb.Open(ctx, project.LayoutAt(root))
	require.NoError(t, err)
	staged := state.UnitState{
		Unit:        "u1",
		Variant:     model.Variant("nb"),
		Status:      model.TargetStatusReviewed,
		Scope:       "docs/index.md",
		TargetHash:  "t1_abc",
		ContentHash: "k1_abc",
	}
	require.NoError(t, embedded.Work().Put(ctx, staged))
	writeBlock(t, embedded, "docs", "k1_abc", "A decision was recorded against this wording.")
	require.NoError(t, embedded.Terms().AddConcept(ctx, terms.Concept{
		ID: "c1", Terms: []terms.Term{{Text: "content memory", Locale: "en", Status: model.TermPreferred}},
	}))
	require.NoError(t, embedded.Close())

	// The same checkout, opened against a workspace for the first time.
	ws, err := workspace.OpenLocal(ctx, filepath.Join(base, "workspaces", "default"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	adopted := openWorkspaceProject(t, ws, root, "prj_adopted")

	carried, err := adopted.Work().Staged(ctx)
	require.NoError(t, err)
	require.Len(t, carried, 1, "the staged decision crossed into the context store")
	assert.Equal(t, "u1", carried[0].Unit)
	assert.Equal(t, "t1_abc", carried[0].TargetHash)

	// The projection kept what it derived and lost what moved out.
	hasBlocks, err := adopted.HasBlocks(ctx)
	require.NoError(t, err)
	assert.True(t, hasBlocks, "the block cache stays in the checkout")

	var residue int
	require.NoError(t, adopted.Projection().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('unit_state', 'tb_concepts', 'tm_entries', 'voice_profiles')`,
	).Scan(&residue))
	assert.Zero(t, residue, "the context tables left the projection")

	// Re-opening finds nothing to adopt and changes nothing.
	require.NoError(t, adopted.Close())
	again := openWorkspaceProject(t, ws, root, "prj_adopted")
	stillStaged, err := again.Work().Staged(ctx)
	require.NoError(t, err)
	assert.Len(t, stillStaged, 1, "adoption runs once and is not repeated")
}

func TestAdoptionLeavesAProjectionWithNoContextTablesAlone(t *testing.T) {
	base := t.TempDir()
	ctx := t.Context()
	ws, err := workspace.OpenLocal(ctx, filepath.Join(base, "workspaces", "default"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })

	db := openWorkspaceProject(t, ws, checkout(t, base, "fresh"), "prj_fresh")
	writeBlock(t, db, "docs", "k1_fresh", "A project that never lived in the embedded layout.")

	has, err := db.HasBlocks(ctx)
	require.NoError(t, err)
	assert.True(t, has)
	staged, err := db.Work().Staged(ctx)
	require.NoError(t, err)
	assert.Empty(t, staged)
}

// writeBlock puts one block into a store's block cache, in its own session.
func writeBlock(t *testing.T, db *projectdb.DB, collection, hash, text string) {
	t.Helper()
	sess, err := db.Blocks().Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock(collection, &blockstore.Block{
		ID:           hash,
		Hash:         hash,
		Translatable: true,
		Source:       []model.Run{model.TextR(text)},
		Properties:   model.BlockProperties{File: collection + "/index.md"},
	}))
	require.NoError(t, sess.Commit())
}
