package safeio_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/safeio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeJoin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "simple relative", input: "word/document.xml"},
		{name: "nested relative", input: "OEBPS/text/chapter1.xhtml"},
		{name: "single file", input: "mimetype"},
		{name: "dot prefix is fine", input: "./word/document.xml"},
		{name: "parent escape", input: "../etc/passwd", wantErr: true},
		{name: "deep parent escape", input: "a/b/../../../etc/passwd", wantErr: true},
		{name: "absolute unix", input: "/etc/passwd", wantErr: true},
		{name: "empty path", input: "", wantErr: true},
		{name: "bare parent", input: "..", wantErr: true},
	}
	root := "/tmp/extract-root"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := safeio.SafeJoin(root, tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, safeio.ErrPathEscape)
				return
			}
			require.NoError(t, err)
			// The result must stay under root.
			rel, relErr := filepath.Rel(root, got)
			require.NoError(t, relErr)
			assert.False(t, filepath.IsAbs(rel))
			assert.NotContains(t, rel, "..")
		})
	}
}

func TestIsLocalPath(t *testing.T) {
	t.Parallel()
	assert.True(t, safeio.IsLocalPath("a/b/c.txt"))
	assert.True(t, safeio.IsLocalPath("file.txt"))
	assert.False(t, safeio.IsLocalPath("../escape"))
	assert.False(t, safeio.IsLocalPath("/abs"))
	assert.False(t, safeio.IsLocalPath(""))
}

func TestOpenInRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("safe"), 0o600))
	// Create a sibling outside the root to attempt to reach.
	parent := filepath.Dir(dir)
	secret := filepath.Join(parent, "secret-"+filepath.Base(dir)+".txt")
	require.NoError(t, os.WriteFile(secret, []byte("secret"), 0o600))
	t.Cleanup(func() { _ = os.Remove(secret) })

	t.Run("reads file within root", func(t *testing.T) {
		f, err := safeio.OpenInRoot(dir, "ok.txt")
		require.NoError(t, err)
		defer f.Close()
		b, err := os.ReadFile(filepath.Join(dir, "ok.txt"))
		require.NoError(t, err)
		assert.Equal(t, "safe", string(b))
	})

	t.Run("rejects traversal before touching fs", func(t *testing.T) {
		_, err := safeio.OpenInRoot(dir, "../"+filepath.Base(secret))
		require.Error(t, err)
		assert.ErrorIs(t, err, safeio.ErrPathEscape)
	})

	t.Run("rejects absolute", func(t *testing.T) {
		_, err := safeio.OpenInRoot(dir, secret)
		require.Error(t, err)
		assert.ErrorIs(t, err, safeio.ErrPathEscape)
	})
}
