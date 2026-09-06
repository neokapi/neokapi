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
	assert.Equal(t, 1, countOf(stderr, "locale spelling"), "said once")
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
