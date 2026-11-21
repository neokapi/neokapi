//go:build integration

package migrations_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
	pgterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/termscope"
	"github.com/neokapi/neokapi/terms"
)

func databaseBeforePilotUpgrade(t *testing.T) *storage.PgDB {
	t.Helper()
	return databaseBeforeTermsVersion(t, 11)
}

// databaseBeforeTermsVersion is a fresh database with every system migrated,
// except terms, which stops before version.
func databaseBeforeTermsVersion(t *testing.T, version int) *storage.PgDB {
	t.Helper()
	db := freshDatabase(t)
	for _, system := range migrations.All() {
		versions := system.Migrations
		if system.Name == "terms" {
			for i, m := range versions {
				if m.Version >= version {
					versions = versions[:i]
					break
				}
			}
		}
		require.NoError(t, storage.MigratePostgresNS(db, system.Table, versions))
	}
	return db
}

// The workspace ID of one tenant equals the slug of another. Both have the same draft and stream.
func TestUpgradeExistingPilotTerms(t *testing.T) {
	db := databaseBeforePilotUpgrade(t)
	ctx := t.Context()
	_, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, slug, name)
		VALUES ('team-a', 'alm', 'Alm'), ('alm', 'bjork', 'Bjørk');
		INSERT INTO kg_changesets (workspace_id, id) VALUES ('team-a', 'draft'), ('alm', 'draft');
		INSERT INTO kg_changesets (workspace_id, id, status) VALUES ('team-a', 'retired', 'merged');
		INSERT INTO kg_pilots (workspace_id, changeset_id, project_id, stream)
		VALUES ('team-a', 'draft', 'p:1', 'prøve:a'), ('team-a', 'draft', 'p:2', 'prøve:a'),
		       ('team-a', 'draft', 'p:1,p:2', 'prøve:a'), ('team-a', 'draft', U&'\00a0', 'prøve:a'),
		       ('team-a', 'retired', 'p:1', 'prøve:a'), ('alm', 'draft', 'p:3', 'prøve:a');
		INSERT INTO tb_concepts (workspace_id, id, stream, domain, properties, do_not_translate, advisory)
		VALUES ('alm', 'a:1', '', 'live', NULL, false, false),
		       ('alm', 'widened', '', 'restricted', '{"project_id":"p:1"}', false, false),
		       ('alm', '__pilot__:c:draft:prøve:a:a:1', 'prøve:a', 'oak', '{"extra":"kept"}', true, true),
		       ('alm', '__pilot__:c:draft:prøve:a:a:2', 'prøve:a', 'ash', '{"project_id":"\u00a0p:1 , other "}', false, false),
		       ('alm', '__pilot__:c:draft:prøve:a:widened', 'prøve:a', 'willow', '{}', false, false),
		       ('alm', '__pilot__:c:removed:prøve:a:x', 'prøve:a', 'orphan', NULL, false, false),
		       ('alm', '__pilot__:c:retired:prøve:a:a:1', 'prøve:a', 'retired', NULL, false, false),
		       ('bjork', 'a:1', '', 'live', '{"project_id":"p:3"}', false, false),
		       ('bjork', '__pilot__:c:draft:prøve:a:a:1', 'prøve:a', 'birch', NULL, false, false);
		INSERT INTO tb_terms (workspace_id, concept_id, text, text_lower, text_normalized, locale, status, forms)
		VALUES ('alm', '__pilot__:c:draft:prøve:a:a:1', 'Tree', 'tree', 'tree', 'en', 'preferred', '["trees"]');
		INSERT INTO tb_relations (workspace_id, id, source_id, target_id, relation, note, stream)
		VALUES ('alm', '__pilot__:r:draft:prøve:a:edge:1',
		        '__pilot__:c:draft:prøve:a:a:1', '__pilot__:c:draft:prøve:a:a:2', 'RELATED', 'link', 'prøve:a');
		INSERT INTO tb_revision (workspace_id, revision)
		VALUES ('alm', 'before'), ('bjork', 'before'), ('untouched', 'stable');`)
	require.NoError(t, err)

	require.NoError(t, migrations.Apply(db, nil))
	alm, err := pgterms.NewPostgresStoreFromDB(db, "alm")
	require.NoError(t, err)
	bjork, err := pgterms.NewPostgresStoreFromDB(db, "bjork")
	require.NoError(t, err)
	projectConcepts := func(store *pgterms.PostgresStore, project string) []terms.Concept {
		t.Helper()
		rows, _, err := termscope.StoreView(store, project).SearchForStream(ctx, "", "", "", "prøve:a", nil, 0, 20)
		require.NoError(t, err)
		return rows
	}
	second := projectConcepts(alm, "p:2")
	require.Len(t, second, 1)
	common := second[0]
	assert.Equal(t, "oak", common.Domain)
	assert.True(t, common.Advisory)
	assert.True(t, common.DoNotTranslate)
	assert.Equal(t, map[string]string{"extra": "kept", "project_id": "p:2"}, common.Properties)
	require.Len(t, common.Terms, 1)
	assert.Equal(t, []string{"trees"}, common.Terms[0].Forms)
	assert.Equal(t, "Tree", common.Terms[0].Text)
	require.Empty(t, projectConcepts(alm, "p:3"))

	first := projectConcepts(alm, "p:1")
	require.Len(t, first, 3)
	oak := slices.IndexFunc(first, func(c terms.Concept) bool { return c.Domain == "oak" })
	ash := slices.IndexFunc(first, func(c terms.Concept) bool { return c.Domain == "ash" })
	require.NotEqual(t, -1, oak)
	require.NotEqual(t, -1, ash)
	assert.NotEqual(t, first[oak].ID, common.ID)
	rels, err := termscope.StoreView(alm, "p:1").RelationsForStream(ctx, first[oak].ID, "prøve:a", nil, nil)
	require.NoError(t, err)
	require.Len(t, rels, 1)
	assert.Equal(t, "link", rels[0].Note)
	assert.Equal(t, first[oak].ID, rels[0].SourceID)
	assert.Equal(t, first[ash].ID, rels[0].TargetID)
	rels, err = termscope.StoreView(alm, "p:2").RelationsForStream(ctx, common.ID, "prøve:a", nil, nil)
	require.NoError(t, err)
	assert.Empty(t, rels)
	third := projectConcepts(bjork, "p:3")
	require.Len(t, third, 1)
	assert.Equal(t, "birch", third[0].Domain)
	assert.Empty(t, projectConcepts(bjork, "p:1"))

	var legacy int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT (SELECT count(*) FROM tb_concepts WHERE starts_with(id, '__pilot__:c:'))
		     + (SELECT count(*) FROM tb_relations WHERE starts_with(id, '__pilot__:r:'))`).Scan(&legacy))
	assert.Zero(t, legacy)
	var beforeReplay string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT revision FROM tb_revision WHERE workspace_id = 'alm'`).Scan(&beforeReplay))
	assert.NotEqual(t, "before", beforeReplay)
	var untouched string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT revision FROM tb_revision WHERE workspace_id = 'untouched'`).Scan(&untouched))
	assert.Equal(t, "stable", untouched)

	_, err = db.ExecContext(ctx, `DELETE FROM tb_schema_migrations WHERE version >= 11`)
	require.NoError(t, err)
	alm, err = pgterms.NewPostgresStoreFromDB(db, "alm")
	require.NoError(t, err)
	afterReplay, err := alm.Revision(ctx)
	require.NoError(t, err)
	assert.Equal(t, beforeReplay, afterReplay)
	assert.Equal(t, second, projectConcepts(alm, "p:2"))

	governance, err := knowledge.NewPostgresKnowledgeStore(db)
	require.NoError(t, err)
	engine := knowledge.NewEngine(nil, alm, governance)
	require.NoError(t, engine.StopPilot(ctx, "team-a", governance, knowledge.ChangeSet{ID: "draft"}, "p:1", "prøve:a"))
	assert.Empty(t, projectConcepts(alm, "p:1"))
	assert.Equal(t, second, projectConcepts(alm, "p:2"))
	assert.Equal(t, third, projectConcepts(bjork, "p:3"))
	live, present, err := alm.GetConcept(ctx, "a:1")
	require.NoError(t, err)
	require.True(t, present)
	assert.Equal(t, "live", live.Domain)
}

