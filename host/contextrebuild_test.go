package host

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// projectionRows renders every row of a project's projection tables and the
// rules it widened, so two states of one store can be compared. The voice
// store stamps an edit's time and the version it archives from the clock, so
// those two columns are left out.
func projectionRows(t *testing.T, app *App, db *projectdb.DB) map[string][]string {
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
		if !strings.HasSuffix(name, "_migrations") && !strings.Contains(name, "_trigram_") && !strings.Contains(name, "_search_") {
			tables = append(tables, name)
		}
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
	ws, err := app.Workspace(ctx)
	require.NoError(t, err)
	widened, err := ws.WidenedRules(ctx, "")
	require.NoError(t, err)
	for _, r := range widened {
		out["workspace_rules"] = append(out["workspace_rules"], r.ID+"|"+string(r.Payload))
	}
	return out
}

// TestRebuildFromAMixedLogEqualsTheIncrementalState records what every surface
// records — suggestions kept, a correction contesting a kept rule and set
// aside again, a rule widened to the workspace and reverted, `kapi apply`
// edits to the terms, the content memory and the voice, and an import of a
// terms bundle — and rebuilds the stores from the log. The rebuilt stores are
// the ones the writes left, and the check reads them the same way.
func TestRebuildFromAMixedLogEqualsTheIncrementalState(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-rebuild")
	ctx := t.Context()

	kept := proposeUtilise(t, app, root, agentIn("s1"))
	_, err := app.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: kept.ID})
	require.NoError(t, err)
	correction, err := app.RecordContextCorrection(ctx, ContextCorrectRequest{
		Actor: person, Project: recipeOf(root), From: "use", To: "utilise",
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)
	_, err = app.DropContextOperation(ctx, ContextDropRequest{Actor: person, Project: recipeOf(root), ID: correction.ID})
	require.NoError(t, err)

	widened, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Term: "sign in", InsteadOf: []string{"login"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: widened.ID})
	require.NoError(t, err)
	_, err = app.WidenContextOperation(ctx, ContextWidenRequest{Actor: person, Project: recipeOf(root), ID: widened.ID, To: WidenToWorkspace})
	require.NoError(t, err)
	reverted, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Term: "select", InsteadOf: []string{"choose"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: reverted.ID})
	require.NoError(t, err)
	_, err = app.RevertContextOperations(ctx, ContextRevertRequest{Actor: person, Project: recipeOf(root), ID: reverted.ID})
	require.NoError(t, err)

	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, recipeOf(root), "")
	for _, e := range []changeEntry{
		{Kind: kindTerm, Op: "upsert", Term: "e-mail", Replacement: "email", Locale: "en", Status: "forbidden"},
		{Kind: kindMemory, Op: "add", Source: "Save", Target: "Lagre", SourceLocale: "en", TargetLocale: "nb"},
		{Kind: kindMemory, Op: "add", Source: "Cancel", Target: "Avbryt", SourceLocale: "en", TargetLocale: "nb"},
		{Kind: kindTerm, Term: "leverage", Replacement: "use", Locale: "en", Status: "forbidden", Advisory: true},
	} {
		res := app.applyRecordedAssetEntry(ctx, cmd, e)
		require.Equal(t, "applied", res.Status, "%s: %s", e.Kind, res.Detail)
	}
	seedVoiceProfile(t, app, cmd)

	bundle := ktb.FromConcepts([]terms.Concept{
		{ID: "c-widget", Terms: []terms.Term{{Text: "widget", Locale: "en", Status: model.TermPreferred}}},
		{ID: "c-gadget", Terms: []terms.Term{{Text: "gadget", Locale: "en", Status: model.TermPreferred}}},
	})
	data, err := ktb.Marshal(bundle)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapi", ktb.ConventionalName), data, 0o600))
	imported, err := app.ImportProjectContext(ctx, recipeOf(root), ContextImportRequest{})
	require.NoError(t, err)
	require.Equal(t, 2, imported.Concepts)

	db, err := app.ProjectDB(ctx, root)
	require.NoError(t, err)
	before := projectionRows(t, app, db)
	require.NotEmpty(t, before["tb_concepts"])
	require.NotEmpty(t, before["tm_entries"])
	require.NotEmpty(t, before["voice_profiles"])
	require.Len(t, before["workspace_rules"], 1)
	verdict := checkWith(t, app, root).Verdict

	res, err := app.RebuildProjectContext(ctx, recipeOf(root))
	require.NoError(t, err)
	assert.Empty(t, res.Failed)
	assert.Positive(t, res.Operations["terms.write"])

	assert.Equal(t, before, projectionRows(t, app, db), "the rebuilt stores are the ones the writes left")
	assert.Equal(t, verdict, checkWith(t, app, root).Verdict, "and the check reads them the same way")
}
