package projector_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

const key = workspace.ProjectKey("prj_docs")

// open binds a projector to a fresh workspace and a project store in it.
func open(t *testing.T) (*projector.Projector, *workspace.Workspace, *projectdb.DB) {
	t.Helper()
	ctx := t.Context()
	ws, err := workspace.OpenLocal(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	contextDB, err := ws.Context(ctx, key)
	require.NoError(t, err)
	db, err := projectdb.Open(ctx, project.LayoutAt(root), projectdb.WithWorkspace(projectdb.Stores{
		Context: contextDB, Graph: ws.Registry(),
	}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	p, err := projector.ForProject(ws, key, db)
	require.NoError(t, err)
	return p, ws, db
}

func concept(id, text string, status model.TermStatus) terms.Concept {
	return terms.Concept{ID: id, Terms: []terms.Term{{Text: text, Locale: "en", Status: status}}}
}

func entry(id, source, target string) memory.Entry {
	return memory.Entry{
		ID:          id,
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: source}}},
			"nb": {{Text: &model.TextRun{Text: target}}},
		},
		Origins: []memory.Origin{{Source: "user", AddedBy: "test"}},
	}
}

// writeMixedLog makes the writes every surface makes, through the projector:
// concepts put, edited and deleted, a relation, content memory added one entry
// at a time and in bulk, an entry deleted, an import session, a voice profile
// created and edited, a rule widened and another narrowed, and a batch that
// reads a bundle.
func writeMixedLog(t *testing.T, p *projector.Projector, n int) {
	t.Helper()
	ctx := t.Context()
	tb, tm, vc, rules := p.Terms(), p.Memory(), p.Voice(), p.Rules()

	require.NoError(t, tb.AddConcept(ctx, concept("c-quickcast", "Quickcast", model.TermPreferred)))
	require.NoError(t, tb.AddConcept(ctx, concept("c-quick-cast", "Quick cast", model.TermForbidden)))
	require.NoError(t, tb.AddRelation(ctx, terms.ConceptRelation{
		ID: "r1", SourceID: "c-quick-cast", TargetID: "c-quickcast", RelationType: graph.LabelBroader,
	}))
	edited := concept("c-quickcast", "Quickcast", model.TermPreferred)
	edited.Definition = "the product's streaming feature"
	require.NoError(t, tb.AddConcept(ctx, edited))
	require.NoError(t, tb.AddConcept(ctx, concept("c-gone", "Gone", model.TermPreferred)))
	require.NoError(t, tb.DeleteConcept(ctx, "c-gone"))

	require.NoError(t, tm.Add(ctx, entry("m-1", "Save", "Lagre")))
	require.NoError(t, tm.Add(ctx, entry("m-2", "Cancel", "Avbryt")))
	require.NoError(t, tm.Delete(ctx, "m-2"))
	require.NoError(t, tm.CreateImportSession(ctx, memory.ImportSession{ID: "s1", FileKey: "seed.tmx", SrcLang: "en"}))
	bulk := make([]memory.Entry, 0, n)
	for i := range n {
		bulk = append(bulk, entry(fmt.Sprintf("b-%05d", i), fmt.Sprintf("Sentence %d", i), fmt.Sprintf("Setning %d", i)))
	}
	require.NoError(t, tm.BulkAddWithStream(ctx, bulk, ""))
	require.NoError(t, tm.RebuildSearchIndex(ctx))

	prof := &coreprofile.VoiceProfile{ID: "house", Name: "House voice"}
	require.NoError(t, vc.CreateProfile(ctx, prof))
	prof.Description = "plain and direct"
	require.NoError(t, vc.UpdateProfile(ctx, prof))
	assert.Equal(t, 2, prof.Version, "an update archives the version it replaces")

	require.NoError(t, rules.WidenRule(ctx, workspace.Rule{ID: "prj_docs\x00a", Kind: "term", Origin: key, Payload: []byte(`{"a":1}`)}))
	require.NoError(t, rules.WidenRule(ctx, workspace.Rule{ID: "prj_docs\x00b", Kind: "term", Origin: key, Payload: []byte(`{"b":1}`)}))
	require.NoError(t, rules.NarrowRule(ctx, "prj_docs\x00b"))

	batch := p.With(projector.Origin{By: "import", Source: ".kapi/terms.json"}).Batch()
	for i := range 40 {
		require.NoError(t, batch.Terms().AddConcept(ctx, concept(fmt.Sprintf("imp-%02d", i), fmt.Sprintf("Imported %d", i), model.TermPreferred)))
	}
	require.NoError(t, batch.Memory().Add(ctx, entry("m-imp", "Open", "Åpne")))
	for i := range 40 {
		// Single writes collected in a batch, which apply in one transaction.
		require.NoError(t, batch.Memory().Add(ctx, entry(fmt.Sprintf("m-batch-%02d", i), fmt.Sprintf("Batched %d", i), "x")))
	}
	require.NoError(t, batch.Commit(ctx))
	for i := range 40 {
		// Single writes, one operation each, which a rebuild replays together.
		require.NoError(t, tm.Add(ctx, entry(fmt.Sprintf("m-single-%02d", i), fmt.Sprintf("Single %d", i), "y")))
	}
}

