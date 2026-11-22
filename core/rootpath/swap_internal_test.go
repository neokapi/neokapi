//go:build unix

package rootpath

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contain runs before the open, so a link swapped in after it is the open's
// to refuse. The open goes through os.Root, which resolves each name inside
// the root and refuses a link that leads out. This drives that second line on
// its own, as a swap after the check would reach it.
func TestTheOpenRefusesALinkContainNeverSaw(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	secret := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("outside"), 0o644))

	for name, swap := range map[string]func(){
		"a file link": func() {
			require.NoError(t, os.Symlink(secret, filepath.Join(root, "doc.txt")))
		},
		"a directory link": func() {
			require.NoError(t, os.Symlink(outside, filepath.Join(root, "dir")))
		},
	} {
		t.Run(name, func(t *testing.T) {
			swap()
			target := map[string]string{"a file link": "doc.txt", "a directory link": filepath.Join("dir", "secret.txt")}[name]
			r, rel, err := openRoot(root, filepath.Join(root, target))
			require.NoError(t, err)
			defer r.Close()
			_, err = r.OpenFile(rel, os.O_RDONLY, 0)
			require.Error(t, err, "os.Root refuses the link")
			_, err = r.OpenFile(rel, os.O_WRONLY|os.O_CREATE, 0o666)
			require.Error(t, err)
			b, _ := os.ReadFile(secret)
			assert.Equal(t, "outside", string(b))
		})
	}
}
