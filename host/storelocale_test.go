package host_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runStatusStderr runs `kapi status --json` on the fixture and returns what it
// wrote to stderr.
func runStatusStderr(t *testing.T, p facetest.Project) string {
	t.Helper()
	a := &host.App{}
	a.InitRegistries()
	defer a.Shutdown()

	cmd := host.NewEnvCommand(t.Context(), "status")
	host.AddProjectFlag(cmd)
	host.AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", p.Recipe))
	require.NoError(t, cmd.Flags().Set("json", "true"))

	var out, errOut capturingWriter
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	require.NoError(t, a.RunStatus(cmd, nil))
	return string(errOut.Bytes())
}

// A store whose rows are keyed by a locale spelling the lookups no longer ask
// for is reported by `kapi status`, once, with the rebuild named. A store the
// stores wrote themselves is keyed canonically and draws no warning.
func TestStatus_WarnsWhenTheStoreHoldsNonCanonicalLocales(t *testing.T) {
	p := facetest.WritePosix(t)
	facetest.ExtractToStore(t, p)

	assert.NotContains(t, runStatusStderr(t, p), "locale spelling", "a store the stores wrote is canonical")

	// A row written before the stores normalized locales.
	ctx := context.Background()
	db, err := projectdb.Open(ctx, project.LayoutAt(p.Root))
	require.NoError(t, err)
	_, err = db.Raw().ExecContext(ctx, `INSERT INTO overlays (kind, block_hash, payload, updated_at) VALUES ('targets/nb_NO', 'b1', '{}', 1)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	stderr := runStatusStderr(t, p)
	assert.Contains(t, stderr, `block cache: 1 row(s) under "targets/nb_NO" (lookups ask for "targets/nb-NO")`)
	assert.Contains(t, stderr, "delete .kapi/work/store.db, then run `kapi up`")
	assert.Contains(t, stderr, "`kapi commit`")
	assert.NotContains(t, stderr, "kapi context locales",
		"a projection row is rebuilt, and the verb that moves authored rows is not offered for it")
	assert.Equal(t, 1, countOf(stderr, "locale spelling"), "said once")
}

// A row in the context store is authored work, so the warning names the verb
// that keys it canonically rather than telling anyone to delete it.
func TestStatus_NamesTheRekeyForContextRows(t *testing.T) {
	p := facetest.WritePosix(t)
	facetest.ExtractToStore(t, p)

	ctx := context.Background()
	a := &host.App{}
	a.InitRegistries()
	db, err := a.ProjectDB(ctx, p.Root)
	require.NoError(t, err)
	_, err = db.Raw().ExecContext(ctx, `
INSERT INTO tb_concepts (id, project_id, stream, domain, definition, properties, created_at, updated_at)
VALUES ('c1', '', '', '', '', NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	require.NoError(t, err)
	_, err = db.Raw().ExecContext(ctx, `
INSERT INTO tb_terms (concept_id, text, text_lower, locale, status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags, forms)
VALUES ('c1', 'kai', 'kai', 'NB-no', 'preferred', '', '', '', 0, NULL, NULL, '[]', '[]')`)
	require.NoError(t, err)
	a.Shutdown()

	stderr := runStatusStderr(t, p)
	assert.Contains(t, stderr, `terms: 1 row(s) under "NB-no" (lookups ask for "nb-NO")`)
	assert.Contains(t, stderr, "kapi context locales --fix")
	assert.NotContains(t, stderr, "delete .kapi/work/store.db",
		"authored rows are moved, never deleted")
}

// ProjectStoreLocales reports the drift, and with the fix asked for it moves
// the authored rows and leaves the projection's to a rebuild.
func TestProjectStoreLocales_MovesTheAuthoredRowsOnly(t *testing.T) {
	p := facetest.WritePosix(t)
	facetest.ExtractToStore(t, p)

	ctx := context.Background()
	a := &host.App{Quiet: true}
	a.InitRegistries()
	defer a.Shutdown()

	db, err := a.ProjectDB(ctx, p.Root)
	require.NoError(t, err)
	_, err = db.Raw().ExecContext(ctx, `
INSERT INTO tb_concepts (id, project_id, stream, domain, definition, properties, created_at, updated_at)
VALUES ('c1', '', '', '', '', NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	require.NoError(t, err)
	_, err = db.Raw().ExecContext(ctx, `
INSERT INTO tb_terms (concept_id, text, text_lower, locale, status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags, forms)
VALUES ('c1', 'kai', 'kai', 'NB-no', 'preferred', '', '', '', 0, NULL, NULL, '[]', '[]')`)
	require.NoError(t, err)
	_, err = db.Projection().ExecContext(ctx,
		`INSERT INTO overlays (kind, block_hash, payload, updated_at) VALUES ('targets/nb_NO', 'b1', '{}', 1)`)
	require.NoError(t, err)

	found, err := a.ProjectStoreLocales(ctx, p.Recipe, false)
	require.NoError(t, err)
	assert.False(t, found.Clean())
	assert.Len(t, found.Drift, 2)
	assert.Empty(t, found.Rekeyed, "a report that was not asked to fix anything writes nothing")

	fixed, err := a.ProjectStoreLocales(ctx, p.Recipe, true)
	require.NoError(t, err)
	require.Len(t, fixed.Rekeyed, 1)
	assert.Equal(t, "terms", fixed.Rekeyed[0].Subsystem)
	assert.Equal(t, 1, fixed.Rekeyed[0].Moved)

	require.Len(t, fixed.Drift, 1, "only the projection's row is left")
	assert.Equal(t, projectdb.PoolProjection, fixed.Drift[0].Pool)
	assert.Equal(t, ".kapi/work/store.db", fixed.Projection)

	var locale string
	require.NoError(t, db.Raw().QueryRowContext(ctx,
		`SELECT locale FROM tb_terms WHERE concept_id = 'c1'`).Scan(&locale))
	assert.Equal(t, "nb-NO", locale)
}

// A project with no store yet is not audited, and the audit opens none: a dry
// run that leaves a database behind has written to the project it promised to
// only read.
func TestWarnStoreLocaleDrift_DoesNotCreateAStoreToAuditIt(t *testing.T) {
	p := facetest.WritePosix(t)
	a := &host.App{}
	a.InitRegistries()
	defer a.Shutdown()

	cmd := host.NewEnvCommand(t.Context(), "status")
	var errOut capturingWriter
	cmd.SetErr(&errOut)
	a.WarnStoreLocaleDrift(cmd, p.Recipe)

	assert.Empty(t, errOut.Bytes())
	assert.NoFileExists(t, project.LayoutAt(p.Root).StorePath())
}

func countOf(s, sub string) int {
	n := 0
	for i := 0; ; {
		j := indexFrom(s, sub, i)
		if j < 0 {
			return n
		}
		n++
		i = j + len(sub)
	}
}

func indexFrom(s, sub string, from int) int {
	if from >= len(s) {
		return -1
	}
	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