// snapshot renders every row of the projection tables, so two stores can be
// compared. The voice store stamps an edit's time and the version it archives
// from the clock, so those two columns are left out.
func snapshot(t *testing.T, ws *workspace.Workspace, db *projectdb.DB) map[string][]string {
	t.Helper()
	ctx := t.Context()
	raw := db.Raw()
	rows, err := raw.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND sql NOT LIKE 'CREATE VIRTUAL%'
		AND (name LIKE 'tm\_%' ESCAPE '\' OR name LIKE 'tb\_%' ESCAPE '\' OR name IN ('voice_profiles', 'voice_profile_versions'))`)
	require.NoError(t, err)
	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		if strings.HasSuffix(name, "_migrations") || strings.Contains(name, "_trigram_") || strings.Contains(name, "_search_") {
			continue
		}
		tables = append(tables, name)
	}
	require.NoError(t, rows.Close())

	out := map[string][]string{}
	for _, table := range tables {
		rows, err := raw.QueryContext(ctx, `SELECT * FROM "`+table+`"`)
		require.NoError(t, err)
		cols, err := rows.Columns()
		require.NoError(t, err)
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			require.NoError(t, rows.Scan(ptrs...))
			var cells []string
			for i, c := range cols {
				// A variant's vid is a surrogate key for its index rows,
				// numbered in the order a write visits its locales.
				if table == "tm_variants" && c == "vid" {
					continue
				}
				if (table == "voice_profiles" && c == "updated_at") || (table == "voice_profile_versions" && c == "created_at") {
					continue
				}
				cells = append(cells, fmt.Sprintf("%s=%v", c, vals[i]))
			}
			out[table] = append(out[table], strings.Join(cells, "|"))
		}
		require.NoError(t, rows.Close())
		sort.Strings(out[table])
	}
	held, err := ws.WidenedRules(ctx, "")
	require.NoError(t, err)
	for _, r := range held {
		out["workspace_rules"] = append(out["workspace_rules"], fmt.Sprintf("%s|%s|%s", r.ID, r.Payload, r.At.Format(time.RFC3339Nano)))
	}
	return out
}

func TestRebuildEqualsTheIncrementalState(t *testing.T) {
	p, ws, db := open(t)
	writeMixedLog(t, p, 3000)
	before := snapshot(t, ws, db)
	require.NotEmpty(t, before["tb_concepts"])
	require.NotEmpty(t, before["tm_entries"])
	require.Len(t, before["workspace_rules"], 1)

	report, err := p.Rebuild(t.Context())
	require.NoError(t, err)
	assert.Empty(t, report.Failed)
	assert.Positive(t, report.Operations[projector.KindMemory])

	assert.Equal(t, before, snapshot(t, ws, db), "a rebuild from the log writes the rows the writes left")

	for _, text := range []string{"Sentence 7", "Batched 7", "Single 7"} {
		hits, err := db.Memory().LookupText(t.Context(), text, "en", "nb", memory.LookupOptions{MinScore: 0.5, MaxResults: 1})
		require.NoError(t, err)
		require.NotEmpty(t, hits, "the search indexes are rebuilt with the rows: %s", text)
		found, total, err := db.Memory().SearchEntries(t.Context(), memory.SearchParams{Query: text, Limit: 1})
		require.NoError(t, err)
		require.Positive(t, total, "full-text search finds %s", text)
		require.NotEmpty(t, found)
	}
}

