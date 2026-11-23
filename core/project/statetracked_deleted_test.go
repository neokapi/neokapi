//go:build unix

package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// State git tracks and the working tree has lost is refused like state that
// is there: kapi would create fresh state at the tracked paths, and a `git
// commit -a` would commit it. A deleted path is compared by name with the
// spelling kapi creates, folded when the volume folds case.
func TestCheckStateTrackedRefusesTrackedStateDeletedFromTheWorkingTree(t *testing.T) {
	for _, tc := range []struct {
		name, tracked, remove string
		needsFolding          bool
	}{
		{"an entry deleted", ".kapi/work/kept", ".kapi/work", false},
		{"the whole state directory deleted", ".kapi/sync/kept", ".kapi", false},
		{"the filters file deleted", ".kapi/" + LocalFiltersFilename, ".kapi/" + LocalFiltersFilename, false},
		{"another spelling deleted", ".kapi/WORK/kept", ".kapi/WORK", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := stateRepo(t)
			if tc.needsFolding {
				require.NoError(t, os.RemoveAll(l.WorkDir()))
				p := filepath.Join(l.Root, filepath.FromSlash(tc.tracked))
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
				require.NoError(t, os.WriteFile(p, []byte("planted"), 0o644))
			}
			gitIn(t, l.Root, "add", "-f", tc.tracked)
			require.NoError(t, os.RemoveAll(filepath.Join(l.Root, filepath.FromSlash(tc.remove))))

			err := CheckStateTracked(t.Context(), l)
			if tc.needsFolding && !caseFolds(l.Root) {
				require.NoError(t, err, "on a case-sensitive volume %s is not the directory kapi creates", tc.tracked)
				return
			}
			require.ErrorContains(t, err, "git tracks")
		})
	}
}
