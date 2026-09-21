package workspace_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/core/workspace/workspacetest"
)

func TestLocalBackendConformance(t *testing.T) {
	workspacetest.RunConformance(t, func(t *testing.T) workspace.Backend {
		return workspace.Local(filepath.Join(t.TempDir(), "workspaces", "default"))
	})
}

func TestLocalBackendLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces", "default")
	b := workspace.Local(root)
	t.Cleanup(func() { _ = b.Close() })

	ctx := t.Context()
	_, err := b.Registry(ctx)
	require.NoError(t, err)
	_, err = b.Project(ctx, "prj_abc")
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(root, workspace.RegistryFileName))
	assert.FileExists(t, filepath.Join(root, workspace.ProjectsDirName, "prj_abc.db"))
	assert.Equal(t, filepath.Join(root, workspace.RegistryFileName), b.RegistryPath())
	assert.Equal(t, filepath.Join(root, workspace.ProjectsDirName, "prj_abc.db"), b.ProjectPath("prj_abc"))
}

func TestTwoCheckoutsShareOneContextStore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces", "default")
	ctx := t.Context()

	// The first checkout writes.
	first := workspace.Local(root)
	db, err := first.Project(ctx, "prj_shared")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE decisions (v TEXT)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO decisions (v) VALUES ('approved')`)
	require.NoError(t, err)
	require.NoError(t, first.Close())

	// The second checkout, a separate backend on the same workspace, reads it.
	second := workspace.Local(root)
	t.Cleanup(func() { _ = second.Close() })
	db2, err := second.Project(ctx, "prj_shared")
	require.NoError(t, err)
	var v string
	require.NoError(t, db2.QueryRowContext(ctx, `SELECT v FROM decisions`).Scan(&v))
	assert.Equal(t, "approved", v)
}

func TestFileNameFor(t *testing.T) {
	tests := []struct {
		name string
		key  workspace.ProjectKey
		want string
	}{
		{"a minted id is a filename as it stands", "prj_abc234xyz", "prj_abc234xyz"},
		{"an empty key has a name of its own", "", "unidentified"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, workspace.FileNameFor(tt.key))
		})
	}

	t.Run("a recipe name is sanitized and disambiguated", func(t *testing.T) {
		got := workspace.FileNameFor("Acme Docs/nb")
		assert.NotContains(t, got, "/")
		assert.NotContains(t, got, " ")
		assert.Contains(t, got, "Acme-Docs-nb")
		assert.NotEqual(t, got, workspace.FileNameFor("Acme-Docs-nb"),
			"two keys that sanitize alike stay apart")
	})

	t.Run("an upper-case key is disambiguated rather than taken verbatim", func(t *testing.T) {
		// A case-insensitive filesystem would otherwise fold "Docs" onto "docs".
		assert.NotEqual(t, "Docs", workspace.FileNameFor("Docs"))
		assert.Equal(t, "docs", workspace.FileNameFor("docs"))
	})
}

func TestKeyForCheckoutIsStableAndDistinct(t *testing.T) {
	a := workspace.KeyForCheckout("/fakehome/src/one")
	b := workspace.KeyForCheckout("/fakehome/src/one")
	c := workspace.KeyForCheckout("/fakehome/src/two")
	assert.Equal(t, a, b, "one checkout is one key")
	assert.NotEqual(t, a, c, "two checkouts are two keys")
	assert.Greater(t, len(string(a)), 8)
}

func TestCloudSyncedWorkspaceIsRefused(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		product string
	}{
		{"iCloud Drive", "/fakehome/Library/Mobile Documents/com~apple~CloudDocs/kapi", "iCloud Drive"},
		{"Dropbox", "/fakehome/Dropbox/kapi/workspaces/default", "Dropbox"},
		{"OneDrive for an organization", "/fakehome/OneDrive - Contoso/kapi", "OneDrive"},
		{"Google Drive", "/fakehome/Google Drive/kapi", "Google Drive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, product, found := workspace.CloudSyncedDir(tt.root)
			require.True(t, found)
			assert.Equal(t, tt.product, product)

			b := workspace.Local(tt.root)
			t.Cleanup(func() { _ = b.Close() })
			_, err := b.Registry(t.Context())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "KAPI_DATA_DIR")
			assert.Contains(t, err.Error(), tt.product)
		})
	}

	t.Run("an ordinary directory is allowed", func(t *testing.T) {
		for _, root := range []string{
			"/fakehome/.local/share/kapi/workspaces/default",
			"/fakehome/src/OneDriveTooling/kapi",
			"/fakehome/Library/Application Support/kapi",
		} {
			_, _, found := workspace.CloudSyncedDir(root)
			assert.False(t, found, root)
		}
	})
}

func TestWorkspaceReadsFromAWriteRestrictedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not restrict the owner on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes a read-only directory anyway")
	}
	root := filepath.Join(t.TempDir(), "workspaces", "default")
	ctx := t.Context()

	writable := workspace.Local(root)
	w, err := workspace.Open(ctx, writable)
	require.NoError(t, err)
	_, err = w.Register(ctx, "prj_sealed", "Sealed", "/fakehome/src/sealed")
	require.NoError(t, err)
	db, err := w.Context(ctx, "prj_sealed")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE terms (v TEXT)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO terms (v) VALUES ('content memory')`)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Seal the workspace against writes, the way a check running in a
	// restricted sandbox meets it.
	projects := filepath.Join(root, workspace.ProjectsDirName)
	for _, dir := range []string{projects, root} {
		require.NoError(t, os.Chmod(dir, 0o555))
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	}

	sealed := workspace.Local(root)
	t.Cleanup(func() { _ = sealed.Close() })
	readOnly, err := workspace.Open(ctx, sealed)
	require.NoError(t, err, "a sealed workspace still opens")
	assert.True(t, readOnly.Describe().ReadOnly, "and says it is read-only")

	regs, err := readOnly.Projects(ctx)
	require.NoError(t, err)
	require.Len(t, regs, 1)
	assert.Equal(t, workspace.ProjectKey("prj_sealed"), regs[0].Key)

	ctxDB, err := readOnly.Context(ctx, "prj_sealed")
	require.NoError(t, err)
	var v string
	require.NoError(t, ctxDB.QueryRowContext(ctx, `SELECT v FROM terms`).Scan(&v))
	assert.Equal(t, "content memory", v)

	_, err = readOnly.Register(ctx, "prj_sealed", "Sealed", "/fakehome/src/sealed")
	require.ErrorIs(t, err, workspace.ErrReadOnly, "and refuses a write rather than losing it")

	_, err = ctxDB.ExecContext(ctx, `INSERT INTO terms (v) VALUES ('lost')`)
	assert.Error(t, err, "a write through the handle is refused too")
}