func TestWritingWhatTheStoreHoldsRecordsNothing(t *testing.T) {
	p, ws, db := open(t)
	ctx := t.Context()
	c := concept("c1", "Quickcast", model.TermPreferred)
	require.NoError(t, p.Terms().AddConcept(ctx, c))
	require.NoError(t, p.Memory().Add(ctx, entry("m1", "Save", "Lagre")))
	head, err := ws.Head(ctx)
	require.NoError(t, err)
	held, _, err := db.Terms().GetConcept(ctx, "c1")
	require.NoError(t, err)

	time.Sleep(1100 * time.Millisecond) // past the store's one-second timestamps
	require.NoError(t, p.Terms().AddConcept(ctx, c))
	require.NoError(t, p.Memory().Add(ctx, entry("m1", "Save", "Lagre")))
	again, err := ws.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, head, again, "an unchanged concept and entry record nothing")
	still, _, err := db.Terms().GetConcept(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, held.UpdatedAt, still.UpdatedAt, "and move no timestamp")
}

func TestCatchUpAppliesWhatAnotherWriterRecorded(t *testing.T) {
	p, ws, db := open(t)
	ctx := t.Context()

	// A second projector stands in for another process: it records into the
	// shared log, and the store is then rewound to before that write.
	elsewhere, err := projector.ForProject(ws, key, db)
	require.NoError(t, err)
	require.NoError(t, elsewhere.Terms().AddConcept(ctx, concept("c1", "Quickcast", model.TermPreferred)))

	// Rewind this store's view to before that write, as a process that was
	// not running would find it.
	_, err = db.Raw().ExecContext(ctx, `UPDATE projector_cursor SET seq = 0`)
	require.NoError(t, err)
	_, err = db.Raw().ExecContext(ctx, `DELETE FROM tb_concepts`)
	require.NoError(t, err)

	require.NoError(t, p.CatchUp(ctx))
	_, ok, err := db.Terms().GetConcept(ctx, "c1")
	require.NoError(t, err)
	assert.True(t, ok, "catching up applies the write the log holds")
}

func TestLargeWritesSplitAcrossOperations(t *testing.T) {
	p, ws, _ := open(t)
	ctx := t.Context()
	bulk := make([]memory.Entry, 0, 4500)
	for i := range 4500 {
		bulk = append(bulk, entry(fmt.Sprintf("b-%05d", i), fmt.Sprintf("Sentence %d", i), "x"))
	}
	require.NoError(t, p.Memory().BulkAddWithStream(ctx, bulk, ""))
	ops, err := ws.Select(ctx, workspace.OpQuery{KindPrefix: projector.KindMemory})
	require.NoError(t, err)
	assert.Len(t, ops, 1, "one write is one operation")
	assert.Contains(t, string(ops[0].Payload), `"blob"`, "and its entries sit in a blob")
}

func TestEmbeddedLayoutAppliesWithoutALog(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	db, err := projectdb.Open(t.Context(), project.LayoutAt(root))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	p, err := projector.ForProject(nil, "", db)
	require.NoError(t, err)
	require.NoError(t, p.Terms().AddConcept(context.Background(), concept("c1", "Quickcast", model.TermPreferred)))
	_, ok, err := db.Terms().GetConcept(t.Context(), "c1")
	require.NoError(t, err)
	assert.True(t, ok)
	_, err = p.Rebuild(t.Context())
	require.Error(t, err, "a store with no log has nothing to rebuild from")
}