func TestUpgradePilotBindingsWithoutTerms(t *testing.T) {
	db := databaseBeforePilotUpgrade(t)
	ctx := t.Context()
	_, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, slug, name) VALUES ('ws', 'oak', 'Oak');
		INSERT INTO kg_changesets (workspace_id, id, status)
		SELECT 'ws', status, status FROM unnest(ARRAY[
			'draft', 'in_review', 'approved', 'merged', 'abandoned', 'superseded'
		]) AS states(status);
		INSERT INTO kg_pilots (workspace_id, changeset_id, project_id, stream)
		SELECT workspace_id, id, 'project', 'main' FROM kg_changesets;
		INSERT INTO kg_pilots (workspace_id, changeset_id, project_id, stream)
		VALUES ('ws', 'draft', '', 'main'), ('ws', 'draft', ' project', 'main'),
		       ('ws', 'draft', 'project', ' '), ('ws', 'missing', 'project', 'main');`)
	require.NoError(t, err)
	_, err = pgterms.NewPostgresStoreFromDB(db, "oak")
	require.NoError(t, err)
	governance, err := knowledge.NewPostgresKnowledgeStore(db)
	require.NoError(t, err)
	forStream, err := governance.ListPilotsForStream(ctx, "ws", "project", "main")
	require.NoError(t, err)
	var active []string
	for _, pilot := range forStream {
		active = append(active, pilot.ChangesetID)
	}
	assert.ElementsMatch(t, []string{"draft", "in_review", "approved"}, active)
	forDraft, err := governance.ListPilots(ctx, "ws", "draft")
	require.NoError(t, err)
	require.Len(t, forDraft, 1)
	assert.Equal(t, "project", forDraft[0].ProjectID)
	assert.Equal(t, "main", forDraft[0].Stream)
}

// A legacy pilot copy, or its original, whose properties are empty, blank,
// not an object or malformed reads as having no properties: the upgrade
// completes and the copy is scoped to its project.
func TestUpgradePilotTermsWithUnreadableProperties(t *testing.T) {
	db := databaseBeforePilotUpgrade(t)
	ctx := t.Context()
	_, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, slug, name) VALUES ('ws', 'oak', 'Oak');
		INSERT INTO kg_changesets (workspace_id, id) VALUES ('ws', 'draft');
		INSERT INTO kg_pilots (workspace_id, changeset_id, project_id, stream) VALUES ('ws', 'draft', 'p:1', 'main');
		INSERT INTO tb_concepts (workspace_id, id, stream, domain, properties)
		VALUES ('oak', 'empty', '', 'live', ''),
		       ('oak', '__pilot__:c:draft:main:empty', 'main', 'empty', ''),
		       ('oak', 'blank', '', 'live', '   '),
		       ('oak', '__pilot__:c:draft:main:blank', 'main', 'blank', ' '),
		       ('oak', 'malformed', '', 'live', '{"project_id":'),
		       ('oak', '__pilot__:c:draft:main:malformed', 'main', 'malformed', '{oops'),
		       ('oak', 'list', '', 'live', '["p:2"]'),
		       ('oak', '__pilot__:c:draft:main:list', 'main', 'list', 'null');`)
	require.NoError(t, err)

	require.NoError(t, migrations.Apply(db, nil))
	store, err := pgterms.NewPostgresStoreFromDB(db, "oak")
	require.NoError(t, err)
	rows, _, err := termscope.StoreView(store, "p:1").SearchForStream(ctx, "", "", "", "main", nil, 0, 20)
	require.NoError(t, err)
	var domains []string
	for _, c := range rows {
		if c.Domain == "live" {
			continue
		}
		domains = append(domains, c.Domain)
		assert.Equal(t, map[string]string{"project_id": "p:1"}, c.Properties, c.Domain)
	}
	assert.ElementsMatch(t, []string{"empty", "blank", "malformed", "list"}, domains)
}
