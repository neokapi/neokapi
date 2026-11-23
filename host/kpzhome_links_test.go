//go:build unix

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKpzRef_LinkOut asserts a KPZ reference whose file beneath the root is a
// link that leads out of it names no KPZ, while a regular one does.
func TestKpzRef_LinkOut(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.kpz")
	require.NoError(t, os.WriteFile(outside, []byte("PK"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "work.kpz"), []byte("PK"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "linked.kpz")))

	h := &kpzHomes{root: root}
	_, path, ok := h.kpzRef("work.kpz!guide.md")
	assert.True(t, ok, "a KPZ inside the root is found")
	assert.Equal(t, filepath.Join(root, "work.kpz"), path)

	_, _, ok = h.kpzRef("linked.kpz!guide.md")
	assert.False(t, ok, "a KPZ reached through a link out of the root is refused")
}
